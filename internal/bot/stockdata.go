package bot

import (
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// fetchStockData fetches quote+news for each ticker. Fundamentals,
// AnalystRating (Phase 3.7), InsiderTx, and InstitutionalFlow are only
// attached when includeFundamentals is set (watchlist tickers, not the broad
// market-mover candidate list) to stay well under Finnhub's free-tier
// 60-requests/minute limit when a candidate list has a dozen-plus tickers —
// /stock/recommendation and /stock/insider-transactions are per-ticker calls
// just like /stock/metric, so they share the same gate rather than getting
// their own. InstitutionalFlow (TWSE, free/keyless) has no such rate-limit
// concern, but shares the gate anyway since a TWSE T86 lookup fetches the
// whole market's report per call — fine for a bounded watchlist, wasteful
// repeated per ticker across a broad candidate list. b.insiderTx/
// b.institutional each internally reject the other market's tickers
// (errTWNotSupported/errUSNotSupported) before making any request, so calling
// both unconditionally per ticker here (like b.analystRating already does)
// costs nothing extra for the market that doesn't apply.
// The one exception (Phase 3.7 追加項, docs/phase-3.7-scanhit-fundamentals.md):
// up to maxScanHitFundamentals scan-hit candidates also get fundamentals/
// analyst rating even when includeFundamentals is false, since those are the
// tickers the model is most likely to call a fresh BUY on with otherwise the
// thinnest data — watchlist's scanReasons argument is always nil so this is a
// no-op there.
// Technicals (RSI/MACD/moving averages, via computeTechnicals) has no such
// gate — Yahoo's history endpoint carries no rate-limit concern, and
// candidates are exactly where the model most needs trend context before
// calling a fresh BUY. positions (ticker -> open position) is looked up via
// loadPositions and attaches cost-basis context for any ticker the user
// actually holds; earnings (ticker -> upcoming earnings) is looked up via
// loadEarnings and attaches an earnings-date warning for any ticker
// reporting soon. scanReasons (ticker -> joined signal message) is looked up
// via db.GetScanHits and attaches why a Phase 2.6 universe-scan candidate
// was surfaced. prevRecs (ticker -> last recommendation on record) is looked
// up via loadPrevRecs and attaches Phase 3.8's recommendation-continuity
// line; a row with an empty Action (pre-Phase-1 data, or a call the model
// omitted) is skipped rather than rendering a blank line. pastLessons
// (ticker -> that ticker's own trade-review lessons, oldest first) is
// looked up via loadPastLessons and attaches Phase 3.9's reflect-then-inject
// feedback loop (see docs/research-tradingagents.md) — the "same ticker:
// bring all of them" half; the cross-ticker "recent N, general" half is a
// separate, prompt-wide GenerateRecommendations parameter, not attached
// here. Pass nil for any of the five if there's nothing to attach.
// slowDataCacheTTL bounds how often fundamentals/analyst-rating/insider-tx/
// earnings-surprise are refetched per ticker (via b.fundamentalsCache/
// b.analystRatingCache/b.insiderTxCache/b.earningsSurpriseCache) — these
// move over days, not within a single day, so a report doesn't need a fresh
// Finnhub call for them every run. Entries just expire and refetch on
// whatever day they age out, no per-ticker staleness bookkeeping needed.
// finnhubRequestDelay paces every real Finnhub-bound call this function
// makes (quote/news always for a US ticker, fundamentals/analyst/insider on
// a cache miss) so a large watchlist can't burst past Finnhub's free-tier
// 60-requests/minute ceiling in the first few seconds of a report run — see
// the "watchlist rate limit" discussion this addresses.
const (
	slowDataCacheTTL    = 4 * 24 * time.Hour
	finnhubRequestDelay = 1100 * time.Millisecond
)

// valuationSnapshotTTLDays is Phase 23 PR6/PR7's 90-day cache lifetime
// (docs/phase-23-strategy-data-uplift.md §3.5) — both SEC's companyfacts and
// FinMind's TaiwanStockPER only move on a real filing/trading day, so
// refetching more often just burns rate budget for no new information.
const valuationSnapshotTTLDays = 90

func (b *Bot) fetchStockData(tickers []string, includeFundamentals bool, positions map[string]db.Position, earnings map[string]data.EarningsEvent, scanReasons map[string]string, prevRecs map[string]db.Recommendation, pastLessons map[string][]db.Lesson) []llm.StockData {
	extraFundamentals := capScanHitTickers(scanReasons, maxScanHitFundamentals)

	// benchCloses lazily fetches and caches each market's benchmark close
	// series (SPY for US, 0050 for TW) — Phase 6 PR2: tickers here can now
	// span both markets (RunWeeklyReview/handleInsight pass every held
	// position regardless of market), so a single shared SPY series is no
	// longer correct for RS63 the way it was pre-Phase-6. A market whose
	// tickers never actually appear in this call (the common case for
	// gatherRecommendationInputs, which always passes a market-homogeneous
	// list) costs zero extra requests.
	benchCloses := make(map[market.MarketID][]float64)
	loadBenchCloses := func(m market.MarketID) []float64 {
		if closes, ok := benchCloses[m]; ok {
			return closes
		}
		var closes []float64
		if candles, err := b.history.GetHistory(service.BenchmarkFor(m), "1y"); err == nil {
			closes = data.Closes(candles)
		}
		benchCloses[m] = closes
		return closes
	}

	var result []llm.StockData
	// One picker for the whole batch: a market-wide story tagged onto a
	// dozen tickers gets one slot in this prompt, not a dozen.
	picker := &service.NewsPicker{}
	for _, t := range tickers {
		if market.Of(t) == market.US {
			time.Sleep(finnhubRequestDelay)
		}
		q, err := b.provider.GetQuote(t)
		if err != nil {
			logger.Errorf("quote %s: %v", t, err)
			continue
		}
		if market.Of(t) == market.US {
			time.Sleep(finnhubRequestDelay)
		}
		fetched, _ := b.provider.GetNews(t, tickerNewsFetch)
		fetched = filterStaleNews(fetched, time.Now().In(cst))
		stock := llm.StockData{Quote: q, News: picker.Pick(fetched, tickerNewsSlots), CompanyName: b.companyName(t)}
		fetchFundamentals := includeFundamentals || extraFundamentals[t]
		if fetchFundamentals && b.fundamentals != nil {
			if fd, err := b.cachedFundamentals(t); err != nil {
				logger.Errorf("fundamentals %s: %v", t, err)
			} else {
				stock.Fundamentals = fd
			}
		}
		if fetchFundamentals && b.analystRating != nil {
			if ar, err := b.cachedAnalystRating(t); err != nil {
				logger.Errorf("analyst rating %s: %v", t, err)
			} else {
				stock.AnalystRating = ar
			}
		}
		if fetchFundamentals && b.insiderTx != nil {
			if tx, err := b.cachedInsiderTx(t); err != nil {
				logger.Errorf("insider transactions %s: %v", t, err)
			} else {
				stock.InsiderTx = tx
			}
		}
		if fetchFundamentals && b.earningsSurprise != nil && market.Of(t) == market.US {
			if es, err := b.cachedEarningsSurprises(t); err != nil {
				logger.Errorf("earnings surprises %s: %v", t, err)
			} else {
				stock.EarningsSurprises = es
			}
		}
		// Institutional flow is a TW-only data source. Guard it here as
		// well as inside TWSE.GetInstitutionalFlow so US recommendation
		// batches do not produce an expected-but-noisy "us market not
		// supported" log for every ticker.
		if fetchFundamentals && b.institutional != nil && market.Of(t) == market.TW {
			if fl, err := b.institutional.GetInstitutionalFlow(t); err != nil {
				logger.Errorf("institutional flow %s: %v", t, err)
			} else {
				stock.InstitutionalFlow = fl
			}
		}
		if fetchFundamentals {
			stock.SECSnapshot = b.cachedValuationSnapshot(t)
		}
		stock.Technicals, stock.Candles, stock.StrategyHits = b.computeTechnicals(t, loadBenchCloses(market.Of(t)))
		if p, ok := positions[t]; ok {
			stock.Position = &llm.Position{Shares: p.Shares, AvgCost: p.AvgCost, OpenedDate: p.OpenedDate}
		}
		if e, ok := earnings[t]; ok {
			stock.Earnings = &llm.Earnings{Date: e.Date, DaysUntil: daysUntil(e.Date), Estimated: e.Estimated}
		}
		if r, ok := scanReasons[t]; ok {
			stock.ScanReason = &r
		}
		if pr, ok := prevRecs[t]; ok && pr.Action != "" {
			stock.PrevRec = &llm.PrevRecommendation{Action: pr.Action, Date: pr.Date, Price: pr.Price, DaysAgo: -daysUntil(pr.Date)}
		}
		if lessons, ok := pastLessons[t]; ok {
			stock.PastLessons = make([]llm.PastLesson, len(lessons))
			for i, l := range lessons {
				stock.PastLessons[i] = llm.PastLesson{Ticker: l.Ticker, Date: l.Date, Lesson: l.Lesson}
			}
		}
		result = append(result, stock)
	}
	return result
}

// cachedFundamentals/cachedAnalystRating/cachedInsiderTx each check their
// cache before hitting Finnhub, and pace the actual call with
// finnhubRequestDelay on a miss — see slowDataCacheTTL's comment for why
// these three (and only these three) are cached across reports.
func (b *Bot) cachedFundamentals(ticker string) (*data.Fundamentals, error) {
	if fd, ok := b.fundamentalsCache.get(ticker); ok {
		return fd, nil
	}
	time.Sleep(finnhubRequestDelay)
	fd, err := b.fundamentals.GetFundamentals(ticker)
	if err != nil {
		return nil, err
	}
	b.fundamentalsCache.set(ticker, fd, slowDataCacheTTL)
	return fd, nil
}

func (b *Bot) cachedAnalystRating(ticker string) (*data.AnalystRating, error) {
	if ar, ok := b.analystRatingCache.get(ticker); ok {
		return ar, nil
	}
	time.Sleep(finnhubRequestDelay)
	ar, err := b.analystRating.GetAnalystRating(ticker)
	if err != nil {
		return nil, err
	}
	b.analystRatingCache.set(ticker, ar, slowDataCacheTTL)
	return ar, nil
}

func (b *Bot) cachedInsiderTx(ticker string) ([]data.InsiderTransaction, error) {
	if tx, ok := b.insiderTxCache.get(ticker); ok {
		return tx, nil
	}
	time.Sleep(finnhubRequestDelay)
	tx, err := b.insiderTx.GetInsiderTransactions(ticker, 10)
	if err != nil {
		return nil, err
	}
	b.insiderTxCache.set(ticker, tx, slowDataCacheTTL)
	return tx, nil
}

// cachedEarningsSurprises is Phase 23 PR8's Finnhub read, cached the same
// way cachedFundamentals/cachedAnalystRating/cachedInsiderTx are (in-memory,
// slowDataCacheTTL) — no separate DB table needed, unlike PR6/PR7's
// valuation snapshot: Finnhub's own trailing-4-quarter response is already
// the entire useful window every time, so there's nothing to accumulate
// across calls the way SEC's full company history was worth persisting.
func (b *Bot) cachedEarningsSurprises(ticker string) ([]data.EarningsSurprise, error) {
	if es, ok := b.earningsSurpriseCache.get(ticker); ok {
		return es, nil
	}
	time.Sleep(finnhubRequestDelay)
	es, err := b.earningsSurprise.GetEarningsSurprises(ticker)
	if err != nil {
		return nil, err
	}
	b.earningsSurpriseCache.set(ticker, es, slowDataCacheTTL)
	return es, nil
}

// cachedValuationSnapshot is PR6/PR7's on-demand valuation-history read —
// US via SEC EDGAR (b.secFundamentals), TW via FinMind's TaiwanStockPER
// (b.twValuation) — sharing one db.fundamental_snapshots cache/90-day TTL
// (§3.5's "按需抓 + 90天TTL"; PR7 explicitly shares PR6's table rather than
// duplicating the caching logic per market). Nil whenever the ticker's
// market has no configured provider — same nil-degrade convention as every
// other optional data source (SEC EDGAR has no ADR/20-F coverage either,
// §4.5, so a TW ticker never reaches b.secFundamentals and vice versa). A
// fetch/decode error falls back to the stale cached row (if any, itself
// possibly nil) rather than nothing, since a 90-day-old number still beats
// no number for a briefing-only line.
func (b *Bot) cachedValuationSnapshot(ticker string) *data.FundamentalSnapshot {
	var provider data.FundamentalHistoryProvider
	needsPriceHistory := false
	switch market.Of(ticker) {
	case market.US:
		provider = b.secFundamentals
		needsPriceHistory = true // SEC gives fiscal-year EPS; the percentile needs a price to pair it against
	case market.TW:
		provider = b.twValuation // FinMind's PER series already carries a percentile pool by itself
	}
	if provider == nil {
		return nil
	}

	cached, err := b.db.GetFundamentalSnapshot(ticker)
	if err != nil {
		logger.Errorf("valuation snapshot cache read %s: %v", ticker, err)
	}
	if cached != nil {
		// modernc.org/sqlite returns a DATETIME DEFAULT CURRENT_TIMESTAMP
		// column as RFC3339 (live-verified: "2026-08-20T14:48:18Z"), not
		// SQLite's own "YYYY-MM-DD HH:MM:SS" text format.
		if fetchedAt, err := time.Parse(time.RFC3339, cached.FetchedAt); err == nil && time.Since(fetchedAt) < valuationSnapshotTTLDays*24*time.Hour {
			return dbToDataSnapshot(cached)
		}
	}

	// Cache miss or stale — fetch fresh. US needs a much longer price
	// history than computeTechnicals' 1y candles for a meaningful
	// multi-fiscal-year P/E percentile (see data.SEC.GetFundamentalSnapshot);
	// TW's FinMind path ignores candles entirely (see data.FinMind's own
	// GetFundamentalSnapshot doc comment), so skip the extra fetch for it.
	var candles []data.Candle
	if needsPriceHistory {
		candles, err = b.history.GetHistory(ticker, "10y")
		if err != nil {
			logger.Errorf("valuation price history %s: %v", ticker, err)
			return dbToDataSnapshot(cached)
		}
	}
	snap, err := provider.GetFundamentalSnapshot(ticker, candles)
	if err != nil {
		logger.Errorf("valuation fundamentals %s: %v", ticker, err)
		return dbToDataSnapshot(cached)
	}
	if snap == nil {
		return dbToDataSnapshot(cached)
	}
	if err := b.db.SaveFundamentalSnapshot(db.FundamentalSnapshot{
		Ticker: snap.Ticker, EPSAnnual: snap.EPSAnnual, PERatio: snap.PERatio, PEPercentile: snap.PEPercentile,
		OCF: snap.OCF, NetIncome: snap.NetIncome, CashFlowQuality: snap.CashFlowQuality, AsOfFiscalYearEnd: snap.AsOfFiscalYearEnd,
	}); err != nil {
		logger.Errorf("valuation snapshot cache write %s: %v", ticker, err)
	}
	return snap
}

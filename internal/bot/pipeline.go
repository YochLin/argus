package bot

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/paper"
	"argus/internal/service"
	"argus/internal/signals"
)

// recommendationInputs bundles everything handleRecommend and RunDailyReport
// both need to build their llm.GenerateRecommendations call — assembled once
// by gatherRecommendationInputs so a new prompt input gets wired in exactly
// one place instead of both call sites having to stay in lockstep by hand
// (see docs/refactor-internal-bot.md). Each caller's own middle (daily
// report's earnings/signal/stop-loss checks) reads straight off these
// fields rather than this struct performing them, since only RunDailyReport
// wants them.
type recommendationInputs struct {
	watchlistTickers []string
	candidateTickers []string // deduped, watchlist tickers excluded
	scanHits         map[string]string
	positions        map[string]db.Position
	earnings         map[string]data.EarningsEvent
	marketNews       []data.NewsItem
	prevRecs         map[string]db.Recommendation
	marketContext    *llm.MarketContext // nil if SPY history and VIX quote both failed
	recentLessons    []llm.PastLesson   // Phase 3.9 cross-ticker feed, see loadRecentLessons
	watchlist        []llm.StockData    // fetchStockData output for watchlistTickers
	candidates       []llm.StockData    // fetchStockData output for candidateTickers
}

// gatherRecommendationInputs assembles the watchlist ∪ candidate set for
// market m, the positions/earnings/market-news/prior-recommendation context
// that feeds the LLM prompt, and the resulting []llm.StockData for both
// ticker sets. Returns the db.GetWatchlistByMarket error verbatim (both
// callers render it via the same KeyWatchlistQueryFailed message and abort).
//
// Candidate sourcing is asymmetric by market (Phase 6 PR2 §5.1, extended by
// the 2026-07-28 TW data-gap PR): US gets market-movers ∪ today's US-market
// scan hits, exactly as before PR2 existed. TW gets TWSE OpenAPI movers
// (b.twMovers, live-verified free/keyless — see internal/data/twse_movers.go)
// ∪ today's TW-market scan hits — exploreCandidates is still US-only (its
// own doc comment explains why), but movers itself is no longer a US-only
// gap. marketNews now also has a TW source (b.twMarketNews, cnyes) instead
// of nil-degrading for TW.
func (b *Bot) gatherRecommendationInputs(m market.MarketID) (recommendationInputs, error) {
	tickers, err := b.db.GetWatchlistByMarket(m)
	if err != nil {
		return recommendationInputs{}, err
	}

	var candidateTickers []string
	if m == market.US {
		candidateTickers, err = b.provider.GetMarketMovers()
		if err != nil {
			logger.Errorf("market movers: %v", err)
		}
	} else if b.twMovers != nil {
		candidateTickers, err = b.twMovers.GetMarketMovers()
		if err != nil {
			logger.Errorf("tw market movers: %v", err)
		}
	}
	scanHits := b.loadScanHits(m)
	dedupedCandidates := mergeCandidates(candidateTickers, scanHits, tickers)
	dedupedCandidates = b.rankAndTruncateCandidates(dedupedCandidates, m, candidatePrefilterCount)
	allTickers := append(append([]string{}, tickers...), dedupedCandidates...)

	positions := b.loadPositions()
	earnings := b.loadEarnings(allTickers)
	marketNews := b.loadMarketNews(m)
	prevRecs := b.loadPrevRecs(allTickers)
	marketContext := b.computeMarketRegime(m)
	pastLessons := b.loadPastLessons(allTickers)
	recentLessons := b.loadRecentLessons()

	watchlist := b.fetchStockData(tickers, true, positions, earnings, nil, prevRecs, pastLessons)
	candidates := b.fetchStockData(dedupedCandidates, false, positions, earnings, scanHits, prevRecs, pastLessons)

	return recommendationInputs{
		watchlistTickers: tickers,
		candidateTickers: dedupedCandidates,
		scanHits:         scanHits,
		positions:        positions,
		earnings:         earnings,
		marketNews:       marketNews,
		prevRecs:         prevRecs,
		marketContext:    marketContext,
		recentLessons:    recentLessons,
		watchlist:        watchlist,
		candidates:       candidates,
	}, nil
}

// recordLLMRun persists Phase 19's LLM-input-transparency audit row (see
// docs/phase-19-llm-transparency.md §4.3) — called from both handleRecommend
// and RunDailyReport right after their shared GenerateRecommendations call,
// so any future GenerateRecommendations-shaped call site reuses the same
// three lines instead of reimplementing the marshal. kind distinguishes a
// manual /recommend from a scheduled daily report ("recommend" /
// "daily_report"). Skipped when raw and model are both empty, which only
// happens when every backend in the chain failed outright (see
// llm.Client.prompt) — there's nothing informative to record beyond the
// error the caller already surfaces to the user. The marshaled payload
// mirrors GenerateRecommendations' own parameters field-for-field with no
// trimming (§3 decision 3: a missing field would defeat the point of a
// transparency feature). Marshal/insert failures are logged, never block
// /recommend or the daily report — this is an audit trail, not a write the
// user is waiting on.
func (b *Bot) recordLLMRun(kind string, m market.MarketID, in recommendationInputs, raw, model string, latencyMs int64) {
	if raw == "" && model == "" {
		return
	}

	payload := struct {
		Watchlist     []llm.StockData    `json:"watchlist"`
		Candidates    []llm.StockData    `json:"candidates"`
		MarketNews    []data.NewsItem    `json:"marketNews"`
		MarketContext *llm.MarketContext `json:"marketContext"`
		RecentLessons []llm.PastLesson   `json:"recentLessons"`
		IsTW          bool               `json:"isTW"`
	}{
		Watchlist:     in.watchlist,
		Candidates:    in.candidates,
		MarketNews:    in.marketNews,
		MarketContext: in.marketContext,
		RecentLessons: in.recentLessons,
		IsTW:          m == market.TW,
	}
	inputJSON, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("llm run: marshal input: %v", err)
		return
	}

	newsCount := len(in.marketNews)
	for _, s := range in.watchlist {
		newsCount += len(s.News)
	}
	for _, s := range in.candidates {
		newsCount += len(s.News)
	}
	gapCount := countCandleGaps(in.watchlist, m) + countCandleGaps(in.candidates, m)

	if err := b.db.InsertLLMRun(kind, m, model, latencyMs, string(inputJSON), raw, len(in.watchlist), len(in.candidates), newsCount, gapCount); err != nil {
		logger.Errorf("llm run: insert: %v", err)
	}
}

// countCandleGaps counts, across every stock's Candles, how many consecutive
// bar pairs skip more than one trading day — a stand-in for "Yahoo's chart
// API silently dropped a bar" (see internal/data's GetHistory doc comment
// for a documented instance of that). US uses internal/market.IsTradingDay's
// real NYSE calendar; TW falls back to a weekday-only check since
// internal/market doesn't cover the TWSE calendar (same limitation the prior
// client-side TS heuristic had for both markets — see the /llm audit page's
// data-quality panel).
func countCandleGaps(stocks []llm.StockData, m market.MarketID) int {
	gaps := 0
	for _, s := range stocks {
		for i := 1; i < len(s.Candles); i++ {
			if tradingDaysBetween(s.Candles[i-1].Date, s.Candles[i].Date, m) > 1 {
				gaps++
			}
		}
	}
	return gaps
}

// tradingDaysBetween counts trading days in (from, to], matching the
// semantics of the TS heuristic it replaces: exactly 1 for two consecutive
// trading days, >1 whenever a bar is missing in between.
func tradingDaysBetween(from, to time.Time, m market.MarketID) int {
	n := 0
	for d := from.AddDate(0, 0, 1); !d.After(to); d = d.AddDate(0, 0, 1) {
		if m == market.US {
			if market.IsTradingDay(d) {
				n++
			}
		} else if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n++
		}
	}
	return n
}

// sendAndSaveRecommendations formats LLM recommendations for Telegram and
// persists them dated today, each with its ticker's current price looked up
// from the already-fetched stock data — /track later compares that stored
// price against the price on review day. sources (ticker -> "watchlist"/
// "movers"/"scan", see recommendationSources) is persisted alongside so
// /track can break its hit rate down by candidate-sourcing path (Phase 3.8).
// Shared by /recommend and RunDailyReport, which otherwise mirror each other.
// Each recommendation goes out as its own message (see sendRecGroup) rather
// than one combined block, so its [Check]/[Buy]/[Sell] quick-action row
// (quick_actions.go, UX quick win) attaches to that ticker specifically —
// Telegram inline keyboards belong to one message, not a sub-section of one.
// m selects which market's account value buildSizingLines sizes BUY
// recommendations against (Phase 6 PR2). candidateCount is passed explicitly
// (rather than read off stockLists' last element positionally) so the
// candidates-empty message stays correct even if a caller's stockLists order
// ever changes.
func (b *Bot) sendAndSaveRecommendations(newsSummary string, recs []llm.Recommendation, sources map[string]string, m market.MarketID, candidateCount int, stockLists ...[]llm.StockData) {
	if newsSummary != "" {
		b.Send(i18n.T(b.lang, i18n.KeyMarketNewsSummaryTitle) + newsSummary)
	}

	prices := make(map[string]float64)
	atrs := make(map[string]float64)
	held := make(map[string]bool)
	for _, list := range stockLists {
		for _, s := range list {
			if s.Quote == nil {
				continue
			}
			prices[s.Quote.Ticker] = s.Quote.Price
			if s.Position != nil {
				held[s.Quote.Ticker] = true
			}
			if s.Technicals != nil && s.Technicals.ATR14 > 0 {
				atrs[s.Quote.Ticker] = s.Technicals.ATR14
			}
		}
	}

	sizing := b.buildSizingLines(recs, prices, atrs, m)

	displayRecs := filterRecsForDisplay(recs, held)
	watchlistRecs, candidateRecs := splitRecsBySource(displayRecs, sources)

	b.Send(i18n.T(b.lang, i18n.KeyRecommendationsTitle))
	b.sendRecGroup(i18n.KeyRecWatchlistSectionTitle, watchlistRecs, sizing, i18n.T(b.lang, i18n.KeyRecWatchlistNoneActionable))
	candidatesEmptyMsg := i18n.T(b.lang, i18n.KeyRecCandidatesAnalyzedNone, candidateCount)
	if candidateCount == 0 {
		candidatesEmptyMsg = i18n.T(b.lang, i18n.KeyRecCandidatesUnavailable)
	}
	b.sendRecGroup(i18n.KeyRecCandidatesSectionTitle, candidateRecs, sizing, candidatesEmptyMsg)

	var dbRecs []db.Recommendation
	for _, r := range recs {
		dbRecs = append(dbRecs, db.Recommendation{
			Ticker: r.Ticker,
			Action: r.Action,
			Reason: r.Reason,
			Price:  prices[r.Ticker],
			Source: sources[r.Ticker],
		})
	}
	if err := b.db.SaveRecommendations(todayDate(), dbRecs); err != nil {
		logger.Errorf("save recommendations: %v", err)
	}

	b.applyPaperTrades(recs, prices, atrs, m)
}

// filterRecsForDisplay is the rendering-side guard for the focused
// recommendation UX. The prompt asks the model to omit non-held watchlist
// HOLD blocks, but a model can still ignore that instruction. Keep the raw
// recommendations for persistence and tracking; only suppress those
// non-actionable display rows here.
func filterRecsForDisplay(recs []llm.Recommendation, held map[string]bool) []llm.Recommendation {
	var filtered []llm.Recommendation
	for _, r := range recs {
		if r.Action == "HOLD" && !held[r.Ticker] {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// splitRecsBySource divides recs into watchlist/held picks vs. new-candidate
// picks (sources classifies each ticker "watchlist"/"movers"/"scan" — see
// recommendationSources) so sendAndSaveRecommendations' message reads as two
// distinct groups instead of one flat list mixing "how's what I already
// have doing" with "here's something new to consider". A ticker missing
// from sources (shouldn't happen — sources is built from the same recs right
// before this call) falls into the candidates group rather than panicking on
// a nil map lookup.
func splitRecsBySource(recs []llm.Recommendation, sources map[string]string) (watchlistRecs, candidateRecs []llm.Recommendation) {
	for _, r := range recs {
		if sources[r.Ticker] == "watchlist" {
			watchlistRecs = append(watchlistRecs, r)
		} else {
			candidateRecs = append(candidateRecs, r)
		}
	}
	return watchlistRecs, candidateRecs
}

// sendRecGroup sends one section of sendAndSaveRecommendations' output: a
// title message followed by each recommendation in the group as its own
// message (with a [Check]/[Buy]/[Sell] quick-action row attached — see
// sendWithTickerActions), so the group's numbering that used to appear in
// one combined block is dropped rather than kept per-message, where it
// would just read as a stray "1." on every single message. Sends no title
// when recs is empty, so a day with no new-candidate picks doesn't leave a
// dangling header with nothing under it — instead sends emptyMsg standalone
// if non-empty. The watchlist section now passes an explicit "nothing
// actionable" message because filtering non-held HOLD rows makes an empty
// watchlist group a normal outcome, not an invisible one.
func (b *Bot) sendRecGroup(titleKey i18n.Key, recs []llm.Recommendation, sizing map[string]string, emptyMsg string) {
	if len(recs) == 0 {
		if emptyMsg != "" {
			b.Send(emptyMsg)
		}
		return
	}
	b.Send(i18n.T(b.lang, titleKey))
	for _, r := range recs {
		b.sendWithTickerActions(r.Ticker, formatRecLine(b.lang, r, sizing, b.tickerLabel(r.Ticker)))
	}
}

// formatRecLine renders one recommendation as a standalone message body —
// pulled out of sendRecGroup so it's testable without a live Bot/Telegram
// API. sizing is buildSizingLines' ticker->KeySizingLine text (Phase 3.11
// PR1 §3.4) — nil or a missing entry just renders no sizing line for that
// rec, same degrade-by-omission convention as everywhere else in this
// pipeline. label is the display form of r.Ticker (see Bot.tickerLabel) —
// passed in rather than computed here so this stays a pure, Bot-free
// function.
func formatRecLine(lang i18n.Lang, r llm.Recommendation, sizing map[string]string, label string) string {
	var sb strings.Builder
	if r.Action != "" {
		fmt.Fprintf(&sb, "*%s* — %s\n%s\n", label, r.Action, r.Reason)
	} else {
		fmt.Fprintf(&sb, "*%s*\n%s\n", label, r.Reason)
	}
	if line, ok := sizing[r.Ticker]; ok {
		sb.WriteString(line)
	}
	return sb.String()
}

// buildSizingLines computes Phase 3.11 PR1's KeySizingLine for every BUY
// recommendation with a usable current price and ATR14 (§3.4): bot-side,
// deterministic arithmetic (paper.SuggestShares), never left to the LLM. Returns
// nil outright when the whole feature is disabled (b.riskPctPerTrade <= 0)
// or there's no account-value figure to size against yet (accountValue);
// a ticker missing a price or ATR14 (candidates always have Technicals, but
// a quote or history fetch can still fail — see fetchStockData) is simply
// left out of the returned map rather than failing the whole batch. m
// selects which market's account value (net worth + cash, never summed
// across markets) sizing is computed against — Phase 6 PR2 threads this
// through now that both US and TW recommendation runs call this (PR1 always
// passed market.US since only US recs existed yet).
func (b *Bot) buildSizingLines(recs []llm.Recommendation, prices, atrs map[string]float64, m market.MarketID) map[string]string {
	if b.riskPctPerTrade <= 0 {
		return nil
	}
	accountVal, ok := b.accountValue(m)
	if !ok {
		return nil
	}

	lines := make(map[string]string)
	for _, r := range recs {
		if r.Action != "BUY" {
			continue
		}
		price, ok := prices[r.Ticker]
		if !ok || price <= 0 {
			continue
		}
		atr, ok := atrs[r.Ticker]
		if !ok || atr <= 0 {
			continue
		}
		stop := price - stopCandidateATRMult*atr
		shares := paper.SuggestShares(accountVal, b.riskPctPerTrade, price, stop)
		if shares <= 0 {
			continue
		}
		riskBudget := accountVal * b.riskPctPerTrade / 100
		lines[r.Ticker] = i18n.T(b.lang, i18n.KeySizingLine, b.money(r.Ticker, riskBudget), b.money(r.Ticker, stop), shares)
	}
	return lines
}

// maxScanHitFundamentals caps how many scan-hit candidates fetchStockData
// will fetch fundamentals/analyst rating for on any single run (see
// capScanHitTickers). Scan hits are normally 0-a few a day, but the
// theoretical worst case is the whole day's universe-scan chunk (~100
// tickers) hitting at once — uncapped, that's +200 Finnhub requests in one
// run. This is a safety cap, not a tunable policy knob (like scanChunkCount/
// benchmarkTicker), so it's a plain const rather than an env var — see
// docs/phase-3.7-scanhit-fundamentals.md.
const maxScanHitFundamentals = 5

// capScanHitTickers selects up to max tickers from scanReasons' keys — in
// lexical order, for determinism (a same-day re-run of /recommend picks the
// same batch) — to receive the watchlist-grade fundamentals/analyst-rating
// fetch normally reserved for includeFundamentals callers (see
// docs/phase-3.7-scanhit-fundamentals.md). Tickers beyond max are logged and
// left out, not dropped from the candidate list entirely — they still carry
// their ScanReason/Technicals, just without fundamentals this run.
func capScanHitTickers(scanReasons map[string]string, max int) map[string]bool {
	if len(scanReasons) == 0 {
		return nil
	}
	tickers := make([]string, 0, len(scanReasons))
	for t := range scanReasons {
		tickers = append(tickers, t)
	}
	sort.Strings(tickers)

	out := make(map[string]bool, max)
	for i, t := range tickers {
		if i >= max {
			logger.Debugf("scan-hit fundamentals: skipping %s (over cap of %d)", t, max)
			continue
		}
		out[t] = true
	}
	return out
}

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
// slowDataCacheTTL bounds how often fundamentals/analyst-rating/insider-tx
// are refetched per ticker (via b.dataCache) — these move over days, not
// within a single day, so a report doesn't need a fresh Finnhub call for
// them every run. Entries just expire and refetch on whatever day they age
// out, no per-ticker staleness bookkeeping needed.
// finnhubRequestDelay paces every real Finnhub-bound call this function
// makes (quote/news always for a US ticker, fundamentals/analyst/insider on
// a cache miss) so a large watchlist can't burst past Finnhub's free-tier
// 60-requests/minute ceiling in the first few seconds of a report run — see
// the "watchlist rate limit" discussion this addresses.
const (
	slowDataCacheTTL    = 4 * 24 * time.Hour
	finnhubRequestDelay = 1100 * time.Millisecond
)

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
		if candles, err := b.history.GetHistory(benchmarkFor(m), "1y"); err == nil {
			closes = data.Closes(candles)
		}
		benchCloses[m] = closes
		return closes
	}

	var result []llm.StockData
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
		news, _ := b.provider.GetNews(t, 5)
		stock := llm.StockData{Quote: q, News: news, CompanyName: b.companyName(t)}
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
			stock.Position = &llm.Position{Shares: p.Shares, AvgCost: p.AvgCost}
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

// cachedFundamentals/cachedAnalystRating/cachedInsiderTx each check
// b.dataCache before hitting Finnhub, and pace the actual call with
// finnhubRequestDelay on a miss — see slowDataCacheTTL's comment for why
// these three (and only these three) are cached across reports.
func (b *Bot) cachedFundamentals(ticker string) (*data.Fundamentals, error) {
	key := "fundamentals:" + ticker
	if v, ok := b.dataCache.get(key); ok {
		return v.(*data.Fundamentals), nil
	}
	time.Sleep(finnhubRequestDelay)
	fd, err := b.fundamentals.GetFundamentals(ticker)
	if err != nil {
		return nil, err
	}
	b.dataCache.set(key, fd, slowDataCacheTTL)
	return fd, nil
}

func (b *Bot) cachedAnalystRating(ticker string) (*data.AnalystRating, error) {
	key := "analystRating:" + ticker
	if v, ok := b.dataCache.get(key); ok {
		return v.(*data.AnalystRating), nil
	}
	time.Sleep(finnhubRequestDelay)
	ar, err := b.analystRating.GetAnalystRating(ticker)
	if err != nil {
		return nil, err
	}
	b.dataCache.set(key, ar, slowDataCacheTTL)
	return ar, nil
}

func (b *Bot) cachedInsiderTx(ticker string) ([]data.InsiderTransaction, error) {
	key := "insiderTx:" + ticker
	if v, ok := b.dataCache.get(key); ok {
		return v.([]data.InsiderTransaction), nil
	}
	time.Sleep(finnhubRequestDelay)
	tx, err := b.insiderTx.GetInsiderTransactions(ticker, 10)
	if err != nil {
		return nil, err
	}
	b.dataCache.set(key, tx, slowDataCacheTTL)
	return tx, nil
}

// cachedEarningsSurprises is Phase 23 PR8's Finnhub read, cached the same
// way cachedFundamentals/cachedAnalystRating/cachedInsiderTx are (in-memory,
// slowDataCacheTTL) — no separate DB table needed, unlike PR6/PR7's
// valuation snapshot: Finnhub's own trailing-4-quarter response is already
// the entire useful window every time, so there's nothing to accumulate
// across calls the way SEC's full company history was worth persisting.
func (b *Bot) cachedEarningsSurprises(ticker string) ([]data.EarningsSurprise, error) {
	key := "earningsSurprise:" + ticker
	if v, ok := b.dataCache.get(key); ok {
		return v.([]data.EarningsSurprise), nil
	}
	time.Sleep(finnhubRequestDelay)
	es, err := b.earningsSurprise.GetEarningsSurprises(ticker)
	if err != nil {
		return nil, err
	}
	b.dataCache.set(key, es, slowDataCacheTTL)
	return es, nil
}

// valuationSnapshotTTLDays is Phase 23 PR6/PR7's 90-day cache lifetime
// (docs/phase-23-strategy-data-uplift.md §3.5) — both SEC's companyfacts and
// FinMind's TaiwanStockPER only move on a real filing/trading day, so
// refetching more often just burns rate budget for no new information.
const valuationSnapshotTTLDays = 90

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

func dbToDataSnapshot(s *db.FundamentalSnapshot) *data.FundamentalSnapshot {
	if s == nil {
		return nil
	}
	return &data.FundamentalSnapshot{
		Ticker: s.Ticker, EPSAnnual: s.EPSAnnual, PERatio: s.PERatio, PEPercentile: s.PEPercentile,
		OCF: s.OCF, NetIncome: s.NetIncome, CashFlowQuality: s.CashFlowQuality, AsOfFiscalYearEnd: s.AsOfFiscalYearEnd,
	}
}

// computeTechnicals fetches ticker's daily-candle history and reduces it to
// the RSI/MACD/moving-average values an LLM prompt needs (see
// llm.Technicals), plus the most recent promptCandleCount raw candles for
// llm.StockData.Candles — both from the one GetHistory call, so the K-line
// context costs no extra fetch. Returns nils (not an error) on a
// history-fetch failure, so callers degrade the same way the fundamentals
// fetch above does.
const (
	bollingerPeriod    = 20
	bollingerNumStdDev = 2.0
	promptCandleCount  = 60
)

func (b *Bot) computeTechnicals(ticker string, spyCloses []float64) (*llm.Technicals, []data.Candle, []llm.StrategyHitInfo) {
	candles, err := b.history.GetHistory(ticker, "1y")
	if err != nil {
		logger.Errorf("history %s: %v", ticker, err)
		return nil, nil, nil
	}
	closes := data.Closes(candles)
	highs := data.Highs(candles)
	lows := data.Lows(candles)
	volumes := data.Volumes(candles)

	t := &llm.Technicals{
		RSI14:       signals.RSI(closes, 14),
		MACDTrend:   signals.MACDTrend(closes),
		MA5:         signals.MA(closes, 5),
		MA20:        signals.MA(closes, 20),
		MA50:        signals.MA(closes, 50),
		MA60:        signals.MA(closes, 60),
		MA200:       signals.MA(closes, 200),
		VolumeRatio: signals.VolumeRatio(volumes, 20),
		ATR14:       signals.ATR(highs, lows, closes, 14),
		MAAlign:     signals.MAAlignment(closes),
		VolumePrice: signals.VolumePriceSignal(closes, volumes),
		NewHigh20:   signals.IsNewHigh(closes, 20),
		NewHigh52w:  signals.IsNewHigh(closes, len(closes)),
	}

	if len(closes) >= 26+9 {
		macdLine, _, _ := signals.MACD(closes)
		t.MACDAboveZero = &macdLine
	}

	if k, d := signals.StochasticSeries(highs, lows, closes, 9, 3); k != nil && len(k) > 0 {
		t.StochK = &k[len(k)-1]
		t.StochD = &d[len(d)-1]
	}

	if bw := signals.BollingerBandwidthSeries(closes, bollingerPeriod, bollingerNumStdDev); bw != nil && len(bw) > 0 {
		t.Bandwidth = &bw[len(bw)-1]
	}

	if rs, ok := signals.RelativeStrength(closes, spyCloses, 63); ok {
		t.RS63 = &rs
	}

	if len(volumes) > 0 {
		t.Volume = volumes[len(volumes)-1]
	}
	if pctB, ok := signals.BollingerPctB(closes, bollingerPeriod, bollingerNumStdDev); ok {
		t.BollingerPctB = &pctB
	}

	screenParams := signals.DefaultScreenParams(market.Of(ticker))
	var stratHits []llm.StrategyHitInfo
	if hit := signals.SqueezeBreakout(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	if hit := signals.BoxBottomRebound(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	if hit := signals.TrendBreakout(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	if hit := signals.TrendPullback(candles, screenParams); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}

	recent := candles
	if len(recent) > promptCandleCount {
		recent = recent[len(recent)-promptCandleCount:]
	}
	return t, recent, stratHits
}

// stopCandidateATRMult is the ATR multiplier for the volatility-adaptive
// stop candidate (§3.2/§3.4 of docs/phase-3.11-trade-risk-management.md) —
// fixed at 2x, same as the existing ATR trailing-stop's own default
// reasoning (Phase 3.8 追加項), not a separate env knob.
const stopCandidateATRMult = 2.0

// stopSuggestion bundles Phase 3.11's three candidate stop-loss reference
// prices (see /stop and the /buy suggestion line) plus the latest close they
// were computed against. Low10/Low20/ATRBased are 0 when there isn't enough
// history for that particular candidate (same "0 = not enough data"
// sentinel signals.MA already uses) — callers must skip rendering a
// zero-valued candidate rather than showing a misleading $0.00.
type stopSuggestion = service.StopSuggestion

// computeStopSuggestion fetches ticker's OHLCV history and reduces it to
// stopSuggestion's three candidates, all computed against LatestClose.
func (b *Bot) computeStopSuggestion(ticker string) (stopSuggestion, bool) {
	if b.risks() == nil {
		return stopSuggestion{}, false
	}
	return b.risks().ComputeStopSuggestion(ticker)
}

// accountValue is Phase 3.11's account-size input for paper.SuggestShares: m's
// latest recorded net worth (position market value as of the last closing
// snapshot) plus m's declared cash — the same cash source /insight and
// RunWeeklyReview already use (see loadCash), not a separate live
// computation. Phase 6 makes this per-market (never summed across TWD/USD,
// see docs/phase-6-tw-market.md §3.2). ok is false only when there's no net
// worth snapshot on record at all for m (e.g. before the first closing
// snapshot has ever run for that market); missing cash just leaves it out
// rather than failing the whole lookup, same as loadCash's own callers
// already tolerate "never set".
func (b *Bot) accountValue(m market.MarketID) (float64, bool) {
	_, total, ok, err := b.db.GetLatestNetWorth(m)
	if err != nil {
		logger.Errorf("account value: net worth: %v", err)
	}
	if !ok {
		return 0, false
	}
	if cash, cashOK, err := b.loadCash(m); err != nil {
		logger.Errorf("account value: cash: %v", err)
	} else if cashOK {
		total += cash
	}
	return total, true
}

// computeMarketRegime builds Phase 3.7 追加項's broad-market context block
// (see docs/phase-3.7-market-regime.md and llm.MarketContext): benchmarkFor
// (m)'s own trend (SPY for US, 0050 for TW as of Phase 6 PR2 — last close
// from a single GetHistory call, no separate GetQuote) and a volatility
// reading — US gets ^VIX's latest level (via the ordinary Multi quote
// chain — Finnhub returns an error-shaped-but-200 body for CFD indices it
// doesn't support, which decodes as an all-zero quote and falls through to
// Yahoo exactly like any other "no data" quote, confirmed by live testing,
// see the design doc). TW gets VolProxyPct instead (2026-07-28 TW data-gap
// PR): no TW volatility-index dataset exists at all, free or paid (FinMind's
// TaiwanOptionVix live-tested behind a paid-tier wall), so this computes
// ATR14/close as a percentage off the same 0050 candles already fetched for
// the benchmark line above — zero extra network call, a rougher proxy than
// an options-implied index, but the best available without one. Either half
// failing just logs and leaves that half's fields at 0 (skipped by
// writeMarketContext's per-field rendering); both failing returns nil so the
// caller sees "no regime data" rather than an all-zero struct.
func (b *Bot) computeMarketRegime(m market.MarketID) *llm.MarketContext {
	var mc llm.MarketContext

	bench := benchmarkFor(m)
	mc.Bench = bench
	candles, err := b.history.GetHistory(bench, "1y")
	if err != nil {
		logger.Errorf("market regime: %s history: %v", bench, err)
	} else if len(candles) > 0 {
		closes := data.Closes(candles)
		mc.SPYPrice = closes[len(closes)-1]
		mc.SPYMA50 = signals.MA(closes, 50)
		mc.SPYMA200 = signals.MA(closes, 200)

		if m == market.TW {
			if atr := signals.ATR(data.Highs(candles), data.Lows(candles), closes, 14); atr > 0 && mc.SPYPrice > 0 {
				mc.VolProxyPct = atr / mc.SPYPrice * 100
			}
		}
	}

	if m == market.US {
		if q, err := b.provider.GetQuote(vixTicker); err != nil {
			logger.Errorf("market regime: %s quote: %v", vixTicker, err)
		} else {
			mc.VIX = q.Price
		}
	}

	if mc.SPYPrice == 0 && mc.VIX == 0 {
		return nil
	}
	return &mc
}

// isBearRegime returns true if the market context indicates a weak/bear regime
// (SPY below its 50-day or 200-day moving average).
func isBearRegime(mc *llm.MarketContext) bool {
	if mc == nil || mc.SPYPrice == 0 {
		return false
	}
	if mc.SPYMA50 > 0 && mc.SPYPrice < mc.SPYMA50 {
		return true
	}
	if mc.SPYMA200 > 0 && mc.SPYPrice < mc.SPYMA200 {
		return true
	}
	return false
}

// loadPositions returns every open position keyed by ticker, for attaching
// cost-basis context to LLM prompts. A query failure logs and degrades to an
// empty map rather than failing the caller — recommendations without cost
// basis are still useful.
func (b *Bot) loadPositions() map[string]db.Position {
	positions, err := b.db.GetPositions()
	if err != nil {
		logger.Errorf("load positions: %v", err)
		return nil
	}
	out := make(map[string]db.Position, len(positions))
	for _, p := range positions {
		out[p.Ticker] = p
	}
	return out
}

// loadPrevRecs returns each ticker's most recent recommendation on record
// (across any past date), keyed by ticker, for Phase 3.8's recommendation
// continuity (see llm.StockData.PrevRec). Degrades to nil on a query failure
// or an empty ticker list — same optional-data pattern as
// fundamentals/earnings/positions.
func (b *Bot) loadPrevRecs(tickers []string) map[string]db.Recommendation {
	if len(tickers) == 0 {
		return nil
	}
	recs, err := b.db.GetLatestRecommendations(tickers)
	if err != nil {
		logger.Errorf("load prev recommendations: %v", err)
		return nil
	}
	return recs
}

// maxRecentLessons caps how many cross-ticker general lessons
// loadRecentLessons feeds into every /recommend/daily-report prompt (Phase
// 3.9's "跨 ticker 最近 N 筆通用教訓" — see
// docs/research-tradingagents.md). A plain const, not an env var, same
// reasoning as maxExploreNominations: this is a prompt-size/relevance
// trade-off, not a user preference, and ACP's Pro/Max auth means there's no
// per-call billing pressure to tune it against.
const maxRecentLessons = 5

// loadPastLessons returns every trade-review lesson on record for any
// ticker in tickers, keyed by ticker — the "same ticker: bring all of them"
// half of Phase 3.9's feedback loop (see docs/research-tradingagents.md and
// llm.StockData.PastLessons). Degrades to nil (not an error) on a query
// failure or an empty ticker list — same optional-data pattern as
// fundamentals/earnings/positions; a prompt without past-lesson context is
// still useful.
func (b *Bot) loadPastLessons(tickers []string) map[string][]db.Lesson {
	if len(tickers) == 0 {
		return nil
	}
	lessons, err := b.db.GetLessonsForTickers(tickers)
	if err != nil {
		logger.Errorf("load past lessons: %v", err)
		return nil
	}
	return lessons
}

// loadRecentLessons returns the most recent maxRecentLessons trade-review
// lessons across every ticker — the "cross ticker: recent N, general" half
// of Phase 3.9's feedback loop (see loadPastLessons above and
// llm.GenerateRecommendations' recentLessons parameter). Degrades to nil on
// a query failure, same optional-data pattern as the rest of this file.
func (b *Bot) loadRecentLessons() []llm.PastLesson {
	rows, err := b.db.GetRecentLessons(maxRecentLessons)
	if err != nil {
		logger.Errorf("load recent lessons: %v", err)
		return nil
	}
	out := make([]llm.PastLesson, len(rows))
	for i, l := range rows {
		out[i] = llm.PastLesson{Ticker: l.Ticker, Date: l.Date, Lesson: l.Lesson}
	}
	return out
}

// portfolioStocks assembles the per-position StockData that the
// portfolio-level LLM calls (/insight and the weekly review) both feed their
// prompts, scoped to a single market: quotes/technicals/fundamentals via
// fetchStockData, plus each position's earnings date, holding thesis and
// vs-benchmark return. Returns nil, nil when m has no positions at all, so
// callers skip that market's message entirely rather than sending an empty
// one. Market-scoped because mixing markets into one prompt sums TWD and USD
// position values into a single meaningless total (see
// llm.buildWeeklyReviewPrompt).
func (b *Bot) portfolioStocks(m market.MarketID, positions []db.Position) ([]llm.StockData, map[string]data.EarningsEvent) {
	var tickers []string
	positionsMap := make(map[string]db.Position, len(positions))
	for _, p := range positions {
		if market.Of(p.Ticker) != m {
			continue
		}
		tickers = append(tickers, p.Ticker)
		positionsMap[p.Ticker] = p
	}
	if len(tickers) == 0 {
		return nil, nil
	}

	earnings := b.loadEarnings(tickers)
	stocks := b.fetchStockData(tickers, true, positionsMap, earnings, nil, nil, nil)

	theses := b.loadTheses(tickers)
	vsBench := b.loadVsSPY(stocks, positionsMap)
	for i := range stocks {
		ticker := stocks[i].Quote.Ticker
		if th, ok := theses[ticker]; ok {
			stocks[i].Thesis = &th
		}
		if v, ok := vsBench[ticker]; ok {
			stocks[i].VsSPY = &v
		}
	}
	return stocks, earnings
}

// loadEarnings returns each ticker's next scheduled earnings date within
// earningsPromptWindowDays, keyed by ticker: Finnhub's real per-company date
// for a US ticker, and (2026-07-28 TW data-gap PR) data.GetTWUpcomingEarnings'
// statutory-deadline proxy for a TW one — the latter needs no API/nil-check,
// it's a pure calendar function, and a TW ticker simply never matches
// Finnhub's US-only calendar so both sources can be queried unconditionally
// without any market branching here. Finnhub's half still degrades to
// nothing (not an error) when it isn't configured or the request fails —
// same optional-data pattern as fundamentals.
func (b *Bot) loadEarnings(tickers []string) map[string]data.EarningsEvent {
	if len(tickers) == 0 {
		return nil
	}
	out := data.GetTWUpcomingEarnings(tickers, earningsPromptWindowDays, b.now())

	if b.earnings != nil {
		events, err := b.earnings.GetUpcomingEarnings(tickers, earningsPromptWindowDays)
		if err != nil {
			logger.Errorf("earnings calendar: %v", err)
		} else if out == nil {
			out = events
		} else {
			for t, e := range events {
				out[t] = e
			}
		}
	}
	return out
}

// loadTheses returns each ticker's recorded holding thesis (see /thesis,
// handleThesis), keyed by ticker — only tickers with one on record appear in
// the map. A per-ticker query failure logs and skips that ticker rather than
// aborting the whole call; unlike fundamentals/earnings this hits local
// SQLite, not a rate-limited external API, so a plain loop (not a batched
// query) is fine at the handful-of-positions scale /insight runs at.
func (b *Bot) loadTheses(tickers []string) map[string]string {
	out := make(map[string]string, len(tickers))
	for _, t := range tickers {
		thesis, ok, err := b.db.GetThesis(t)
		if err != nil {
			logger.Errorf("load thesis %s: %v", t, err)
			continue
		}
		if ok {
			out[t] = thesis
		}
	}
	return out
}

// computeVsSPY is the pure percentage math behind the Phase 3.6 expansion's
// "逐檔 vs SPY" item: a position's own holding-period return next to SPY's
// over the same period. Split out from loadVsSPY (which owns the DB/quote
// lookups) so the arithmetic is independently testable, same shape as
// breachAlertDecision.
func computeVsSPY(currentPrice, avgCost, spyPrice, spyEntryClose float64, bench string) llm.VsSPYReturn {
	return llm.VsSPYReturn{
		TickerPct: (currentPrice - avgCost) / avgCost * 100,
		SPYPct:    (spyPrice - spyEntryClose) / spyEntryClose * 100,
		Bench:     bench,
	}
}

// loadVsSPY computes computeVsSPY for every position in stocks that has both
// a BUY date on record (db.GetEarliestBuyDate) and a same-date benchmark
// close in daily_snapshots (populated by snapshotBenchmark since Phase 3.8)
// — a position missing either is simply omitted from the result, not an
// error (e.g. a pre-Phase-3.8 buy predates the benchmark ever being
// snapshotted). Reuses stocks' already-fetched Quote.Price rather than a
// second GetQuote call per ticker. Phase 6 PR2: stocks can now span both
// markets (RunWeeklyReview/handleInsight pass every held position), so the
// benchmark is selected per position via market.Of (SPY for US, 0050 for
// TW) rather than a single shared SPY quote — benchQuotes lazily fetches and
// caches each market's current quote, same "only pay for what's actually
// used" shape as fetchStockData's loadBenchCloses.
func (b *Bot) loadVsSPY(stocks []llm.StockData, positions map[string]db.Position) map[string]llm.VsSPYReturn {
	benchQuotes := make(map[market.MarketID]*data.Quote)
	loadBenchQuote := func(m market.MarketID) *data.Quote {
		if q, ok := benchQuotes[m]; ok {
			return q
		}
		ticker := benchmarkFor(m)
		q, err := b.provider.GetQuote(ticker)
		if err != nil {
			logger.Errorf("vs-spy: benchmark %s quote: %v", ticker, err)
			q = nil
		}
		benchQuotes[m] = q
		return q
	}

	out := make(map[string]llm.VsSPYReturn, len(stocks))
	for _, s := range stocks {
		ticker := s.Quote.Ticker
		p, ok := positions[ticker]
		if !ok || p.AvgCost == 0 {
			continue
		}
		m := market.Of(ticker)
		benchQuote := loadBenchQuote(m)
		if benchQuote == nil {
			continue
		}
		buyDate, ok, err := b.db.GetEarliestBuyDate(ticker)
		if err != nil {
			logger.Errorf("vs-spy: earliest buy %s: %v", ticker, err)
			continue
		}
		if !ok {
			continue
		}
		benchEntryClose, ok, err := b.db.GetSnapshotClose(benchmarkFor(m), buyDate)
		if err != nil {
			logger.Errorf("vs-spy: benchmark snapshot %s: %v", ticker, err)
			continue
		}
		if !ok || benchEntryClose == 0 {
			continue
		}
		out[ticker] = computeVsSPY(s.Quote.Price, p.AvgCost, benchQuote.Price, benchEntryClose, benchmarkFor(m))
	}
	return out
}

// loadMarketNews returns up to marketNewsLimit general market/macro news
// items for market m's recommendation prompt news-summary section — Finnhub
// (b.marketNews) for US, cnyes (b.twMarketNews) for TW (2026-07-28 TW
// data-gap PR; TW previously nil-degraded here unconditionally). Degrades to
// nil (not an error) when the relevant provider isn't configured/nil or the
// request fails — same optional-data pattern as fundamentals/earnings; a nil
// result means GenerateRecommendations simply omits the summary.
func (b *Bot) loadMarketNews(m market.MarketID) []data.NewsItem {
	provider := b.marketNews
	if m == market.TW {
		provider = b.twMarketNews
	}
	if provider == nil {
		return nil
	}
	items, err := provider.GetMarketNews(marketNewsLimit)
	if err != nil {
		logger.Errorf("market news: %v", err)
		return nil
	}
	return items
}

// loadScanHits returns today's Phase 2.6 universe-scan hits keyed by ticker
// (joined reason string per ticker) via db.GetScanHits, filtered to market m
// — scan_hits carries no market column of its own (see internal/db's Phase 6
// migration note: every reader that has the ticker in hand can derive it),
// so this filters client-side via market.Of, same pattern
// recordNetWorthSnapshot uses for db.GetPositions(). Degrades to nil (not an
// error) on a query failure — candidates without a scan reason still go
// through movers as before.
func (b *Bot) loadScanHits(m market.MarketID) map[string]string {
	hits, err := b.db.GetScanHits(todayDate())
	if err != nil {
		logger.Errorf("scan hits: %v", err)
		return nil
	}
	out := make(map[string]string, len(hits))
	for t, reason := range hits {
		if market.Of(t) == m {
			out[t] = reason
		}
	}
	return out
}

// computeTrackRows re-runs /track's core computation for the given lookback
// window: for each recommendation since then, look up the price at
// recommendation time and now, score BUY/SELL hits against the same-period
// market benchmark (TrackHit), and render each into a display line. Shared by
// handleTrack (its own full display) and RunWeeklyReview's strategy-feedback
// block (which only needs rows, to summarize via summarizeTrack — lines
// go unused there, same "compute once, let callers use what they need"
// shape as fetchStockData). ok=false means there were no recommendations in
// the window at all (not an error) — callers render that as "nothing to
// report" rather than an empty summary.
func (b *Bot) computeTrackRows(days int) (rows []trackRow, lines []string, ok bool, err error) {
	tracker := b.recommendations()
	if tracker == nil {
		return nil, nil, false, fmt.Errorf("recommendation service unavailable")
	}
	report, err := tracker.Track(days)
	if err != nil {
		return nil, nil, false, err
	}
	if !report.HasRecommendations {
		return nil, nil, false, nil
	}

	loggedBenchmarkErrors := make(map[string]bool)
	for _, detail := range report.Details {
		if !detail.HasBasePrice {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineNoPrice, detail.Date, detail.Ticker, detail.Action))
			continue
		}
		if !detail.HasQuote {
			if detail.QuoteErr != nil {
				logger.Errorf("track: quote %s: %v", detail.Ticker, detail.QuoteErr)
			}
			lines = append(lines, i18n.T(b.lang, i18n.KeyQuoteUnavailable, detail.Ticker))
			continue
		}
		if detail.BenchmarkErr != nil && !loggedBenchmarkErrors[detail.BenchmarkTicker] {
			logger.Errorf("track: benchmark %s: %v", detail.BenchmarkTicker, detail.BenchmarkErr)
			loggedBenchmarkErrors[detail.BenchmarkTicker] = true
		}

		verdict := ""
		if detail.Action == "BUY" || detail.Action == "SELL" {
			verdict = "❌"
			if detail.Hit {
				verdict = "✅"
			}
		}
		if detail.HasBenchmark {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineVsSPY, detail.Date, detail.Ticker, detail.Action, b.money(detail.Ticker, detail.BasePrice), b.money(detail.Ticker, detail.CurrentPrice), detail.ChangePct, detail.BenchmarkTicker, detail.BenchmarkChangePct, verdict))
		} else {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLine, detail.Date, detail.Ticker, detail.Action, b.money(detail.Ticker, detail.BasePrice), b.money(detail.Ticker, detail.CurrentPrice), detail.ChangePct, verdict))
		}
	}

	return report.Rows, lines, true, nil
}

// renderEarningsPreview formats a consolidated "upcoming earnings" list from
// a ticker->EarningsEvent map (as returned by loadEarnings), limited to
// events within days days out and sorted soonest-first — RunWeeklyReview's
// Sunday-planning view (PLAN.md's Phase 3.6 PR2), distinct from
// checkEarningsAlerts' narrower 3-day proactive alert and from each
// position's own per-ticker earnings line already embedded by
// writeStockSection via fetchStockData. Returns "" when nothing falls in
// the window, so RunWeeklyReview can skip the block entirely.
func renderEarningsPreview(lang i18n.Lang, earnings map[string]data.EarningsEvent, days int) string {
	type entry struct {
		ticker    string
		date      string
		daysUntil int
		estimated bool
	}
	var entries []entry
	for t, e := range earnings {
		d := daysUntil(e.Date)
		if d < 0 || d > days {
			continue
		}
		entries = append(entries, entry{ticker: t, date: e.Date, daysUntil: d, estimated: e.Estimated})
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].daysUntil < entries[j].daysUntil })

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyEarningsPreviewTitle))
	for _, e := range entries {
		key := i18n.KeyWeeklyEarningsPreviewLine
		if e.estimated {
			key = i18n.KeyWeeklyEarningsPreviewLineEstimated
		}
		sb.WriteString(i18n.T(lang, key, e.ticker, e.date, e.daysUntil))
	}
	return sb.String()
}

// mergeCandidates combines the market-movers list with today's Phase 2.6
// universe-scan hits into the final candidate ticker list: movers first
// (existing behavior preserved), then any scan-hit ticker not already
// present, finally excluding anything already on the watchlist (exclude).
func mergeCandidates(movers []string, scanHits map[string]string, exclude []string) []string {
	seen := make(map[string]bool, len(movers)+len(scanHits))
	excluded := make(map[string]bool, len(exclude))
	for _, t := range exclude {
		excluded[t] = true
	}

	var out []string
	add := func(t string) {
		if seen[t] || excluded[t] {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, t := range movers {
		add(t)
	}
	for t := range scanHits {
		add(t)
	}
	return out
}

// candidatePrefilterCount is Phase 23 PR9's rule-score cutoff
// (docs/phase-23-strategy-data-uplift.md §5: "規則分粗篩到 20") — the daily
// candidate pool (movers ∪ scan hits, 40-55/day before this) is too big for
// a "credible briefing"; the LLM's own KeyRecTaskBlock cap (最多 5 檔) does
// the final pick, so this prefilter only needs to not drop a real candidate,
// not be precise (§4.2/§4.3).
const candidatePrefilterCount = 20

// rankAndTruncateCandidates keeps only the top n of tickers by a rule score
// combining relative strength, liquidity, and distance to the nearest
// support/resistance level (§4.2's ranking-factor layer — deliberately no
// hard filter, §4.2: "硬濾網一律空的"). A no-op when tickers already fits
// within n. Order of the kept tickers doesn't matter to any caller.
func (b *Bot) rankAndTruncateCandidates(tickers []string, m market.MarketID, n int) []string {
	if len(tickers) <= n {
		return tickers
	}

	var benchCloses []float64
	if candles, err := b.history.GetHistory(benchmarkFor(m), "1y"); err == nil {
		benchCloses = data.Closes(candles)
	}

	type rawScore struct {
		ticker         string
		relStrength    float64
		hasRelStrength bool
		avgDollarVol   float64
		distToLevel    float64
		hasDistToLevel bool
	}
	rows := make([]rawScore, 0, len(tickers))
	for _, t := range tickers {
		row := rawScore{ticker: t}
		candles, err := b.history.GetHistory(t, "1y")
		if err != nil || len(candles) < 60 {
			rows = append(rows, row) // unscoreable — neutral score, not dropped outright
			continue
		}
		closes := data.Closes(candles)
		if rs, ok := signals.RelativeStrength(closes, benchCloses, 63); ok {
			row.relStrength, row.hasRelStrength = rs, true
		}
		volumes := data.Volumes(candles)
		tail := 20
		if len(candles) < tail {
			tail = len(candles)
		}
		var dollarSum float64
		for i := len(candles) - tail; i < len(candles); i++ {
			dollarSum += closes[i] * float64(volumes[i])
		}
		row.avgDollarVol = dollarSum / float64(tail)
		lastClose := closes[len(closes)-1]
		if levels := signals.PriceLevels(candles); len(levels) > 0 && lastClose > 0 {
			best := math.MaxFloat64
			for _, lvl := range levels {
				if d := math.Abs(lastClose-lvl.Price) / lastClose; d < best {
					best = d
				}
			}
			row.distToLevel, row.hasDistToLevel = best, true
		}
		rows = append(rows, row)
	}

	// Each factor is min-max normalized to [0,1] across this batch — a
	// batch-relative rank is all "粗篩到 20" needs, and sidesteps picking an
	// absolute scale across three unrelated units (%, $, %).
	rs := make([]float64, len(rows))
	vol := make([]float64, len(rows))
	dist := make([]float64, len(rows))
	for i, r := range rows {
		rs[i], vol[i], dist[i] = r.relStrength, r.avgDollarVol, r.distToLevel
	}
	rsNorm, volNorm, distNorm := normalize01(rs), normalize01(vol), normalize01(dist)

	type ranked struct {
		ticker string
		score  float64
	}
	out := make([]ranked, len(rows))
	for i, r := range rows {
		score := volNorm[i]
		if r.hasRelStrength {
			score += rsNorm[i]
		} else {
			score += 0.5
		}
		if r.hasDistToLevel {
			score += 1 - distNorm[i] // closer to a level scores higher
		} else {
			score += 0.5
		}
		out[i] = ranked{r.ticker, score}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })
	if len(out) > n {
		out = out[:n]
	}
	kept := make([]string, len(out))
	for i, r := range out {
		kept[i] = r.ticker
	}
	return kept
}

// normalize01 min-max scales vals to [0,1]. An empty batch or a batch where
// every value is equal returns 0.5 for each — a neutral score that
// contributes nothing to the ranking rather than a divide-by-zero.
func normalize01(vals []float64) []float64 {
	out := make([]float64, len(vals))
	if len(vals) == 0 {
		return out
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		for i := range out {
			out[i] = 0.5
		}
		return out
	}
	for i, v := range vals {
		out[i] = (v - lo) / (hi - lo)
	}
	return out
}

// recommendationSources maps every ticker eligible for today's LLM call to
// where it came from ("watchlist"/"scan"/"explore"/"movers"), for Phase
// 3.8's /track breakdown by candidate-sourcing path. candidates is the
// already-deduped list returned by mergeCandidates, with Phase 2.6 解凍's
// exploreCandidates results already appended by RunDailyReport (nil/empty
// explore for handleRecommend, which doesn't run that step — see
// docs/phase-2.6-two-stage-llm-exploration.md). Priority is watchlist > scan
// > explore > movers: a ticker present in both scanHits and that list is
// attributed to "scan" rather than "movers" or "explore" — that's the most
// specific signal that actually surfaced it with a stated reason (see
// llm.StockData.ScanReason), even if it also happened to be trending or
// LLM-nominated; scan beats explore because scan hit is our own concrete
// technical signal, explore is just a one-line model nomination (in
// practice these shouldn't overlap at all — exploreCandidates' dedup step
// already excludes anything already a candidate — this ordering is a
// defensive guard, not an expected case).
func recommendationSources(watchlist, candidates []string, scanHits map[string]string, explore map[string]string) map[string]string {
	out := make(map[string]string, len(watchlist)+len(candidates))
	for _, t := range watchlist {
		out[t] = "watchlist"
	}
	for _, t := range candidates {
		// mergeCandidates already excludes watchlist tickers from candidates
		// in normal use, so this shouldn't fire in practice — kept as a
		// defensive guard so "watchlist" always wins over "movers"/"scan"/
		// "explore" for a ticker present in both, rather than depending on
		// which loop ran last.
		if out[t] == "watchlist" {
			continue
		}
		if _, ok := scanHits[t]; ok {
			out[t] = "scan"
		} else if _, ok := explore[t]; ok {
			out[t] = "explore"
		} else {
			out[t] = "movers"
		}
	}
	return out
}

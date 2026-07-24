package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/market"
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
// Candidate sourcing is asymmetric by market (Phase 6 PR2 §5.1): US gets
// market-movers ∪ today's US-market scan hits, exactly as before PR2 existed
// — GetMarketMovers/exploreCandidates are both US-only features (see their
// own doc comments) with no TW equivalent. TW gets *only* today's TW-market
// scan hits (RunUniverseScan(ctx, market.TW)'s output) — there is no TW
// movers/explore source in this phase. marketNews is similarly US-only for
// now: Finnhub's general-news endpoint has no TW equivalent until PR3's
// FinMind router lands, so a TW call gets an explicit nil here rather than
// attaching US-market news to a TW report (see
// docs/phase-6-tw-market.md §2's "之前走 nil-degrade").
func (b *Bot) gatherRecommendationInputs(m market.MarketID) (recommendationInputs, error) {
	tickers, err := b.db.GetWatchlistByMarket(m)
	if err != nil {
		return recommendationInputs{}, err
	}

	var candidateTickers []string
	if m == market.US {
		candidateTickers, err = b.provider.GetMarketMovers()
		if err != nil {
			log.Printf("market movers: %v", err)
		}
	}
	scanHits := b.loadScanHits(m)
	dedupedCandidates := mergeCandidates(candidateTickers, scanHits, tickers)
	allTickers := append(append([]string{}, tickers...), dedupedCandidates...)

	positions := b.loadPositions()
	earnings := b.loadEarnings(allTickers)
	var marketNews []data.NewsItem
	if m == market.US {
		marketNews = b.loadMarketNews()
	}
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
// recommendations against (Phase 6 PR2).
func (b *Bot) sendAndSaveRecommendations(newsSummary string, recs []llm.Recommendation, sources map[string]string, m market.MarketID, stockLists ...[]llm.StockData) {
	if newsSummary != "" {
		b.Send(i18n.T(b.lang, i18n.KeyMarketNewsSummaryTitle) + newsSummary)
	}

	prices := make(map[string]float64)
	atrs := make(map[string]float64)
	for _, list := range stockLists {
		for _, s := range list {
			if s.Quote == nil {
				continue
			}
			prices[s.Quote.Ticker] = s.Quote.Price
			if s.Technicals != nil && s.Technicals.ATR14 > 0 {
				atrs[s.Quote.Ticker] = s.Technicals.ATR14
			}
		}
	}

	sizing := b.buildSizingLines(recs, prices, atrs, m)

	watchlistRecs, candidateRecs := splitRecsBySource(recs, sources)

	b.Send(i18n.T(b.lang, i18n.KeyRecommendationsTitle))
	b.sendRecGroup(i18n.KeyRecWatchlistSectionTitle, watchlistRecs, sizing)
	b.sendRecGroup(i18n.KeyRecCandidatesSectionTitle, candidateRecs, sizing)

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
		log.Printf("save recommendations: %v", err)
	}
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
// would just read as a stray "1." on every single message. Sends nothing at
// all — title included — when recs is empty, so a day with no new-candidate
// picks doesn't leave a dangling header with nothing under it.
func (b *Bot) sendRecGroup(titleKey i18n.Key, recs []llm.Recommendation, sizing map[string]string) {
	if len(recs) == 0 {
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
// deterministic arithmetic (suggestShares), never left to the LLM. Returns
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
		shares := suggestShares(accountVal, b.riskPctPerTrade, price, stop)
		if shares <= 0 {
			continue
		}
		riskBudget := accountVal * b.riskPctPerTrade / 100
		lines[r.Ticker] = i18n.T(b.lang, i18n.KeySizingLine, riskBudget, stop, shares)
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
			log.Printf("scan-hit fundamentals: skipping %s (over cap of %d)", t, max)
			continue
		}
		out[t] = true
	}
	return out
}

// fetchStockData fetches quote+news for each ticker. Fundamentals and
// AnalystRating (Phase 3.7) are only attached when includeFundamentals is set
// (watchlist tickers, not the broad market-mover candidate list) to stay well
// under Finnhub's free-tier 60-requests/minute limit when a candidate list
// has a dozen-plus tickers — /stock/recommendation is a per-ticker call just
// like /stock/metric, so it shares the same gate rather than getting its own.
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
		q, err := b.provider.GetQuote(t)
		if err != nil {
			log.Printf("quote %s: %v", t, err)
			continue
		}
		news, _ := b.provider.GetNews(t, 5)
		stock := llm.StockData{Quote: q, News: news, CompanyName: b.companyName(t)}
		fetchFundamentals := includeFundamentals || extraFundamentals[t]
		if fetchFundamentals && b.fundamentals != nil {
			if fd, err := b.fundamentals.GetFundamentals(t); err != nil {
				log.Printf("fundamentals %s: %v", t, err)
			} else {
				stock.Fundamentals = fd
			}
		}
		if fetchFundamentals && b.analystRating != nil {
			if ar, err := b.analystRating.GetAnalystRating(t); err != nil {
				log.Printf("analyst rating %s: %v", t, err)
			} else {
				stock.AnalystRating = ar
			}
		}
		stock.Technicals, stock.Candles, stock.StrategyHits = b.computeTechnicals(t, loadBenchCloses(market.Of(t)))
		if p, ok := positions[t]; ok {
			stock.Position = &llm.Position{Shares: p.Shares, AvgCost: p.AvgCost}
		}
		if e, ok := earnings[t]; ok {
			stock.Earnings = &llm.Earnings{Date: e.Date, DaysUntil: daysUntil(e.Date)}
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
		log.Printf("history %s: %v", ticker, err)
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

	var stratHits []llm.StrategyHitInfo
	if hit := signals.SqueezeBreakout(candles); hit != nil {
		stratHits = append(stratHits, llm.StrategyHitInfo{Name: hit.Name, DaysAgo: hit.DaysAgo})
	}
	if hit := signals.BoxBottomRebound(candles); hit != nil {
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
type stopSuggestion struct {
	LatestClose float64
	Low10       float64
	Low20       float64
	ATRBased    float64
}

// computeStopSuggestion fetches ticker's OHLCV history and reduces it to
// stopSuggestion's three candidates, all computed against LatestClose — the
// most recent daily close in that same history, not a separate live quote,
// so /stop's "price must be below the latest close" validation compares
// against exactly the number the candidates were derived from. Falls back to
// a live quote for LatestClose only (leaving Low10/Low20/ATRBased at 0) when
// history can't be fetched at all — e.g. a brand-new watchlist addition
// Yahoo has no history for yet; ok is false only when that fallback also
// fails, meaning there's no usable reference price at all.
func (b *Bot) computeStopSuggestion(ticker string) (stopSuggestion, bool) {
	var s stopSuggestion

	candles, err := b.history.GetHistory(ticker, "1y")
	if err != nil || len(candles) == 0 {
		log.Printf("stop suggestion %s: history: %v", ticker, err)
		q, qerr := b.provider.GetQuote(ticker)
		if qerr != nil {
			log.Printf("stop suggestion %s: quote fallback: %v", ticker, qerr)
			return stopSuggestion{}, false
		}
		s.LatestClose = q.Price
		return s, true
	}

	closes := data.Closes(candles)
	highs := data.Highs(candles)
	lows := data.Lows(candles)
	s.LatestClose = closes[len(closes)-1]
	s.Low10 = signals.LowestClose(closes, 10)
	s.Low20 = signals.LowestClose(closes, 20)
	if atr := signals.ATR(highs, lows, closes, 14); atr > 0 {
		s.ATRBased = s.LatestClose - stopCandidateATRMult*atr
	}
	return s, true
}

// suggestShares is Phase 3.11's R-based position-sizing formula (§3.4):
// floor(riskBudget ÷ perShareRisk), where riskBudget = accountValue ×
// riskPct/100 and perShareRisk = price − stop. Bot-side deterministic
// arithmetic on purpose — the LLM is asked to name a stop level in its BUY
// reasoning (see KeyTechGuidanceBlock) but never trusted to do this
// division itself. Returns 0 (meaning: don't show a sizing line) for any
// non-positive/invalid input, including stop >= price, which would make
// perShareRisk zero or negative.
func suggestShares(accountValue, riskPct, price, stop float64) int {
	if accountValue <= 0 || riskPct <= 0 || price <= 0 || stop <= 0 || stop >= price {
		return 0
	}
	riskBudget := accountValue * riskPct / 100
	perShareRisk := price - stop
	shares := int(riskBudget / perShareRisk)
	if shares < 0 {
		return 0
	}
	return shares
}

// accountValue is Phase 3.11's account-size input for suggestShares: m's
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
		log.Printf("account value: net worth: %v", err)
	}
	if !ok {
		return 0, false
	}
	if cash, cashOK, err := b.loadCash(m); err != nil {
		log.Printf("account value: cash: %v", err)
	} else if cashOK {
		total += cash
	}
	return total, true
}

// computeMarketRegime builds Phase 3.7 追加項's broad-market context block
// (see docs/phase-3.7-market-regime.md and llm.MarketContext): benchmarkFor
// (m)'s own trend (SPY for US, 0050 for TW as of Phase 6 PR2 — last close
// from a single GetHistory call, no separate GetQuote) and, US only,
// ^VIX's latest level (via the ordinary Multi quote chain — Finnhub returns
// an error-shaped-but-200 body for CFD indices it doesn't support, which
// decodes as an all-zero quote and falls through to Yahoo exactly like any
// other "no data" quote, confirmed by live testing, see the design doc). VIX
// is skipped entirely for TW — there's no equivalent volatility-index ticker
// wired up for the TWSE/TPEx market, and the design doc doesn't call for one
// (see docs/phase-6-tw-market.md §5.1). Either half failing just logs and
// leaves that half's fields at 0 (skipped by writeMarketContext's per-field
// rendering); both failing returns nil so the caller sees "no regime data"
// rather than an all-zero struct.
func (b *Bot) computeMarketRegime(m market.MarketID) *llm.MarketContext {
	var mc llm.MarketContext

	bench := benchmarkFor(m)
	candles, err := b.history.GetHistory(bench, "1y")
	if err != nil {
		log.Printf("market regime: %s history: %v", bench, err)
	} else if len(candles) > 0 {
		closes := data.Closes(candles)
		mc.SPYPrice = closes[len(closes)-1]
		mc.SPYMA50 = signals.MA(closes, 50)
		mc.SPYMA200 = signals.MA(closes, 200)
	}

	if m == market.US {
		if q, err := b.provider.GetQuote(vixTicker); err != nil {
			log.Printf("market regime: %s quote: %v", vixTicker, err)
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
		log.Printf("load positions: %v", err)
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
		log.Printf("load prev recommendations: %v", err)
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
		log.Printf("load past lessons: %v", err)
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
		log.Printf("load recent lessons: %v", err)
		return nil
	}
	out := make([]llm.PastLesson, len(rows))
	for i, l := range rows {
		out[i] = llm.PastLesson{Ticker: l.Ticker, Date: l.Date, Lesson: l.Lesson}
	}
	return out
}

// loadEarnings returns each ticker's next scheduled earnings date within
// earningsPromptWindowDays, keyed by ticker. Degrades to nil (not an error)
// when Finnhub isn't configured or the request fails — same optional-data
// pattern as fundamentals.
func (b *Bot) loadEarnings(tickers []string) map[string]data.EarningsEvent {
	if b.earnings == nil || len(tickers) == 0 {
		return nil
	}
	events, err := b.earnings.GetUpcomingEarnings(tickers, earningsPromptWindowDays)
	if err != nil {
		log.Printf("earnings calendar: %v", err)
		return nil
	}
	return events
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
			log.Printf("load thesis %s: %v", t, err)
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
func computeVsSPY(currentPrice, avgCost, spyPrice, spyEntryClose float64) llm.VsSPYReturn {
	return llm.VsSPYReturn{
		TickerPct: (currentPrice - avgCost) / avgCost * 100,
		SPYPct:    (spyPrice - spyEntryClose) / spyEntryClose * 100,
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
			log.Printf("vs-spy: benchmark %s quote: %v", ticker, err)
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
			log.Printf("vs-spy: earliest buy %s: %v", ticker, err)
			continue
		}
		if !ok {
			continue
		}
		benchEntryClose, ok, err := b.db.GetSnapshotClose(benchmarkFor(m), buyDate)
		if err != nil {
			log.Printf("vs-spy: benchmark snapshot %s: %v", ticker, err)
			continue
		}
		if !ok || benchEntryClose == 0 {
			continue
		}
		out[ticker] = computeVsSPY(s.Quote.Price, p.AvgCost, benchQuote.Price, benchEntryClose)
	}
	return out
}

// loadMarketNews returns up to marketNewsLimit general market/macro news
// items for the recommendation prompt's market-news summary section.
// Degrades to nil (not an error) when Finnhub isn't configured or the
// request fails — same optional-data pattern as fundamentals/earnings; a nil
// result means GenerateRecommendations simply omits the summary.
func (b *Bot) loadMarketNews() []data.NewsItem {
	if b.marketNews == nil {
		return nil
	}
	items, err := b.marketNews.GetMarketNews(marketNewsLimit)
	if err != nil {
		log.Printf("market news: %v", err)
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
		log.Printf("scan hits: %v", err)
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
// SPY move (trackHit), and render each into a display line. Shared by
// handleTrack (its own full display) and RunWeeklyReview's strategy-feedback
// block (which only needs rows, to summarize via summarizeTrack — lines
// go unused there, same "compute once, let callers use what they need"
// shape as fetchStockData). ok=false means there were no recommendations in
// the window at all (not an error) — callers render that as "nothing to
// report" rather than an empty summary.
func (b *Bot) computeTrackRows(days int) (rows []trackRow, lines []string, ok bool, err error) {
	fromDate := time.Now().In(cst).AddDate(0, 0, -days).Format("2006-01-02")
	recs, err := b.db.GetRecommendationsSince(fromDate)
	if err != nil {
		return nil, nil, false, err
	}
	if len(recs) == 0 {
		return nil, nil, false, nil
	}

	// One quote per distinct ticker, however often it was recommended.
	quotes := make(map[string]*data.Quote)
	// benchQuotes lazily fetches and caches each market's current benchmark
	// quote (SPY for US, 0050 for TW) — Phase 6 PR2: recs here can span both
	// markets, so a single shared SPY quote is no longer correct (see
	// trackHit's own per-row benchmark selection below).
	benchQuotes := make(map[market.MarketID]*data.Quote)
	loadBenchQuote := func(m market.MarketID) *data.Quote {
		if q, ok := benchQuotes[m]; ok {
			return q
		}
		ticker := benchmarkFor(m)
		q, err := b.provider.GetQuote(ticker)
		if err != nil {
			log.Printf("track: benchmark %s quote: %v", ticker, err)
			q = nil
		}
		benchQuotes[m] = q
		return q
	}

	for _, r := range recs {
		action := r.Action
		if action == "" {
			action = "—"
		}

		base := r.Price
		if base == 0 {
			if c, ok, err := b.db.GetSnapshotClose(r.Ticker, r.Date); err == nil && ok {
				base = c
			}
		}
		if base == 0 {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineNoPrice, r.Date, r.Ticker, action))
			continue
		}

		q, seen := quotes[r.Ticker]
		if !seen {
			var err error
			q, err = b.provider.GetQuote(r.Ticker)
			if err != nil {
				log.Printf("track: quote %s: %v", r.Ticker, err)
				q = nil
			}
			quotes[r.Ticker] = q
		}
		if q == nil {
			lines = append(lines, i18n.T(b.lang, i18n.KeyQuoteUnavailable, r.Ticker))
			continue
		}

		changePct := (q.Price - base) / base * 100

		recMarket := market.Of(r.Ticker)
		benchTicker := benchmarkFor(recMarket)
		benchQuote := loadBenchQuote(recMarket)

		var benchChangePct float64
		haveBench := false
		if benchQuote != nil {
			if benchBase, ok, err := b.db.GetSnapshotClose(benchTicker, r.Date); err == nil && ok && benchBase != 0 {
				benchChangePct = (benchQuote.Price - benchBase) / benchBase * 100
				haveBench = true
			}
		}

		verdict := ""
		if r.Action == "BUY" || r.Action == "SELL" {
			hit := trackHit(r.Action, changePct, benchChangePct, haveBench)
			verdict = "❌"
			if hit {
				verdict = "✅"
			}
			rows = append(rows, trackRow{
				Action:    r.Action,
				Source:    displaySource(r.Source),
				Market:    recMarket,
				ChangePct: changePct,
				Hit:       hit,
			})
		}

		if haveBench {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLineVsSPY, r.Date, r.Ticker, action, base, q.Price, changePct, benchTicker, benchChangePct, verdict))
		} else {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrackLine, r.Date, r.Ticker, action, base, q.Price, changePct, verdict))
		}
	}

	return rows, lines, true, nil
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
	}
	var entries []entry
	for t, e := range earnings {
		d := daysUntil(e.Date)
		if d < 0 || d > days {
			continue
		}
		entries = append(entries, entry{ticker: t, date: e.Date, daysUntil: d})
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].daysUntil < entries[j].daysUntil })

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWeeklyEarningsPreviewTitle))
	for _, e := range entries {
		sb.WriteString(i18n.T(lang, i18n.KeyWeeklyEarningsPreviewLine, e.ticker, e.date, e.daysUntil))
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

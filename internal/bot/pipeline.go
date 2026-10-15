package bot

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
	dedupedCandidates := service.MergeCandidates(candidateTickers, scanHits, tickers)
	dedupedCandidates = service.RankAndTruncateCandidates(b.history, dedupedCandidates, service.BenchmarkFor(m), candidatePrefilterCount)
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
	gapCount := service.CountCandleGaps(in.watchlist, in.candidates)

	if err := b.db.InsertLLMRun(kind, m, model, latencyMs, string(inputJSON), raw, len(in.watchlist), len(in.candidates), newsCount, gapCount); err != nil {
		logger.Errorf("llm run: insert: %v", err)
	}
}

// recordPriceEventLLMRun files one ExplainPriceEvent call into Phase 19's
// llm_runs audit trail under kind "price_event" — the price-event path's
// counterpart of recordLLMRun above, which can't be reused directly because
// its payload is a recommendationInputs (watchlist/candidates/market
// context), none of which exists here. Same "log, never block" contract as
// recordLLMRun, and called before the caller's LLM error check so a failed
// call is recorded too — the case where seeing what the model actually
// replied matters most. watchlist_count/candidate_count/candle_gap_count go
// in as 0: they have no meaning for a single-ticker event summary, and
// minting columns for a second kind of run would defeat the point of one
// shared table.
func (b *Bot) recordPriceEventLLMRun(ev signals.PriceEvent, m market.MarketID, news []data.NewsItem, summary, model string, latencyMs int64) {
	if summary == "" && model == "" {
		return
	}

	payload := struct {
		Ticker        string          `json:"ticker"`
		GapPct        float64         `json:"gapPct"`
		ChangePct     float64         `json:"changePct"`
		CumulativePct float64         `json:"cumulativePct"`
		News          []data.NewsItem `json:"news"`
	}{ev.Ticker, ev.GapPct, ev.ChangePct, ev.CumulativePct, news}
	inputJSON, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("llm run: marshal price event input: %v", err)
		return
	}

	if err := b.db.InsertLLMRun("price_event", m, model, latencyMs, string(inputJSON), summary, 0, 0, len(news), 0); err != nil {
		logger.Errorf("llm run: insert price event: %v", err)
	}
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
// run. This is a safety cap, not a tunable policy knob (like scanChunkCount),
// so it's a plain const rather than an env var — see
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

// candidatePrefilterCount is Phase 23 PR9's rule-score cutoff
// (docs/phase-23-strategy-data-uplift.md §5: "規則分粗篩到 20") — the daily
// candidate pool (movers ∪ scan hits, 40-55/day before this) is too big for
// a "credible briefing"; the LLM's own KeyRecTaskBlock cap (最多 5 檔) does
// the final pick, so this prefilter only needs to not drop a real candidate,
// not be precise (§4.2/§4.3).
const candidatePrefilterCount = 20

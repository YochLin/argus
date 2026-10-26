package bot

import (
	"context"
	"errors"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
	"argus/internal/signals"
)

// twMarketClosedStaleness is how old a 0050 quote's timestamp must be before
// runDailyReport treats the TW market as closed (Phase 6 PR2 §3.3) — same
// 12h threshold RunClosingSnapshot already uses to catch a stale/holiday
// quote, just checked at report-open time instead of after the fact. There
// is deliberately no TW holiday-calendar package to check against instead
// (unlike US's market.IsTradingDay) — Lunar New Year and ad-hoc typhoon
// closures aren't calculable from a fixed annual rule set, so a live
// quote-freshness check is used instead; it costs one extra quote fetch on
// every run (even ordinary trading days) but correctly catches every kind of
// closure, including ones a fixed calendar never could.
const twMarketClosedStaleness = service.TWMarketClosedStaleness

// RunDailyReport is the US-market daily report's scheduler entry point
// (23:30 CST daily, unchanged since before Phase 6) — a thin wrapper around
// runDailyReport(ctx, market.US) so the scheduler/`/dailyreport` command
// call sites don't need to know about the market parameter.
func (b *Bot) RunDailyReport(ctx context.Context) {
	b.runDailyReport(ctx, market.US)
}

// RunTWDailyReport is Phase 6 PR2's TW-market daily report entry point
// (11:30 CST Mon-Fri, moved from an original 15:00 post-close slot — see
// docs/tw-daily-report-schedule.md — to mirror RunDailyReport's intraday
// analysis timing instead of a post-close recap) — the TW counterpart of
// RunDailyReport above.
func (b *Bot) RunTWDailyReport(ctx context.Context) {
	b.runDailyReport(ctx, market.TW)
}

// isTWMarketClosed fetches a 0050 quote and reports whether its timestamp is
// stale enough to mean "TW market is closed today" — see
// twMarketClosedStaleness. A quote-fetch failure is treated the same as
// "closed" (fail safe: skip the report rather than risk running a full LLM
// analysis with no way to tell whether today's data is fresh), logged for
// visibility. It compares the quote timestamp against now, not a fixed
// anchor time, so it stays correct regardless of what hour RunTWDailyReport
// fires at (11:30, moved from the original 15:00 — see
// docs/tw-daily-report-schedule.md).
func (b *Bot) isTWMarketClosed() bool {
	q, err := b.provider.GetQuote(service.BenchmarkFor(market.TW))
	if err != nil {
		logger.Errorf("tw market closed check: quote: %v", err)
		return true
	}
	return time.Since(q.Timestamp) > twMarketClosedStaleness
}

// runDailyReport fetches data, detects signals, generates LLM
// recommendations, and sends the daily report for market m. Called by the
// scheduler via RunDailyReport (US, 23:30 CST daily) and RunTWDailyReport
// (TW, 11:30 CST Mon-Fri) — see docs/phase-6-tw-market.md §5.1 for the
// TW-specific behavior called out below.
//
// The US cron fires every day with no weekday/holiday restriction (unlike
// RunClosingSnapshot, which is Tue–Sat only) — on a US market holiday it
// would otherwise still run a full LLM analysis off whatever stale
// prior-session quotes the providers return and push a report implying
// that's today's price action. market.IsTradingDay checks that before
// anything else gets fetched; time.Now().In(cst) is safe to feed it
// directly rather than resolving a real US Eastern time first because this
// job only ever runs at the fixed 23:30 CST cron time — see IsTradingDay's
// own doc comment for why that specific hour makes Taiwan's date and the US
// trading date the same value. The TW cron (Mon-Fri only) has no such
// calendar to check against, so it uses isTWMarketClosed's live 0050
// quote-freshness heuristic instead (§3.3) — both branches send their own
// market-scoped "closed" message and return before fetching anything else.
func (b *Bot) runDailyReport(ctx context.Context, m market.MarketID) {
	defer b.recoverJobPanic("daily report")

	if m == market.US {
		if !market.IsTradingDay(b.now().In(cst)) {
			b.Send(i18n.T(b.lang, i18n.KeyDailyReportMarketClosed))
			return
		}
		// After the report, whichever way it ends (an LLM failure returns
		// early); see runMomentumWatch.
		defer b.runMomentumWatch(ctx)
	} else if b.isTWMarketClosed() {
		b.Send(i18n.T(b.lang, i18n.KeyTWDailyReportMarketClosed))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyDailyReportStart))

	in, err := b.gatherRecommendationInputs(m)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	b.checkEarningsAlerts(in.watchlistTickers, in.earnings)

	// Detect signals on watchlist
	var allSignals []signals.Signal
	prices := make(map[string]float64, len(in.watchlist))
	atrs := make(map[string]float64, len(in.watchlist))
	ma5s := make(map[string]float64, len(in.watchlist))
	for _, s := range in.watchlist {
		if s.Quote != nil {
			prices[s.Quote.Ticker] = s.Quote.Price
			allSignals = append(allSignals, b.detector.CheckQuote(s.Quote)...)
		}
		if s.Quote != nil && s.Technicals != nil && s.Technicals.ATR14 > 0 {
			atrs[s.Quote.Ticker] = s.Technicals.ATR14
		}
		if s.Quote != nil && s.Technicals != nil && s.Technicals.MA5 > 0 {
			ma5s[s.Quote.Ticker] = s.Technicals.MA5
		}
	}
	isBear := service.IsBearRegime(in.marketContext)
	for _, t := range in.watchlistTickers {
		candles, err := b.history.GetHistory(t, "1y")
		if err != nil {
			logger.Errorf("history %s: %v", t, err)
			continue
		}
		allSignals = append(allSignals, b.checkStatefulSignals(t, candles, isBear)...)
	}
	if len(allSignals) > 0 {
		b.SendSignalAlert(allSignals)
	}

	// Exit-discipline checks (Phase 3.8): rule-based, independent of the LLM
	// call below, so a down LLM provider doesn't suppress them. Daily-report
	// only, by design — no intraday/at-price monitoring (see PLAN.md).
	// in.positions comes from loadPositions(), which is all-market (see
	// gatherRecommendationInputs) — it must be filtered to m here, or a
	// position in the *other* market would still get checked (priceFor falls
	// back to a live quote fetch for any ticker missing from prices, so it
	// wouldn't even fail quietly) and this exit-discipline sweep would fire
	// twice a day for every position, once from each market's report.
	positionList := positionsSlice(filterPositionsByMarket(in.positions, m))
	b.checkStopLossAlerts(positionList, prices)
	b.checkTrailingStopAlerts(positionList, prices, atrs)
	b.checkTargetAlerts(positionList, prices)
	b.checkMA5BreakAlerts(positionList, prices, ma5s)
	if m == market.TW {
		b.checkRestrictedAlerts(positionList, b.scans().RestrictedTickers(ctx, m))
	}

	// Buy alerts (unlike the exit-discipline checks above) watch tickers the
	// user may not hold yet, so they're checked here too, not folded into
	// positionList — see checkBuyAlerts' own doc comment for why this runs
	// at both this checkpoint and RunClosingSnapshot's.
	if buyAlerts, err := b.db.GetBuyAlertsByMarket(m); err != nil {
		logger.Errorf("buy alerts: %v", err)
	} else {
		b.checkBuyAlerts(buyAlerts, prices)
	}

	// Two-stage LLM exploration (Phase 2.6 解凍) is US-only: it validates
	// nominations via data.IsUSEquitySymbol, which would reject every TW
	// ticker shape outright. marketNews is no longer TW-empty on its own
	// (cnyes now feeds b.twMarketNews — see loadMarketNews) so this guard is
	// the only thing keeping exploreCandidates from running against TW news.
	var explore map[string]string
	if m == market.US {
		explore = b.exploreCandidates(ctx, &in)
	}

	summary, recs, raw, model, latencyMs, err := b.llm.GenerateRecommendations(ctx, in.watchlist, in.candidates, in.marketNews, in.marketContext, in.recentLessons, m == market.TW)
	b.recordLLMRun("daily_report", m, in, raw, model, latencyMs)
	if err != nil {
		if errors.Is(err, llm.ErrRecommendationParseFailed) {
			b.Send(i18n.T(b.lang, i18n.KeyRecParseFailed, err))
		} else {
			b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		}
		return
	}

	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyNoRecommendationsToday))
		return
	}

	sources := service.RecommendationSources(in.watchlistTickers, in.candidateTickers, in.scanHits, explore)
	b.sendAndSaveRecommendations(summary, recs, sources, m, len(in.candidateTickers), in.watchlist, in.candidates)
}

// exploreCandidates is Phase 2.6 解凍's two-stage LLM exploration (see
// docs/phase-2.6-two-stage-llm-exploration.md), called only from
// RunDailyReport — /recommend doesn't get this extra one-shot LLM call, per
// the design doc's interactive-latency trade-off. Skips entirely (nil, no
// LLM call) when in.marketNews is empty: a nomination with no news basis is
// pure model prior, the highest hallucination risk for the least
// information. The model's raw nominations are validated in three steps
// before being trusted, each failure logged and dropped rather than
// aborting the whole batch: symbol shape (data.IsUSEquitySymbol), dedup
// against every existing list (watchlist ∪ candidates ∪ positions), and a
// real GetQuote (Yahoo/Finnhub both return an all-zero-but-200 response for
// an invalid/delisted ticker, which the existing quote parsing already
// treats as "no data" — a hallucinated or delisted ticker fails here).
// Valid nominations are appended directly into in's candidate fields so
// GenerateRecommendations/sendAndSaveRecommendations need zero changes to
// pick them up, and returned as a ticker->reason map for
// service.RecommendationSources to label "explore".
func (b *Bot) exploreCandidates(ctx context.Context, in *recommendationInputs) map[string]string {
	if len(in.marketNews) == 0 {
		return nil
	}

	excludeSet := make(map[string]bool, len(in.watchlistTickers)+len(in.candidateTickers)+len(in.positions))
	var exclude []string
	addExclude := func(t string) {
		if !excludeSet[t] {
			excludeSet[t] = true
			exclude = append(exclude, t)
		}
	}
	for _, t := range in.watchlistTickers {
		addExclude(t)
	}
	for _, t := range in.candidateTickers {
		addExclude(t)
	}
	for t := range in.positions {
		addExclude(t)
	}

	noms, err := b.llm.ExploreCandidates(ctx, in.marketNews, exclude)
	if err != nil {
		logger.Errorf("explore candidates: %v", err)
		return nil
	}

	var valid []string
	reasons := make(map[string]string, len(noms))
	for _, n := range noms {
		if !data.IsUSEquitySymbol(n.Ticker) {
			logger.Infof("explore candidates: rejecting %q: not a plain US-equity symbol shape", n.Ticker)
			continue
		}
		if excludeSet[n.Ticker] {
			logger.Infof("explore candidates: rejecting %s: already on an existing list", n.Ticker)
			continue
		}
		if _, err := b.provider.GetQuote(n.Ticker); err != nil {
			logger.Errorf("explore candidates: rejecting %s: quote failed: %v", n.Ticker, err)
			continue
		}
		valid = append(valid, n.Ticker)
		reasons[n.Ticker] = i18n.T(b.lang, i18n.KeyExploreReasonLabel, n.Reason)
		excludeSet[n.Ticker] = true // guards against the model repeating a ticker across its own nominations
	}
	if len(valid) == 0 {
		return nil
	}

	earnings := b.loadEarnings(valid)
	prevRecs := b.loadPrevRecs(valid)
	stocks := b.fetchStockData(valid, false, in.positions, earnings, reasons, prevRecs, nil)

	in.candidateTickers = append(in.candidateTickers, valid...)
	in.candidates = append(in.candidates, stocks...)

	return reasons
}

package bot

import (
	"context"
	"errors"
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
	"argus/internal/notification"
	"argus/internal/service"
	"argus/internal/signals"
)

// recoverJobPanic recovers from a panic inside a scheduler-invoked job
// (RunDailyReport/RunClosingSnapshot), logging it and alerting the user over
// Telegram. Without this, a panic in either job would kill that goroutine
// silently — the bot keeps running and answering commands, but the VPS is
// unattended, so a failed daily report or closing snapshot would otherwise
// go completely unnoticed. job names the job for the log line and alert.
func (b *Bot) recoverJobPanic(job string) {
	if r := recover(); r != nil {
		logger.Errorf("%s: panic: %v", job, r)
		b.publishAlert("job_panic", notification.LevelCritical, i18n.T(b.lang, i18n.KeyJobPanic, job, r))
	}
}

// SendSignalAlert sends signal notifications to the chat.
func (b *Bot) SendSignalAlert(sigs []signals.Signal) {
	if len(sigs) == 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeySignalAlertTitle))
	for _, s := range sigs {
		sb.WriteString("• " + s.Message + "\n")
	}
	b.publishAlert("signal_alert", notification.LevelInfo, sb.String())
}

// benchmarkFor returns the daily-snapshot benchmark ticker for m: SPY for
// US, 0050 for TW (Phase 6, same "snapshot alongside the watchlist" role
// benchmarkTicker/snapshotBenchmark already played for US-only — see
// docs/phase-6-tw-market.md §3.3). 0050 is allowed to simultaneously be a
// TW watchlist/position ticker; SaveSnapshot's INSERT OR REPLACE makes a
// same-(ticker,date) double-write from both paths safe.
func benchmarkFor(m market.MarketID) string {
	return service.BenchmarkFor(m)
}

// RunClosingSnapshot records the just-closed session's OHLCV for every
// watchlist ticker in market m into daily_snapshots. Called by the
// scheduler once per market: 05:30 CST for US (after the US close) and
// 14:30 CST Mon-Fri for TW (after the TW close) — see
// docs/phase-6-tw-market.md §3.3. Date semantics differ by market: at
// 05:30 CST the US trading date is still Taiwan's "yesterday" (hence the
// -1 day), but the TW closing snapshot runs the same afternoon as the TW
// session it's recording, so it's dated today.
// It's a silent background job: results go to the DB and errors to the log,
// not to Telegram.
func (b *Bot) RunClosingSnapshot(ctx context.Context, m market.MarketID) {
	defer b.recoverJobPanic("closing snapshot")

	tickers, err := b.db.GetWatchlistByMarket(m)
	if err != nil {
		logger.Errorf("closing snapshot: watchlist: %v", err)
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	now := time.Now().In(cst)
	date := now.Format("2006-01-02")
	if m == market.US {
		date = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	closing := b.snapshots().FetchClosingQuotes(tickers, m, date)
	prices := closing.Prices
	logger.Infof("closing snapshot: done for %s market=%s (%d tickers)", date, m, len(tickers))

	b.recordPriceEvents(ctx, closing.PriceEvents, m, date)

	b.snapshotBenchmark(date, m)
	b.recordNetWorthSnapshot(date, m, prices)
	b.runPaperClose(m, date, prices)

	// Buy alerts get checked here too, not just from runDailyReport — see
	// checkBuyAlerts' doc comment for why a ticker the user doesn't hold
	// yet warrants both of a market's daily checkpoints rather than riding
	// along with the position-only exit-discipline sweep.
	if buyAlerts, err := b.db.GetBuyAlertsByMarket(m); err != nil {
		logger.Errorf("closing snapshot: buy alerts: %v", err)
	} else {
		b.checkBuyAlerts(buyAlerts, prices)
	}

	// Phase 12: US-only (options.go's OptionChainProvider is US-only, see
	// its own doc comment) — both the expiry-scan confirm-flow and the
	// daily ATM IV snapshot ride the US closing snapshot rather than
	// getting a cron entry of their own.
	if m == market.US {
		b.checkOptionExpiry(prices, date)
		b.recordDailyATMIV(tickers, prices, date)
	}
}

// priceEventWriteupCap caps how many of a single RunClosingSnapshot run's
// triggered price events get an LLM writeup + Telegram push, chosen by move
// size descending (docs/phase-20-price-event-log.md §2 "單次觸發上限"). A
// starting-point constant per that doc's §6 risk note, not calibrated
// against real trigger frequency yet — revisit if it's routinely maxed out.
const priceEventWriteupCap = 3

// recordPriceEvents is RunClosingSnapshot's Phase 20 tail step (see
// docs/phase-20-price-event-log.md §4.4): dedup hits against price_events'
// (ticker, date) uniqueness (HasPriceEvent — guards a rerun from
// re-triggering the same day), rank the rest by move size, LLM-writeup and
// push the top priceEventWriteupCap, and store everything past that cap with
// no summary plus one overflow notice. Called after every watchlist
// ticker's quote is already saved to daily_snapshots, so an LLM call's
// latency or failure never delays that write — same reasoning
// RunClosingSnapshot's own doc comment gives for keeping this out of the
// per-ticker fetch loop. A single ticker's LLM/save failure only logs and
// moves on, same "one bad ticker doesn't block the rest" convention as the
// snapshot loop above it.
func (b *Bot) recordPriceEvents(ctx context.Context, hits []signals.PriceEvent, m market.MarketID, date string) {
	if len(hits) == 0 {
		return
	}

	var pending []signals.PriceEvent
	for _, ev := range hits {
		has, err := b.db.HasPriceEvent(ev.Ticker, date)
		if err != nil {
			logger.Errorf("price events: dedup check %s: %v", ev.Ticker, err)
			continue
		}
		if has {
			continue
		}
		pending = append(pending, ev)
	}
	if len(pending) == 0 {
		return
	}
	service.SortPriceEventsByMoveSize(pending)

	writeup, overflow := pending, []signals.PriceEvent(nil)
	if len(pending) > priceEventWriteupCap {
		writeup, overflow = pending[:priceEventWriteupCap], pending[priceEventWriteupCap:]
	}

	for _, ev := range writeup {
		news, _ := b.provider.GetNews(ev.Ticker, tickerNewsFetch)
		news = (&newsPicker{}).pick(filterNewsNearDate(news, date), tickerNewsSlots)
		facts := priceEventFacts(ev)
		facts.ATRMultiple, facts.VolumeRatio = b.priceEventScale(ev.Ticker, ev.ChangePct)
		summary, model, latencyMs, err := b.llm.ExplainPriceEvent(ctx, facts, news)
		b.recordPriceEventLLMRun(ev, m, news, summary, model, latencyMs)
		if err != nil {
			logger.Errorf("price events: LLM %s: %v", ev.Ticker, err)
			continue
		}
		if err := b.db.SavePriceEvent(db.PriceEvent{Ticker: ev.Ticker, Market: string(m), Date: date, GapPct: ev.GapPct, ChangePct: ev.ChangePct, CumulativePct: ev.CumulativePct, Summary: summary}); err != nil {
			logger.Errorf("price events: save %s: %v", ev.Ticker, err)
			continue
		}
		b.publishAlert("price_event", notification.LevelInfo, i18n.T(b.lang, i18n.KeyPriceEventResultTitle, ev.Ticker, summary))
	}

	if len(overflow) == 0 {
		return
	}
	var sb strings.Builder
	for _, ev := range overflow {
		if err := b.db.SavePriceEvent(db.PriceEvent{Ticker: ev.Ticker, Market: string(m), Date: date, GapPct: ev.GapPct, ChangePct: ev.ChangePct, CumulativePct: ev.CumulativePct}); err != nil {
			logger.Errorf("price events: save %s: %v", ev.Ticker, err)
			continue
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyPriceEventOverflowTickerLine, ev.Ticker, ev.GapPct, ev.ChangePct, ev.CumulativePct))
	}
	b.publishAlert("price_event", notification.LevelInfo, i18n.T(b.lang, i18n.KeyPriceEventOverflowLine, sb.String()))
}

// priceEventScale gives the event summary a sense of how unusual the move
// actually is: the day's change as a multiple of ATR(14), and volume against
// its 20-day average (Phase 20 後續 PR5). 7% is a routine day for NVDA and a
// once-a-year one for KO, and a percentage alone can't say which this was.
//
// The day's change is the numerator even when the cumulative-decline
// threshold is what fired — ATR measures a single session's range, so
// comparing a five-day slide against it would be a category error. Costs one
// GetHistory per writeup ticker, at most priceEventWriteupCap a day, and
// reuses computeTechnicals rather than recomputing ATR here (spyCloses is
// nil: relative strength isn't part of this). Returns 0, 0 on any missing
// piece — the prompt then simply omits the line rather than printing a
// multiple derived from a partial series.
func (b *Bot) priceEventScale(ticker string, changePct float64) (atrMultiple, volumeRatio float64) {
	t, candles, _ := b.computeTechnicals(ticker, nil)
	if t == nil || len(candles) == 0 {
		return 0, 0
	}
	if close := candles[len(candles)-1].Close; t.ATR14 > 0 && close > 0 {
		atrMultiple = math.Abs(changePct) / (t.ATR14 / close * 100)
	}
	return atrMultiple, t.VolumeRatio
}

// snapshotBenchmark records benchmarkFor(m)'s (SPY/0050) closing price into
// daily_snapshots under the same date as the watchlist snapshot, so /track's
// relative-to-market hit rate (Phase 3.8) has same-day benchmark data to
// compare against without ever needing to replay history through a live API
// call. Thin wrapper around snapshotBenchmarkTo targeting b.db — see that
// function for the actual logic.
func (b *Bot) snapshotBenchmark(date string, m market.MarketID) {
	b.snapshotBenchmarkTo(b.db, date, m)
}

// snapshotBenchmarkTo is snapshotBenchmark's logic parameterized over which
// *db.DB to write into — Phase 11 PR3's runPaperClose calls this with
// b.paperDB so the live paper account's benchmark-overlay data (BenchmarkReplay,
// PR4) comes from the exact same fetch/stale-quote-guard code path as the
// real dashboard's, instead of a second implementation that could drift.
// Thin wrapper around service.SnapshotService.SnapshotBenchmarkTo (Phase 24
// Stage 1 Report & Snapshot Service extraction).
func (b *Bot) snapshotBenchmarkTo(target *db.DB, date string, m market.MarketID) {
	b.snapshots().SnapshotBenchmarkTo(target, benchmarkFor(m), date)
}

// recordNetWorthSnapshot is a thin wrapper around
// service.SnapshotService.RecordNetWorthSnapshot (Phase 24 Stage 1 Report &
// Snapshot Service extraction).
func (b *Bot) recordNetWorthSnapshot(date string, m market.MarketID, prices map[string]float64) {
	b.snapshots().RecordNetWorthSnapshot(m, date, prices)
}

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
	q, err := b.provider.GetQuote(benchmarkFor(market.TW))
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
	isBear := isBearRegime(in.marketContext)
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

// checkStatefulSignals is a thin wrapper around service.ScanService's
// CheckStatefulSignals (Phase 24 Stage 1 Scan & Strategy Service extraction)
// that adds the strategy-hit caveats the service owns the text of — see
// service.DecorateStrategyHits for what they are and why they are applied
// there rather than at render time.
func (b *Bot) checkStatefulSignals(ticker string, candles []data.Candle, isBearRegime bool) []signals.Signal {
	return service.DecorateStrategyHits(b.scans().CheckStatefulSignals(ticker, candles), isBearRegime, b.lang)
}

// checkEarningsAlerts sends one batched Telegram message warning about
// watchlist tickers (positions are always on the watchlist via /buy's
// auto-add, so this covers held positions too) with earnings due within
// earningsAlertDays. Deduped via signal_states (family "earnings", state =
// the earnings date string) so it fires once per reporting date rather than
// every day the ticker sits inside the alert window.
func (b *Bot) checkEarningsAlerts(tickers []string, earnings map[string]data.EarningsEvent) {
	var lines []string
	for _, t := range tickers {
		e, ok := earnings[t]
		if !ok {
			continue
		}
		alertDays := earningsAlertDays
		if market.Of(t) == market.TW {
			alertDays = earningsAlertDaysTW
		}
		days := daysUntil(e.Date)
		if days < 0 || days > alertDays {
			continue
		}

		prev, err := b.db.GetSignalState(t, earningsSignalFamily)
		if err != nil {
			logger.Errorf("earnings alert state %s: %v", t, err)
		}
		if prev == e.Date {
			continue
		}

		key := i18n.KeyEarningsAlertLine
		if e.Estimated {
			key = i18n.KeyEarningsAlertLineEstimated
		}
		lines = append(lines, i18n.T(b.lang, key, t, e.Date, days))
		if err := b.db.SetSignalState(t, earningsSignalFamily, e.Date); err != nil {
			logger.Errorf("earnings alert state %s: %v", t, err)
		}
	}
	if len(lines) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyEarningsAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.publishAlert("earnings_alert", notification.LevelInfo, sb.String())
}

// checkRestrictedAlerts (Phase 16, TW only) warns once per held position
// that enters TWSE/TPEx disposition/attention status. Same batched-title-
// plus-lines shape as checkStopLossAlerts above; the dedup/persistence
// itself lives in RiskService.EvaluateRestrictedAlerts (Phase 24 tech debt
// 2) since it's the same breach-decision pattern as the other risk checks.
// restricted is nil (no-op) for US or when Shioaji isn't configured — see
// restrictedTickers.
func (b *Bot) checkRestrictedAlerts(positions []db.Position, restricted map[string]string) {
	if b.risks() == nil {
		return
	}
	alerts, err := b.risks().EvaluateRestrictedAlerts(positions, restricted)
	if err != nil {
		logger.Errorf("restricted alert evaluation: %v", err)
		return
	}
	if len(alerts) == 0 {
		return
	}

	var lines []string
	for _, a := range alerts {
		lines = append(lines, i18n.T(b.lang, i18n.KeyRestrictedStockAlertLine, b.tickerLabel(a.Ticker), a.Reason))
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyRestrictedStockAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.publishAlert("restricted_stock", notification.LevelWarning, sb.String())
}

const (
	stopLossSignalFamily     = "stop_loss"
	trailingStopSignalFamily = "trailing_stop"
	// breachedState is the signal_states value recorded while a stop-loss/
	// trailing-stop threshold stays breached; any other value (including "",
	// the unset default) means "not currently breached".
	breachedState = "breached"
)

// priceFor returns ticker's current price, preferring an already-fetched
// quote from prices (built by the caller from data it fetched for another
// purpose, e.g. RunDailyReport's watchlist stock data) and falling back to a
// direct quote fetch for any ticker prices doesn't cover — the same
// prefetch-with-fallback shape recordNetWorthSnapshot has always used for
// position tickers outside the watchlist prefetch.
func (b *Bot) priceFor(ticker string, prices map[string]float64) (float64, bool) {
	if p, ok := prices[ticker]; ok {
		return p, true
	}
	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		logger.Errorf("quote %s: %v", ticker, err)
		return 0, false
	}
	return q.Price, true
}

// filterPositionsByMarket returns the subset of positions (a ticker->
// db.Position map, as loadPositions returns) whose ticker belongs to m —
// runDailyReport's exit-discipline checks need this since loadPositions
// itself is all-market but each market's daily report must only alert on
// its own positions (see runDailyReport's own doc comment on this call
// site).
func filterPositionsByMarket(positions map[string]db.Position, m market.MarketID) map[string]db.Position {
	out := make(map[string]db.Position, len(positions))
	for t, p := range positions {
		if market.Of(t) == m {
			out[t] = p
		}
	}
	return out
}

// positionsSlice converts loadPositions' ticker->position map into a slice
// sorted by ticker, purely so the stop-loss/trailing-stop alert messages
// render in a stable, deterministic order — Go map iteration order is
// randomized.
func positionsSlice(positions map[string]db.Position) []db.Position {
	out := make([]db.Position, 0, len(positions))
	for _, p := range positions {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ticker < out[j].Ticker })
	return out
}

// checkStopLossAlerts is Phase 3.11 PR1's two-tier stop-loss check (§3.3).
func (b *Bot) checkStopLossAlerts(positions []db.Position, prices map[string]float64) {
	if b.risks() == nil {
		return
	}
	alerts, err := b.risks().EvaluateStopLoss(positions, prices, b.stopLossPct, b.stopLossPctTW)
	if err != nil {
		logger.Errorf("stop loss evaluation: %v", err)
		return
	}
	if len(alerts) == 0 {
		return
	}

	var lines []string
	for _, a := range alerts {
		if a.IsCustomStop {
			lines = append(lines, i18n.T(b.lang, i18n.KeyStopPriceHit, b.tickerLabel(a.Ticker), b.money(a.Ticker, a.StopPrice), b.money(a.Ticker, a.CurrentPrice)))
		} else {
			lines = append(lines, i18n.T(b.lang, i18n.KeyStopLossAlertLine, b.tickerLabel(a.Ticker), b.money(a.Ticker, a.AvgCost), b.money(a.Ticker, a.CurrentPrice), a.LossPct))
		}
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyStopLossAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.publishAlert("stop_loss", notification.LevelCritical, sb.String())
}

// checkTrailingStopAlerts warns about any open position whose close-price
// drawdown from its post-first-buy peak has just breached the trailing-stop
// threshold.
func (b *Bot) checkTrailingStopAlerts(positions []db.Position, prices map[string]float64, atrs map[string]float64) {
	if b.risks() == nil {
		return
	}
	alerts, err := b.risks().EvaluateTrailingStop(positions, prices, atrs, b.trailingStopPct, b.trailingStopPctTW, b.trailingStopATRMult)
	if err != nil {
		logger.Errorf("trailing stop evaluation: %v", err)
		return
	}
	if len(alerts) == 0 {
		return
	}

	var lines []string
	for _, a := range alerts {
		if a.ATRBased {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrailingStopAlertLineATR, b.tickerLabel(a.Ticker), b.money(a.Ticker, a.PeakPrice), b.money(a.Ticker, a.CurrentPrice), a.DrawdownPct, a.ThresholdPct, a.TrailingATRMult))
		} else {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrailingStopAlertLine, b.tickerLabel(a.Ticker), b.money(a.Ticker, a.PeakPrice), b.money(a.Ticker, a.CurrentPrice), a.DrawdownPct))
		}
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyTrailingStopAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.publishAlert("trailing_stop", notification.LevelWarning, sb.String())
}

const (
	targetRMultiple = service.TargetRMultiple
	trailProfitPct  = service.TrailProfitPct

	targetSignalFamily   = service.TargetSignalFamily
	ma5TrailSignalFamily = service.MA5TrailSignalFamily
	hitState             = service.HitState
)

// checkTargetAlerts warns once when a position with a stop price set
// first closes at or above its 2R target.
func (b *Bot) checkTargetAlerts(positions []db.Position, prices map[string]float64) {
	if b.risks() == nil {
		return
	}
	alerts, err := b.risks().EvaluateTargetAlerts(positions, prices)
	if err != nil {
		logger.Errorf("target alert evaluation: %v", err)
		return
	}
	if len(alerts) == 0 {
		return
	}

	var lines []string
	for _, a := range alerts {
		lines = append(lines, i18n.T(b.lang, i18n.KeyTargetReached, b.tickerLabel(a.Ticker), a.RMultiple, b.money(a.Ticker, a.TargetPrice), b.money(a.Ticker, a.CurrentPrice)))
	}

	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.publishAlert("target_reached", notification.LevelInfo, sb.String())
}

// checkMA5BreakAlerts warns once when a position that's up at least
// trailProfitPct (10%) unrealized closes below its MA5.
func (b *Bot) checkMA5BreakAlerts(positions []db.Position, prices map[string]float64, ma5s map[string]float64) {
	if b.risks() == nil {
		return
	}
	alerts, err := b.risks().EvaluateMA5BreakAlerts(positions, prices, ma5s)
	if err != nil {
		logger.Errorf("ma5 break evaluation: %v", err)
		return
	}
	if len(alerts) == 0 {
		return
	}

	var lines []string
	for _, a := range alerts {
		lines = append(lines, i18n.T(b.lang, i18n.KeyMA5Break, b.tickerLabel(a.Ticker), b.money(a.Ticker, a.MA5), b.money(a.Ticker, a.CurrentPrice)))
	}

	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.publishAlert("ma5_break", notification.LevelWarning, sb.String())
}

// checkBuyAlerts is the buy-alert counterpart of checkStopLossAlerts etc.
func (b *Bot) checkBuyAlerts(alerts []db.BuyAlert, prices map[string]float64) {
	if b.risks() == nil {
		return
	}
	triggers, err := b.risks().EvaluateBuyAlerts(alerts, prices)
	if err != nil {
		logger.Errorf("buy alert evaluation: %v", err)
		return
	}
	if len(triggers) == 0 {
		return
	}

	var lines []string
	for _, t := range triggers {
		lines = append(lines, i18n.T(b.lang, i18n.KeyBuyAlertHitLine, b.tickerLabel(t.Alert.Ticker), b.money(t.Alert.Ticker, t.Alert.Price), b.money(t.Alert.Ticker, t.CurrentPrice), b.buyAlertDirPhrase(t.Alert.Direction)))
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyBuyAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.publishAlert("buy_alert", notification.LevelInfo, sb.String())
}

// weeklyNetWorthLine renders RunWeeklyReview's opening line: total position
// value and its % change from about a week ago — net_worth_snapshots' first
// reader since RunClosingSnapshot's recordNetWorthSnapshot started writing
// it in Phase 2 (Phase 3.6 PR2). The business logic (which baseline to
// compare against, when to skip) lives in service.WeeklyNetWorthChange
// (Phase 24 Stage 3); this is just the i18n rendering of its result,
// picking the TWD-labeled key pair for market.TW, same precedent as
// sendPortfolioSection's US/TW key selection.
func (b *Bot) weeklyNetWorthLine(m market.MarketID, cash float64, haveCash bool) (string, error) {
	latest, pctChange, ok, err := service.WeeklyNetWorthChange(b.db, m)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}

	if m == market.TW {
		if haveCash {
			return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLineWithCashTWD, latest, pctChange, latest+cash), nil
		}
		return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLineTWD, latest, pctChange), nil
	}
	if haveCash {
		return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLineWithCash, latest, pctChange, latest+cash), nil
	}
	return i18n.T(b.lang, i18n.KeyWeeklyNetWorthLine, latest, pctChange), nil
}

// RunWeeklyReview is Phase 3.6 PR2's Sunday portfolio review: the same
// per-position data assembly handleInsight uses (positions, technicals,
// fundamentals, earnings, thesis, vs-SPY, cash), plus this week's /track
// summary folded into the same LLM call (so the model can comment on
// recommendation accuracy alongside its portfolio judgment — see
// llm.Client.WeeklyReview), wrapped with a net-worth opening line and a
// next-week earnings preview appended after. Scheduled for Sunday (US
// markets closed) — a review rhythm, not a reactive one, deliberately below
// the project's daily-cadence ceiling (see PLAN.md's Phase 3.6 note).
// Wired up only after several manual /insight runs had proven the
// underlying prompt, so an untuned prompt never lands in the push channel
// (see docs/phase-3.6-portfolio-insight.md).
//
// One message per market, each from its own LLM call over only that market's
// holdings, cash, /track summary and earnings preview: a single combined call
// produced a review that compared TWD position sizes against USD ones,
// summed both into one "total assets" figure, and interleaved US and TW
// judgments in one wall of prose. Markets with no positions are skipped
// entirely (no empty per-market message), same convention as
// RunMonthlyReport's per-market blocks.
func (b *Bot) RunWeeklyReview(ctx context.Context) {
	defer b.recoverJobPanic("weekly review")

	positions, err := b.db.GetPositions()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyQueryFailed, err))
		return
	}
	if len(positions) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPortfolioEmpty))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyWeeklyReviewStart))

	// Computed once for both markets — computeTrackRows fetches a quote per
	// recommended ticker, and running it twice would double that traffic just
	// to throw half the rows away each time. Each market's call filters it.
	var trackRows []trackRow
	if rows, _, ok, err := b.computeTrackRows(7); err != nil {
		logger.Errorf("weekly review: track rows: %v", err)
	} else if ok {
		trackRows = rows
	}

	for _, m := range []market.MarketID{market.US, market.TW} {
		b.runWeeklyReviewMarket(ctx, m, positions, trackRows)
	}
}

// runWeeklyReviewMarket is RunWeeklyReview's per-market half: one LLM call
// and one message covering market m only. Returns silently when m has no
// positions.
func (b *Bot) runWeeklyReviewMarket(ctx context.Context, m market.MarketID, positions []db.Position, trackRows []trackRow) {
	stocks, earnings := b.portfolioStocks(m, positions)
	if len(stocks) == 0 {
		return
	}

	cash, haveCash, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("weekly review: load cash (%s): %v", m, err)
	}

	var trackSummary string
	var marketRows []trackRow
	for _, r := range trackRows {
		if r.Market == m {
			marketRows = append(marketRows, r)
		}
	}
	if len(marketRows) > 0 {
		overall, bySource, byMarket := summarizeTrack(marketRows)
		trackSummary = renderTrackSummary(b.lang, overall, bySource, byMarket)
	}

	result, err := b.llm.WeeklyReview(ctx, stocks, cash, haveCash, trackSummary, m == market.TW)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	titleKey := i18n.KeyWeeklyReviewResultTitleUS
	if m == market.TW {
		titleKey = i18n.KeyWeeklyReviewResultTitleTW
	}

	var sb strings.Builder
	// Skipped individually when this market has no snapshot history yet
	// (weeklyNetWorthLine's own "" = skip contract).
	if line, err := b.weeklyNetWorthLine(m, cash, haveCash); err != nil {
		logger.Errorf("weekly review: net worth line (%s): %v", m, err)
	} else if line != "" {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString(i18n.T(b.lang, titleKey, result))
	sb.WriteString(renderEarningsPreview(b.lang, earnings, 7))

	b.Send(sb.String())
}

// RunMonthlyReport is Phase 3.6 追加項's net-worth monthly report (see
// docs/phase-3.6-monthly-report.md): a deliberately non-LLM data archive for
// the prior full calendar month — deterministic (same DB contents always
// produce the same report) and unaffected by the LLM provider chain being
// down, unlike RunWeeklyReview's judgment-based prose. Phase 6 PR2 splits
// this into per-market blocks (§5.3) via buildMonthlyReportBlock — a market
// with no net_worth_snapshots row anywhere in the month has its whole block
// skipped (see that function), same "查無資料就跳過不發空報告" convention as
// every other optional block in this file; if *neither* market has any data
// at all, nothing is sent (log-only), same as the pre-Phase-6 single-block
// behavior for a fresh install's first month.
func (b *Bot) RunMonthlyReport(ctx context.Context) {
	defer b.recoverJobPanic("monthly report")

	from, to := service.MonthRange(time.Now().In(cst))

	usBlock, usOK := b.buildMonthlyReportBlock(market.US, from, to)
	twBlock, twOK := b.buildMonthlyReportBlock(market.TW, from, to)
	if !usOK && !twOK {
		logger.Infof("monthly report: no net worth snapshots for %s..%s in either market, skipping", from, to)
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTitle, from[:7]))
	if usOK {
		sb.WriteString(i18n.T(b.lang, i18n.KeyPortfolioSectionUS))
		sb.WriteString(usBlock)
	}
	if twOK {
		sb.WriteString(i18n.T(b.lang, i18n.KeyPortfolioSectionTW))
		sb.WriteString(twBlock)
	}

	b.Send(sb.String())
}

// buildMonthlyReportBlock renders one market's monthly-report body
// (sparkline, month-end change, max drawdown, realized P&L, transaction
// count, benchmark move, cash) — Phase 6 PR2's per-market split of what used
// to be RunMonthlyReport's single US-only body (§5.3). ok is false when m
// has no net_worth_snapshots row anywhere in [from, to], meaning the whole
// block should be skipped rather than shown empty; every other line is
// independently optional (guarded by the corresponding Have* field) and
// just omitted, same degrade-per-field convention as the rest of this file.
// The actual data gathering/derivation lives in
// service.BuildMonthlyReportBlock (Phase 24 Stage 3) — this function is
// just the i18n rendering of its result. Content lines deliberately keep
// their pre-Phase-6 "$"-formatted keys (KeyMonthlyReportChangeLine et al.,
// reused unchanged for the TW block too) rather than gaining a TWD variant
// each — the caller's KeyPortfolioSectionUS/TW header is what establishes
// which currency a block's numbers are in, matching this project's accepted
// "只有新增的聚合行 key 直接把幣別做進文案" simplification (see
// docs/phase-6-tw-market.md §3.2); only the genuinely new benchmark line
// (SPY vs. 0050 — a different benchmark name, not just a currency symbol)
// gets its own TW-specific key.
func (b *Bot) buildMonthlyReportBlock(m market.MarketID, from, to string) (string, bool) {
	cash, haveCash, err := b.loadCash(m)
	if err != nil {
		logger.Errorf("monthly report: load cash (%s): %v", m, err)
	}

	block, ok := service.BuildMonthlyReportBlock(b.db, m, from, to, cash, haveCash)
	if !ok {
		return "", false
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSparklineLine, service.Sparkline(block.Values)))
	if block.HaveChange {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportChangeLine, block.Latest, block.ChangePct))
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportDrawdownLine, block.DrawdownPct))
	if block.SellCount > 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportRealizedLine, block.Realized))
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTxCountLine, block.TxCount))
	if block.HaveBenchmark {
		if m == market.TW {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTWBenchmarkLine, block.BenchmarkPct))
		} else {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSPYLine, block.BenchmarkPct))
		}
	}
	if block.HaveCash {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportCashLine, block.Latest+block.Cash, block.Cash))
	}

	return sb.String(), true
}

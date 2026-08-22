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
		b.Send(i18n.T(b.lang, i18n.KeyJobPanic, job, r))
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
	b.Send(sb.String())
}

// benchmarkFor returns the daily-snapshot benchmark ticker for m: SPY for
// US, 0050 for TW (Phase 6, same "snapshot alongside the watchlist" role
// benchmarkTicker/snapshotBenchmark already played for US-only — see
// docs/phase-6-tw-market.md §3.3). 0050 is allowed to simultaneously be a
// TW watchlist/position ticker; SaveSnapshot's INSERT OR REPLACE makes a
// same-(ticker,date) double-write from both paths safe.
func benchmarkFor(m market.MarketID) string {
	if m == market.TW {
		return "0050"
	}
	return benchmarkTicker
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
	prices := make(map[string]float64, len(tickers))
	eventThresholds := signals.DefaultEventThresholds(m)
	var priceEventHits []signals.PriceEvent
	for _, t := range tickers {
		q, err := b.provider.GetQuote(t)
		if err != nil {
			logger.Errorf("closing snapshot: quote %s: %v", t, err)
			continue
		}
		// On a market holiday the cron still fires but providers return the
		// previous session's quote; its timestamp is then a full day old,
		// and saving it would file old data under the wrong date.
		if time.Since(q.Timestamp) > 12*time.Hour {
			logger.Warnf("closing snapshot: %s quote is stale (%s), skipping (holiday?)", t, q.Timestamp.Format(time.RFC3339))
			continue
		}
		prices[t] = q.Price
		// Phase 20: collected here, not acted on until after the loop —
		// see recordPriceEvents' doc comment for why the LLM call can't
		// live inside this already-sequential per-ticker fetch loop.
		if ev := signals.CheckPriceEvent(q, eventThresholds); ev != nil {
			priceEventHits = mergePriceEventHit(priceEventHits, *ev)
		}
		// Cumulative-decline check: today's close vs. the close
		// CumulativeWindowDays sessions ago, queried before SaveSnapshot
		// below writes today's own row (see GetRecentCloses' doc comment).
		// A merge into an existing same-ticker hit, not a separate append —
		// price_events' (ticker, date) unique index allows only one row per
		// ticker per day.
		if closes, err := b.db.GetRecentCloses(t, eventThresholds.CumulativeWindowDays); err != nil {
			logger.Errorf("closing snapshot: recent closes %s: %v", t, err)
		} else if len(closes) == eventThresholds.CumulativeWindowDays {
			if ev := signals.CheckCumulativeDecline(t, closes[0], q.Price, eventThresholds); ev != nil {
				priceEventHits = mergePriceEventHit(priceEventHits, *ev)
			}
		}
		snap := db.DailySnapshot{
			Ticker:        t,
			Date:          date,
			Open:          q.Open,
			Close:         q.Price,
			High:          q.High,
			Low:           q.Low,
			Volume:        q.Volume,
			ChangePercent: q.ChangePercent,
		}
		if err := b.db.SaveSnapshot(snap); err != nil {
			logger.Errorf("closing snapshot: save %s: %v", t, err)
		}
	}
	logger.Infof("closing snapshot: done for %s market=%s (%d tickers)", date, m, len(tickers))

	b.recordPriceEvents(ctx, priceEventHits, m, date)

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
	sort.Slice(pending, func(i, j int) bool {
		return priceEventMoveSize(pending[i]) > priceEventMoveSize(pending[j])
	})

	writeup, overflow := pending, []signals.PriceEvent(nil)
	if len(pending) > priceEventWriteupCap {
		writeup, overflow = pending[:priceEventWriteupCap], pending[priceEventWriteupCap:]
	}

	for _, ev := range writeup {
		news, _ := b.provider.GetNews(ev.Ticker, 5)
		summary, err := b.llm.ExplainPriceEvent(ctx, ev.Ticker, ev.GapPct, ev.ChangePct, ev.CumulativePct, news)
		if err != nil {
			logger.Errorf("price events: LLM %s: %v", ev.Ticker, err)
			continue
		}
		if err := b.db.SavePriceEvent(db.PriceEvent{Ticker: ev.Ticker, Market: string(m), Date: date, GapPct: ev.GapPct, ChangePct: ev.ChangePct, CumulativePct: ev.CumulativePct, Summary: summary}); err != nil {
			logger.Errorf("price events: save %s: %v", ev.Ticker, err)
			continue
		}
		b.Send(i18n.T(b.lang, i18n.KeyPriceEventResultTitle, ev.Ticker, summary))
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
	b.Send(i18n.T(b.lang, i18n.KeyPriceEventOverflowLine, sb.String()))
}

// priceEventMoveSize is recordPriceEvents' writeup-priority ranking key —
// the largest of gap/change/cumulative-decline magnitude, so a ticker with
// only a big cumulative decline still competes fairly against one with a
// large single-day move.
func priceEventMoveSize(ev signals.PriceEvent) float64 {
	return math.Max(math.Max(math.Abs(ev.GapPct), math.Abs(ev.ChangePct)), math.Abs(ev.CumulativePct))
}

// mergePriceEventHit adds ev to hits, merging into an existing same-ticker
// entry instead of appending a duplicate — price_events' (ticker, date)
// unique index means a single-day and cumulative-decline hit for the same
// ticker on the same day must become one row, not two.
func mergePriceEventHit(hits []signals.PriceEvent, ev signals.PriceEvent) []signals.PriceEvent {
	for i := range hits {
		if hits[i].Ticker != ev.Ticker {
			continue
		}
		if ev.GapPct != 0 {
			hits[i].GapPct = ev.GapPct
		}
		if ev.ChangePct != 0 {
			hits[i].ChangePct = ev.ChangePct
		}
		if ev.CumulativePct != 0 {
			hits[i].CumulativePct = ev.CumulativePct
		}
		return hits
	}
	return append(hits, ev)
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
// Same stale-quote guard as RunClosingSnapshot's per-ticker loop. Silent on
// failure, same as the rest of this job — a missing benchmark row just makes
// /track (or, for paper.db, the trailing stop / equity curve) fall back to
// whatever that caller already does without same-day benchmark data.
func (b *Bot) snapshotBenchmarkTo(target *db.DB, date string, m market.MarketID) {
	ticker := benchmarkFor(m)
	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		logger.Errorf("closing snapshot: benchmark %s: %v", ticker, err)
		return
	}
	if time.Since(q.Timestamp) > 12*time.Hour {
		logger.Warnf("closing snapshot: benchmark %s quote is stale (%s), skipping (holiday?)", ticker, q.Timestamp.Format(time.RFC3339))
		return
	}
	snap := db.DailySnapshot{
		Ticker:        ticker,
		Date:          date,
		Open:          q.Open,
		Close:         q.Price,
		High:          q.High,
		Low:           q.Low,
		Volume:        q.Volume,
		ChangePercent: q.ChangePercent,
	}
	if err := target.SaveSnapshot(snap); err != nil {
		logger.Errorf("closing snapshot: save benchmark %s: %v", ticker, err)
	}
}

// recordNetWorthSnapshot totals market m's open positions' value as of the
// closing snapshot and stores it dated the same day (Phase 6: per-market row,
// never summed across markets — see SaveNetWorthSnapshot). prices reuses the
// quotes RunClosingSnapshot already fetched for watchlist tickers (positions
// are auto-added to the watchlist on /buy, so this covers the common case);
// any position ticker missing from it gets a direct quote fetch as a
// fallback (see priceFor).
func (b *Bot) recordNetWorthSnapshot(date string, m market.MarketID, prices map[string]float64) {
	positions, err := b.db.GetPositions()
	if err != nil {
		logger.Errorf("net worth snapshot: positions: %v", err)
		return
	}

	var total float64
	var haveAny bool
	for _, p := range positions {
		if market.Of(p.Ticker) != m {
			continue
		}
		haveAny = true
		price, ok := b.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		total += p.Shares * price
	}
	if !haveAny {
		return
	}
	if err := b.db.SaveNetWorthSnapshot(date, m, total); err != nil {
		logger.Errorf("net worth snapshot: save: %v", err)
	}
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
const twMarketClosedStaleness = 12 * time.Hour

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
		b.checkRestrictedAlerts(positionList, b.restrictedTickers(ctx, m))
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

	sources := recommendationSources(in.watchlistTickers, in.candidateTickers, in.scanHits, explore)
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
// recommendationSources to label "explore".
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

// checkStatefulSignals runs the RSI/MACD and strategy checks that diff against
// the last state persisted in signal_states.
func (b *Bot) checkStatefulSignals(ticker string, candles []data.Candle, isBearRegime bool) []signals.Signal {
	var out []signals.Signal
	closes := data.Closes(candles)

	prevRSI, err := b.db.GetSignalState(ticker, signals.FamilyRSI)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
	}
	sig, newRSI := b.detector.CheckRSIState(ticker, closes, prevRSI)
	if sig != nil {
		out = append(out, *sig)
	}
	if newRSI != prevRSI {
		if err := b.db.SetSignalState(ticker, signals.FamilyRSI, newRSI); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
		}
	}

	prevMACD, err := b.db.GetSignalState(ticker, signals.FamilyMACD)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
	}
	sig, newMACD := b.detector.CheckMACDCross(ticker, closes, prevMACD)
	if sig != nil {
		out = append(out, *sig)
	}
	if newMACD != prevMACD {
		if err := b.db.SetSignalState(ticker, signals.FamilyMACD, newMACD); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
		}
	}

	// Strategy 1: Squeeze Breakout
	prevSqueeze, err := b.db.GetSignalState(ticker, signals.FamilyStrategySqueeze)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategySqueeze, err)
	}
	sig, newSqueeze := b.detector.CheckSqueezeBreakout(ticker, candles, prevSqueeze)
	if sig != nil {
		if isBearRegime {
			sig.Message += "\n" + i18n.T(b.lang, i18n.KeyStrategyBearRegimeWarning)
		}
		out = append(out, *sig)
	}
	if newSqueeze != prevSqueeze {
		if err := b.db.SetSignalState(ticker, signals.FamilyStrategySqueeze, newSqueeze); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategySqueeze, err)
		}
	}

	// Strategy 2: Box Bottom Rebound
	prevBox, err := b.db.GetSignalState(ticker, signals.FamilyStrategyBox)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBox, err)
	}
	sig, newBox := b.detector.CheckBoxBottom(ticker, candles, prevBox)
	if sig != nil {
		if isBearRegime {
			sig.Message += "\n" + i18n.T(b.lang, i18n.KeyStrategyBearRegimeWarning)
		}
		out = append(out, *sig)
	}
	if newBox != prevBox {
		if err := b.db.SetSignalState(ticker, signals.FamilyStrategyBox, newBox); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBox, err)
		}
	}

	// Strategy 3: Trend Breakout (Phase 14 網 3) — revenue-growth gate is
	// short-circuited: only evaluated when the (zero-request) technical AND
	// already passed, so the FinMind/Finnhub hit stays ~0-5/day (see
	// docs/phase-14-strategy-screens-2.md §4.2c).
	prevBreakout, err := b.db.GetSignalState(ticker, signals.FamilyStrategyBreakout)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBreakout, err)
	}
	sig, newBreakout := b.detector.CheckTrendBreakout(ticker, candles, prevBreakout)
	if sig != nil {
		p := signals.DefaultScreenParams(market.Of(ticker))
		if p.RequireRevenueGrowth && !b.revenueGrowthOK(ticker, p.MinRevenueGrowthPct) {
			sig = nil
		}
	}
	if sig != nil {
		if isBearRegime {
			sig.Message += "\n" + i18n.T(b.lang, i18n.KeyStrategyBearRegimeWarning)
		}
		out = append(out, *sig)
	}
	if newBreakout != prevBreakout {
		if err := b.db.SetSignalState(ticker, signals.FamilyStrategyBreakout, newBreakout); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBreakout, err)
		}
	}

	// Strategy 4: Trend Pullback (Phase 14 網 4)
	prevPullback, err := b.db.GetSignalState(ticker, signals.FamilyStrategyPullback)
	if err != nil {
		logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyPullback, err)
	}
	sig, newPullback := b.detector.CheckTrendPullback(ticker, candles, prevPullback)
	if sig != nil {
		if isBearRegime {
			sig.Message += "\n" + i18n.T(b.lang, i18n.KeyStrategyBearRegimeWarning)
		}
		out = append(out, *sig)
	}
	if newPullback != prevPullback {
		if err := b.db.SetSignalState(ticker, signals.FamilyStrategyPullback, newPullback); err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyPullback, err)
		}
	}

	// Strategy 5: Trust Follow (Phase 15 網 5, TW only) — the FinMind call is
	// short-circuited behind TrustFollowTechnicalGate the same way Strategy
	// 3's revenue-growth gate is short-circuited (see revenueGrowthOK's doc
	// comment): only tickers that already clear the candle-only liquidity/
	// trend/deviation conditions are worth a network request, keeping this to
	// a handful of TW tickers per day rather than the whole universe.
	p := signals.DefaultScreenParams(market.Of(ticker))
	if p.RequireTrustData && b.trustNet != nil && signals.TrustFollowTechnicalGate(candles, p) {
		prevTrust, err := b.db.GetSignalState(ticker, signals.FamilyStrategyTrust)
		if err != nil {
			logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyTrust, err)
		}
		rows, err := b.trustNet.GetTrustNetSeries(ticker, len(candles))
		if err != nil {
			logger.Errorf("trust net %s: %v", ticker, err)
		} else {
			trustAligned := signals.AlignTrustNet(candles, rows)
			foreignAligned := signals.AlignForeignNet(candles, rows)
			sig, newTrust := b.detector.CheckTrustFollow(ticker, candles, trustAligned, foreignAligned, prevTrust)
			if sig != nil {
				if isBearRegime {
					sig.Message += "\n" + i18n.T(b.lang, i18n.KeyStrategyBearRegimeWarning)
				}
				out = append(out, *sig)
			}
			if newTrust != prevTrust {
				if err := b.db.SetSignalState(ticker, signals.FamilyStrategyTrust, newTrust); err != nil {
					logger.Errorf("signal state %s/%s: %v", ticker, signals.FamilyStrategyTrust, err)
				}
			}
		}
	}

	return out
}

// revenueGrowthOK is Phase 14 §4.2c's short-circuit fundamentals gate for
// 網 3【趨勢突破】's TW-only revenue-growth condition: called only after the
// technical AND already passed (0-5 tickers/day, not the full universe), so
// routing through cachedFundamentals keeps this to one FinMind request per
// ticker per slowDataCacheTTL window rather than one per scan. minPct is
// ScreenParams.MinRevenueGrowthPct; the underlying field differs by market
// (data.Fundamentals.MonthRevenueYoYPct is TW-only) but b.fundamentals
// already routes US/TW via FundamentalsRouter, so this reads whichever field
// is non-zero for ticker's market.
func (b *Bot) revenueGrowthOK(ticker string, minPct float64) bool {
	if b.fundamentals == nil {
		return false
	}
	fd, err := b.cachedFundamentals(ticker)
	if err != nil {
		logger.Errorf("revenue growth gate %s: %v", ticker, err)
		return false
	}
	growth := fd.MonthRevenueYoYPct
	if market.Of(ticker) != market.TW {
		growth = fd.RevenueGrowthYoY
	}
	return growth > minPct
}

// scanChunkCount and universeScanRequestDelay govern Phase 2.6's daily
// candidate-pool scan. Originally the universe (~500 S&P 500 + manual
// tickers) was split into 5 rotating slices so each day only fetched ~100
// histories; that traded freshness for request volume, which cost more than
// it saved — a Squeeze Breakout seen 4 days late is a chase, not an entry,
// and the RSI/MACD state machines silently collapsed any round trip that
// started and finished inside one rotation. Since the scan is an unattended
// 05:45 cron with no ctx timeout, wall-clock is free where request *rate* is
// not, so the chunking is off (chunkCount 1 = whole universe daily) and the
// budget is spent on a longer per-request delay instead: ~500 tickers × 1s
// is ~10 min, still well under the 06:00 backup, at a third of the old
// requests-per-second. Both stay tunable knobs rather than inlined constants
// — if Yahoo ever starts 429ing, raise the delay first, and only go back to
// chunkCount 2+ if that isn't enough.
const (
	scanChunkCount           = 1
	universeScanRequestDelay = 1 * time.Second
)

// universeScanChunk returns the slice of tickers to scan for dayIndex (an
// ever-increasing day counter, e.g. time.Now().YearDay()), rotating through
// all of tickers over chunkCount calls. Pure and stateless — no persisted
// scan cursor needed — so coverage is deterministic given the same tickers
// and dayIndex, at the cost of chunk boundaries shifting slightly as the
// universe's membership changes day to day (harmless: PLAN.md tolerates
// staleness on the order of months for this data).
func universeScanChunk(tickers []string, chunkCount, dayIndex int) []string {
	if len(tickers) == 0 || chunkCount <= 0 {
		return nil
	}
	size := (len(tickers) + chunkCount - 1) / chunkCount
	idx := dayIndex % chunkCount
	if idx < 0 {
		idx += chunkCount
	}
	start := idx * size
	if start >= len(tickers) {
		return nil
	}
	end := start + size
	if end > len(tickers) {
		end = len(tickers)
	}
	return tickers[start:end]
}

// RunUniverseScan is the US-market universe scan's scheduler entry point
// (05:45 CST Tue-Sat, unchanged since before Phase 6) — a thin wrapper
// around runUniverseScan(ctx, market.US).
func (b *Bot) RunUniverseScan(ctx context.Context) {
	b.runUniverseScan(ctx, market.US)
}

// RunTWUniverseScan is Phase 6 PR2's TW-market universe scan entry point
// (14:40 CST Mon-Fri, see docs/phase-6-tw-market.md §3.3/§5.2) — the TW
// counterpart of RunUniverseScan above, scanning the tw150 pool
// (source='tw', seeded by db.seedTW150) instead of the S&P 500 pool.
func (b *Bot) RunTWUniverseScan(ctx context.Context) {
	b.runUniverseScan(ctx, market.TW)
}

// runUniverseScan is Phase 2.6's candidate-pool scan, generalized by
// Phase 6 PR2 to run per-market: it checks market m's universe entries
// (all of them daily as of scanChunkCount 1, see that const's comment;
// still routed through universeScanChunk so the rotation can come back as a
// one-line change) (filtered via market.Of(ticker) — not by
// source, since a manually /universe add'ed TW ticker is source='manual'
// and must still be scanned as TW, see docs/phase-6-tw-market.md §5.2)
// excluding anything already on m's own watchlist (which gets a full
// RSI/MACD check daily anyway) for a fresh RSI/MACD signal via the same
// checkStatefulSignals used for the watchlist — safe to share signal_states
// with it since the universe and watchlist ticker sets never overlap, and
// safe to share across US/TW runs of this same function since a ticker only
// ever belongs to one market. Any hit is logged to scan_hits for
// runDailyReport/handleRecommend to pick up the same day and upgrade into an
// LLM candidate. Silent background job like RunClosingSnapshot: results go
// to the DB/log, not Telegram — the eventual daily report is the
// user-facing surface.
func (b *Bot) runUniverseScan(ctx context.Context, m market.MarketID) {
	defer b.recoverJobPanic("universe scan")

	// Trading-day gate (Phase 13 §8) — silent, no Telegram send, same
	// closed-market signals RunClosingSnapshot/runDailyReport already use
	// per market (US: NYSE calendar; TW: isTWMarketClosed's live
	// quote-freshness heuristic, see its own doc comment for why TW has no
	// fixed holiday calendar). Without this, a holiday rerun would scan
	// stale/unchanged data and risk a duplicate scan_hits row for the same
	// signal. US checks yesterday's CST date, not today's — this job runs at
	// 05:45 CST (like RunClosingSnapshot's 05:30), which is already the next
	// calendar day in Taiwan relative to the US session just closed, so
	// checking today's date misjudges Saturday (a genuine trading day,
	// Friday's session) as a weekend skip and can misjudge the day after a
	// US holiday too.
	if m == market.US {
		if !market.IsTradingDay(b.now().In(cst).AddDate(0, 0, -1)) {
			logger.Infof("universe scan: market=%s closed, skipping", m)
			return
		}
	} else if b.isTWMarketClosed() {
		logger.Infof("universe scan: market=%s closed, skipping", m)
		return
	}

	entries, err := b.db.GetUniverse()
	if err != nil {
		logger.Errorf("universe scan: universe: %v", err)
		return
	}
	watchlist, err := b.db.GetWatchlistByMarket(m)
	if err != nil {
		logger.Errorf("universe scan: watchlist: %v", err)
		return
	}
	watchSet := make(map[string]bool, len(watchlist))
	for _, t := range watchlist {
		watchSet[t] = true
	}

	var tickers []string
	for _, e := range entries {
		if market.Of(e.Ticker) == m && !watchSet[e.Ticker] {
			tickers = append(tickers, e.Ticker)
		}
	}

	mc := b.computeMarketRegime(m)
	isBear := isBearRegime(mc)

	// Phase 16: skip TWSE/TPEx disposition (處置) or attention (注意)
	// tickers — the bot would otherwise happily recommend a stock currently
	// in split-auction trading with no idea anything's wrong. Fetched once
	// per scan (not per ticker), nil when SHIOAJI_ADDR isn't set (no-op,
	// same nil-degrade convention as every other optional provider).
	restricted := b.restrictedTickers(ctx, m)

	chunk := universeScanChunk(tickers, scanChunkCount, time.Now().In(cst).YearDay())
	date := todayDate()
	hits := 0
	for i, t := range chunk {
		select {
		case <-ctx.Done():
			logger.Warnf("universe scan: cancelled after %d/%d tickers", i, len(chunk))
			return
		default:
		}

		if reason, ok := restricted[t]; ok {
			logger.Infof("universe scan: skipping %s: %s", t, reason)
			continue
		}

		candles, err := b.history.GetHistory(t, "1y")
		if err != nil {
			logger.Errorf("universe scan: history %s: %v", t, err)
			continue
		}
		for _, sig := range b.checkStatefulSignals(t, candles, isBear) {
			if err := b.db.SaveScanHit(t, date, sig.Message); err != nil {
				logger.Errorf("universe scan: save hit %s: %v", t, err)
				continue
			}
			hits++
		}

		if i < len(chunk)-1 {
			time.Sleep(universeScanRequestDelay)
		}
	}
	logger.Infof("universe scan: market=%s checked %d tickers, %d hits", m, len(chunk), hits)
}

// restrictedTickers returns TW disposition (處置)/attention (注意) tickers
// mapped to a human-readable reason, for runUniverseScan and
// checkRestrictedAlerts to skip/warn about — nil for US (no such
// classification exists there) or when b.sinopac is unset (SHIOAJI_ADDR
// not configured), same nil-degrade convention as every other optional
// provider. Not cached across calls: RegulatoryPunish/RegulatoryNotice are
// each one free GET request, and this is called at most twice a day (once
// per job).
func (b *Bot) restrictedTickers(ctx context.Context, m market.MarketID) map[string]string {
	if m != market.TW || b.sinopac == nil {
		return nil
	}
	out := make(map[string]string)
	if punish, err := b.sinopac.RegulatoryPunish(ctx); err != nil {
		logger.Errorf("restricted tickers: regulatory punish: %v", err)
	} else {
		for code, reason := range punish {
			out[code] = reason
		}
	}
	if notice, err := b.sinopac.RegulatoryNotice(ctx); err != nil {
		logger.Errorf("restricted tickers: regulatory notice: %v", err)
	} else {
		for code, reason := range notice {
			if _, ok := out[code]; !ok {
				out[code] = reason
			}
		}
	}
	return out
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
	b.Send(sb.String())
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
	b.Send(sb.String())
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

// breachAlertDecision implements the dedup rule shared by the stop-loss and
// trailing-stop checks: alert once when an adverse move (a positive
// percentage — unrealized loss for stop-loss, drawdown from peak for
// trailing-stop) first crosses thresholdPct, stay silent on later calls
// while it remains breached, and reset once it recovers back under the
// threshold so a later re-breach alerts again. Mirrors the RSI/MACD dedup
// shape in checkStatefulSignals, generalized to a single scalar threshold.
// prevState is the raw signal_states value; newState is what the caller
// should persist back via db.SetSignalState ("" clears it, matching
// GetSignalState's own "unset" representation) — callers should only write
// it back when it differs from prevState, same as checkStatefulSignals does.
func breachAlertDecision(adverseMovePct, thresholdPct float64, prevState string) (breached, shouldAlert bool, newState string) {
	return service.BreachAlertDecision(adverseMovePct, thresholdPct, prevState)
}

func stopBreachDecision(close, stopPrice float64, prevState string) (breached, shouldAlert bool, newState string) {
	return service.StopBreachDecision(close, stopPrice, prevState)
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
	b.Send(sb.String())
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
	b.Send(sb.String())
}

const (
	targetRMultiple = service.TargetRMultiple
	trailProfitPct  = service.TrailProfitPct

	targetSignalFamily   = service.TargetSignalFamily
	ma5TrailSignalFamily = service.MA5TrailSignalFamily
	hitState             = service.HitState
)

func targetReachedDecision(close, targetPrice float64, prevState string) (reached, shouldAlert bool, newState string) {
	return service.TargetReachedDecision(close, targetPrice, prevState)
}

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
	b.Send(sb.String())
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
	b.Send(sb.String())
}

func buyAlertTriggered(price float64, alert db.BuyAlert) bool {
	return service.BuyAlertTriggered(price, alert)
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
	b.Send(sb.String())
}

// weeklyNetWorthLine renders RunWeeklyReview's opening line: total position
// value and its % change from about a week ago — net_worth_snapshots' first
// reader since RunClosingSnapshot's recordNetWorthSnapshot started writing
// it in Phase 2 (Phase 3.6 PR2). Returns "" (not an error) when there's no
// snapshot yet, or no baseline from roughly a week ago to compare against
// (e.g. a fresh install, or a holding period under a week) — skip the line
// rather than show a misleading 0%, same "ok=false means skip" pattern
// GetPeakClose's callers use.
// weeklyNetWorthLine reports market m's total position value and its %
// change from about a week ago. Phase 6 PR2 threads m through (PR1 only
// added the signature without a TW caller yet — see
// docs/phase-6-tw-market.md §5.3) and picks the TWD-labeled key pair for
// market.TW, same precedent as sendPortfolioSection's US/TW key selection.
func (b *Bot) weeklyNetWorthLine(m market.MarketID, cash float64, haveCash bool) (string, error) {
	latestDateStr, latest, ok, err := b.db.GetLatestNetWorth(m)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}

	latestDate, err := time.Parse("2006-01-02", latestDateStr)
	if err != nil {
		return "", err
	}
	weekAgo := latestDate.AddDate(0, 0, -7).Format("2006-01-02")

	prior, ok, err := b.db.GetNetWorthOnOrBefore(weekAgo, m)
	if err != nil {
		return "", err
	}
	if !ok || prior == 0 {
		return "", nil
	}

	pctChange := (latest - prior) / prior * 100
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

	from, to := monthRange(time.Now().In(cst))

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
// block should be skipped rather than shown empty; every other input inside
// the block is independently optional and just omits its own line, same
// degrade-per-field convention as the rest of this file. Content lines
// deliberately keep their pre-Phase-6 "$"-formatted keys (KeyMonthlyReportChangeLine
// et al., reused unchanged for the TW block too) rather than gaining a TWD
// variant each — the caller's KeyPortfolioSectionUS/TW header is what
// establishes which currency a block's numbers are in, matching this
// project's accepted "只有新增的聚合行 key 直接把幣別做進文案" simplification
// (see docs/phase-6-tw-market.md §3.2); only the genuinely new benchmark
// line (SPY vs. 0050 — a different benchmark name, not just a currency
// symbol) gets its own TW-specific key.
func (b *Bot) buildMonthlyReportBlock(m market.MarketID, from, to string) (string, bool) {
	points, err := b.db.GetNetWorthRange(from, to, m)
	if err != nil {
		logger.Errorf("monthly report: net worth range (%s): %v", m, err)
		return "", false
	}
	if len(points) == 0 {
		return "", false
	}

	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Total
	}
	latest := values[len(values)-1]

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSparklineLine, sparkline(values)))

	// Monthly return convention is "prior month-end vs. this month-end" (not
	// this month's first row, which would miss the change on the very first
	// trading day of the month). Falls back to this month's own first value
	// when there's no prior-month baseline yet (e.g. the first month on
	// record); if that's the only point too, there's nothing to diff
	// against, so the line is skipped entirely.
	fromDate, _ := time.Parse("2006-01-02", from)
	priorMonthEnd := fromDate.AddDate(0, 0, -1).Format("2006-01-02")
	baseline, haveBaseline, err := b.db.GetNetWorthOnOrBefore(priorMonthEnd, m)
	if err != nil {
		logger.Errorf("monthly report: baseline net worth (%s): %v", m, err)
	}
	if !haveBaseline && len(values) > 1 {
		baseline, haveBaseline = values[0], true
	}
	if haveBaseline && baseline != 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportChangeLine, latest, (latest-baseline)/baseline*100))
	}

	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportDrawdownLine, maxDrawdownPct(values)))

	if count, sellCount, realized, err := b.db.GetTransactionStatsByMarket(from, to, m); err != nil {
		logger.Errorf("monthly report: transaction stats (%s): %v", m, err)
	} else {
		if sellCount > 0 {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportRealizedLine, realized))
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTxCountLine, count))
	}

	if first, last, ok, err := b.db.GetSnapshotCloseRange(benchmarkFor(m), from, to); err != nil {
		logger.Errorf("monthly report: benchmark range (%s): %v", m, err)
	} else if ok && first != 0 {
		pct := (last - first) / first * 100
		if m == market.TW {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTWBenchmarkLine, pct))
		} else {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSPYLine, pct))
		}
	}

	if cash, haveCash, err := b.loadCash(m); err != nil {
		logger.Errorf("monthly report: load cash (%s): %v", m, err)
	} else if haveCash {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportCashLine, latest+cash, cash))
	}

	return sb.String(), true
}

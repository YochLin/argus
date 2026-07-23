package bot

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
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
		log.Printf("%s: panic: %v", job, r)
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
		log.Printf("closing snapshot: watchlist: %v", err)
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	now := time.Now().In(cst)
	date := now.Format("2006-01-02")
	if m == market.US {
		date = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	prices := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		q, err := b.provider.GetQuote(t)
		if err != nil {
			log.Printf("closing snapshot: quote %s: %v", t, err)
			continue
		}
		// On a market holiday the cron still fires but providers return the
		// previous session's quote; its timestamp is then a full day old,
		// and saving it would file old data under the wrong date.
		if time.Since(q.Timestamp) > 12*time.Hour {
			log.Printf("closing snapshot: %s quote is stale (%s), skipping (holiday?)", t, q.Timestamp.Format(time.RFC3339))
			continue
		}
		prices[t] = q.Price
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
			log.Printf("closing snapshot: save %s: %v", t, err)
		}
	}
	log.Printf("closing snapshot: done for %s market=%s (%d tickers)", date, m, len(tickers))

	b.snapshotBenchmark(date, m)
	b.recordNetWorthSnapshot(date, m, prices)
}

// snapshotBenchmark records benchmarkFor(m)'s (SPY/0050) closing price into
// daily_snapshots under the same date as the watchlist snapshot, so /track's
// relative-to-market hit rate (Phase 3.8) has same-day benchmark data to
// compare against without ever needing to replay history through a live API
// call. Same stale-quote guard as the per-ticker loop above. Silent on
// failure, same as the rest of this job — a missing benchmark row just makes
// /track fall back to its absolute-direction hit rule for that date.
func (b *Bot) snapshotBenchmark(date string, m market.MarketID) {
	ticker := benchmarkFor(m)
	q, err := b.provider.GetQuote(ticker)
	if err != nil {
		log.Printf("closing snapshot: benchmark %s: %v", ticker, err)
		return
	}
	if time.Since(q.Timestamp) > 12*time.Hour {
		log.Printf("closing snapshot: benchmark %s quote is stale (%s), skipping (holiday?)", ticker, q.Timestamp.Format(time.RFC3339))
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
	if err := b.db.SaveSnapshot(snap); err != nil {
		log.Printf("closing snapshot: save benchmark %s: %v", ticker, err)
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
		log.Printf("net worth snapshot: positions: %v", err)
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
		log.Printf("net worth snapshot: save: %v", err)
	}
}

// RunDailyReport fetches data, detects signals, generates LLM recommendations,
// and sends the daily report. Called by the scheduler.
//
// The cron behind this fires every day with no weekday/holiday
// restriction (unlike RunClosingSnapshot, which is Tue–Sat only) — on a US
// market holiday it would otherwise still run a full LLM analysis off
// whatever stale prior-session quotes the providers return and push a
// report implying that's today's price action. market.IsTradingDay checks
// that before anything else gets fetched; time.Now().In(cst) is safe to
// feed it directly rather than resolving a real US Eastern time first
// because this job only ever runs at the fixed 23:30 CST cron time — see
// IsTradingDay's own doc comment for why that specific hour makes Taiwan's
// date and the US trading date the same value.
func (b *Bot) RunDailyReport(ctx context.Context) {
	defer b.recoverJobPanic("daily report")

	if !market.IsTradingDay(time.Now().In(cst)) {
		b.Send(i18n.T(b.lang, i18n.KeyDailyReportMarketClosed))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyDailyReportStart))

	in, err := b.gatherRecommendationInputs()
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
			log.Printf("history %s: %v", t, err)
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
	positionList := positionsSlice(in.positions)
	b.checkStopLossAlerts(positionList, prices)
	b.checkTrailingStopAlerts(positionList, prices, atrs)
	b.checkTargetAlerts(positionList, prices)
	b.checkMA5BreakAlerts(positionList, prices, ma5s)

	explore := b.exploreCandidates(ctx, &in)

	summary, recs, err := b.llm.GenerateRecommendations(ctx, in.watchlist, in.candidates, in.marketNews, in.marketContext, in.recentLessons)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyNoRecommendationsToday))
		return
	}

	sources := recommendationSources(in.watchlistTickers, in.candidateTickers, in.scanHits, explore)
	b.sendAndSaveRecommendations(summary, recs, sources, in.watchlist, in.candidates)
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
		log.Printf("explore candidates: %v", err)
		return nil
	}

	var valid []string
	reasons := make(map[string]string, len(noms))
	for _, n := range noms {
		if !data.IsUSEquitySymbol(n.Ticker) {
			log.Printf("explore candidates: rejecting %q: not a plain US-equity symbol shape", n.Ticker)
			continue
		}
		if excludeSet[n.Ticker] {
			log.Printf("explore candidates: rejecting %s: already on an existing list", n.Ticker)
			continue
		}
		if _, err := b.provider.GetQuote(n.Ticker); err != nil {
			log.Printf("explore candidates: rejecting %s: quote failed: %v", n.Ticker, err)
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
		log.Printf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
	}
	sig, newRSI := b.detector.CheckRSIState(ticker, closes, prevRSI)
	if sig != nil {
		out = append(out, *sig)
	}
	if newRSI != prevRSI {
		if err := b.db.SetSignalState(ticker, signals.FamilyRSI, newRSI); err != nil {
			log.Printf("signal state %s/%s: %v", ticker, signals.FamilyRSI, err)
		}
	}

	prevMACD, err := b.db.GetSignalState(ticker, signals.FamilyMACD)
	if err != nil {
		log.Printf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
	}
	sig, newMACD := b.detector.CheckMACDCross(ticker, closes, prevMACD)
	if sig != nil {
		out = append(out, *sig)
	}
	if newMACD != prevMACD {
		if err := b.db.SetSignalState(ticker, signals.FamilyMACD, newMACD); err != nil {
			log.Printf("signal state %s/%s: %v", ticker, signals.FamilyMACD, err)
		}
	}

	// Strategy 1: Squeeze Breakout
	prevSqueeze, err := b.db.GetSignalState(ticker, signals.FamilyStrategySqueeze)
	if err != nil {
		log.Printf("signal state %s/%s: %v", ticker, signals.FamilyStrategySqueeze, err)
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
			log.Printf("signal state %s/%s: %v", ticker, signals.FamilyStrategySqueeze, err)
		}
	}

	// Strategy 2: Box Bottom Rebound
	prevBox, err := b.db.GetSignalState(ticker, signals.FamilyStrategyBox)
	if err != nil {
		log.Printf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBox, err)
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
			log.Printf("signal state %s/%s: %v", ticker, signals.FamilyStrategyBox, err)
		}
	}

	return out
}

// scanChunkCount and universeScanRequestDelay govern Phase 2.6's daily
// candidate-pool scan: the universe (~500 S&P 500 + manual tickers) is split
// into scanChunkCount rotating slices — matching the closing-snapshot cadence
// of Tue–Sat, 5 trading days/week — so a full pass covers roughly 100
// tickers/day. universeScanRequestDelay throttles Yahoo history requests
// within a chunk, per PLAN.md's explicit note not to hammer it.
const (
	scanChunkCount           = 5
	universeScanRequestDelay = 300 * time.Millisecond
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

// RunUniverseScan is Phase 2.6's chunked candidate-pool scan: it checks
// today's rotating slice of the universe (excluding anything already on the
// watchlist, which gets a full RSI/MACD check daily anyway) for a fresh
// RSI/MACD signal via the same checkStatefulSignals used for the watchlist —
// safe to share signal_states with it since the two ticker sets never
// overlap. Any hit is logged to scan_hits for RunDailyReport/handleRecommend
// to pick up the same day and upgrade into an LLM candidate. Silent
// background job like RunClosingSnapshot: results go to the DB/log, not
// Telegram — the eventual daily report is the user-facing surface.
func (b *Bot) RunUniverseScan(ctx context.Context) {
	defer b.recoverJobPanic("universe scan")

	entries, err := b.db.GetUniverse()
	if err != nil {
		log.Printf("universe scan: universe: %v", err)
		return
	}
	watchlist, err := b.db.GetWatchlist()
	if err != nil {
		log.Printf("universe scan: watchlist: %v", err)
		return
	}
	watchSet := make(map[string]bool, len(watchlist))
	for _, t := range watchlist {
		watchSet[t] = true
	}

	var tickers []string
	for _, e := range entries {
		if !watchSet[e.Ticker] {
			tickers = append(tickers, e.Ticker)
		}
	}

	mc := b.computeMarketRegime()
	isBear := isBearRegime(mc)

	chunk := universeScanChunk(tickers, scanChunkCount, time.Now().In(cst).YearDay())
	date := todayDate()
	hits := 0
	for i, t := range chunk {
		select {
		case <-ctx.Done():
			log.Printf("universe scan: cancelled after %d/%d tickers", i, len(chunk))
			return
		default:
		}

		candles, err := b.history.GetHistory(t, "1y")
		if err != nil {
			log.Printf("universe scan: history %s: %v", t, err)
			continue
		}
		for _, sig := range b.checkStatefulSignals(t, candles, isBear) {
			if err := b.db.SaveScanHit(t, date, sig.Message); err != nil {
				log.Printf("universe scan: save hit %s: %v", t, err)
				continue
			}
			hits++
		}

		if i < len(chunk)-1 {
			time.Sleep(universeScanRequestDelay)
		}
	}
	log.Printf("universe scan: checked %d tickers, %d hits", len(chunk), hits)
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
		days := daysUntil(e.Date)
		if days < 0 || days > earningsAlertDays {
			continue
		}

		prev, err := b.db.GetSignalState(t, earningsSignalFamily)
		if err != nil {
			log.Printf("earnings alert state %s: %v", t, err)
		}
		if prev == e.Date {
			continue
		}

		lines = append(lines, i18n.T(b.lang, i18n.KeyEarningsAlertLine, t, e.Date, days))
		if err := b.db.SetSignalState(t, earningsSignalFamily, e.Date); err != nil {
			log.Printf("earnings alert state %s: %v", t, err)
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
		log.Printf("quote %s: %v", ticker, err)
		return 0, false
	}
	return q.Price, true
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
	if adverseMovePct < thresholdPct {
		return false, false, ""
	}
	if prevState == breachedState {
		return true, false, breachedState
	}
	return true, true, breachedState
}

// stopBreachDecision is stopBreachAlertDecision's absolute-price sibling
// (Phase 3.11 PR1 §3.3): alert once when close first drops below stopPrice,
// stay silent on later calls while it remains below, and reset once it
// recovers back at-or-above stopPrice so a later re-breach alerts again.
// Deliberately not a call into breachAlertDecision with some
// price-to-percent conversion bolted on — that function's thresholdPct <= 0
// already means "disabled", a contract that has no meaning for an absolute
// price level and would just invite confusing the two call sites.
func stopBreachDecision(close, stopPrice float64, prevState string) (breached, shouldAlert bool, newState string) {
	if close >= stopPrice {
		return false, false, ""
	}
	if prevState == breachedState {
		return true, false, breachedState
	}
	return true, true, breachedState
}

// checkStopLossAlerts is Phase 3.11 PR1's two-tier stop-loss check (§3.3):
// a position with a per-trade stop_price set (via /stop) is checked against
// that absolute price (stopBreachDecision); one without falls back to the
// original global STOP_LOSS_PCT percentage check (b.stopLossPct, 0 disables
// it), unchanged from before this phase. A position only ever takes one of
// the two branches, so both safely share the same signal_states family
// (stopLossSignalFamily) without state collisions. Rule-based and
// independent of the LLM, so it still fires when every LLM provider is
// down. positions is expected sorted by ticker (see positionsSlice); prices
// is the current-price lookup built by the caller (see priceFor).
func (b *Bot) checkStopLossAlerts(positions []db.Position, prices map[string]float64) {
	var lines []string
	for _, p := range positions {
		price, ok := b.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}

		prev, err := b.db.GetSignalState(p.Ticker, stopLossSignalFamily)
		if err != nil {
			log.Printf("stop loss state %s: %v", p.Ticker, err)
		}

		if p.StopPrice > 0 {
			_, shouldAlert, newState := stopBreachDecision(price, p.StopPrice, prev)
			if newState != prev {
				if err := b.db.SetSignalState(p.Ticker, stopLossSignalFamily, newState); err != nil {
					log.Printf("stop loss state %s: %v", p.Ticker, err)
				}
			}
			if !shouldAlert {
				continue
			}
			lines = append(lines, i18n.T(b.lang, i18n.KeyStopPriceHit, p.Ticker, p.StopPrice, price))
			continue
		}

		if b.stopLossPct <= 0 {
			continue
		}
		lossPct := (p.AvgCost - price) / p.AvgCost * 100
		_, shouldAlert, newState := breachAlertDecision(lossPct, b.stopLossPct, prev)
		if newState != prev {
			if err := b.db.SetSignalState(p.Ticker, stopLossSignalFamily, newState); err != nil {
				log.Printf("stop loss state %s: %v", p.Ticker, err)
			}
		}
		if !shouldAlert {
			continue
		}
		lines = append(lines, i18n.T(b.lang, i18n.KeyStopLossAlertLine, p.Ticker, p.AvgCost, price, lossPct))
	}
	if len(lines) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyStopLossAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.Send(sb.String())
}

// trailingStopThreshold combines the fixed-percentage and ATR-based trailing-
// stop distances into a single threshold percentage (Phase 3.8 追加項, see
// docs/phase-3.8-atr-trailing-stop.md). atrMult <= 0 means the ATR-based check
// is disabled (the default), so the fixed percentage always wins in that case
// — this is what makes TRAILING_STOP_ATR_MULT=0 leave existing behavior
// byte-for-byte unchanged. When both are enabled and atr is available, the
// two are combined via min: the fixed percentage becomes a risk-budget
// ceiling ("no matter how volatile, tolerate at most this much drawdown") and
// the ATR-based distance tightens within it for lower-volatility tickers —
// see the design doc for why min (not a straight replacement, and not max).
// ok is false when neither distance is usable (fixed disabled and ATR either
// disabled or unavailable) — the caller should skip the check entirely rather
// than alert off a threshold of 0. atrBased tells the caller which i18n line
// to render.
func trailingStopThreshold(fixedPct, atrMult, atr, peak float64) (thresholdPct float64, atrBased, ok bool) {
	atrPct := 0.0
	atrOK := atrMult > 0 && atr > 0 && peak > 0
	if atrOK {
		atrPct = atrMult * atr / peak * 100
	}

	switch {
	case fixedPct > 0 && atrOK:
		if atrPct < fixedPct {
			return atrPct, true, true
		}
		return fixedPct, false, true
	case fixedPct > 0:
		return fixedPct, false, true
	case atrOK:
		return atrPct, true, true
	default:
		return 0, false, false
	}
}

// checkTrailingStopAlerts warns about any open position whose close-price
// drawdown from its post-first-buy peak has just breached the trailing-stop
// threshold (see trailingStopThreshold — either b.trailingStopPct alone, or
// combined with an ATR(14)-based distance when TRAILING_STOP_ATR_MULT > 0).
// The peak is computed on demand from daily_snapshots closes on or after the
// ticker's earliest recorded BUY date (db.GetEarliestBuyDate/GetPeakClose)
// rather than a separately maintained running-high column — a held ticker is
// always on the watchlist (via /buy's auto-add), so it already gets a daily
// closing snapshot. Skips (logs, no alert) a ticker with no BUY transaction
// or no snapshot history yet, rather than risk a false alarm off an unknown
// peak. atrs is the prefetched ticker->ATR14 map built by RunDailyReport from
// its watchlist StockData (same prefetch-with-fallback shape as prices — see
// priceFor); a ticker missing from atrs falls back to a direct
// b.computeTechnicals call, and if that also fails to yield an ATR, the
// ATR-based distance is simply unavailable for it (trailingStopThreshold's
// fixed-percentage-only branch, or a skip if that's disabled too). Same dedup
// shape as checkStopLossAlerts (see breachAlertDecision), under its own
// signal_states family so the two checks don't share state.
func (b *Bot) checkTrailingStopAlerts(positions []db.Position, prices map[string]float64, atrs map[string]float64) {
	if b.trailingStopPct <= 0 && b.trailingStopATRMult <= 0 {
		return
	}
	var lines []string
	for _, p := range positions {
		buyDate, ok, err := b.db.GetEarliestBuyDate(p.Ticker)
		if err != nil {
			log.Printf("trailing stop: earliest buy %s: %v", p.Ticker, err)
			continue
		}
		if !ok {
			continue
		}
		peak, ok, err := b.db.GetPeakClose(p.Ticker, buyDate)
		if err != nil {
			log.Printf("trailing stop: peak close %s: %v", p.Ticker, err)
			continue
		}
		if !ok || peak <= 0 {
			continue
		}
		price, ok := b.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		drawdownPct := (peak - price) / peak * 100

		atr, ok := atrs[p.Ticker]
		if !ok && b.trailingStopATRMult > 0 {
			if t, _, _ := b.computeTechnicals(p.Ticker, nil); t != nil {
				atr = t.ATR14
			}
		}
		thresholdPct, atrBased, ok := trailingStopThreshold(b.trailingStopPct, b.trailingStopATRMult, atr, peak)
		if !ok {
			log.Printf("trailing stop: no usable threshold for %s (fixed=%.2f atrMult=%.2f atr=%.2f)", p.Ticker, b.trailingStopPct, b.trailingStopATRMult, atr)
			continue
		}

		prev, err := b.db.GetSignalState(p.Ticker, trailingStopSignalFamily)
		if err != nil {
			log.Printf("trailing stop state %s: %v", p.Ticker, err)
		}
		_, shouldAlert, newState := breachAlertDecision(drawdownPct, thresholdPct, prev)
		if newState != prev {
			if err := b.db.SetSignalState(p.Ticker, trailingStopSignalFamily, newState); err != nil {
				log.Printf("trailing stop state %s: %v", p.Ticker, err)
			}
		}
		if !shouldAlert {
			continue
		}
		if atrBased {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrailingStopAlertLineATR, p.Ticker, peak, price, drawdownPct, thresholdPct, b.trailingStopATRMult))
		} else {
			lines = append(lines, i18n.T(b.lang, i18n.KeyTrailingStopAlertLine, p.Ticker, peak, price, drawdownPct))
		}
	}
	if len(lines) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyTrailingStopAlertTitle))
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.Send(sb.String())
}

// Phase 3.11 PR2 (§4 of docs/phase-3.11-trade-risk-management.md): rule-based
// exit alerts that build on PR1's per-trade stop_price, same daily-report-only
// asymmetry as every other check in this file. targetRMultiple/trailProfitPct
// are fixed package constants, not env-configurable — unlike STOP_LOSS_PCT/
// TRAILING_STOP_PCT these aren't independent risk tolerances the user tunes,
// they're the textbook 2R/10% figures the design doc's source material uses,
// and both new checks are inherently opt-in already (no stop set = no target;
// no meaningful profit yet = no 5MA check), so there's no "0 disables it"
// knob to wire up. targetSignalFamily/ma5TrailSignalFamily each get their own
// signal_states family so neither collides with stopLossSignalFamily/
// trailingStopSignalFamily or with each other.
const (
	targetRMultiple = 2.0
	trailProfitPct  = 10.0

	targetSignalFamily   = "target"
	ma5TrailSignalFamily = "ma5_trail"
	// hitState mirrors breachedState's role but under a name that reads
	// correctly for "target reached"/"5MA broken" rather than "breached" —
	// same "" = not currently hit convention.
	hitState = "hit"
)

// targetReachedDecision is the alert-once/reset decision for the 2R
// take-profit check (§4.1): the mirror image of stopBreachDecision — alerts
// the first time close reaches (or passes) targetPrice, stays quiet while it
// remains there, and resets once it falls back under so a later re-touch
// alerts again (accepted as symmetric with the stop-loss checks' own
// re-alert-after-recovery behavior, per the design doc).
func targetReachedDecision(close, targetPrice float64, prevState string) (reached, shouldAlert bool, newState string) {
	if close < targetPrice {
		return false, false, ""
	}
	if prevState == hitState {
		return true, false, hitState
	}
	return true, true, hitState
}

// checkTargetAlerts warns once when a position with a stop price set (§3.1's
// /stop) first closes at or above its 2R target (avgCost +
// targetRMultiple*(avgCost-stopPrice)) — "take half off, defend the rest with
// the 5MA" per the design doc's source material. A position without a stop
// price has no R to measure a target against, so it's skipped rather than
// improvising one off some other threshold (unlike checkStopLossAlerts, this
// check has no global-percentage fallback branch at all). A stop at or above
// cost (shouldn't happen — /stop validates against price, not cost, so a very
// unusual manual entry could still produce one) is also skipped, since the
// target formula would move backwards. positions/prices follow
// checkStopLossAlerts' own conventions (sorted slice, prefetch-with-fallback
// price lookup via priceFor).
func (b *Bot) checkTargetAlerts(positions []db.Position, prices map[string]float64) {
	var lines []string
	for _, p := range positions {
		if p.StopPrice <= 0 || p.StopPrice >= p.AvgCost {
			continue
		}
		price, ok := b.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		target := p.AvgCost + targetRMultiple*(p.AvgCost-p.StopPrice)

		prev, err := b.db.GetSignalState(p.Ticker, targetSignalFamily)
		if err != nil {
			log.Printf("target state %s: %v", p.Ticker, err)
		}
		_, shouldAlert, newState := targetReachedDecision(price, target, prev)
		if newState != prev {
			if err := b.db.SetSignalState(p.Ticker, targetSignalFamily, newState); err != nil {
				log.Printf("target state %s: %v", p.Ticker, err)
			}
		}
		if !shouldAlert {
			continue
		}
		lines = append(lines, i18n.T(b.lang, i18n.KeyTargetReached, p.Ticker, targetRMultiple, target, price))
	}
	if len(lines) == 0 {
		return
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
	}
	b.Send(sb.String())
}

// checkMA5BreakAlerts warns once when a position that's up at least
// trailProfitPct (10%) unrealized closes below its MA5 — "the strong-trend
// line just failed" per the design doc's source material ("魚身防守"). The
// profit gate is deliberate (see the design doc): without it this would fire
// on every ordinary pullback for a flat, long-held position, since MA5 is
// tight enough that price dips under it constantly in a non-trending stock.
// This is a companion to the ATR-based trailing stop, not a replacement —
// the ATR version manages deep drawdowns from any position, this one manages
// the cadence of a position that's already run hard. Reuses stopBreachDecision
// (identical "alert once when close first drops below threshold, reset on
// recovery" shape — MA5 is just another absolute-price threshold, same as a
// stop price) rather than a near-duplicate function. ma5s is a prefetched
// ticker->MA5 map (same prefetch-with-fallback shape as checkTrailingStopAlerts'
// atrs — a ticker missing from it falls back to one computeTechnicals call).
func (b *Bot) checkMA5BreakAlerts(positions []db.Position, prices map[string]float64, ma5s map[string]float64) {
	var lines []string
	for _, p := range positions {
		if p.AvgCost <= 0 {
			continue
		}
		price, ok := b.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		profitPct := (price - p.AvgCost) / p.AvgCost * 100
		if profitPct < trailProfitPct {
			continue
		}

		ma5, ok := ma5s[p.Ticker]
		if !ok {
			if t, _, _ := b.computeTechnicals(p.Ticker, nil); t != nil {
				ma5 = t.MA5
			}
		}
		if ma5 <= 0 {
			continue
		}

		prev, err := b.db.GetSignalState(p.Ticker, ma5TrailSignalFamily)
		if err != nil {
			log.Printf("ma5 trail state %s: %v", p.Ticker, err)
		}
		_, shouldAlert, newState := stopBreachDecision(price, ma5, prev)
		if newState != prev {
			if err := b.db.SetSignalState(p.Ticker, ma5TrailSignalFamily, newState); err != nil {
				log.Printf("ma5 trail state %s: %v", p.Ticker, err)
			}
		}
		if !shouldAlert {
			continue
		}
		lines = append(lines, i18n.T(b.lang, i18n.KeyMA5Break, p.Ticker, ma5, price))
	}
	if len(lines) == 0 {
		return
	}
	var sb strings.Builder
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
// weeklyNetWorthLine reports on market.US only for now — Phase 6 PR2 adds
// the TW counterpart (see docs/phase-6-tw-market.md §5.3); PR1 only threads
// the new per-market signature through without changing weekly-review
// behavior.
func (b *Bot) weeklyNetWorthLine(cash float64, haveCash bool) (string, error) {
	latestDateStr, latest, ok, err := b.db.GetLatestNetWorth(market.US)
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

	prior, ok, err := b.db.GetNetWorthOnOrBefore(weekAgo, market.US)
	if err != nil {
		return "", err
	}
	if !ok || prior == 0 {
		return "", nil
	}

	pctChange := (latest - prior) / prior * 100
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

	tickers := make([]string, len(positions))
	positionsMap := make(map[string]db.Position, len(positions))
	for i, p := range positions {
		tickers[i] = p.Ticker
		positionsMap[p.Ticker] = p
	}

	earnings := b.loadEarnings(tickers)
	stocks := b.fetchStockData(tickers, true, positionsMap, earnings, nil, nil, nil)

	theses := b.loadTheses(tickers)
	vsSPY := b.loadVsSPY(stocks, positionsMap)
	for i := range stocks {
		ticker := stocks[i].Quote.Ticker
		if th, ok := theses[ticker]; ok {
			stocks[i].Thesis = &th
		}
		if v, ok := vsSPY[ticker]; ok {
			stocks[i].VsSPY = &v
		}
	}

	cashUSD, haveCashUSD, err := b.loadCash(market.US)
	if err != nil {
		log.Printf("weekly review: load cash (USD): %v", err)
	}
	cashTWD, haveCashTWD, err := b.loadCash(market.TW)
	if err != nil {
		log.Printf("weekly review: load cash (TWD): %v", err)
	}

	var trackSummary string
	if rows, _, ok, err := b.computeTrackRows(7); err != nil {
		log.Printf("weekly review: track rows: %v", err)
	} else if ok {
		overall, bySource := summarizeTrack(rows)
		trackSummary = renderTrackSummary(b.lang, overall, bySource)
	}

	result, err := b.llm.WeeklyReview(ctx, stocks, cashUSD, haveCashUSD, cashTWD, haveCashTWD, trackSummary)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	var sb strings.Builder
	if line, err := b.weeklyNetWorthLine(cashUSD, haveCashUSD); err != nil {
		log.Printf("weekly review: net worth line: %v", err)
	} else if line != "" {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString(i18n.T(b.lang, i18n.KeyWeeklyReviewResultTitle, result))
	sb.WriteString(renderEarningsPreview(b.lang, earnings, 7))

	b.Send(sb.String())
}

// RunMonthlyReport is Phase 3.6 追加項's net-worth monthly report (see
// docs/phase-3.6-monthly-report.md): a deliberately non-LLM data archive for
// the prior full calendar month — deterministic (same DB contents always
// produce the same report) and unaffected by the LLM provider chain being
// down, unlike RunWeeklyReview's judgment-based prose. Sends nothing at all
// (log-only) when there's no net_worth_snapshots row anywhere in the month
// — a fresh install's first month has no series worth archiving; every
// other input is optional and just skips its own line instead (see the
// design doc's per-block degrade rules).
func (b *Bot) RunMonthlyReport(ctx context.Context) {
	defer b.recoverJobPanic("monthly report")

	from, to := monthRange(time.Now().In(cst))

	// market.US only for now — Phase 6 PR2 splits the monthly report into
	// per-market blocks (see docs/phase-6-tw-market.md §5.3).
	points, err := b.db.GetNetWorthRange(from, to, market.US)
	if err != nil {
		log.Printf("monthly report: net worth range: %v", err)
		return
	}
	if len(points) == 0 {
		log.Printf("monthly report: no net worth snapshots for %s..%s, skipping", from, to)
		return
	}

	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Total
	}
	latest := values[len(values)-1]

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTitle, from[:7]))
	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSparklineLine, sparkline(values)))

	// Monthly return convention is "prior month-end vs. this month-end" (not
	// this month's first row, which would miss the change on the very first
	// trading day of the month). Falls back to this month's own first value
	// when there's no prior-month baseline yet (e.g. the first month on
	// record); if that's the only point too, there's nothing to diff
	// against, so the line is skipped entirely.
	fromDate, _ := time.Parse("2006-01-02", from)
	priorMonthEnd := fromDate.AddDate(0, 0, -1).Format("2006-01-02")
	baseline, haveBaseline, err := b.db.GetNetWorthOnOrBefore(priorMonthEnd, market.US)
	if err != nil {
		log.Printf("monthly report: baseline net worth: %v", err)
	}
	if !haveBaseline && len(values) > 1 {
		baseline, haveBaseline = values[0], true
	}
	if haveBaseline && baseline != 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportChangeLine, latest, (latest-baseline)/baseline*100))
	}

	sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportDrawdownLine, maxDrawdownPct(values)))

	if count, sellCount, realized, err := b.db.GetTransactionStats(from, to); err != nil {
		log.Printf("monthly report: transaction stats: %v", err)
	} else {
		if sellCount > 0 {
			sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportRealizedLine, realized))
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportTxCountLine, count))
	}

	if first, last, ok, err := b.db.GetSnapshotCloseRange(benchmarkTicker, from, to); err != nil {
		log.Printf("monthly report: spy range: %v", err)
	} else if ok && first != 0 {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportSPYLine, (last-first)/first*100))
	}

	if cash, haveCash, err := b.loadCash(market.US); err != nil {
		log.Printf("monthly report: load cash: %v", err)
	} else if haveCash {
		sb.WriteString(i18n.T(b.lang, i18n.KeyMonthlyReportCashLine, latest+cash, cash))
	}

	b.Send(sb.String())
}

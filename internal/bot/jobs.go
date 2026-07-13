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

// RunClosingSnapshot records the just-closed US session's OHLCV for every
// watchlist ticker into daily_snapshots. Called by the scheduler at 05:30
// CST — after the US close — so unlike the daily report (which runs an
// hour or two into the session, not at the close) this captures genuine
// closing data. At that hour the US trading date is Taiwan's "yesterday",
// which is why the snapshot is dated one day back.
// It's a silent background job: results go to the DB and errors to the log,
// not to Telegram.
func (b *Bot) RunClosingSnapshot(ctx context.Context) {
	defer b.recoverJobPanic("closing snapshot")

	tickers, err := b.db.GetWatchlist()
	if err != nil {
		log.Printf("closing snapshot: watchlist: %v", err)
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	date := time.Now().In(cst).AddDate(0, 0, -1).Format("2006-01-02")
	prices := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		q, err := b.provider.GetQuote(t)
		if err != nil {
			log.Printf("closing snapshot: quote %s: %v", t, err)
			continue
		}
		// On a US market holiday the cron still fires but providers return
		// the previous session's quote; its timestamp is then a full day
		// old, and saving it would file old data under the wrong date.
		if time.Since(q.Timestamp) > 12*time.Hour {
			log.Printf("closing snapshot: %s quote is stale (%s), skipping (US holiday?)", t, q.Timestamp.Format(time.RFC3339))
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
	log.Printf("closing snapshot: done for %s (%d tickers)", date, len(tickers))

	b.snapshotBenchmark(date)
	b.recordNetWorthSnapshot(date, prices)
}

// snapshotBenchmark records benchmarkTicker's (SPY) closing price into
// daily_snapshots under the same date as the watchlist snapshot, so /track's
// relative-to-market hit rate (Phase 3.8) has same-day SPY data to compare
// against without ever needing to replay history through a live API call.
// Same stale-quote guard as the per-ticker loop above (a US holiday still
// fires the cron but returns the prior session's quote). Silent on failure,
// same as the rest of this job — a missing benchmark row just makes /track
// fall back to its absolute-direction hit rule for that date.
func (b *Bot) snapshotBenchmark(date string) {
	q, err := b.provider.GetQuote(benchmarkTicker)
	if err != nil {
		log.Printf("closing snapshot: benchmark %s: %v", benchmarkTicker, err)
		return
	}
	if time.Since(q.Timestamp) > 12*time.Hour {
		log.Printf("closing snapshot: benchmark %s quote is stale (%s), skipping (US holiday?)", benchmarkTicker, q.Timestamp.Format(time.RFC3339))
		return
	}
	snap := db.DailySnapshot{
		Ticker:        benchmarkTicker,
		Date:          date,
		Open:          q.Open,
		Close:         q.Price,
		High:          q.High,
		Low:           q.Low,
		Volume:        q.Volume,
		ChangePercent: q.ChangePercent,
	}
	if err := b.db.SaveSnapshot(snap); err != nil {
		log.Printf("closing snapshot: save benchmark %s: %v", benchmarkTicker, err)
	}
}

// recordNetWorthSnapshot totals every open position's value as of the
// closing snapshot and stores it dated the same day, so a net worth curve
// can be drawn later. prices reuses the quotes RunClosingSnapshot already
// fetched for watchlist tickers (positions are auto-added to the watchlist
// on /buy, so this covers the common case); any position ticker missing
// from it gets a direct quote fetch as a fallback (see priceFor).
func (b *Bot) recordNetWorthSnapshot(date string, prices map[string]float64) {
	positions, err := b.db.GetPositions()
	if err != nil {
		log.Printf("net worth snapshot: positions: %v", err)
		return
	}
	if len(positions) == 0 {
		return
	}

	var total float64
	for _, p := range positions {
		price, ok := b.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		total += p.Shares * price
	}
	if err := b.db.SaveNetWorthSnapshot(date, total); err != nil {
		log.Printf("net worth snapshot: save: %v", err)
	}
}

// RunDailyReport fetches data, detects signals, generates LLM recommendations,
// and sends the daily report. Called by the scheduler.
func (b *Bot) RunDailyReport(ctx context.Context) {
	defer b.recoverJobPanic("daily report")

	b.Send(i18n.T(b.lang, i18n.KeyDailyReportStart))

	tickers, err := b.db.GetWatchlist()
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyWatchlistQueryFailed, err))
		return
	}

	candidateTickers, err := b.provider.GetMarketMovers()
	if err != nil {
		log.Printf("market movers: %v", err)
	}
	scanHits := b.loadScanHits()
	dedupedCandidates := mergeCandidates(candidateTickers, scanHits, tickers)
	allTickers := append(append([]string{}, tickers...), dedupedCandidates...)

	positions := b.loadPositions()
	earnings := b.loadEarnings(allTickers)
	b.checkEarningsAlerts(tickers, earnings)
	marketNews := b.loadMarketNews()
	prevRecs := b.loadPrevRecs(allTickers)

	// Detect signals on watchlist
	var allSignals []signals.Signal
	watchlist := b.fetchStockData(tickers, true, positions, earnings, nil, prevRecs)
	prices := make(map[string]float64, len(watchlist))
	for _, s := range watchlist {
		if s.Quote != nil {
			prices[s.Quote.Ticker] = s.Quote.Price
			allSignals = append(allSignals, b.detector.CheckQuote(s.Quote)...)
		}
	}
	for _, t := range tickers {
		closes, err := b.history.GetHistory(t)
		if err != nil {
			log.Printf("history %s: %v", t, err)
			continue
		}
		allSignals = append(allSignals, b.checkStatefulSignals(t, closes)...)
	}
	if len(allSignals) > 0 {
		b.SendSignalAlert(allSignals)
	}

	// Exit-discipline checks (Phase 3.8): rule-based, independent of the LLM
	// call below, so a down LLM provider doesn't suppress them. Daily-report
	// only, by design — no intraday/at-price monitoring (see PLAN.md).
	positionList := positionsSlice(positions)
	b.checkStopLossAlerts(positionList, prices)
	b.checkTrailingStopAlerts(positionList, prices)

	candidates := b.fetchStockData(dedupedCandidates, false, positions, earnings, scanHits, prevRecs)

	summary, recs, err := b.llm.GenerateRecommendations(ctx, watchlist, candidates, marketNews)
	if err != nil {
		b.Send(i18n.T(b.lang, i18n.KeyLLMFailed, err))
		return
	}

	if len(recs) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyNoRecommendationsToday))
		return
	}

	sources := recommendationSources(tickers, dedupedCandidates, scanHits)
	b.sendAndSaveRecommendations(summary, recs, sources, watchlist, candidates)
}

// checkStatefulSignals runs the RSI/MACD checks that diff against the last
// state persisted in signal_states: RSI only alerts when it newly enters an
// extreme zone (no repeat alert while it stays there on consecutive days),
// and MACD only alerts on an actual golden/death cross rather than every day
// a trend holds. A failed state read falls back to "" — worst case one
// duplicate alert, better than dropping the check entirely.
func (b *Bot) checkStatefulSignals(ticker string, closes []float64) []signals.Signal {
	var out []signals.Signal

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

		closes, err := b.history.GetHistory(t)
		if err != nil {
			log.Printf("universe scan: history %s: %v", t, err)
			continue
		}
		for _, sig := range b.checkStatefulSignals(t, closes) {
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

// checkStopLossAlerts warns about any open position whose unrealized loss
// has just breached STOP_LOSS_PCT (b.stopLossPct, 0 disables the check
// entirely). Rule-based and independent of the LLM, so it still fires when
// every LLM provider is down. positions is expected sorted by ticker (see
// positionsSlice); prices is the current-price lookup built by the caller
// (see priceFor).
func (b *Bot) checkStopLossAlerts(positions []db.Position, prices map[string]float64) {
	if b.stopLossPct <= 0 {
		return
	}
	var lines []string
	for _, p := range positions {
		price, ok := b.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		lossPct := (p.AvgCost - price) / p.AvgCost * 100

		prev, err := b.db.GetSignalState(p.Ticker, stopLossSignalFamily)
		if err != nil {
			log.Printf("stop loss state %s: %v", p.Ticker, err)
		}
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

// checkTrailingStopAlerts warns about any open position whose close-price
// drawdown from its post-first-buy peak has just breached TRAILING_STOP_PCT
// (b.trailingStopPct, 0 disables the check). The peak is computed on demand
// from daily_snapshots closes on or after the ticker's earliest recorded BUY
// date (db.GetEarliestBuyDate/GetPeakClose) rather than a separately
// maintained running-high column — a held ticker is always on the watchlist
// (via /buy's auto-add), so it already gets a daily closing snapshot. Skips
// (logs, no alert) a ticker with no BUY transaction or no snapshot history
// yet, rather than risk a false alarm off an unknown peak. Same dedup shape
// as checkStopLossAlerts (see breachAlertDecision), under its own
// signal_states family so the two checks don't share state.
func (b *Bot) checkTrailingStopAlerts(positions []db.Position, prices map[string]float64) {
	if b.trailingStopPct <= 0 {
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

		prev, err := b.db.GetSignalState(p.Ticker, trailingStopSignalFamily)
		if err != nil {
			log.Printf("trailing stop state %s: %v", p.Ticker, err)
		}
		_, shouldAlert, newState := breachAlertDecision(drawdownPct, b.trailingStopPct, prev)
		if newState != prev {
			if err := b.db.SetSignalState(p.Ticker, trailingStopSignalFamily, newState); err != nil {
				log.Printf("trailing stop state %s: %v", p.Ticker, err)
			}
		}
		if !shouldAlert {
			continue
		}
		lines = append(lines, i18n.T(b.lang, i18n.KeyTrailingStopAlertLine, p.Ticker, peak, price, drawdownPct))
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

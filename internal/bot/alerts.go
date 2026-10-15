package bot

import (
	"sort"
	"strings"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/notification"
	"argus/internal/service"
	"argus/internal/signals"
)

const (
	stopLossSignalFamily     = "stop_loss"
	trailingStopSignalFamily = "trailing_stop"
	// breachedState is the signal_states value recorded while a stop-loss/
	// trailing-stop threshold stays breached; any other value (including "",
	// the unset default) means "not currently breached".
	breachedState = "breached"
)

const (
	targetRMultiple = service.TargetRMultiple
	trailProfitPct  = service.TrailProfitPct

	targetSignalFamily   = service.TargetSignalFamily
	ma5TrailSignalFamily = service.MA5TrailSignalFamily
	hitState             = service.HitState
)

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

// checkStatefulSignals is a thin wrapper around service.ScanService's
// CheckStatefulSignals (Phase 24 Stage 1 Scan & Strategy Service extraction)
// that adds the strategy-hit caveats the service owns the text of — see
// service.DecorateStrategyHits for what they are and why they are applied
// there rather than at render time.
func (b *Bot) checkStatefulSignals(ticker string, candles []data.Candle, isBearRegime bool) []signals.Signal {
	scans := b.scans()
	sigs := service.DecorateStrategyHits(scans.CheckStatefulSignals(ticker, candles), isBearRegime, b.lang)
	scans.RecordStrategyAlerts(sigs, candles, db.AlertChannelWatchlist)
	return sigs
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

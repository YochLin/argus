package bot

import (
	"context"
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/notification"
	"argus/internal/service"
	"argus/internal/signals"
)

// runMomentumWatch pushes the momentum-breakout watch (signals.TypeMomentumSetup):
// S&P 500 names whose base is complete, split into those that broke over it
// so far today and those still waiting. It rides the end of the US daily
// report (23:30 CST, an hour into the session) rather than its own cron
// entry, so it inherits that job's trading-day guard; the ~500 paced history
// fetches put the message several minutes after the report. Each pick is
// recorded as a strategy alert, so the chart can show what was pushed.
func (b *Bot) runMomentumWatch(ctx context.Context) {
	defer b.recoverJobPanic("momentum watch")
	if b.db == nil || b.history == nil {
		return
	}
	universe, err := b.db.GetUniverse()
	if err != nil {
		b.publishAlert("momentum_watch", notification.LevelWarning, i18n.T(b.lang, i18n.KeyMomentumFailed, err))
		return
	}
	var tickers []string
	for _, u := range universe {
		if u.Source == "sp500" {
			tickers = append(tickers, u.Ticker)
		}
	}
	now := b.now()
	w, err := service.ScanMomentumBreakout(ctx, b.history, b.provider, b.earnings, tickers, now, service.MomentumRequestDelay)
	if err != nil {
		b.publishAlert("momentum_watch", notification.LevelWarning, i18n.T(b.lang, i18n.KeyMomentumFailed, err))
		return
	}
	date := now.In(cst).Format("2006-01-02")
	text, lines := formatMomentumWatch(b.lang, date, w)
	for _, p := range w.Picks {
		typ := signals.TypeMomentumSetup
		if p.Broke {
			typ = signals.TypeMomentumBreakout
		}
		if err := b.db.SaveStrategyAlert(p.Ticker, typ, date, db.AlertChannelMomentum, lines[p.Ticker]); err != nil {
			logger.Errorf("momentum watch: record %s: %v", p.Ticker, err)
		}
	}
	b.publishAlert("momentum_watch", notification.LevelInfo, text)
}

// formatMomentumWatch is the pushed message, plus each pick's own line (what
// gets recorded with its strategy alert).
func formatMomentumWatch(lang i18n.Lang, date string, w *service.MomentumWatch) (string, map[string]string) {
	r := w.Regime
	parts := []string{i18n.T(lang, i18n.KeyMomentumHeader, date)}
	if !r.On {
		parts = append(parts, i18n.T(lang, i18n.KeyMomentumRegimeOff, r.MA10, r.MA10Prev, r.MA20, r.MA20Prev))
		return strings.Join(parts, "\n"), nil
	}
	parts = append(parts, i18n.T(lang, i18n.KeyMomentumRegimeOn, r.MA10, r.MA10Prev, r.MA20, r.MA20Prev))
	if w.EarningsUnknown {
		parts = append(parts, i18n.T(lang, i18n.KeyMomentumEarningsUnknown))
	}
	lines := make(map[string]string, len(w.Picks))
	var broke, setup []string
	for _, p := range w.Picks {
		q := p.Quote
		if p.Broke {
			open, wide := "", ""
			if p.OpenBreak() {
				open = i18n.T(lang, i18n.KeyMomentumOpenBreak)
			}
			if p.WideStop() {
				wide = i18n.T(lang, i18n.KeyMomentumWideStop)
			}
			lines[p.Ticker] = i18n.T(lang, i18n.KeyMomentumBroke, p.Ticker, q.Price, p.Pivot, q.Open, open, q.Low, p.StopPct, p.ADRPct, wide)
			broke = append(broke, lines[p.Ticker])
			continue
		}
		gap := (p.Pivot - q.Price) / p.Pivot * 100
		lines[p.Ticker] = i18n.T(lang, i18n.KeyMomentumSetup, p.Ticker, p.Pivot, q.Price, gap, p.ADRPct, p.Mom3mPct, p.BaseDays)
		setup = append(setup, lines[p.Ticker])
	}
	if len(w.Picks) == 0 {
		parts = append(parts, "", i18n.T(lang, i18n.KeyMomentumNone))
	}
	if len(broke) > 0 {
		parts = append(parts, "", i18n.T(lang, i18n.KeyMomentumBrokeTitle, len(broke)))
		parts = append(parts, broke...)
	}
	if len(setup) > 0 {
		parts = append(parts, "", i18n.T(lang, i18n.KeyMomentumSetupTitle, len(setup)))
		parts = append(parts, setup...)
	}
	parts = append(parts, "", i18n.T(lang, i18n.KeyMomentumUnvalidated))
	return strings.Join(parts, "\n"), lines
}

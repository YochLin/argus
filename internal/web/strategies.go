package web

import (
	"time"

	"argus/internal/logger"
)

// Phase 27 P5 — the strategy signals on a ticker's chart. These are the
// pushes that were actually sent (strategy_alerts, written by
// service.ScanService.RecordStrategyAlerts), not a replay of the rules over
// history, so a marker means "this fired and you were told". Nothing here
// decides anything: the frontend names the strategy and its validation state
// from Type; Message and the verdict are stored text, passed through.

// verdictWindowDays is how far after a signal's date to look for the LLM's
// call on it. A watchlist signal is raised inside the daily report, so its
// recommendation carries the same date; a scan hit is promoted to a candidate
// by the next report, and a weekend or holiday can sit in between.
const verdictWindowDays = 4

type strategyVerdict struct {
	Date   string `json:"date"`   // the recommendation's date
	Action string `json:"action"` // BUY | SELL | HOLD
	Reason string `json:"reason"`
}

type strategyAlertResponse struct {
	Type    string `json:"type"`    // signals.Type* ("strategy_squeeze_breakout", ...)
	Date    string `json:"date"`    // the candle the alert was computed on
	Channel string `json:"channel"` // watchlist | scan
	Message string `json:"message"` // the text that was pushed
	// Verdict is the first recommendation for this ticker within
	// verdictWindowDays of Date; null when the LLM never weighed in.
	Verdict *strategyVerdict `json:"verdict"`
}

// buildStrategyAlerts returns ticker's recorded alerts, oldest first. A read
// failure only logs: the chart is still worth showing without its markers.
func buildStrategyAlerts(database dbReader, ticker string) []strategyAlertResponse {
	out := []strategyAlertResponse{}
	if database == nil {
		return out
	}
	alerts, err := database.StrategyAlertsFor(ticker)
	if err != nil {
		logger.Errorf("web: strategy alerts for %s: %v", ticker, err)
		return out
	}
	if len(alerts) == 0 {
		return out
	}

	// One bounded read covers every alert: they come back ordered by date.
	last, err := time.Parse("2006-01-02", alerts[len(alerts)-1].SignalDate)
	if err != nil {
		last = time.Now()
	}
	recs, err := database.GetRecommendationsForTicker(ticker, alerts[0].SignalDate, last.AddDate(0, 0, verdictWindowDays).Format("2006-01-02"))
	if err != nil {
		logger.Errorf("web: strategy alert verdicts for %s: %v", ticker, err)
	}

	for _, a := range alerts {
		item := strategyAlertResponse{Type: a.Strategy, Date: a.SignalDate, Channel: a.Channel, Message: a.Message}
		end := a.SignalDate
		if t, err := time.Parse("2006-01-02", a.SignalDate); err == nil {
			end = t.AddDate(0, 0, verdictWindowDays).Format("2006-01-02")
		}
		for _, r := range recs { // oldest first, so the first match is the nearest
			if r.Date >= a.SignalDate && r.Date <= end && r.Action != "" {
				item.Verdict = &strategyVerdict{Date: r.Date, Action: r.Action, Reason: r.Reason}
				break
			}
		}
		out = append(out, item)
	}
	return out
}

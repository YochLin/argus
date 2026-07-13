package bot

import (
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
)

// formatChatContext renders the read-only background block prefixed to
// chat messages: each ticker's most recent close plus, for tickers actually
// held, cost basis and unrealized P&L against that close. tickers is the
// order to render in; positions/snapshots are keyed by ticker. Returns ""
// for an empty tickers list so callers can skip prefixing entirely.
func formatChatContext(lang i18n.Lang, tickers []string, positions map[string]db.Position, snapshots map[string]db.DailySnapshot) string {
	if len(tickers) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyChatContextHeader))
	for _, t := range tickers {
		snap, ok := snapshots[t]
		if !ok {
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextTickerNoData, t))
			continue
		}
		if p, held := positions[t]; held {
			unrealizedPct := (snap.Close - p.AvgCost) / p.AvgCost * 100
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextPositionLine, t, snap.Date, snap.Close, snap.ChangePercent, p.Shares, p.AvgCost, unrealizedPct))
		} else {
			sb.WriteString(i18n.T(lang, i18n.KeyChatContextWatchLine, t, snap.Date, snap.Close, snap.ChangePercent))
		}
	}
	sb.WriteString(i18n.T(lang, i18n.KeyChatContextFooter))
	return sb.String()
}

// daysUntil returns the whole number of days from today (Taiwan time) until
// dateStr (YYYY-MM-DD), which may be negative for a past date. Both sides
// are compared as date-only values (not instants) so it's not sensitive to
// what time of day it's called.
func daysUntil(dateStr string) int {
	target, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	today, _ := time.Parse("2006-01-02", time.Now().In(cst).Format("2006-01-02"))
	return int(target.Sub(today).Hours() / 24)
}

func formatQuote(lang i18n.Lang, q *data.Quote) string {
	arrow := "▲"
	if q.ChangePercent < 0 {
		arrow = "▼"
	}
	return i18n.T(lang, i18n.KeyQuoteLine, q.Ticker, q.Price, arrow, q.ChangePercent, q.Open, q.High, q.Low)
}

func todayDate() string {
	return time.Now().In(cst).Format("2006-01-02")
}

// dedup returns tickers in a that are not present in b.
func dedup(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, t := range b {
		set[t] = true
	}
	var out []string
	for _, t := range a {
		if !set[t] {
			out = append(out, t)
		}
	}
	return out
}

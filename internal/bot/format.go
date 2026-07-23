package bot

import (
	"sort"
	"strings"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
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

// renderTrackSummary formats the hit-rate/avg-return/by-source/by-market
// breakdown — shared by /track's own display (handleTrack) and
// RunWeeklyReview's strategy-feedback block, which additionally asks the
// model to comment on it. Returns "" when nothing's been evaluated yet (no
// BUY/SELL row with a resolvable price), so callers can skip the block
// entirely rather than show an empty summary. byMarket (Phase 6 PR2 §5.3)
// mirrors bySource's own "only show the breakdown when there's more than one
// group" gate — a single-market user (the common case pre-Phase-6, and any
// TW-only or US-only holder) never sees a one-row market breakdown.
func renderTrackSummary(lang i18n.Lang, overall trackSourceStats, bySource map[string]trackSourceStats, byMarket map[market.MarketID]trackSourceStats) string {
	if overall.Evaluated == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyTrackSummary, overall.Hits, overall.Evaluated, overall.HitRate()))
	sb.WriteString(i18n.T(lang, i18n.KeyTrackAvgReturnLine, overall.AvgBuyPct(), overall.BuyCount, overall.AvgSellPct(), overall.SellCount))
	if len(bySource) > 1 {
		sb.WriteString(i18n.T(lang, i18n.KeyTrackBySourceHeader))
		for _, source := range sortedSourceKeys(bySource) {
			s := bySource[source]
			sb.WriteString(i18n.T(lang, i18n.KeyTrackBySourceLine, source, s.Hits, s.Evaluated, s.HitRate()))
		}
	}
	if len(byMarket) > 1 {
		sb.WriteString(i18n.T(lang, i18n.KeyTrackByMarketHeader))
		for _, m := range sortedMarketKeys(byMarket) {
			s := byMarket[m]
			sb.WriteString(i18n.T(lang, i18n.KeyTrackByMarketLine, string(m), s.Hits, s.Evaluated, s.HitRate()))
		}
	}
	return sb.String()
}

// sortedMarketKeys returns byMarket's keys in stable order (US before TW),
// mirroring sortedSourceKeys' role for the by-source breakdown.
func sortedMarketKeys(byMarket map[market.MarketID]trackSourceStats) []market.MarketID {
	keys := make([]market.MarketID, 0, len(byMarket))
	for k := range byMarket {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// sparklineChars are sparkline's 8 Unicode block levels, low to high.
var sparklineChars = []rune("▁▂▃▄▅▆▇█")

// sparkline renders values (e.g. a month's daily net-worth totals) as a
// single line of block characters via min-max normalization — Phase 3.6
// 追加項's monthly report (see docs/phase-3.6-monthly-report.md) deliberately
// doesn't pull in a charting dependency; a monospace Telegram line already
// conveys the month's shape. Returns "" for an empty slice. A flat series
// (max == min, which includes the single-point case) renders every
// character at the middle level rather than dividing by zero.
func sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	runes := make([]rune, len(values))
	if max == min {
		mid := sparklineChars[len(sparklineChars)/2]
		for i := range runes {
			runes[i] = mid
		}
		return string(runes)
	}
	for i, v := range values {
		idx := int((v - min) / (max - min) * float64(len(sparklineChars)-1))
		runes[i] = sparklineChars[idx]
	}
	return string(runes)
}

// maxDrawdownPct returns the largest peak-to-trough decline within values,
// as a positive percentage — 0 for fewer than 2 points or a series that
// never dips below its running high. Tracks a running peak and keeps the
// worst drawdown seen from it at any later point, rather than just
// comparing the first and last values (which would miss a mid-month dip
// that had already recovered by month-end).
func maxDrawdownPct(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	peak := values[0]
	var maxDD float64
	for _, v := range values[1:] {
		if v > peak {
			peak = v
			continue
		}
		if peak == 0 {
			continue
		}
		if dd := (peak - v) / peak * 100; dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

// monthRange returns the [from, to] date-string bounds (YYYY-MM-DD,
// inclusive) of the full calendar month immediately before now's month —
// RunMonthlyReport's "last complete month" window. AddDate's own calendar
// arithmetic handles the January-rolls-back-to-December-of-the-prior-year
// case with no special-casing needed.
func monthRange(now time.Time) (from, to string) {
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	firstOfLastMonth := firstOfThisMonth.AddDate(0, -1, 0)
	lastOfLastMonth := firstOfThisMonth.AddDate(0, 0, -1)
	return firstOfLastMonth.Format("2006-01-02"), lastOfLastMonth.Format("2006-01-02")
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

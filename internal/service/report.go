package service

import (
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
)

// sparklineChars are Sparkline's 8 Unicode block levels, low to high.
var sparklineChars = []rune("▁▂▃▄▅▆▇█")

// Sparkline renders values (e.g. a month's daily net-worth totals) as a
// single line of block characters via min-max normalization — Phase 3.6
// 追加項's monthly report (see docs/phase-3.6-monthly-report.md) deliberately
// doesn't pull in a charting dependency; a monospace Telegram line already
// conveys the month's shape. Returns "" for an empty slice. A flat series
// (max == min, which includes the single-point case) renders every
// character at the middle level rather than dividing by zero.
func Sparkline(values []float64) string {
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

// MaxDrawdownPct returns the largest peak-to-trough decline within values,
// as a positive percentage — 0 for fewer than 2 points or a series that
// never dips below its running high. Tracks a running peak and keeps the
// worst drawdown seen from it at any later point, rather than just
// comparing the first and last values (which would miss a mid-month dip
// that had already recovered by month-end).
func MaxDrawdownPct(values []float64) float64 {
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

// MonthRange returns the [from, to] date-string bounds (YYYY-MM-DD,
// inclusive) of the full calendar month immediately before now's month —
// RunMonthlyReport's "last complete month" window. AddDate's own calendar
// arithmetic handles the January-rolls-back-to-December-of-the-prior-year
// case with no special-casing needed.
func MonthRange(now time.Time) (from, to string) {
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	firstOfLastMonth := firstOfThisMonth.AddDate(0, -1, 0)
	lastOfLastMonth := firstOfThisMonth.AddDate(0, 0, -1)
	return firstOfLastMonth.Format("2006-01-02"), lastOfLastMonth.Format("2006-01-02")
}

// MonthlyReportStore is the persistence boundary BuildMonthlyReportBlock
// needs — narrowed to the reads it performs, same convention as
// RiskStore/SnapshotStore/ATMIVStore.
type MonthlyReportStore interface {
	GetNetWorthRange(from, to string, m market.MarketID) ([]db.NetWorthPoint, error)
	GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error)
	GetTransactionStatsByMarket(from, to string, m market.MarketID) (count, sellCount int, realized float64, err error)
	GetSnapshotCloseRange(ticker, from, to string) (first, last float64, ok bool, err error)
}

// MonthlyReportBlock is one market's monthly-report numeric content —
// BuildMonthlyReportBlock's pure output. Every field past Values/Latest is
// independently optional (Have* guards it) rather than 0-sentinel, matching
// this job's existing "查無資料就跳過該行" convention (see jobs.go's
// buildMonthlyReportBlock, which renders each Have* field as its own
// i18n line) — the business logic and the per-language text stay in
// separate places, same split every other Phase 24 service extraction uses.
type MonthlyReportBlock struct {
	Values        []float64
	Latest        float64
	HaveChange    bool
	ChangePct     float64
	DrawdownPct   float64
	TxCount       int
	SellCount     int
	Realized      float64
	HaveBenchmark bool
	BenchmarkPct  float64
	Cash          float64
	HaveCash      bool
}

// BuildMonthlyReportBlock gathers m's net-worth history for [from, to] (a
// full calendar month, see MonthRange) plus derived stats — month-end
// change, max drawdown, realized P&L, transaction count, benchmark move.
// ok is false when m has no net_worth_snapshots row anywhere in [from, to],
// meaning the whole block should be skipped rather than shown empty. cash/
// haveCash are passed in rather than fetched here since the cash setting
// lives behind bot's cashSettingKeyFor, not a market-scoped DB query this
// service otherwise needs.
func BuildMonthlyReportBlock(store MonthlyReportStore, m market.MarketID, from, to string, cash float64, haveCash bool) (MonthlyReportBlock, bool) {
	points, err := store.GetNetWorthRange(from, to, m)
	if err != nil {
		logger.Errorf("monthly report: net worth range (%s): %v", m, err)
		return MonthlyReportBlock{}, false
	}
	if len(points) == 0 {
		return MonthlyReportBlock{}, false
	}

	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Total
	}
	latest := values[len(values)-1]
	block := MonthlyReportBlock{Values: values, Latest: latest, Cash: cash, HaveCash: haveCash}

	// Monthly return convention is "prior month-end vs. this month-end" (not
	// this month's first row, which would miss the change on the very first
	// trading day of the month). Falls back to this month's own first value
	// when there's no prior-month baseline yet (e.g. the first month on
	// record); if that's the only point too, there's nothing to diff
	// against, so the line is skipped entirely.
	fromDate, _ := time.Parse("2006-01-02", from)
	priorMonthEnd := fromDate.AddDate(0, 0, -1).Format("2006-01-02")
	baseline, haveBaseline, err := store.GetNetWorthOnOrBefore(priorMonthEnd, m)
	if err != nil {
		logger.Errorf("monthly report: baseline net worth (%s): %v", m, err)
	}
	if !haveBaseline && len(values) > 1 {
		baseline, haveBaseline = values[0], true
	}
	if haveBaseline && baseline != 0 {
		block.HaveChange = true
		block.ChangePct = (latest - baseline) / baseline * 100
	}

	block.DrawdownPct = MaxDrawdownPct(values)

	if count, sellCount, realized, err := store.GetTransactionStatsByMarket(from, to, m); err != nil {
		logger.Errorf("monthly report: transaction stats (%s): %v", m, err)
	} else {
		block.TxCount, block.SellCount, block.Realized = count, sellCount, realized
	}

	if first, last, ok, err := store.GetSnapshotCloseRange(BenchmarkFor(m), from, to); err != nil {
		logger.Errorf("monthly report: benchmark range (%s): %v", m, err)
	} else if ok && first != 0 {
		block.HaveBenchmark = true
		block.BenchmarkPct = (last - first) / first * 100
	}

	return block, true
}

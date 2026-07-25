package web

import (
	"sort"
	"time"

	"argus/internal/market"
)

// YearlyPnL is one calendar year's row in the monthly P&L grid — a fund
// factsheet-style year x month table (Phase 8 PR3, see
// docs/phase-8-trader-analytics.md §5.1). Months[i] is nil for a month with
// no data day at all (not 0) — a real $0 month and "no data yet" must render
// differently (an empty cell, not a "$0" one), same "explicit null over a
// misleading zero" convention as dashboardResponse's period-return fields.
type YearlyPnL struct {
	Year   int        `json:"year"`
	Months []*float64 `json:"months"` // index 0 = January ... 11 = December
	Total  float64    `json:"total"`
}

type monthlyResponse struct {
	Years []YearlyPnL `json:"years"`
}

// buildMonthly assembles /api/monthly: pnl.go's DailyPnL replay engine (the
// same source the calendar heatmap reads) aggregated to month granularity.
// Deliberately amount-based, not a % of net_worth_snapshots — a mid-period
// buy/deposit would pollute a % figure (docs/phase-8-trader-analytics.md §1
// decision 4); DailyPnL's own same-day fill-price correction already keeps
// the dollar figure honest, and this is exactly the same replay dashboard's
// cumulative curve and the calendar heatmap already use, just summed at a
// coarser grain — so a month's total here always matches summing that
// month's calendar cells by hand.
func buildMonthly(database dbReader, m market.MarketID) (monthlyResponse, error) {
	allTxs, err := database.GetAllTransactions()
	if err != nil {
		return monthlyResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)
	if len(txs) == 0 {
		return monthlyResponse{}, nil
	}

	tickerSet := make(map[string]bool, len(txs))
	for _, t := range txs {
		tickerSet[t.Ticker] = true
	}
	tickers := make([]string, 0, len(tickerSet))
	for t := range tickerSet {
		tickers = append(tickers, t)
	}
	from := txs[0].Date // GetAllTransactions is date-ordered; the first row is the earliest.
	to := time.Now().Format("2006-01-02")

	snapshots, err := database.GetDailySnapshotsForTickers(tickers, from, to)
	if err != nil {
		return monthlyResponse{}, err
	}
	daily := DailyPnL(txs, snapshots)

	type yearMonth struct {
		year  int
		month int
	}
	sums := make(map[yearMonth]float64)
	for _, d := range daily {
		t, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			continue
		}
		sums[yearMonth{t.Year(), int(t.Month())}] += d.Value
	}

	years := make(map[int]*YearlyPnL)
	var yearOrder []int
	for ym, total := range sums {
		yp, ok := years[ym.year]
		if !ok {
			yp = &YearlyPnL{Year: ym.year, Months: make([]*float64, 12)}
			years[ym.year] = yp
			yearOrder = append(yearOrder, ym.year)
		}
		v := total
		yp.Months[ym.month-1] = &v
		yp.Total += total
	}
	sort.Ints(yearOrder)

	resp := monthlyResponse{Years: make([]YearlyPnL, 0, len(yearOrder))}
	for _, y := range yearOrder {
		resp.Years = append(resp.Years, *years[y])
	}
	return resp, nil
}

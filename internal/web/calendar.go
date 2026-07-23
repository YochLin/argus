package web

import (
	"time"

	"argus/internal/market"
)

// monthBounds parses a "YYYY-MM" month string into its first/last calendar
// day (as "YYYY-MM-DD" strings, matching daily_snapshots/transactions'
// date format). ok is false for an unparseable month, letting the caller
// fall back to the current month rather than erroring the whole request.
func monthBounds(month string) (start, end string, ok bool) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", "", false
	}
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02"), true
}

// buildCalendar assembles the /api/calendar response for one month: the
// subset of the DailyPnL replay engine's output (pnl.go — same engine PR1's
// dashboard curve uses) falling within that month, plus the month's raw
// transactions for the click-a-day detail panel. Week/month summary rows
// (design doc's A3) are deliberately not computed here — they're just a sum
// over Days, cheap enough to leave to the frontend rather than opening a
// second endpoint for it.
func buildCalendar(database dbReader, month string, m market.MarketID) (calendarResponse, error) {
	monthStart, monthEnd, ok := monthBounds(month)
	if !ok {
		monthStart, monthEnd, _ = monthBounds(time.Now().Format("2006-01"))
	}
	// Re-derive month from the resolved bounds rather than trusting the
	// caller's (possibly malformed) input back into the response.
	month = monthStart[:7]

	resp := calendarResponse{
		Month:        month,
		Days:         []DateValue{},
		Transactions: []transactionResponse{},
	}

	allTxs, err := database.GetAllTransactions()
	if err != nil {
		return calendarResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)
	if len(txs) == 0 {
		return resp, nil
	}

	for _, t := range txs {
		if t.Date >= monthStart && t.Date <= monthEnd {
			resp.Transactions = append(resp.Transactions, transactionResponse{
				Date:        t.Date,
				Ticker:      t.Ticker,
				Side:        t.Side,
				Shares:      t.Shares,
				Price:       t.Price,
				Fee:         t.Fee,
				RealizedPnL: t.RealizedPnL,
			})
		}
	}

	// DailyPnL needs the full history from the first-ever transaction (for
	// correct running share balances) through this month's end — clamped to
	// today, since daily_snapshots has nothing beyond the most recent close
	// — same "from"/"to" shape buildDashboard uses. txs is date-ordered
	// (GetAllTransactions), so txs[0].Date is the earliest.
	today := time.Now().Format("2006-01-02")
	to := monthEnd
	if today < to {
		to = today
	}
	from := txs[0].Date
	if from > to {
		// Requested month is entirely before the first trade (or entirely
		// in the future) — nothing to compute, but the month's own
		// transactions (if any) above already stand.
		return resp, nil
	}

	tickerSet := make(map[string]bool, len(txs))
	for _, t := range txs {
		tickerSet[t.Ticker] = true
	}
	tickers := make([]string, 0, len(tickerSet))
	for t := range tickerSet {
		tickers = append(tickers, t)
	}

	snapshots, err := database.GetDailySnapshotsForTickers(tickers, from, to)
	if err != nil {
		return calendarResponse{}, err
	}

	for _, d := range DailyPnL(txs, snapshots) {
		if d.Date >= monthStart && d.Date <= monthEnd {
			resp.Days = append(resp.Days, d)
		}
	}

	return resp, nil
}

package web

import (
	"sort"

	"argus/internal/db"
)

// closeOnOrBefore returns the value at the rightmost key in sorted (ascending)
// dates that is <= date, or ok=false if every date in dates is after it —
// the "most recent available close, carrying forward over a gap" lookup
// docs/phase-8-trader-analytics.md §4.2 specifies for both a trade's pricing
// and DailyPnL's own snapshot-gap handling.
func closeOnOrBefore(dates []string, closes map[string]float64, date string) (float64, bool) {
	idx := sort.Search(len(dates), func(i int) bool { return dates[i] > date })
	if idx == 0 {
		return 0, false
	}
	return closes[dates[idx-1]], true
}

// applyRealShares updates the running real-share balance for a transaction's
// ticker — the denominator BenchmarkReplay needs to turn a partial SELL into
// the matching proportion of the benchmark holding to liquidate.
func applyRealShares(realShares map[string]float64, t db.Transaction) {
	switch t.Side {
	case "BUY":
		realShares[t.Ticker] += t.Shares
	case "SELL":
		realShares[t.Ticker] -= t.Shares
	}
}

// BenchmarkReplay mirrors every real trade in txs into a same-cash-flow
// benchmark trade priced off benchCloses (date -> benchmark close), producing
// a cumulative "P&L amount" curve directly overlayable on CumulativeCurve's
// own output (Phase 8 PR2, docs/phase-8-trader-analytics.md §4.2 — "同金流
// 買 SPY/0050 模擬", not a normalized-% overlay). For each ticker: every BUY
// invests shares*price+fee into benchmark shares at that day's (or the most
// recent prior available) benchmark close, held under that ticker's own
// name; every SELL liquidates the same *proportion* of that ticker's
// benchmark holding that the real trade liquidated of the real position,
// crediting proceeds at the same close-price lookup. The curve's value at
// each date is today's benchmark holdings market value, plus cumulative
// sell proceeds, minus cumulative amount invested — the same "cumulative
// P&L" semantics CumulativeCurve returns.
//
// txs must be date-ordered (db.GetAllTransactions's own order — balances are
// carried forward day to day). Output dates are exactly benchCloses' own
// keys: a trade dated before the earliest benchmark date is skipped (its
// real-share balance still gets tracked, so a later partial sell's
// proportion stays correct) — the design doc's accepted gap that the
// benchmark curve can only be drawn from the day daily_snapshots' SPY/0050
// history actually begins, never earlier.
func BenchmarkReplay(txs []db.Transaction, benchCloses map[string]float64) []DateValue {
	if len(benchCloses) == 0 {
		return nil
	}
	dates := make([]string, 0, len(benchCloses))
	for d := range benchCloses {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	firstDate := dates[0]

	benchShares := make(map[string]float64) // ticker -> benchmark shares held under that ticker's name
	realShares := make(map[string]float64)  // ticker -> real shares held, for sell-proportion math
	var invested, proceeds float64

	ti := 0
	out := make([]DateValue, 0, len(dates))
	for _, date := range dates {
		for ti < len(txs) && txs[ti].Date <= date {
			t := txs[ti]
			ti++
			before := realShares[t.Ticker]
			applyRealShares(realShares, t)

			if t.Date < firstDate {
				continue
			}
			price, ok := closeOnOrBefore(dates, benchCloses, t.Date)
			if !ok || price == 0 {
				continue
			}
			switch t.Side {
			case "BUY":
				amount := t.Shares*t.Price + t.Fee
				benchShares[t.Ticker] += amount / price
				invested += amount
			case "SELL":
				if before <= 0 {
					continue
				}
				frac := t.Shares / before
				if frac > 1 {
					frac = 1
				}
				sold := benchShares[t.Ticker] * frac
				proceeds += sold * price
				benchShares[t.Ticker] -= sold
			}
		}

		var holdings float64
		for _, sh := range benchShares {
			holdings += sh * benchCloses[date]
		}
		out = append(out, DateValue{Date: date, Value: holdings + proceeds - invested})
	}
	return out
}

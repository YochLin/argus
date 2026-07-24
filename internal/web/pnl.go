// Package web implements Argus's read-only dashboard (Phase 5 PR1, see
// docs/phase-5-web-dashboard.md): an in-process HTTP server, gated by the
// WEB_ADDR env var, serving a JSON API plus an embedded Vite/React SPA.
package web

import (
	"sort"
	"time"

	"argus/internal/db"
)

// DateValue is one point in a date-ordered time series (YYYY-MM-DD, a
// dollar amount) — the shape both the daily P&L series and its cumulative
// running total use, and the shape future PRs' calendar/month-summary
// endpoints will want too. JSON tags matter here: the API response embeds
// this type directly (dashboardResponse.Curve) rather than a separate DTO.
type DateValue struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// tickerBalance is one checkpoint in a ticker's running share-balance
// history: Shares is the balance immediately after every transaction dated
// on or before Date has been applied.
type tickerBalance struct {
	Date   string
	Shares float64
}

// replayBalances walks txs (must be date-ordered — db.GetAllTransactions
// already returns them that way) into one running-balance checkpoint list
// per ticker. This is the building block openingShares uses to answer "how
// many shares were held coming into day D" without re-walking full history
// for every date queried by DailyPnL.
func replayBalances(txs []db.Transaction) map[string][]tickerBalance {
	balances := make(map[string]float64)
	out := make(map[string][]tickerBalance)

	i := 0
	for i < len(txs) {
		date := txs[i].Date
		j := i
		for j < len(txs) && txs[j].Date == date {
			t := txs[j]
			switch t.Side {
			case "BUY":
				balances[t.Ticker] += t.Shares
			case "SELL":
				balances[t.Ticker] -= t.Shares
			}
			j++
		}
		// One checkpoint per ticker touched on this date, reflecting the
		// balance after all of that date's transactions — a ticker bought
		// and sold same-day only needs its net end-of-day balance recorded
		// once, not one checkpoint per transaction.
		touched := make(map[string]bool)
		for k := i; k < j; k++ {
			touched[txs[k].Ticker] = true
		}
		for ticker := range touched {
			out[ticker] = append(out[ticker], tickerBalance{Date: date, Shares: balances[ticker]})
		}
		i = j
	}
	return out
}

// openingShares returns how many shares were held immediately before any
// of date's own transactions are applied — the last checkpoint strictly
// before date, or 0 if there were no transactions before then.
// checkpoints must be date-ordered (replayBalances produces them that way).
func openingShares(checkpoints []tickerBalance, date string) float64 {
	var shares float64
	for _, c := range checkpoints {
		if c.Date >= date {
			break
		}
		shares = c.Shares
	}
	return shares
}

// DailyPnL computes each trading day's P&L (realized + unrealized) from
// the mark-to-market delta of shares already held coming into the day,
// plus a same-day correction for shares actually bought or sold that day —
// see docs/phase-5-web-dashboard.md's "每日損益" section for the base
// formula (the sell-side correction: a sale transacts at its real fill
// price, not that day's close). The buy-side correction is a symmetric
// addition beyond the doc's literal wording: without it, a position opened
// today contributes exactly 0 to today's P&L (openingShares is 0 on the
// entry day), silently dropping the buy-price-to-close gain/loss from the
// cumulative curve forever.
//
// Each ticker's mark-to-market delta is computed against *that ticker's
// own* previous available close, not a global "yesterday" — deliberately,
// so one ticker's gap (see docs/phase-5-web-dashboard.md's "watchlist gap"
// limitation: manually /remove'd while still held, then re-added) doesn't
// distort another ticker's delta just because it happens to land on a date
// where some other ticker in the portfolio does have a snapshot. The
// output date axis is still the sorted union of every date present in
// snapshots (a day with only one of several held tickers reporting still
// shows up, with the other tickers simply contributing 0 that day).
func DailyPnL(txs []db.Transaction, snapshots []db.DailySnapshot) []DateValue {
	balances := replayBalances(txs)

	closes := make(map[string]map[string]float64) // ticker -> date -> close
	tickerDates := make(map[string][]string)      // ticker -> sorted dates with a close
	dateSet := make(map[string]bool)
	for _, s := range snapshots {
		if closes[s.Ticker] == nil {
			closes[s.Ticker] = make(map[string]float64)
		}
		closes[s.Ticker][s.Date] = s.Close
		dateSet[s.Date] = true
	}
	if len(dateSet) == 0 {
		return nil
	}
	for ticker, byDate := range closes {
		ds := make([]string, 0, len(byDate))
		for d := range byDate {
			ds = append(ds, d)
		}
		sort.Strings(ds)
		tickerDates[ticker] = ds
	}

	pnlByDate := make(map[string]float64)

	// Mark-to-market: walk each ticker's own consecutive available closes,
	// crediting the delta to the later date.
	for ticker, ds := range tickerDates {
		checkpoints := balances[ticker]
		for k := 1; k < len(ds); k++ {
			prevDate, currDate := ds[k-1], ds[k]
			opening := openingShares(checkpoints, currDate)
			if opening == 0 {
				continue
			}
			pnlByDate[currDate] += (closes[ticker][currDate] - closes[ticker][prevDate]) * opening
		}
	}

	// Same-day buy/sell corrections, only where that day's close is known
	// (a transaction dated a day with no snapshot — shouldn't normally
	// happen — simply can't be corrected and is skipped).
	for _, t := range txs {
		closeToday, ok := closes[t.Ticker][t.Date]
		if !ok {
			continue
		}
		switch t.Side {
		case "SELL":
			pnlByDate[t.Date] += (t.Price - closeToday) * t.Shares
		case "BUY":
			pnlByDate[t.Date] += (closeToday - t.Price) * t.Shares
		}
	}

	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	out := make([]DateValue, len(dates))
	for i, date := range dates {
		out[i] = DateValue{Date: date, Value: pnlByDate[date]}
	}
	return out
}

// CumulativeCurve returns the running total of daily (the shape the
// dashboard's main P&L curve and the Net P&L / Max Drawdown KPIs both
// read from — "KPI 數字與圖形互相印證" in the design doc's own words).
func CumulativeCurve(daily []DateValue) []DateValue {
	out := make([]DateValue, len(daily))
	var running float64
	for i, d := range daily {
		running += d.Value
		out[i] = DateValue{Date: d.Date, Value: running}
	}
	return out
}

// MaxDrawdownAbs returns the largest peak-to-trough decline (in dollars)
// within a cumulative P&L curve, tracking a running peak the same way
// bot.maxDrawdownPct does for net worth — but returns an absolute amount
// rather than a percentage, since a P&L curve (unlike net worth) can cross
// zero, where a percentage off a near-zero or negative peak is meaningless.
func MaxDrawdownAbs(cumulative []DateValue) float64 {
	if len(cumulative) < 2 {
		return 0
	}
	peak := cumulative[0].Value
	var maxDD float64
	for _, v := range cumulative[1:] {
		if v.Value > peak {
			peak = v.Value
			continue
		}
		if dd := peak - v.Value; dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

// FilterSells returns just the SELL rows of txs, in order — the KPI
// functions below all sample "every SELL transaction" per
// docs/phase-5-web-dashboard.md's statistical-unit decision (not FIFO
// lots, not whole trade rounds).
func FilterSells(txs []db.Transaction) []db.Transaction {
	var sells []db.Transaction
	for _, t := range txs {
		if t.Side == "SELL" {
			sells = append(sells, t)
		}
	}
	return sells
}

// WinRate returns the fraction (0–1) of sells with positive realized P&L.
// Returns 0 for no sells.
func WinRate(sells []db.Transaction) float64 {
	if len(sells) == 0 {
		return 0
	}
	var wins int
	for _, s := range sells {
		if s.RealizedPnL > 0 {
			wins++
		}
	}
	return float64(wins) / float64(len(sells))
}

// ProfitFactor is gross profit over gross loss across every sell (>1 means
// winners outweigh losers in dollar terms). Returns 0 when there are no
// losses to divide by — an all-winners history has no meaningful ratio,
// not an infinite one.
func ProfitFactor(sells []db.Transaction) float64 {
	var grossProfit, grossLoss float64
	for _, s := range sells {
		if s.RealizedPnL > 0 {
			grossProfit += s.RealizedPnL
		} else {
			grossLoss += -s.RealizedPnL
		}
	}
	if grossLoss == 0 {
		return 0
	}
	return grossProfit / grossLoss
}

// Expectancy is the average realized P&L per sell. Returns 0 for no sells.
func Expectancy(sells []db.Transaction) float64 {
	if len(sells) == 0 {
		return 0
	}
	var total float64
	for _, s := range sells {
		total += s.RealizedPnL
	}
	return total / float64(len(sells))
}

// YTDStart/QTDStart/HTDStart return the start date ("YYYY-MM-DD") of the
// calendar period now falls in — year, quarter, and half-year respectively.
// HTD's halves are Jan 1 (H1) and Jul 1 (H2). These take now as a parameter
// (rather than calling time.Now() themselves) purely so tests can pin down
// period-boundary edge cases (year/quarter/half rollovers) deterministically
// — buildDashboard still calls time.Now() itself, same un-injected-clock
// convention it already uses for its "to" cutoff.
func YTDStart(now time.Time) string {
	return time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
}

func QTDStart(now time.Time) string {
	quarterStartMonth := time.Month((int(now.Month()-1)/3)*3 + 1)
	return time.Date(now.Year(), quarterStartMonth, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
}

func HTDStart(now time.Time) string {
	half := time.January
	if now.Month() >= time.July {
		half = time.July
	}
	return time.Date(now.Year(), half, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
}

// curveValueBefore returns the value of the last curve point dated strictly
// before date, or 0 if there's no such point (the whole curve postdates
// date — i.e. the account's trading history only starts partway into the
// period being measured, so the period's P&L is simply the curve's full
// value with no earlier point to subtract).
func curveValueBefore(curve []DateValue, date string) float64 {
	var v float64
	for _, c := range curve {
		if c.Date >= date {
			break
		}
		v = c.Value
	}
	return v
}

// PeriodReturnPct computes a period's cash-flow-neutral return % — period
// P&L (curve's value at periodStart's boundary subtracted from its latest
// value) divided by baseline (net worth at the start of the period). This is
// deliberately not a raw net-worth diff: docs/phase-5-web-dashboard.md
// already rejected that approach for the daily P&L calendar because a
// deposit/buy on the first day of the period would be miscounted as profit
// — curve (from DailyPnL/CumulativeCurve) already carries that same-day
// fill-price correction, so it's the correct numerator here too.
// ok=false when there's no usable baseline (haveBaseline is false, or
// baseline is exactly 0 — division would be meaningless) — callers must
// treat that as "can't compute," never render a misleading 0%.
func PeriodReturnPct(curve []DateValue, periodStart string, baseline float64, haveBaseline bool) (pct float64, ok bool) {
	if !haveBaseline || baseline == 0 {
		return 0, false
	}
	var endValue float64
	if len(curve) > 0 {
		endValue = curve[len(curve)-1].Value
	}
	periodPnL := endValue - curveValueBefore(curve, periodStart)
	return periodPnL / baseline * 100, true
}

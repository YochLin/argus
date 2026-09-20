package service

import (
	"sort"
	"time"

	"argus/internal/db"
)

// DateValue is one point in a date-ordered time series (YYYY-MM-DD, a dollar amount).
type DateValue struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type tickerBalance struct {
	Date   string
	Shares float64
}

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

// DailyPnL computes each trading day's P&L (realized + unrealized) from the
// mark-to-market delta of shares already held coming into the day, plus a same-day
// correction for shares bought or sold that day.
func DailyPnL(txs []db.Transaction, snapshots []db.DailySnapshot) []DateValue {
	balances := replayBalances(txs)

	closes := make(map[string]map[string]float64)
	tickerDates := make(map[string][]string)
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

// CumulativeCurve returns the running total of daily P&L.
func CumulativeCurve(daily []DateValue) []DateValue {
	out := make([]DateValue, len(daily))
	var running float64
	for i, d := range daily {
		running += d.Value
		out[i] = DateValue{Date: d.Date, Value: running}
	}
	return out
}

// MaxDrawdownAbs returns the largest peak-to-trough decline (in dollars) within a cumulative P&L curve.
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

// DrawdownSeries returns curve's underwater series (curve[i].Value minus running peak).
func DrawdownSeries(curve []DateValue) []DateValue {
	if len(curve) == 0 {
		return nil
	}
	out := make([]DateValue, len(curve))
	peak := curve[0].Value
	for i, c := range curve {
		if c.Value > peak {
			peak = c.Value
		}
		out[i] = DateValue{Date: c.Date, Value: c.Value - peak}
	}
	return out
}

// FilterSells returns just the SELL rows of txs in order.
func FilterSells(txs []db.Transaction) []db.Transaction {
	var sells []db.Transaction
	for _, t := range txs {
		if t.Side == "SELL" {
			sells = append(sells, t)
		}
	}
	return sells
}

// WinRate returns the fraction (0-1) of sells with positive realized P&L.
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

// ProfitFactor is gross profit over gross loss across every sell.
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

// Expectancy is the average realized P&L per sell.
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

// YTDStart returns the start date ("YYYY-MM-DD") of the calendar year now falls in.
func YTDStart(now time.Time) string {
	return time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
}

// QTDStart returns the start date ("YYYY-MM-DD") of the calendar quarter now falls in.
func QTDStart(now time.Time) string {
	quarterStartMonth := time.Month((int(now.Month()-1)/3)*3 + 1)
	return time.Date(now.Year(), quarterStartMonth, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
}

// HTDStart returns the start date ("YYYY-MM-DD") of the calendar half-year now falls in.
func HTDStart(now time.Time) string {
	half := time.January
	if now.Month() >= time.July {
		half = time.July
	}
	return time.Date(now.Year(), half, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
}

// CurveValueBefore returns the value of the last curve point dated strictly before date.
func CurveValueBefore(curve []DateValue, date string) float64 {
	var v float64
	for _, c := range curve {
		if c.Date >= date {
			break
		}
		v = c.Value
	}
	return v
}

// CurvePeriodReturnPct computes a period's cash-flow-neutral return %.
func CurvePeriodReturnPct(curve []DateValue, periodStart string, baseline float64, haveBaseline bool) (pct float64, ok bool) {
	if !haveBaseline || baseline == 0 {
		return 0, false
	}
	var endValue float64
	if len(curve) > 0 {
		endValue = curve[len(curve)-1].Value
	}
	periodPnL := endValue - CurveValueBefore(curve, periodStart)
	return periodPnL / baseline * 100, true
}

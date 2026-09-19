package service

import (
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

// DailyCostBasis re-derives, for every date a round's legs touch, the
// weighted-average cost basis in effect immediately after that date's
// transactions.
func DailyCostBasis(legs []db.Transaction) map[string]float64 {
	out := make(map[string]float64)
	var shares, cost float64
	for _, l := range legs {
		switch l.Side {
		case "BUY":
			cost = (shares*cost + l.Shares*l.Price + l.Fee) / (shares + l.Shares)
			shares += l.Shares
		case "SELL":
			shares -= l.Shares
		}
		out[l.Date] = cost
	}
	return out
}

// CostBasisOnOrBefore returns the latest checkpoint dated on or before date.
// dates must be sorted ascending.
func CostBasisOnOrBefore(dates []string, checkpoints map[string]float64, date string) (float64, bool) {
	var cost float64
	found := false
	for _, d := range dates {
		if d > date {
			break
		}
		cost = checkpoints[d]
		found = true
	}
	return cost, found
}

// MAEMFEResult is one round's approximate MAE/MFE (Max Adverse / Favorable Excursion).
type MAEMFEResult struct {
	MAEPct float64 // deepest drawdown from cost basis (%) — 0 if no data
	MFEPct float64 // highest run-up from cost basis (%) — 0 if no data
	OK     bool
}

// ComputeMAEMFE scans candles restricted to [windowStart, windowEnd] against legs' own daily cost basis.
func ComputeMAEMFE(candles []data.Candle, legs []db.Transaction, windowStart, windowEnd time.Time) MAEMFEResult {
	checkpoints := DailyCostBasis(legs)
	dates := make([]string, 0, len(checkpoints))
	for d := range checkpoints {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	if len(dates) == 0 {
		return MAEMFEResult{}
	}

	var res MAEMFEResult
	for _, c := range candles {
		if c.Date.Before(windowStart) || c.Date.After(windowEnd) {
			continue
		}
		dateStr := c.Date.Format("2006-01-02")
		cost, found := CostBasisOnOrBefore(dates, checkpoints, dateStr)
		if !found || cost <= 0 {
			continue
		}
		lowPct := (c.Low - cost) / cost * 100
		highPct := (c.High - cost) / cost * 100
		if !res.OK || lowPct < res.MAEPct {
			res.MAEPct = lowPct
		}
		if !res.OK || highPct > res.MFEPct {
			res.MFEPct = highPct
		}
		res.OK = true
	}
	return res
}

// RoundReturnPct is a closed round's overall realized return %: total realized P&L over total invested.
func RoundReturnPct(legs []db.Transaction) (pct float64, ok bool) {
	var invested, realized float64
	for _, l := range legs {
		switch l.Side {
		case "BUY":
			invested += l.Price*l.Shares + l.Fee
		case "SELL":
			realized += l.RealizedPnL
		}
	}
	if invested <= 0 {
		return 0, false
	}
	return realized / invested * 100, true
}

// RoundMAEMFE computes single-round MAE/MFE restricted to the round's [start, end-or-now] window.
func RoundMAEMFE(candles []data.Candle, legs []db.Transaction, start, end string, now time.Time) MAEMFEResult {
	windowStart, err := time.Parse("2006-01-02", start)
	if err != nil {
		return MAEMFEResult{}
	}
	windowEnd := now
	if end != "" {
		if t, err := time.Parse("2006-01-02", end); err == nil {
			windowEnd = t
		}
	}
	return ComputeMAEMFE(candles, legs, windowStart, windowEnd)
}

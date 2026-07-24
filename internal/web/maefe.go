package web

import (
	"log"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

// dailyCostBasis re-derives, for every date a round's legs touch, the
// weighted-average cost basis in effect immediately after that date's
// transactions — the same formula db.RecordBuy uses for positions.avg_cost
// ((existingShares*existingCost + shares*price + fee) / totalShares),
// applied from a clean slate since a round always starts at 0 shares (no
// cross-round carry-over to worry about, unlike the live positions table).
// A SELL leg leaves cost basis unchanged (db.RecordSell never touches
// avg_cost) but still decrements the running share count, since that count
// is the weighting denominator for the *next* BUY in the round.
func dailyCostBasis(legs []db.Transaction) map[string]float64 {
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

// costBasisOnOrBefore returns the latest checkpoint dated on or before date
// — "on" (not strictly before) because a BUY's own day should already
// reflect that day's new cost basis. dates must be sorted ascending.
func costBasisOnOrBefore(dates []string, checkpoints map[string]float64, date string) (float64, bool) {
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

// maeMFEResult is one round's approximate MAE/MFE — see
// docs/phase-5-web-dashboard.md §A2. Percentages are relative to that day's
// weighted-average cost basis (not the round's final/entry cost alone), so
// a round entered in more than one BUY isn't distorted by pricing every day
// against only the first fill.
type maeMFEResult struct {
	MAEPct float64 // most negative % (deepest drawdown from cost basis) — 0 if no data
	MFEPct float64 // most positive % (highest run-up from cost basis) — 0 if no data
	OK     bool
}

// computeMAEMFE scans candles restricted to [windowStart, windowEnd]
// (inclusive, calendar dates) against legs' own daily cost basis. Candles
// outside the round's actual holding window (e.g. the padded context
// buildRoundDetail fetches for chart display) must be pre-filtered by the
// caller — this function does the date-range filtering itself so both
// call sites (a single round's detail page, and the report page's
// aggregate over every closed round) share the exact same window logic.
func computeMAEMFE(candles []data.Candle, legs []db.Transaction, windowStart, windowEnd time.Time) maeMFEResult {
	checkpoints := dailyCostBasis(legs)
	dates := make([]string, 0, len(checkpoints))
	for d := range checkpoints {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	if len(dates) == 0 {
		return maeMFEResult{}
	}

	var res maeMFEResult
	for _, c := range candles {
		if c.Date.Before(windowStart) || c.Date.After(windowEnd) {
			continue
		}
		dateStr := c.Date.Format("2006-01-02")
		cost, found := costBasisOnOrBefore(dates, checkpoints, dateStr)
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

// roundReturnPct is a closed round's overall realized return: total
// realized P&L over total invested (Σ buy.Price*buy.Shares+buy.Fee across
// the round's BUY legs — the same weighted-average-cost dollar total
// dailyCostBasis's running `cost*shares` represents at the point of the
// last BUY, recomputed directly here since dailyCostBasis is keyed by date
// rather than "final total"). Only meaningful for a fully closed round —
// an open round's realized P&L so far understates what the position will
// eventually return, so callers must not call this for round.EndDate == "".
func roundReturnPct(legs []db.Transaction) (pct float64, ok bool) {
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

// roundMAEMFE is buildRoundDetail's single-round MAE/MFE, computed from the
// candles it already fetched (no extra history call) restricted to the
// round's own [start, end-or-now] window — deliberately narrower than the
// ±15-day padded window buildRoundDetail fetches for chart display context,
// since MAE/MFE is specifically about the holding period itself, not the
// lead-in/lead-out the chart shows for visual context.
func roundMAEMFE(candles []data.Candle, legs []db.Transaction, start, end string, now time.Time) maeMFEResult {
	startT, err := time.Parse("2006-01-02", start)
	if err != nil {
		return maeMFEResult{}
	}
	endT := now
	if end != "" {
		endT, err = time.Parse("2006-01-02", end)
		if err != nil {
			return maeMFEResult{}
		}
	}
	return computeMAEMFE(candles, legs, startT, endT)
}

// MAEMFESummary is the report page's aggregate over every closed round in
// the market — design doc §A2's "平均只實現了 MFE 的 X%" (an average
// "captured" ratio, not a "given back" one — CapturedPct is realized return
// over MFE, so 100 means the round's exit captured the entire favorable
// excursion and 0 means none of it did).
type MAEMFESummary struct {
	AvgCapturedPct float64 `json:"avgCapturedPct"`
	N              int     `json:"n"` // rounds included in the average
	LowSample      bool    `json:"lowSample"`
}

// buildMAEMFESummary computes the aggregate over every closed round found
// in txs, fetching each distinct ticker's history at most once (cached
// locally) rather than once per round — a ticker with several closed
// rounds only costs one Yahoo call. A ticker whose history fetch fails is
// logged and its rounds simply don't contribute to the average (same
// attach-what's-available degrade convention as internal/bot's
// fetchStockData), not a whole-response failure — the report's other
// sections are still useful without this one ticker's contribution.
func buildMAEMFESummary(txs []db.Transaction, history data.HistoryProvider) MAEMFESummary {
	byTicker := make(map[string][]db.Transaction)
	for _, t := range txs {
		byTicker[t.Ticker] = append(byTicker[t.Ticker], t)
	}

	historyCache := make(map[string][]data.Candle)

	var totalCaptured float64
	var n int
	for ticker, tickerTxs := range byTicker {
		for _, r := range segmentRounds(tickerTxs) {
			if r.EndDate == "" {
				continue // open rounds have no final realized return to compare against MFE
			}
			retPct, ok := roundReturnPct(r.Legs)
			if !ok {
				continue
			}

			candles, cached := historyCache[ticker]
			if !cached {
				var err error
				candles, err = history.GetHistory(ticker, "max")
				if err != nil {
					log.Printf("web: maemfe: get history for %s: %v", ticker, err)
					candles = nil
				}
				historyCache[ticker] = candles
			}
			if candles == nil {
				continue
			}

			startT, err := time.Parse("2006-01-02", r.StartDate)
			if err != nil {
				continue
			}
			endT, err := time.Parse("2006-01-02", r.EndDate)
			if err != nil {
				continue
			}
			mm := computeMAEMFE(candles, r.Legs, startT, endT)
			if !mm.OK || mm.MFEPct <= 0 {
				continue // no meaningful favorable excursion to compare the realized return against
			}
			totalCaptured += retPct / mm.MFEPct * 100
			n++
		}
	}

	summary := MAEMFESummary{N: n, LowSample: n < minSampleSize}
	if n > 0 {
		summary.AvgCapturedPct = totalCaptured / float64(n)
	}
	return summary
}

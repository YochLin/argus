package web

import (
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

type maeMFEResult = service.MAEMFEResult

var (
	dailyCostBasis      = service.DailyCostBasis
	costBasisOnOrBefore = service.CostBasisOnOrBefore
	computeMAEMFE       = service.ComputeMAEMFE
	roundReturnPct      = service.RoundReturnPct
	roundMAEMFE         = service.RoundMAEMFE
)

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
					logger.Errorf("web: maemfe: get history for %s: %v", ticker, err)
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

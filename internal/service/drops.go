package service

import (
	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// dropBenchmarkRange is enough for any drop on the 2y chart.
const dropBenchmarkRange = "2y"

// BenchmarkCloses maps date to the close of ticker's market benchmark, nil if
// it can't be had: a drop still stands without its market comparison. candles
// are ticker's own, reused when ticker is the benchmark.
func BenchmarkCloses(history data.HistoryProvider, ticker string, candles []data.Candle) (string, map[string]float64) {
	benchmark := BenchmarkFor(market.Of(ticker))
	bars := candles
	if ticker != benchmark {
		var err error
		if bars, err = history.GetHistory(benchmark, dropBenchmarkRange); err != nil {
			logger.Errorf("drops %s: benchmark %s unavailable: %v", ticker, benchmark, err)
			return benchmark, nil
		}
	}
	closes := make(map[string]float64, len(bars))
	for _, c := range bars {
		closes[c.Date.Format("2006-01-02")] = c.Close
	}
	return benchmark, closes
}

// DropIndexPct is the benchmark's move over d: from the last close before it
// fell to its last bar. nil when closes has no bar for either end.
func DropIndexPct(closes map[string]float64, candles []data.Candle, d signals.DropEvent) *float64 {
	from, ok1 := closes[candles[d.Start-1].Date.Format("2006-01-02")]
	to, ok2 := closes[candles[d.End].Date.Format("2006-01-02")]
	if !ok1 || !ok2 || from <= 0 {
		return nil
	}
	pct := (to/from - 1) * 100
	return &pct
}

// DropOn returns the drop still running on date: one whose last bar is the
// last candle, dated date. nil on a day that added nothing to one, which
// includes a market holiday (no candle for date).
func DropOn(candles []data.Candle, date string) *signals.DropEvent {
	n := len(candles)
	if n == 0 || candles[n-1].Date.Format("2006-01-02") != date {
		return nil
	}
	drops := signals.DetectDrops(candles)
	if len(drops) == 0 || drops[len(drops)-1].End != n-1 {
		return nil
	}
	return &drops[len(drops)-1]
}

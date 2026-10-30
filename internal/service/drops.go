package service

import (
	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// dropBenchmarkRange is enough for any drop on the 2y chart.
const dropBenchmarkRange = "2y"

// DropIndexFor is the index a drop is compared with. It is the index itself
// rather than BenchmarkFor's ETF: "did the market fall too" is a question
// about the market, and a dividend-paying ETF's ex-date would read as a fall.
func DropIndexFor(m market.MarketID) string {
	if m == market.TW {
		return "^TWII"
	}
	return "^GSPC"
}

// DropIndexCloses maps date to the close of ticker's market index, nil if it
// can't be had: a drop still stands without its market comparison.
func DropIndexCloses(history data.HistoryProvider, ticker string) (string, map[string]float64) {
	index := DropIndexFor(market.Of(ticker))
	bars, err := history.GetHistory(index, dropBenchmarkRange)
	if err != nil {
		logger.Errorf("drops %s: index %s unavailable: %v", ticker, index, err)
		return index, nil
	}
	closes := make(map[string]float64, len(bars))
	for _, c := range bars {
		closes[c.Date.Format("2006-01-02")] = c.Close
	}
	return index, closes
}

// DropTechBreaks says which chart levels d's last close fell through: the
// 60-day average, and the lowest low of the 20 bars before the drop. These are
// the facts a "technical" cause rests on; false when the history is too short.
func DropTechBreaks(candles []data.Candle, d signals.DropEvent) (ma60, low20 bool) {
	last := candles[d.End].Close
	if pre := d.Start - 1; pre+1 >= 60 {
		ma := signals.MA(data.Closes(candles[pre-59:pre+1]), 60)
		ma60 = d.PreClose >= ma && last < ma
	}
	if d.Start >= 20 {
		low := candles[d.Start-20].Low
		for _, c := range candles[d.Start-20 : d.Start] {
			if c.Low < low {
				low = c.Low
			}
		}
		low20 = last < low
	}
	return ma60, low20
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

package signals

import "argus/internal/data"

// TickerMoneyFlow computes MFI-style raw money flow over the trailing
// period trading days of candles — Phase 18's sector money-flow treemap
// input. Typical price = (H+L+C)/3; a day's typicalPrice×volume is added to
// posMF when typical price rose versus the previous day, negMF otherwise
// (a flat typical price counts as negMF, matching the classic MFI
// convention). Unlike the textbook MFI indicator this stops short of the
// final 100-100/(1+ratio) normalization step — the plan calls for a signed
// dollar amount (posMF-negMF), not a 0-100 index — so callers get the raw
// sums directly and net them however the caller wants to aggregate (e.g.
// summed across a whole sector before netting, not per-ticker then
// averaged — see the sector-flow scheduler job).
//
// period is trading days, not calendar days (1 ≈ 1 day, 5 ≈ 1 week, 20 ≈ 1
// month). candles must be oldest-first (data.Candle's usual convention) and
// carry at least period+1 entries — one extra leading day to compare the
// first summed day's typical price against — or every return value is 0.
//
// dollarVol is Σ(Close×Volume) over the same window, unsigned — TW's
// market-cap-less treemap size proxy (see
// docs/phase-18-sector-money-flow.md §3) and a general relevance measure
// regardless of market.
func TickerMoneyFlow(candles []data.Candle, period int) (posMF, negMF, dollarVol float64) {
	if period <= 0 || len(candles) < period+1 {
		return 0, 0, 0
	}
	start := len(candles) - period
	prevTypical := typicalPrice(candles[start-1])
	for i := start; i < len(candles); i++ {
		c := candles[i]
		typical := typicalPrice(c)
		flow := typical * float64(c.Volume)
		if typical > prevTypical {
			posMF += flow
		} else {
			negMF += flow
		}
		dollarVol += c.Close * float64(c.Volume)
		prevTypical = typical
	}
	return posMF, negMF, dollarVol
}

func typicalPrice(c data.Candle) float64 {
	return (c.High + c.Low + c.Close) / 3
}

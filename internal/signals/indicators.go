package signals

import (
	"math"
)

const (
	StateVolUpPriceUp     = "vol_up_price_up"
	StateVolDownPriceDown = "vol_down_price_down"
	StateVolDownPriceUp   = "vol_down_price_up"
	StateVolUpPriceDown   = "vol_up_price_down"
)

// bollingerBands computes the SMA (mean), upper, and lower Bollinger Bands
// for the trailing period elements in closes.
func bollingerBands(closes []float64, period int, numStdDev float64) (mean, upper, lower float64, ok bool) {
	if len(closes) < period || period <= 0 {
		return 0, 0, 0, false
	}
	window := closes[len(closes)-period:]
	mean = MA(closes, period)
	var variance float64
	for _, c := range window {
		d := c - mean
		variance += d * d
	}
	stdDev := math.Sqrt(variance / float64(period))
	upper = mean + numStdDev*stdDev
	lower = mean - numStdDev*stdDev
	return mean, upper, lower, true
}

// StochasticSeries computes Taiwanese-style KD (RSV over rsvPeriod, K and D smoothed by smooth factor 1/smooth).
// Default usage is rsvPeriod=9, smooth=3.
// RSV = (C - L9) / (H9 - L9) * 100; if H9 == L9, RSV reuses previous RSV (or 50.0 if first bar).
// Smooth formula: K = (2/3)*K_prev + (1/3)*RSV, D = (2/3)*D_prev + (1/3)*K.
// Returns (nil, nil) if history is shorter than rsvPeriod.
func StochasticSeries(highs, lows, closes []float64, rsvPeriod, smooth int) (k, d []float64) {
	n := len(closes)
	if len(highs) < n || len(lows) < n || n < rsvPeriod || rsvPeriod <= 0 {
		return nil, nil
	}

	k = make([]float64, n)
	d = make([]float64, n)

	prevK := 50.0
	prevD := 50.0
	prevRSV := 50.0

	for i := 0; i < n; i++ {
		if i < rsvPeriod-1 {
			k[i] = 50.0
			d[i] = 50.0
			continue
		}

		maxH := highs[i-rsvPeriod+1]
		minL := lows[i-rsvPeriod+1]
		for j := i - rsvPeriod + 2; j <= i; j++ {
			if highs[j] > maxH {
				maxH = highs[j]
			}
			if lows[j] < minL {
				minL = lows[j]
			}
		}

		var rsv float64
		if maxH == minL {
			rsv = prevRSV
		} else {
			rsv = (closes[i] - minL) / (maxH - minL) * 100.0
		}
		prevRSV = rsv

		curK := (2.0/3.0)*prevK + (1.0/3.0)*rsv
		curD := (2.0/3.0)*prevD + (1.0/3.0)*curK

		k[i] = curK
		d[i] = curD

		prevK = curK
		prevD = curD
	}

	return k, d
}

// BollingerBandwidthSeries computes (upper - lower) / middle for every point where period closes exist.
// Returns nil if len(closes) < period.
func BollingerBandwidthSeries(closes []float64, period int, numStdDev float64) []float64 {
	n := len(closes)
	if n < period || period <= 0 {
		return nil
	}

	bw := make([]float64, n)
	for i := 0; i < n; i++ {
		if i < period-1 {
			bw[i] = 0
			continue
		}
		sub := closes[:i+1]
		mean, upper, lower, ok := bollingerBands(sub, period, numStdDev)
		if !ok || mean == 0 {
			bw[i] = 0
		} else {
			bw[i] = (upper - lower) / mean
		}
	}
	return bw
}

// MAAlignment checks moving average alignment:
// StateBullish if MA5 > MA20 > MA60, StateBearish if MA5 < MA20 < MA60, "" otherwise or when data insufficient.
func MAAlignment(closes []float64) string {
	ma5 := MA(closes, 5)
	ma20 := MA(closes, 20)
	ma60 := MA(closes, 60)
	if ma5 == 0 || ma20 == 0 || ma60 == 0 {
		return ""
	}
	if ma5 > ma20 && ma20 > ma60 {
		return StateBullish
	}
	if ma5 < ma20 && ma20 < ma60 {
		return StateBearish
	}
	return ""
}

// VolumePriceSignal classifies latest price/volume movement into 4 quadrants:
// Price direction: today close vs yesterday close.
// Volume direction: today volume vs avg of preceding 5 days (via VolumeRatio >= 1.3 or <= 0.7).
// Returns "" if data is insufficient or neutral.
func VolumePriceSignal(closes []float64, volumes []int64) string {
	n := len(closes)
	if n < 2 || len(volumes) < n {
		return ""
	}

	cToday := closes[n-1]
	cPrev := closes[n-2]
	if cToday == cPrev {
		return ""
	}

	vr := VolumeRatio(volumes, 5)
	if vr == 0 {
		return ""
	}

	priceUp := cToday > cPrev
	volUp := vr >= 1.3
	volDown := vr <= 0.7

	if volUp && priceUp {
		return StateVolUpPriceUp
	}
	if volDown && !priceUp {
		return StateVolDownPriceDown
	}
	if volDown && priceUp {
		return StateVolDownPriceUp
	}
	if volUp && !priceUp {
		return StateVolUpPriceDown
	}
	return ""
}

// IsNewHigh returns true if the latest close is >= the highest close in the trailing lookback bars.
// Returns false if len(closes) < lookback.
func IsNewHigh(closes []float64, lookback int) bool {
	n := len(closes)
	if n < lookback || lookback <= 0 {
		return false
	}
	window := closes[n-lookback:]
	latest := window[len(window)-1]
	for _, c := range window[:len(window)-1] {
		if c > latest {
			return false
		}
	}
	return true
}

// LowestClose returns the lowest close among the trailing n bars — Phase
// 3.11's structural stop-loss reference (10d/20d swing low, see
// docs/phase-3.11-trade-risk-management.md §3.2). Returns 0 (the same
// "not enough data" sentinel MA already uses) when fewer than n closes are
// available, rather than computing a misleadingly shallow low over whatever
// is on hand.
func LowestClose(closes []float64, n int) float64 {
	if len(closes) < n || n <= 0 {
		return 0
	}
	window := closes[len(closes)-n:]
	low := window[0]
	for _, c := range window[1:] {
		if c < low {
			low = c
		}
	}
	return low
}

// RelativeStrength computes (stock return % - SPY return %) over the trailing lookback bars.
// Returns ok=false if either series has insufficient length.
func RelativeStrength(closes, spyCloses []float64, lookback int) (rs float64, ok bool) {
	n := len(closes)
	m := len(spyCloses)
	if n < lookback || m < lookback || lookback <= 0 {
		return 0, false
	}

	cStart := closes[n-lookback]
	cEnd := closes[n-1]
	spyStart := spyCloses[m-lookback]
	spyEnd := spyCloses[m-1]

	if cStart == 0 || spyStart == 0 {
		return 0, false
	}

	cRet := (cEnd - cStart) / cStart * 100.0
	spyRet := (spyEnd - spyStart) / spyStart * 100.0

	return cRet - spyRet, true
}

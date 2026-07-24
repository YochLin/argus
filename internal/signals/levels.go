package signals

import (
	"sort"
	"time"

	"argus/internal/data"
)

// Package constants for PriceLevels — deliberately not env-configurable
// (docs/phase-7-support-resistance.md §3.2): these are algorithm internals,
// not a user risk-tolerance knob like STOP_LOSS_PCT, same reasoning as
// targetRMultiple being a fixed const in internal/bot.
const (
	pivotWindow         = 3   // bars on each side a swing point must dominate
	minLevelTouches     = 2   // clusters with fewer touches are noise, dropped
	levelClusterATRMult = 0.5 // cluster tolerance = 0.5 x ATR(14)
)

// PriceLevel is one horizontal support/resistance zone: a cluster of swing
// highs/lows that repeatedly reversed price near the same level.
type PriceLevel struct {
	Price     float64   // cluster center - mean of member pivot prices
	Touches   int       // number of swing points merged into this cluster
	FirstDate time.Time // earliest member pivot's date
	LastDate  time.Time // latest member pivot's date
}

// pivot is one swing high or low point found in step 1, before clustering.
type pivot struct {
	price float64
	date  time.Time
}

// PriceLevels reduces daily candles to horizontal support/resistance levels.
// Returns levels sorted ascending by price; nil when there isn't enough
// history. Callers classify each level as support or resistance themselves
// by comparing Price against the latest close - a level's role flips when
// price crosses it (old resistance becomes support), so the level itself
// carries no side. Pure function, no state, no DB/Telegram - same convention
// as the rest of this package. See docs/phase-7-support-resistance.md §3 for
// the full algorithm writeup.
func PriceLevels(candles []data.Candle) []PriceLevel {
	n := len(candles)
	if n < 2*pivotWindow+1 {
		return nil
	}

	highs := data.Highs(candles)
	lows := data.Lows(candles)
	closes := data.Closes(candles)

	var pivots []pivot
	for i := pivotWindow; i < n-pivotWindow; i++ {
		if isPivotHigh(highs, i) {
			pivots = append(pivots, pivot{price: highs[i], date: candles[i].Date})
		}
		if isPivotLow(lows, i) {
			pivots = append(pivots, pivot{price: lows[i], date: candles[i].Date})
		}
	}
	if len(pivots) == 0 {
		return nil
	}

	sort.Slice(pivots, func(i, j int) bool { return pivots[i].price < pivots[j].price })

	tol := levelClusterATRMult * ATR(highs, lows, closes, 14)
	if tol == 0 {
		tol = 0.01 * closes[len(closes)-1]
	}

	var levels []PriceLevel
	clusterSum := pivots[0].price
	clusterCount := 1
	first, last := pivots[0].date, pivots[0].date

	closeCluster := func() {
		if clusterCount >= minLevelTouches {
			levels = append(levels, PriceLevel{
				Price:     clusterSum / float64(clusterCount),
				Touches:   clusterCount,
				FirstDate: first,
				LastDate:  last,
			})
		}
	}

	for _, p := range pivots[1:] {
		mean := clusterSum / float64(clusterCount)
		if p.price-mean <= tol {
			clusterSum += p.price
			clusterCount++
			if p.date.Before(first) {
				first = p.date
			}
			if p.date.After(last) {
				last = p.date
			}
			continue
		}
		closeCluster()
		clusterSum = p.price
		clusterCount = 1
		first, last = p.date, p.date
	}
	closeCluster()

	return levels
}

// isPivotHigh reports whether highs[i] is the strict maximum among
// highs[i-pivotWindow : i+pivotWindow+1].
func isPivotHigh(highs []float64, i int) bool {
	for j := i - pivotWindow; j <= i+pivotWindow; j++ {
		if j == i {
			continue
		}
		if highs[j] >= highs[i] {
			return false
		}
	}
	return true
}

// isPivotLow reports whether lows[i] is the strict minimum among
// lows[i-pivotWindow : i+pivotWindow+1].
func isPivotLow(lows []float64, i int) bool {
	for j := i - pivotWindow; j <= i+pivotWindow; j++ {
		if j == i {
			continue
		}
		if lows[j] <= lows[i] {
			return false
		}
	}
	return true
}

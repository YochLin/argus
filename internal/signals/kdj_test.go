package signals

import (
	"math"
	"testing"

	"argus/internal/data"
)

// kdjCandles builds 200 bars of steady uptrend — enough to leave the MA100
// far below the close, so no case here is accidentally testing the trend
// gate — followed by the closes that path walks out from there, as
// (bars, per-bar change) segments.
func kdjCandles(path ...[2]float64) []data.Candle {
	closes := make([]float64, 0, 400)
	for i := 0; i < 200; i++ {
		closes = append(closes, float64(i)+1) // 1 -> 200, MA100 ~= 151 at bar 200
	}
	px := closes[len(closes)-1]
	for _, seg := range path {
		for i := 0; i < int(seg[0]); i++ {
			px += seg[1]
			closes = append(closes, px)
		}
	}
	return mtfCandles(len(closes), func(i int) float64 { return closes[i] })
}

func kdOf(candles []data.Candle) []float64 {
	k, _ := KDSeries(candles, len(candles))
	return k
}

// TestKDSeries pins the windowing: the tail has to match what the full-series
// computation would have produced, or every check built on it is reading a
// different indicator than the one StochasticSeries documents.
func TestKDSeries(t *testing.T) {
	cs := kdjCandles([2]float64{60, 2}, [2]float64{40, -3}, [2]float64{60, 1})
	full, fullD := StochasticSeries(data.Highs(cs), data.Lows(cs), data.Closes(cs), 9, 3)
	k, d := KDSeries(cs, 5)
	if len(k) != 5 {
		t.Fatalf("KDSeries returned %d values, want 5", len(k))
	}
	for i := range k {
		j := len(full) - 5 + i
		if math.Abs(k[i]-full[j]) > 1e-6 || math.Abs(d[i]-fullD[j]) > 1e-6 {
			t.Errorf("bar %d: KDSeries = (%.6f, %.6f), full series = (%.6f, %.6f)", i, k[i], d[i], full[j], fullD[j])
		}
	}
	if k, _ := KDSeries(cs, 5000); k != nil {
		t.Error("a tail longer than the history should return nil")
	}
}

func TestCheckKDJAboveMA100Exact(t *testing.T) {
	// A pullback deep enough to push K under 20, shallow enough to stay well
	// above the MA100 — then a recovery that crosses K back up through it.
	cs := kdjCandles([2]float64{10, 2}, [2]float64{8, -3}, [2]float64{10, 5})
	k := kdOf(cs)
	cross := 0
	for i := 201; i < len(cs); i++ {
		if k[i-1] <= 20 && k[i] > 20 {
			cross = i
			break
		}
	}
	if cross == 0 {
		t.Fatal("fixture never crosses K up through 20 — it is not exercising the check")
	}
	if !CheckKDJAboveMA100Exact(cs[:cross+1]) {
		t.Errorf("bar %d crosses K up through 20 (%.1f -> %.1f) and should trigger", cross, k[cross-1], k[cross])
	}
	if CheckKDJAboveMA100Exact(cs[:cross+2]) {
		t.Error("the bar after the cross is not itself a cross")
	}
	if CheckKDJAboveMA100Exact(cs[:cross]) {
		t.Error("a bar still under 20 should not trigger")
	}

	// Same KD shape, but the whole thing happens below the MA100: the trend
	// gate has to reject every bar of it.
	down := mtfCandles(260, func(i int) float64 {
		px := 200 - float64(i)*0.5 // MA100 sits above the close throughout
		if i >= 240 {
			px += float64(i-240) * 3
		}
		return px
	})
	for i := 210; i < len(down); i++ {
		if CheckKDJAboveMA100Exact(down[:i+1]) {
			t.Fatalf("bar %d: fired below the MA100", i)
		}
	}
}

func TestCheckKDJPullback50Exact(t *testing.T) {
	// Two shallow pullbacks — 3 bars down each, which parks K in the 20-50
	// band without reaching oversold — separated by a leg up. Only the bar
	// that turns up out of the SECOND one may fire.
	up, dip := [2]float64{10, 2}, [2]float64{3, -5}
	cs := kdjCandles(up, dip, [2]float64{8, 2}, dip, [2]float64{6, 2})
	k := kdOf(cs)

	var turns []int
	for i := 202; i < len(cs)-1; i++ {
		if isKDTouch(k, i) {
			if k[i] <= 20 {
				t.Fatalf("bar %d: fixture dipped to K=%.1f, it is meant to stay above 20", i, k[i])
			}
			turns = append(turns, i+1) // the signal is the turn-UP bar
		}
	}
	if len(turns) != 2 {
		t.Fatalf("fixture produced %d pullbacks into (20,50], want exactly 2 (K: %v)", len(turns), k[200:])
	}
	for i, bar := range turns {
		if got, want := CheckKDJPullback50Exact(cs[:bar+1]), i == 1; got != want {
			t.Errorf("pullback #%d (bar %d, K %.1f -> %.1f): fired = %v, want %v", i+1, bar, k[bar-1], k[bar], got, want)
		}
	}

	// Four shallow pullbacks with a dip through 20 between the second and
	// third. If the count restarts at the oversold bar the way the rule
	// says, the LAST turn-up is a second touch and fires; if it did not,
	// that bar would be a fourth touch and nothing would fire at all.
	deep := [2]float64{7, -5}
	reset := kdjCandles(up, dip, [2]float64{8, 2}, dip, [2]float64{10, 2}, deep,
		[2]float64{14, 2}, dip, [2]float64{8, 2}, dip, [2]float64{6, 2})
	rk := kdOf(reset)
	var fired []int
	for i := 202; i < len(reset); i++ {
		if CheckKDJPullback50Exact(reset[:i+1]) {
			fired = append(fired, i)
		}
	}
	oversoldBar := 0
	for i := 201; i < len(rk); i++ {
		if rk[i] <= 20 {
			oversoldBar = i
			break
		}
	}
	if oversoldBar == 0 {
		t.Fatal("fixture never pushed K under 20 — it is not exercising the reset")
	}
	if len(fired) != 2 {
		t.Fatalf("want one trigger on each side of the oversold bar, got %d (%v)", len(fired), fired)
	}
	if !(fired[0] < oversoldBar && fired[1] > oversoldBar) {
		t.Errorf("triggers %v both sit on the same side of the oversold bar (%d) — the count did not restart", fired, oversoldBar)
	}
}

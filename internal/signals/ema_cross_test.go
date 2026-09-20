package signals

import (
	"math"
	"math/rand"
	"testing"

	"argus/internal/data"
)

func TestEMA(t *testing.T) {
	// Hand-computed: SMA(3) of 1,2,3 = 2, then k=0.5 over 4 and 5.
	closes := []float64{1, 2, 3, 4, 5}
	if got, want := EMA(closes, 3), 4.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("EMA(3) = %v, want %v", got, want)
	}
	// A flat series has to come out at the level itself, seeding included.
	flat := make([]float64, 200)
	for i := range flat {
		flat[i] = 42
	}
	if got := EMA(flat, 10); math.Abs(got-42) > 1e-9 {
		t.Fatalf("EMA of a flat series = %v, want 42", got)
	}
	if got := EMA(closes, 9); got != 0 {
		t.Fatalf("EMA with too little history = %v, want 0", got)
	}
}

func TestCheckEMACrossExact(t *testing.T) {
	// 40 flat bars, then one up bar: the up bar crosses, the bar after it is
	// still above the EMA but is no longer a cross.
	cs := mtfCandles(42, func(i int) float64 {
		if i < 40 {
			return 100
		}
		return 110
	})
	if !CheckEMACrossExact(cs[:41]) {
		t.Error("the bar that reclaims EMA10 should trigger")
	}
	if CheckEMACrossExact(cs[:42]) {
		t.Error("a second bar above EMA10 is not a cross")
	}
	if CheckEMACrossExact(cs[:40]) {
		t.Error("a flat series should not trigger")
	}
	if CheckEMACrossExact(cs[:5]) {
		t.Error("too little history should not trigger")
	}
}

// TestEMACrossImpliesRisingEMA pins the identity CheckEMACrossUpExact's doc
// comment rests on: on an EMA, a close above the average always drags the
// average up on that same bar, so "EMA rising today" is not an extra filter.
// If this ever fails, the doc comment's reasoning is wrong and the _up
// variant is gating on something other than what it claims.
func TestEMACrossImpliesRisingEMA(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	px := 100.0
	closes := make([]float64, 0, 500)
	for i := 0; i < 500; i++ {
		px *= 1 + (rng.Float64()-0.5)*0.06
		closes = append(closes, px)
	}
	var crosses int
	for n := 12; n <= len(closes); n++ {
		today, prev := EMA(closes[:n], 10), EMA(closes[:n-1], 10)
		crossed := closes[n-2] <= prev && closes[n-1] > today
		if crossed {
			crosses++
			if today <= prev {
				t.Fatalf("bar %d: crossed above EMA10 but EMA10 did not rise (%v -> %v)", n, prev, today)
			}
		}
	}
	if crosses < 20 {
		t.Fatalf("only %d crosses in 500 bars — the series is not exercising the check", crosses)
	}
}

func TestCheckEMACrossUpExact(t *testing.T) {
	// A sustained downtrend, then one bar that jumps back above the EMA.
	// The cross fires, but the EMA10 is lower than it was a week ago, so the
	// rising-EMA variant must reject it.
	down := mtfCandles(41, func(i int) float64 {
		if i < 40 {
			return 100 - float64(i)
		}
		return 95
	})
	if !CheckEMACrossExact(down) {
		t.Fatal("the V-bottom bar should cross above EMA10")
	}
	if CheckEMACrossUpExact(down) {
		t.Error("EMA10 sat below its level a week earlier — the _up variant must reject it")
	}

	// Same shape twice — an uptrend of +2/bar with a 2-bar dip below the EMA,
	// then a reclaim — differing only in how deep the dip goes. The shallow
	// one leaves EMA10 above where it was a week ago, the deep one does not,
	// so the filter has to separate them while the base screen fires on both.
	dipped := func(depth float64) []data.Candle {
		return mtfCandles(41, func(i int) float64 {
			if i >= 38 && i < 40 {
				return 100 + float64(i)*2 - depth
			}
			return 100 + float64(i)*2
		})
	}
	for _, tc := range []struct {
		depth float64
		want  bool
	}{{20, true}, {35, false}} {
		cs := dipped(tc.depth)
		if !CheckEMACrossExact(cs) {
			t.Fatalf("depth %v: the reclaim bar should cross above EMA10", tc.depth)
		}
		if got := CheckEMACrossUpExact(cs); got != tc.want {
			t.Errorf("depth %v: _up variant = %v, want %v", tc.depth, got, tc.want)
		}
	}
}

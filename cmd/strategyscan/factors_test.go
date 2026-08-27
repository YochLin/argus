package main

import (
	"math"
	"testing"
	"time"

	"argus/internal/data"
)

// candlesFrom builds oldest-first daily bars from closes, flat OHLC.
func candlesFrom(closes []float64, volume int64) []data.Candle {
	out := make([]data.Candle, len(closes))
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, c := range closes {
		out[i] = data.Candle{
			Date: start.AddDate(0, 0, i),
			Open: c, High: c, Low: c, Close: c,
			Volume: volume,
		}
	}
	return out
}

// The "-1" in 12-1 is the entire point of the factor: the most recent month
// is short-term reversal, and including it is the classic way to cancel
// momentum out. A stock that rallied all year and then collapsed in the last
// 21 bars must still read as strong momentum, while plain 252-day relative
// strength reads deeply negative on the same series.
func TestMom12_1ExcludesTheLastMonth(t *testing.T) {
	const n = 253
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = 100 + float64(i)
	}
	for i := n - mom12_1Skip; i < n; i++ {
		closes[i] = 50 // collapse, entirely inside the skipped window
	}
	bench := make([]float64, n)
	for i := range bench {
		bench[i] = 100
	}

	got := computeRankFactors(candlesFrom(closes, 1_000), candlesFrom(bench, 1_000))

	want := (closes[n-1-mom12_1Skip] - closes[0]) / closes[0] * 100
	if math.Abs(got.Mom12_1-want) > 1e-6 {
		t.Errorf("Mom12_1 = %.4f, want %.4f (the last %d bars must not be in the window)", got.Mom12_1, want, mom12_1Skip)
	}
	if got.RS252 >= 0 {
		t.Errorf("RS252 = %.4f, want negative — the plain 252-day window does see the collapse", got.RS252)
	}
}

// SupportDist and AbsLevelDist must disagree exactly where the shipped
// ranking fix does: a close parked under resistance. AbsLevelDist is dumped
// only so that fix can be checked against returns, so a run where the two
// columns were identical would be measuring nothing.
func TestSupportDistIgnoresResistanceButAbsLevelDistDoesNot(t *testing.T) {
	closes := make([]float64, 120)
	for i := range closes {
		phase := i % 20
		closes[i] = 90 + float64(phase)*2
		if phase >= 10 {
			closes[i] = 110 - float64(phase-10)*2
		}
	}
	for i := len(closes) - 5; i < len(closes); i++ {
		closes[i] = 105
	}
	candles := candlesFrom(closes, 1_000)

	got := computeRankFactors(candles, candles)

	if wantSupport := (105.0 - 90.0) / 105.0; math.Abs(got.SupportDist-wantSupport) > 0.02 {
		t.Errorf("SupportDist = %.4f, want ~%.4f (down to support at 90)", got.SupportDist, wantSupport)
	}
	if wantAbs := (110.0 - 105.0) / 105.0; math.Abs(got.AbsLevelDist-wantAbs) > 0.02 {
		t.Errorf("AbsLevelDist = %.4f, want ~%.4f (up to resistance at 110)", got.AbsLevelDist, wantAbs)
	}
}

// A missing benchmark has to leave the benchmark-relative factors NaN. The
// zero value would reach the CSV as a real 0 and rank as an average stock,
// which is the one failure mode this dump cannot tolerate.
func TestComputeRankFactorsNoBenchmarkIsNaNNotZero(t *testing.T) {
	candles := candlesFrom([]float64{10, 11, 12, 13, 14}, 2_000)
	got := computeRankFactors(candles, nil)

	for name, v := range map[string]float64{"RS63": got.RS63, "RS252": got.RS252, "Mom12_1": got.Mom12_1} {
		if !math.IsNaN(v) {
			t.Errorf("%s = %v, want NaN with no benchmark", name, v)
		}
	}
	// Dollar volume needs no benchmark, and falls back to however many bars
	// exist when fewer than 20 do: mean close (12) x volume.
	if want := 12.0 * 2_000; math.Abs(got.DollarVol20-want) > 1e-6 {
		t.Errorf("DollarVol20 = %v, want %v (mean over the 5 available bars)", got.DollarVol20, want)
	}
}

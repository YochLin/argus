package signals

import (
	"testing"
	"time"

	"argus/internal/data"
)

// momentumCandles builds one candle per close, high/low amps[i] either side.
func momentumCandles(closes, amps []float64) []data.Candle {
	c := make([]data.Candle, len(closes))
	for i, x := range closes {
		c[i] = data.Candle{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: x, High: x + amps[i], Low: x - amps[i], Close: x, Volume: 1e6}
	}
	return c
}

func TestDetectMomentumBase(t *testing.T) {
	build := func(runUp bool) []data.Candle {
		var closes, amps []float64
		for i := range 99 {
			x := 99.0
			if runUp {
				x = 50 + float64(i)*0.5
			}
			closes, amps = append(closes, x), append(amps, 1)
		}
		closes, amps = append(closes, 100), append(amps, 1) // the base's high: 101
		for j := range 10 {
			closes, amps = append(closes, 98), append(amps, 2.5-float64(j)*0.2) // higher lows, narrowing
		}
		return momentumCandles(closes, amps)
	}
	b, ok := DetectMomentumBase(build(true))
	if !ok || b.Pivot != 101 || b.Days != 11 {
		t.Fatalf("base after a run-up: got %+v ok %v, want pivot 101 over 11 days", b, ok)
	}
	if _, ok := DetectMomentumBase(build(false)); ok {
		t.Fatal("a base with no prior 30% run must not qualify")
	}
	if got := ADRPct(momentumCandles([]float64{100, 100}, []float64{1, 1}), 2); got < 2.02 || got > 2.03 {
		t.Fatalf("ADRPct = %v, want 101/99-1 = 2.02%%", got)
	}
}

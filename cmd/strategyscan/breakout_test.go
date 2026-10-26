package main

import (
	"math"
	"testing"
	"time"

	"argus/internal/data"
)

func mbCandles(closes, amps []float64) []data.Candle {
	c := make([]data.Candle, len(closes))
	for i, x := range closes {
		c[i] = data.Candle{Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: x, High: x + amps[i], Low: x - amps[i], Close: x, Volume: 1e6}
	}
	return c
}

func TestMBSimulate(t *testing.T) {
	s := newMBSeries(mbCandles(
		[]float64{100, 101, 103, 105, 101},
		[]float64{1, 1, 1, 1, 2}, // day 4's low 99 breaks the stop moved to entry
	))
	noMA := make([]float64, len(s.c))
	tr := mbTrade{t: 0, entry: 100, stop: 95}
	mbSimulate(s, &tr, noMA, false)
	// half at day 3's close 105, the rest at the breakeven stop 100.
	if math.Abs(tr.ret-2.5) > 1e-9 || len(tr.fills) != 2 {
		t.Fatalf("ret %v fills %v, want 2.5%% over 2 fills", tr.ret, tr.fills)
	}

	// A day-3 loser is held, not sold, and the MA is ignored until the first
	// close above entry (day 5 here) sells half.
	late := newMBSeries(mbCandles(
		[]float64{100, 99, 98, 97, 99, 104, 101},
		[]float64{1, 1, 1, 1, 1, 1, 3},
	))
	tr = mbTrade{t: 0, entry: 100, stop: 95}
	mbSimulate(late, &tr, []float64{0, 200, 200, 200, 200, 0, 0}, false)
	if math.Abs(tr.ret-2) > 1e-9 || tr.fills[0].why != "partial" || tr.fills[0].date != late.c[5].Date.Format("2006-01-02") {
		t.Fatalf("ret %v fills %v, want half at 104 on day 5, rest at the breakeven stop 100", tr.ret, tr.fills)
	}

	tr = mbTrade{t: 0, entry: 100, stop: 95}
	ma := []float64{0, 0, 0, 200, 0} // day 3 closes under the MA: half on profit, rest on the MA
	mbSimulate(s, &tr, ma, false)
	if math.Abs(tr.ret-5) > 1e-9 {
		t.Fatalf("ret %v, want 5%% (all out at 105 on day 3)", tr.ret)
	}
}

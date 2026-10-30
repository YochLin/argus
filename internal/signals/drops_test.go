package signals

import (
	"math"
	"testing"
	"time"

	"argus/internal/data"
)

// dropBars builds flat-range candles from closes; open is the previous close.
func dropBars(closes ...float64) []data.Candle {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]data.Candle, len(closes))
	for i, c := range closes {
		open := c
		if i > 0 {
			open = closes[i-1]
		}
		out[i] = data.Candle{Date: day.AddDate(0, 0, i), Open: open, High: max(open, c) + 0.5, Low: min(open, c) - 0.5, Close: c}
	}
	return out
}

func TestDetectDrops_Streak(t *testing.T) {
	bars := dropBars(100, 98, 96, 93, 95, 96, 97, 98, 99, 100, 101, 102, 103)
	got := DetectDrops(bars)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	e := got[0]
	if e.Kind != DropKindStreak || e.Start != 1 || e.End != 3 {
		t.Fatalf("event = %+v", e)
	}
	if math.Abs(e.Pct-(-7)) > 1e-9 || e.PreClose != 100 || e.Low != 92.5 {
		t.Errorf("event = %+v", e)
	}
	if e.RecoverBars == nil || *e.RecoverBars != 6 {
		t.Errorf("RecoverBars = %v, want 6 (close 100 is six bars after End)", e.RecoverBars)
	}
	if e.Fwd10Pct != nil && e.End+10 >= len(bars) {
		t.Errorf("Fwd10Pct set past the data: %v", *e.Fwd10Pct)
	}
	if e.GapPct != nil {
		t.Errorf("GapPct = %v, want nil", *e.GapPct)
	}
}

func TestDetectDrops_StreakBelowThresholds(t *testing.T) {
	for name, closes := range map[string][]float64{
		"three bars, too shallow": {100, 99, 98, 97},
		"deep, only two bars":     {100, 95, 90, 91},
		"a flat close ends a run": {100, 97, 97, 94},
	} {
		if got := DetectDrops(dropBars(closes...)); len(got) != 0 {
			t.Errorf("%s: got %+v", name, got)
		}
	}
}

func TestDetectDrops_GapStandsAloneOrJoinsItsStreak(t *testing.T) {
	alone := dropBars(100, 100, 100, 100)
	alone[2].Open, alone[2].High, alone[2].Low = 97, 100, 97
	got := DetectDrops(alone)
	if len(got) != 1 || got[0].Kind != DropKindGap || got[0].Start != 2 || got[0].End != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].GapPct == nil || math.Abs(*got[0].GapPct-(-3)) > 1e-9 {
		t.Errorf("GapPct = %v", got[0].GapPct)
	}

	// Inside a streak the gap annotates it rather than being its own event.
	in := dropBars(100, 98, 96, 93)
	in[2].Open, in[2].High = 94, 97
	got = DetectDrops(in)
	if len(got) != 1 || got[0].Kind != DropKindStreak || got[0].GapPct == nil {
		t.Fatalf("got %+v", got)
	}

	// Below -2.5% but the open stays above the previous low: no gap in price.
	shallow := dropBars(100, 100, 100)
	shallow[1].Low = 90
	shallow[2].Open = 97
	if got := DetectDrops(shallow); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestDetectDrops_OrderedOldestFirst(t *testing.T) {
	bars := dropBars(100, 100, 100, 100, 98, 96, 93, 93)
	bars[1].Open, bars[1].High, bars[1].Low = 97, 100, 97
	got := DetectDrops(bars)
	if len(got) != 2 || got[0].Kind != DropKindGap || got[1].Kind != DropKindStreak {
		t.Fatalf("got %+v", got)
	}
}

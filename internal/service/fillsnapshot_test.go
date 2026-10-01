package service

import (
	"math"
	"reflect"
	"testing"
	"time"

	"argus/internal/data"
)

// fillCandles builds n daily bars from 2026-01-01 (weekends included — the
// snapshot doesn't care) with close(i) given by f and a flat 1000 volume.
func fillCandles(n int, f func(i int) float64) []data.Candle {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]data.Candle, n)
	for i := range out {
		c := f(i)
		out[i] = data.Candle{Date: start.AddDate(0, 0, i), Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 1000}
	}
	return out
}

func rising(i int) float64 { return 100 + float64(i) }

func TestFillBar(t *testing.T) {
	cs := fillCandles(100, rising)
	day := func(i int) string { return cs[i].Date.Format("2006-01-02") }

	if i, ok := FillBar(cs, day(80)); !ok || i != 80 {
		t.Fatalf("exact day: got %d %v", i, ok)
	}
	// A date past the last bar maps back to it while the gap is a weekend or so…
	if i, ok := FillBar(cs, cs[99].Date.AddDate(0, 0, 3).Format("2006-01-02")); !ok || i != 99 {
		t.Fatalf("3 days past the end: got %d %v", i, ok)
	}
	// …but not when the candles stop long before the fill.
	if _, ok := FillBar(cs, cs[99].Date.AddDate(0, 0, 30).Format("2006-01-02")); ok {
		t.Fatal("a fill a month past the data must have no snapshot")
	}
	if _, ok := FillBar(cs, day(40)); ok {
		t.Fatal("fewer than 60 bars of history must have no snapshot")
	}
	if _, ok := FillBar(cs, "2025-12-01"); ok {
		t.Fatal("a fill before the first bar must have no snapshot")
	}
	if _, ok := FillBar(cs, "not a date"); ok {
		t.Fatal("an unparseable date must have no snapshot")
	}
}

// The whole point of the snapshot: what it says about a day must not change
// with what happened afterwards.
func TestSnapshotAtIgnoresTheFuture(t *testing.T) {
	cs := fillCandles(200, func(i int) float64 { return 100 + 10*math.Sin(float64(i)/7) + float64(i)/10 })
	const i = 120
	want := SnapshotAt(cs, i)

	wild := append([]data.Candle(nil), cs...)
	for j := i + 1; j < len(wild); j++ {
		wild[j].Open, wild[j].High, wild[j].Low, wild[j].Close, wild[j].Volume = 1, 1e6, 0.01, 5000, 1e9
	}
	if got := SnapshotAt(wild, i); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot moved when only later bars changed:\n got %+v\nwant %+v", got, want)
	}
}

func TestSnapshotAtReadings(t *testing.T) {
	// A straight climb: RSI pinned high, MACD positive since the start, the
	// MAs stacked up, today's volume 3x its average.
	cs := fillCandles(100, rising)
	cs[99].Volume = 3000
	s := SnapshotAt(cs, 99)

	if s.Date != cs[99].Date.Format("2006-01-02") || s.Close != 199 {
		t.Errorf("bar: %+v", s)
	}
	if math.Abs(s.DayChangePct-(199.0/198-1)*100) > 1e-9 {
		t.Errorf("day change %v", s.DayChangePct)
	}
	if s.RSI14 != 100 || s.RSIZone != RSIHot {
		t.Errorf("rsi %v %s", s.RSI14, s.RSIZone)
	}
	if s.MACDH <= 0 || s.MACDCrossDays < 2 {
		t.Errorf("macd hist %v for %d days", s.MACDH, s.MACDCrossDays)
	}
	if s.Trend != TrendBull || s.CloseVsMA20Pct <= 0 || s.MA20Slope5dPct <= 0 {
		t.Errorf("trend %s vs ma20 %v slope %v", s.Trend, s.CloseVsMA20Pct, s.MA20Slope5dPct)
	}
	if math.Abs(s.VolRatio20-3) > 1e-9 || s.VolState != VolUp {
		t.Errorf("volume %v %s", s.VolRatio20, s.VolState)
	}
	// The spike is not in its own baseline.
	if math.Abs(s.VolRatio5v20-1) > 1e-9 {
		t.Errorf("5d/20d volume %v", s.VolRatio5v20)
	}
}

func TestSnapshotAtDowntrendAndDryVolume(t *testing.T) {
	cs := fillCandles(100, func(i int) float64 { return 300 - float64(i) })
	cs[99].Volume = 500
	s := SnapshotAt(cs, 99)
	if s.Trend != TrendBear || s.RSI14 != 0 || s.RSIZone != RSICold {
		t.Errorf("down: trend %s rsi %v %s", s.Trend, s.RSI14, s.RSIZone)
	}
	if s.MACDH >= 0 {
		t.Errorf("macd hist %v should be negative", s.MACDH)
	}
	if s.VolState != VolDown {
		t.Errorf("volume state %s", s.VolState)
	}
}

func TestSnapshotAtMACDCrossDays(t *testing.T) {
	// Fall, then a hard turn up: the histogram flips positive some bars after
	// the turn, and the count restarts from there.
	cs := fillCandles(120, func(i int) float64 {
		if i < 80 {
			return 200 - float64(i)
		}
		return 120 + 3*float64(i-80)
	})
	s := SnapshotAt(cs, 119)
	if s.MACDH <= 0 {
		t.Fatalf("expected a positive histogram after the rebound, got %v", s.MACDH)
	}
	if s.MACDCrossDays < 2 || s.MACDCrossDays > 39 {
		t.Errorf("cross days %d should count from the flip, not from the turn or the start", s.MACDCrossDays)
	}
}

func TestHindsightAfter(t *testing.T) {
	cs := fillCandles(100, rising) // close = 100+i, high = close+1, low = close-1
	h := HindsightAfter(cs, 60, 160)
	check := func(name string, got *float64, want float64) {
		t.Helper()
		if got == nil || math.Abs(*got-want) > 1e-9 {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
	check("fwd5", h.Fwd5Pct, (165.0/160-1)*100)
	check("fwd20", h.Fwd20Pct, (180.0/160-1)*100)
	check("max up", h.MaxUpPct, (181.0/160-1)*100)
	check("max down", h.MaxDownPct, (160.0/160-1)*100) // lowest low is bar 61's: 161-1

	// Too close to the end of the data: later moves are absent, not zero.
	h = HindsightAfter(cs, 97, 197)
	if h.Fwd5Pct != nil || h.Fwd20Pct != nil || h.MaxUpPct == nil {
		t.Errorf("near the end: %+v", h)
	}
	// The last bar has nothing after it at all.
	if h = HindsightAfter(cs, 99, 199); h.MaxUpPct != nil || h.MaxDownPct != nil {
		t.Errorf("last bar: %+v", h)
	}
	if h = HindsightAfter(cs, 60, 0); h != (FillHindsight{}) {
		t.Errorf("no fill price: %+v", h)
	}
}

func TestFillHistoryRange(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	// A fill ~250 days ago needs the 100 days before it too, which a 1y window
	// (365 days) would not reach.
	if got := FillHistoryRange("2026-01-24", now); got != "2y" {
		t.Errorf("250-day-old fill: %s", got)
	}
	if got := FillHistoryRange("2026-09-01", now); got != "1y" {
		t.Errorf("recent fill: %s", got)
	}
	if got := FillHistoryRange("garbage", now); got != "2y" {
		t.Errorf("bad date: %s", got)
	}
}

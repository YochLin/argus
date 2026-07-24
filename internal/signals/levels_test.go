package signals

import (
	"math"
	"testing"
	"time"

	"argus/internal/data"
)

// levelBaseCandles builds n days of perfectly flat OHLC (High=101, Low=99,
// Close=100, constant) so ATR(14) - and therefore PriceLevels' clustering
// tolerance - comes out to a fixed, easy-to-reason-about value (true range
// is 2 every day, so ATR=2, tol=0.5*2=1) regardless of how many days are in
// play. Callers overlay individual bars to create pivots without disturbing
// the trailing-14 ATR window, as long as those overlays stay clear of the
// last 14 indices.
func levelBaseCandles(n int) []data.Candle {
	candles := make([]data.Candle, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		candles[i] = data.Candle{
			Date:   now.AddDate(0, 0, -n+i),
			Open:   100,
			High:   101,
			Low:    99,
			Close:  100,
			Volume: 1_000_000,
		}
	}
	return candles
}

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPriceLevelsInsufficientBars(t *testing.T) {
	candles := levelBaseCandles(5) // < 2*pivotWindow+1 == 7
	if levels := PriceLevels(candles); levels != nil {
		t.Fatalf("expected nil for insufficient bars, got %+v", levels)
	}
}

func TestPriceLevelsDoubleTopMerges(t *testing.T) {
	candles := levelBaseCandles(40)
	candles[10].High = 110.3
	candles[20].High = 110.5

	levels := PriceLevels(candles)
	if len(levels) != 1 {
		t.Fatalf("expected exactly 1 level, got %d: %+v", len(levels), levels)
	}
	want := (110.3 + 110.5) / 2
	if !approxEqual(levels[0].Price, want) {
		t.Errorf("Price = %v, want %v", levels[0].Price, want)
	}
	if levels[0].Touches != 2 {
		t.Errorf("Touches = %d, want 2", levels[0].Touches)
	}
	if !levels[0].FirstDate.Equal(candles[10].Date) || !levels[0].LastDate.Equal(candles[20].Date) {
		t.Errorf("FirstDate/LastDate = %v/%v, want %v/%v",
			levels[0].FirstDate, levels[0].LastDate, candles[10].Date, candles[20].Date)
	}
}

func TestPriceLevelsFarApartPivotsAreNoise(t *testing.T) {
	candles := levelBaseCandles(40)
	candles[10].High = 110 // isolated single touch
	candles[20].High = 150 // isolated single touch, far from the first

	if levels := PriceLevels(candles); len(levels) != 0 {
		t.Fatalf("expected no levels (each cluster has only 1 touch), got %+v", levels)
	}
}

func TestPriceLevelsHighAndLowMerge(t *testing.T) {
	candles := levelBaseCandles(40)
	candles[10].High = 110 // pivot high @ 110

	// Elevate the immediate neighbors of index 20 so index 20's Low (109.7)
	// - a price close to the pivot high above - registers as a strict local
	// minimum (a pivot low), simulating the same level acting as resistance
	// once and support another time.
	for _, i := range []int{17, 18, 19, 21, 22, 23} {
		candles[i].Low = 200
	}
	candles[20].Low = 109.7

	levels := PriceLevels(candles)
	if len(levels) != 1 {
		t.Fatalf("expected exactly 1 merged level, got %d: %+v", len(levels), levels)
	}
	want := (110.0 + 109.7) / 2
	if !approxEqual(levels[0].Price, want) {
		t.Errorf("Price = %v, want %v", levels[0].Price, want)
	}
	if levels[0].Touches != 2 {
		t.Errorf("Touches = %d, want 2", levels[0].Touches)
	}
}

func TestPriceLevelsSortedAscending(t *testing.T) {
	candles := levelBaseCandles(50)
	// Pair A: a double top around 110, well clear of the trailing-14 ATR window.
	candles[8].High = 110.2
	candles[14].High = 110.4
	// Pair B: a double top around 130, also clear of the trailing-14 window
	// (indices 36-49) and far enough from pair A to stay a separate cluster.
	candles[20].High = 130.2
	candles[26].High = 130.4

	levels := PriceLevels(candles)
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %+v", len(levels), levels)
	}
	if levels[0].Price >= levels[1].Price {
		t.Errorf("levels not sorted ascending: %+v", levels)
	}
	if !approxEqual(levels[0].Price, (110.2+110.4)/2) {
		t.Errorf("levels[0].Price = %v, want ~110.3", levels[0].Price)
	}
	if !approxEqual(levels[1].Price, (130.2+130.4)/2) {
		t.Errorf("levels[1].Price = %v, want ~130.3", levels[1].Price)
	}
}

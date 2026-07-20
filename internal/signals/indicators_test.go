package signals

import (
	"testing"
)

func TestStochasticSeries(t *testing.T) {
	// Insufficient data
	k, d := StochasticSeries([]float64{10}, []float64{5}, []float64{8}, 9, 3)
	if k != nil || d != nil {
		t.Fatalf("expected nil for short data")
	}

	// Simple fixture: 10 bars
	highs := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	lows := []float64{5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	closes := []float64{8, 9, 10, 11, 12, 13, 14, 15, 16, 19}

	kSeries, dSeries := StochasticSeries(highs, lows, closes, 9, 3)
	if len(kSeries) != 10 || len(dSeries) != 10 {
		t.Fatalf("expected length 10, got %d, %d", len(kSeries), len(dSeries))
	}
	// Initial 8 bars default to 50
	for i := 0; i < 8; i++ {
		if kSeries[i] != 50 || dSeries[i] != 50 {
			t.Errorf("expected 50 at index %d, got K=%.2f, D=%.2f", i, kSeries[i], dSeries[i])
		}
	}
	// Index 8: 9-bar window highs[0..8] max 18, lows[0..8] min 5. Close 16.
	// RSV = (16-5)/(18-5)*100 = 11/13 * 100 = 84.615...
	// K = 2/3*50 + 1/3*84.615 = 33.333 + 28.205 = 61.538
	if kSeries[8] < 60 || kSeries[8] > 63 {
		t.Errorf("unexpected K[8]: %.2f", kSeries[8])
	}
	// Index 9: close 19 == max 19. RSV = 100. K should increase.
	if kSeries[9] <= kSeries[8] {
		t.Errorf("expected K[9] > K[8]")
	}
}

func TestBollingerBandwidthSeries(t *testing.T) {
	// Insufficient data
	if bw := BollingerBandwidthSeries([]float64{1, 2}, 5, 2.0); bw != nil {
		t.Fatalf("expected nil for short data")
	}

	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = float64(10 + i%3)
	}

	bw := BollingerBandwidthSeries(closes, 20, 2.0)
	if len(bw) != 25 {
		t.Fatalf("expected length 25, got %d", len(bw))
	}
	if bw[18] != 0 {
		t.Errorf("expected bw[18] == 0, got %f", bw[18])
	}
	if bw[19] <= 0 {
		t.Errorf("expected bw[19] > 0, got %f", bw[19])
	}
}

func TestMAAlignment(t *testing.T) {
	// Insufficient data (< 60 bars)
	closes := make([]float64, 50)
	if align := MAAlignment(closes); align != "" {
		t.Fatalf("expected empty alignment for short data")
	}

	// Bullish alignment: 5MA > 20MA > 60MA
	closesBull := make([]float64, 100)
	for i := range closesBull {
		closesBull[i] = float64(10 + i*2) // steady uptrend
	}
	if align := MAAlignment(closesBull); align != StateBullish {
		t.Errorf("expected StateBullish, got %q", align)
	}

	// Bearish alignment: 5MA < 20MA < 60MA
	closesBear := make([]float64, 100)
	for i := range closesBear {
		closesBear[i] = float64(300 - i*2) // steady downtrend
	}
	if align := MAAlignment(closesBear); align != StateBearish {
		t.Errorf("expected StateBearish, got %q", align)
	}
}

func TestVolumePriceSignal(t *testing.T) {
	// To test signals, we need at least 6 bars of closes and volumes
	closes := []float64{10, 10, 10, 10, 10, 12}      // price up
	volumes := []int64{100, 100, 100, 100, 100, 200} // vol ratio 200/100 = 2.0 >= 1.3 (vol up)

	sig := VolumePriceSignal(closes, volumes)
	if sig != StateVolUpPriceUp {
		t.Errorf("expected StateVolUpPriceUp, got %q", sig)
	}

	// Vol down, price down
	closes2 := []float64{10, 10, 10, 10, 10, 8}
	volumes2 := []int64{100, 100, 100, 100, 100, 50} // vol ratio 50/100 = 0.5 <= 0.7
	if sig := VolumePriceSignal(closes2, volumes2); sig != StateVolDownPriceDown {
		t.Errorf("expected StateVolDownPriceDown, got %q", sig)
	}
}

func TestIsNewHigh(t *testing.T) {
	closes := []float64{10, 12, 11, 14, 13, 15}
	if !IsNewHigh(closes, 5) {
		t.Errorf("expected true for 15 in [12, 11, 14, 13, 15]")
	}
	if IsNewHigh([]float64{10, 15, 14, 13, 12}, 5) {
		t.Errorf("expected false for 12")
	}
}

func TestLowestClose(t *testing.T) {
	closes := []float64{20, 15, 12, 18, 16, 19}
	if got := LowestClose(closes, 3); got != 16 {
		t.Errorf("LowestClose(last 3) = %v, want 16", got)
	}
	if got := LowestClose(closes, 5); got != 12 {
		t.Errorf("LowestClose(last 5) = %v, want 12", got)
	}
	if got := LowestClose(closes, 10); got != 0 {
		t.Errorf("LowestClose() with insufficient data = %v, want 0", got)
	}
}

func TestRelativeStrength(t *testing.T) {
	stock := []float64{100, 110} // +10%
	spy := []float64{400, 420}   // +5%

	rs, ok := RelativeStrength(stock, spy, 2)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if diff := rs - 5.0; diff > 0.001 || diff < -0.001 {
		t.Errorf("expected RS 5.0, got %f", rs)
	}
}

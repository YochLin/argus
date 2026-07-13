package signals

import (
	"math"
	"testing"

	"argus/internal/data"
	"argus/internal/i18n"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestRSI(t *testing.T) {
	t.Run("insufficient data returns neutral 50", func(t *testing.T) {
		if got := RSI([]float64{1, 2, 3}, 5); got != 50 {
			t.Errorf("RSI() = %v, want 50", got)
		}
	})

	t.Run("all gains yields 100", func(t *testing.T) {
		if got := RSI([]float64{1, 2, 3, 4, 5}, 4); got != 100 {
			t.Errorf("RSI() = %v, want 100", got)
		}
	})

	t.Run("all losses yields 0", func(t *testing.T) {
		if got := RSI([]float64{5, 4, 3, 2, 1}, 4); got != 0 {
			t.Errorf("RSI() = %v, want 0", got)
		}
	})

	t.Run("mixed gains and losses", func(t *testing.T) {
		got := RSI([]float64{44, 45, 44, 46, 45}, 4)
		if !almostEqual(got, 60) {
			t.Errorf("RSI() = %v, want 60", got)
		}
	})

	t.Run("uses the most recent window, not the oldest", func(t *testing.T) {
		// First 5 points fall sharply (would read as oversold on their own);
		// the latest 15 points (period+1=15) rise every single day. A
		// regression back to reading closes[0:period+1] instead of the tail
		// would compute RSI over the falling segment and return well under
		// 100 here.
		closes := []float64{
			100, 90, 80, 70, 60,
			61, 63, 65, 67, 69, 71, 73, 75, 77, 79, 81, 83, 85, 87, 89,
		}
		if got := RSI(closes, 14); got != 100 {
			t.Errorf("RSI() = %v, want 100 (should reflect the latest 15 closes, not the oldest)", got)
		}
	})
}

func TestMA(t *testing.T) {
	t.Run("insufficient data returns 0", func(t *testing.T) {
		if got := MA([]float64{1, 2, 3}, 5); got != 0 {
			t.Errorf("MA() = %v, want 0", got)
		}
	})

	t.Run("averages the trailing window, not the oldest", func(t *testing.T) {
		closes := []float64{100, 100, 100, 10, 20, 30}
		if got := MA(closes, 3); !almostEqual(got, 20) {
			t.Errorf("MA() = %v, want 20", got)
		}
	})
}

func TestVolumeRatio(t *testing.T) {
	t.Run("insufficient data returns 0", func(t *testing.T) {
		if got := VolumeRatio([]int64{1, 2, 3}, 5); got != 0 {
			t.Errorf("VolumeRatio() = %v, want 0", got)
		}
	})

	t.Run("compares latest against the average of the preceding window, excluding itself", func(t *testing.T) {
		volumes := []int64{100, 100, 100, 100, 400}
		if got := VolumeRatio(volumes, 4); !almostEqual(got, 4) {
			t.Errorf("VolumeRatio() = %v, want 4", got)
		}
	})

	t.Run("zero baseline average returns 0 rather than dividing by zero", func(t *testing.T) {
		volumes := []int64{0, 0, 0, 0, 500}
		if got := VolumeRatio(volumes, 4); got != 0 {
			t.Errorf("VolumeRatio() = %v, want 0", got)
		}
	})
}

func TestEmaSeries(t *testing.T) {
	got := emaSeries([]float64{1, 2, 3, 4}, 3)
	want := []float64{1, 1.5, 2.25, 3.125}
	if len(got) != len(want) {
		t.Fatalf("emaSeries() length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !almostEqual(got[i], want[i]) {
			t.Errorf("emaSeries()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMACD(t *testing.T) {
	t.Run("insufficient data returns zeros", func(t *testing.T) {
		closes := make([]float64, 34) // 26+9-1
		m, s, h := MACD(closes)
		if m != 0 || s != 0 || h != 0 {
			t.Errorf("MACD() = (%v, %v, %v), want (0, 0, 0)", m, s, h)
		}
	})

	t.Run("flat closes yields zero macd/signal/histogram", func(t *testing.T) {
		closes := make([]float64, 40)
		for i := range closes {
			closes[i] = 100
		}
		m, s, h := MACD(closes)
		if m != 0 || s != 0 || h != 0 {
			t.Errorf("MACD() = (%v, %v, %v), want (0, 0, 0)", m, s, h)
		}
	})

	t.Run("uptrend yields positive macd above signal", func(t *testing.T) {
		closes := make([]float64, 40)
		for i := range closes {
			closes[i] = 100 + float64(i)
		}
		m, s, h := MACD(closes)
		wantM, wantS, wantH := 6.386727, 6.143887, 0.242840
		if !almostEqualTo(m, wantM, 1e-5) || !almostEqualTo(s, wantS, 1e-5) || !almostEqualTo(h, wantH, 1e-5) {
			t.Errorf("MACD() = (%.6f, %.6f, %.6f), want (%.6f, %.6f, %.6f)", m, s, h, wantM, wantS, wantH)
		}
	})

	t.Run("downtrend yields negative macd below signal", func(t *testing.T) {
		closes := make([]float64, 40)
		for i := range closes {
			closes[i] = 200 - float64(i)
		}
		m, s, h := MACD(closes)
		wantM, wantS, wantH := -6.386727, -6.143887, -0.242840
		if !almostEqualTo(m, wantM, 1e-5) || !almostEqualTo(s, wantS, 1e-5) || !almostEqualTo(h, wantH, 1e-5) {
			t.Errorf("MACD() = (%.6f, %.6f, %.6f), want (%.6f, %.6f, %.6f)", m, s, h, wantM, wantS, wantH)
		}
	})
}

func almostEqualTo(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func TestDetectorCheckQuote(t *testing.T) {
	d := NewDetector(i18n.EN)

	t.Run("below threshold produces no signal", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", ChangePercent: 1.0}
		if sigs := d.CheckQuote(q); len(sigs) != 0 {
			t.Errorf("CheckQuote() = %v, want no signals", sigs)
		}
	})

	t.Run("large gain produces price signal", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", ChangePercent: 5.0, Price: 150}
		sigs := d.CheckQuote(q)
		if len(sigs) != 1 || sigs[0].Type != "price" {
			t.Fatalf("CheckQuote() = %v, want one price signal", sigs)
		}
	})

	t.Run("large drop produces price signal", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", ChangePercent: -5.0, Price: 150}
		sigs := d.CheckQuote(q)
		if len(sigs) != 1 || sigs[0].Type != "price" {
			t.Fatalf("CheckQuote() = %v, want one price signal", sigs)
		}
	})
}

func TestDetectorCheckRSI(t *testing.T) {
	d := NewDetector(i18n.EN)

	t.Run("overbought", func(t *testing.T) {
		closes := make([]float64, 15)
		for i := range closes {
			closes[i] = 100 + float64(i)
		}
		sig := d.CheckRSI("AAPL", closes)
		if sig == nil || sig.Type != "rsi_overbought" {
			t.Fatalf("CheckRSI() = %v, want rsi_overbought", sig)
		}
	})

	t.Run("oversold", func(t *testing.T) {
		closes := make([]float64, 15)
		for i := range closes {
			closes[i] = 100 - float64(i)
		}
		sig := d.CheckRSI("AAPL", closes)
		if sig == nil || sig.Type != "rsi_oversold" {
			t.Fatalf("CheckRSI() = %v, want rsi_oversold", sig)
		}
	})

	t.Run("neutral produces no signal", func(t *testing.T) {
		closes := []float64{44, 45, 44, 46, 45}
		if sig := d.CheckRSI("AAPL", closes); sig != nil {
			t.Errorf("CheckRSI() = %v, want nil", sig)
		}
	})
}

func TestDetectorCheckMACD(t *testing.T) {
	d := NewDetector(i18n.EN)

	t.Run("uptrend produces bullish signal", func(t *testing.T) {
		closes := make([]float64, 40)
		for i := range closes {
			closes[i] = 100 + float64(i)
		}
		sig := d.CheckMACD("AAPL", closes)
		if sig == nil || sig.Type != "macd_bullish" {
			t.Fatalf("CheckMACD() = %v, want macd_bullish", sig)
		}
	})

	t.Run("downtrend produces bearish signal", func(t *testing.T) {
		closes := make([]float64, 40)
		for i := range closes {
			closes[i] = 200 - float64(i)
		}
		sig := d.CheckMACD("AAPL", closes)
		if sig == nil || sig.Type != "macd_bearish" {
			t.Fatalf("CheckMACD() = %v, want macd_bearish", sig)
		}
	})

	t.Run("insufficient data produces no signal", func(t *testing.T) {
		closes := []float64{1, 2, 3}
		if sig := d.CheckMACD("AAPL", closes); sig != nil {
			t.Errorf("CheckMACD() = %v, want nil", sig)
		}
	})
}

func TestDetectorCheckRSIState(t *testing.T) {
	d := NewDetector(i18n.EN)

	overbought := make([]float64, 15)
	oversold := make([]float64, 15)
	for i := range overbought {
		overbought[i] = 100 + float64(i)
		oversold[i] = 100 - float64(i)
	}

	t.Run("first entry into overbought signals", func(t *testing.T) {
		sig, state := d.CheckRSIState("AAPL", overbought, "")
		if sig == nil || sig.Type != "rsi_overbought" || state != StateOverbought {
			t.Fatalf("CheckRSIState() = %v, %q; want rsi_overbought signal, state overbought", sig, state)
		}
	})

	t.Run("staying overbought does not repeat the signal", func(t *testing.T) {
		sig, state := d.CheckRSIState("AAPL", overbought, StateOverbought)
		if sig != nil || state != StateOverbought {
			t.Fatalf("CheckRSIState() = %v, %q; want nil signal, state unchanged", sig, state)
		}
	})

	t.Run("flipping to oversold signals again", func(t *testing.T) {
		sig, state := d.CheckRSIState("AAPL", oversold, StateOverbought)
		if sig == nil || sig.Type != "rsi_oversold" || state != StateOversold {
			t.Fatalf("CheckRSIState() = %v, %q; want rsi_oversold signal, state oversold", sig, state)
		}
	})

	t.Run("returning to normal is silent but recorded", func(t *testing.T) {
		closes := []float64{44, 45, 44, 46, 45}
		sig, state := d.CheckRSIState("AAPL", closes, StateOverbought)
		if sig != nil || state != StateNormal {
			t.Fatalf("CheckRSIState() = %v, %q; want nil signal, state normal", sig, state)
		}
	})
}

func TestDetectorCheckMACDCross(t *testing.T) {
	d := NewDetector(i18n.EN)

	uptrend := make([]float64, 40)
	downtrend := make([]float64, 40)
	for i := range uptrend {
		uptrend[i] = 100 + float64(i)
		downtrend[i] = 200 - float64(i)
	}

	t.Run("first observation records baseline without signaling", func(t *testing.T) {
		sig, state := d.CheckMACDCross("AAPL", uptrend, "")
		if sig != nil || state != StateBullish {
			t.Fatalf("CheckMACDCross() = %v, %q; want nil signal, state bullish", sig, state)
		}
	})

	t.Run("holding trend does not repeat the signal", func(t *testing.T) {
		sig, state := d.CheckMACDCross("AAPL", uptrend, StateBullish)
		if sig != nil || state != StateBullish {
			t.Fatalf("CheckMACDCross() = %v, %q; want nil signal, state unchanged", sig, state)
		}
	})

	t.Run("bearish to bullish flip is a golden cross", func(t *testing.T) {
		sig, state := d.CheckMACDCross("AAPL", uptrend, StateBearish)
		if sig == nil || sig.Type != "macd_golden_cross" || state != StateBullish {
			t.Fatalf("CheckMACDCross() = %v, %q; want macd_golden_cross, state bullish", sig, state)
		}
	})

	t.Run("bullish to bearish flip is a death cross", func(t *testing.T) {
		sig, state := d.CheckMACDCross("AAPL", downtrend, StateBullish)
		if sig == nil || sig.Type != "macd_death_cross" || state != StateBearish {
			t.Fatalf("CheckMACDCross() = %v, %q; want macd_death_cross, state bearish", sig, state)
		}
	})

	t.Run("insufficient data keeps the previous state", func(t *testing.T) {
		sig, state := d.CheckMACDCross("AAPL", []float64{1, 2, 3}, StateBullish)
		if sig != nil || state != StateBullish {
			t.Fatalf("CheckMACDCross() = %v, %q; want nil signal, state preserved", sig, state)
		}
	})
}

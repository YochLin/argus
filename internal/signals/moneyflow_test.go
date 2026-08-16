package signals

import (
	"testing"

	"argus/internal/data"
)

func mfCandle(high, low, close float64, volume int64) data.Candle {
	return data.Candle{High: high, Low: low, Close: close, Volume: volume}
}

func TestTickerMoneyFlow(t *testing.T) {
	// Typical prices: 100, 90 (down), 100 (up), 120 (up).
	candles := []data.Candle{
		mfCandle(102, 98, 100, 1000), // baseline day, typical 100 — not summed
		mfCandle(92, 88, 90, 500),    // typical 90 < 100 -> negMF
		mfCandle(102, 98, 100, 800),  // typical 100 > 90 -> posMF
		mfCandle(122, 118, 120, 300), // typical 120 > 100 -> posMF
	}

	posMF, negMF, dollarVol := TickerMoneyFlow(candles, 3)

	wantNeg := 90.0 * 500
	wantPos := 100.0*800 + 120.0*300
	if negMF != wantNeg {
		t.Errorf("negMF = %v, want %v", negMF, wantNeg)
	}
	if posMF != wantPos {
		t.Errorf("posMF = %v, want %v", posMF, wantPos)
	}
	wantDollarVol := 90.0*500 + 100.0*800 + 120.0*300
	if dollarVol != wantDollarVol {
		t.Errorf("dollarVol = %v, want %v", dollarVol, wantDollarVol)
	}
}

func TestTickerMoneyFlowFlatCountsNegative(t *testing.T) {
	candles := []data.Candle{
		mfCandle(102, 98, 100, 1000),
		mfCandle(102, 98, 100, 400), // same typical price as baseline -> negMF
	}
	posMF, negMF, _ := TickerMoneyFlow(candles, 1)
	if posMF != 0 {
		t.Errorf("posMF = %v, want 0 (flat typical price should not count as positive)", posMF)
	}
	if want := 100.0 * 400; negMF != want {
		t.Errorf("negMF = %v, want %v", negMF, want)
	}
}

func TestTickerMoneyFlowInsufficientData(t *testing.T) {
	candles := []data.Candle{mfCandle(102, 98, 100, 1000)}
	posMF, negMF, dollarVol := TickerMoneyFlow(candles, 5)
	if posMF != 0 || negMF != 0 || dollarVol != 0 {
		t.Errorf("with insufficient candles, got (%v, %v, %v), want all 0", posMF, negMF, dollarVol)
	}
}

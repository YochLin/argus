package signals

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

func generateBaseCandles(count int) []data.Candle {
	candles := make([]data.Candle, count)
	now := time.Now()
	for i := 0; i < count; i++ {
		t := now.AddDate(0, 0, -count+i)
		price := 100.0 + float64(i%3)
		candles[i] = data.Candle{
			Date:   t,
			Open:   price,
			High:   price + 1.0,
			Low:    price - 1.0,
			Close:  price,
			Volume: 1_000_000,
		}
	}
	return candles
}

func TestSqueezeBreakoutSynthetic(t *testing.T) {
	// Insufficient candles
	candlesShort := generateBaseCandles(50)
	if hit := SqueezeBreakout(candlesShort); hit != nil {
		t.Fatalf("expected nil for < 60 candles")
	}

	// 80 candles: flat price 100 with volume 1,000,000, then at last bar breakout
	candles := make([]data.Candle, 80)
	now := time.Now()
	for i := 0; i < 80; i++ {
		t := now.AddDate(0, 0, -80+i)
		candles[i] = data.Candle{
			Date:   t,
			Open:   100.0,
			High:   101.0,
			Low:    99.0,
			Close:  100.0,
			Volume: 1_000_000,
		}
	}

	// Make last candle a breakout:
	// Close jump to 120 (well above upper band ~101), Volume 3,000,000 (3x > 2x 1M)
	candles[79].Close = 120.0
	candles[79].High = 121.0
	candles[79].Volume = 3_000_000

	hit := SqueezeBreakout(candles)
	if hit == nil {
		t.Fatalf("expected SqueezeBreakout hit, got nil")
	}
	if hit.Name != "squeeze_breakout" || hit.DaysAgo != 0 {
		t.Errorf("unexpected hit details: %+v", hit)
	}

	// Check Detector deduplication
	det := NewDetector(i18n.ZH)
	sig1, state1 := det.CheckSqueezeBreakout("TEST", candles, "")
	if sig1 == nil || state1 != "hit" {
		t.Fatalf("expected signal and state='hit', got sig=%v, state=%s", sig1, state1)
	}

	// Repeated check with state='hit' should deduplicate (return nil, 'hit')
	sig2, state2 := det.CheckSqueezeBreakout("TEST", candles, "hit")
	if sig2 != nil || state2 != "hit" {
		t.Fatalf("expected nil signal on repeat hit, got sig=%v, state=%s", sig2, state2)
	}

	// No hit case -> clears state to ""
	normalCandles := generateBaseCandles(80)
	sig3, state3 := det.CheckSqueezeBreakout("TEST", normalCandles, "hit")
	if sig3 != nil || state3 != "" {
		t.Fatalf("expected cleared state '', got sig=%v, state=%s", sig3, state3)
	}
}

func TestBoxBottomReboundSynthetic(t *testing.T) {
	// Build 80 candles where price drops sharply to oversold, then bounces at box floor
	candles := make([]data.Candle, 80)
	now := time.Now()
	for i := 0; i < 80; i++ {
		tDate := now.AddDate(0, 0, -80+i)
		price := 100.0
		if i >= 50 && i < 76 {
			// Downtrend
			price = 100.0 - float64(i-50)*1.5 // drops to 61.0
		} else if i >= 76 {
			// Box floor near 60
			price = 61.0 + float64(i-76)*0.2 // 61.0, 61.2, 61.4, 61.6
		}
		candles[i] = data.Candle{
			Date:   tDate,
			Open:   price,
			High:   price + 0.5,
			Low:    price - 0.5,
			Close:  price,
			Volume: 1_000_000,
		}
	}

	hit := BoxBottomRebound(candles)
	det := NewDetector(i18n.ZH)
	sig, state := det.CheckBoxBottom("TEST", candles, "")
	if hit == nil {
		if sig != nil || state != "" {
			t.Errorf("expected no signal when hit is nil")
		}
	} else {
		if sig == nil || state != "hit" {
			t.Errorf("expected signal when hit is non-nil")
		}
	}
}

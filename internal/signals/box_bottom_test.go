package signals

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

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

	hit := BoxBottomRebound(candles, DefaultScreenParams(market.US))
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

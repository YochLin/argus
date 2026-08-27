package signals

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
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
	usParams := DefaultScreenParams(market.US)

	// Insufficient candles
	candlesShort := generateBaseCandles(50)
	if hit := SqueezeBreakout(candlesShort, usParams); hit != nil {
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

	hit := SqueezeBreakout(candles, usParams)
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

// buildTrendBreakoutCandles builds an 80-bar steady uptrend (MA5>MA20>MA60,
// rising, so IsNewHigh/MAAlignment both hold) then overrides the last bar
// into a solid breakout: new 60d high, attack volume, small upper wick,
// deviation from MA20 within the default US 15% gate.
func buildTrendBreakoutCandles() []data.Candle {
	n := 80
	candles := make([]data.Candle, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		price := 100.0 + float64(i)*0.3
		candles[i] = data.Candle{
			Date:   now.AddDate(0, 0, -n+i),
			Open:   price - 0.2,
			High:   price + 0.3,
			Low:    price - 0.5,
			Close:  price,
			Volume: 500_000,
		}
	}
	last := &candles[n-1]
	prevClose := candles[n-2].Close
	last.Open = prevClose + 0.1
	last.Close = prevClose + 3.0
	last.High = last.Close + 0.2
	last.Low = last.Open - 0.1
	last.Volume = 2_000_000
	return candles
}

func TestTrendBreakoutSynthetic(t *testing.T) {
	usParams := DefaultScreenParams(market.US)

	candlesShort := generateBaseCandles(50)
	if hit := TrendBreakout(candlesShort, usParams); hit != nil {
		t.Fatalf("expected nil for < 60 candles")
	}

	candles := buildTrendBreakoutCandles()
	hit := TrendBreakout(candles, usParams)
	if hit == nil {
		t.Fatalf("expected TrendBreakout hit, got nil")
	}
	if hit.Name != "trend_breakout" || hit.DaysAgo != 0 {
		t.Errorf("unexpected hit details: %+v", hit)
	}
	if !CheckTrendBreakoutExact(candles, usParams) {
		t.Fatalf("expected CheckTrendBreakoutExact true on full fixture")
	}

	// Each condition removed individually should fail the AND.
	t.Run("fails without liquidity", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		for i := len(c) - 6; i < len(c)-1; i++ {
			c[i].Volume = 100 // below MinAvgVolume5d
		}
		if CheckTrendBreakoutExact(c, usParams) {
			t.Errorf("expected false when 5-day avg volume is too thin")
		}
	})

	t.Run("fails without new high", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		c[len(c)-1].Close = c[len(c)-2].Close - 1 // no longer a new high
		c[len(c)-1].High = c[len(c)-1].Close + 0.2
		if CheckTrendBreakoutExact(c, usParams) {
			t.Errorf("expected false without a new high")
		}
	})

	t.Run("fails without attack volume", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		c[len(c)-1].Volume = 500_000 // same as baseline, ratio ~1.0 < 1.5
		if CheckTrendBreakoutExact(c, usParams) {
			t.Errorf("expected false without attack volume")
		}
	})

	t.Run("fails when deviation exceeds gate", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		c[len(c)-1].Close = c[len(c)-2].Close + 40 // blow past MaxMA20DevPct
		c[len(c)-1].High = c[len(c)-1].Close + 0.2
		if CheckTrendBreakoutExact(c, usParams) {
			t.Errorf("expected false when deviation exceeds MaxMA20DevPct")
		}
	})

	t.Run("fails without solid bull bar", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		c[len(c)-1].High = c[len(c)-1].Close + 10 // huge upper wick
		if CheckTrendBreakoutExact(c, usParams) {
			t.Errorf("expected false with an oversized upper wick")
		}
	})

	// Detector dedup: hit -> repeat hit is nil, no-hit clears state.
	det := NewDetector(i18n.ZH)
	sig1, state1 := det.CheckTrendBreakout("TEST", candles, "")
	if sig1 == nil || state1 != "hit" {
		t.Fatalf("expected signal and state='hit', got sig=%v, state=%s", sig1, state1)
	}
	sig2, state2 := det.CheckTrendBreakout("TEST", candles, "hit")
	if sig2 != nil || state2 != "hit" {
		t.Fatalf("expected nil signal on repeat hit, got sig=%v, state=%s", sig2, state2)
	}
	sig3, state3 := det.CheckTrendBreakout("TEST", generateBaseCandles(80), "hit")
	if sig3 != nil || state3 != "" {
		t.Fatalf("expected cleared state '', got sig=%v, state=%s", sig3, state3)
	}

	// Triggered 3 days ago: append 3 flat bars after the breakout so the
	// most recent bar no longer qualifies but offset=3 still does.
	withTail := append([]data.Candle(nil), candles...)
	flatPrice := withTail[len(withTail)-1].Close
	for i := 0; i < 3; i++ {
		withTail = append(withTail, data.Candle{
			Date:   time.Now().AddDate(0, 0, i+1),
			Open:   flatPrice,
			High:   flatPrice + 0.1,
			Low:    flatPrice - 0.1,
			Close:  flatPrice,
			Volume: 500_000,
		})
	}
	hitDelayed := TrendBreakout(withTail, usParams)
	if hitDelayed == nil || hitDelayed.DaysAgo != 3 {
		t.Fatalf("expected DaysAgo=3, got %+v", hitDelayed)
	}

	// Triggered 6 days ago falls outside the 5-day lookback window.
	withLongTail := append([]data.Candle(nil), candles...)
	for i := 0; i < 6; i++ {
		withLongTail = append(withLongTail, data.Candle{
			Date:   time.Now().AddDate(0, 0, i+1),
			Open:   flatPrice,
			High:   flatPrice + 0.1,
			Low:    flatPrice - 0.1,
			Close:  flatPrice,
			Volume: 500_000,
		})
	}
	if hit := TrendBreakout(withLongTail, usParams); hit != nil {
		t.Fatalf("expected nil once the breakout bar is outside the lookback window, got %+v", hit)
	}
}

// buildTrendPullbackCandles builds a long uptrend (MA60 rising, close above
// MA60) then a controlled 10-bar pullback toward MA20 with shrinking volume,
// ending in a hammer reversal bar on the last day.
func buildTrendPullbackCandles() []data.Candle {
	n := 100
	candles := make([]data.Candle, n)
	now := time.Now()
	base := 100.0
	for i := 0; i < n; i++ {
		price := base + float64(i)*0.5
		candles[i] = data.Candle{
			Date:   now.AddDate(0, 0, -n+i),
			Open:   price - 0.2,
			High:   price + 0.3,
			Low:    price - 0.5,
			Close:  price,
			Volume: 1_000_000,
		}
	}
	pullStart := n - 10
	peak := candles[pullStart-1].Close
	for i := pullStart; i < n-1; i++ {
		steps := float64(i - pullStart + 1)
		price := peak - steps*0.6
		candles[i].Open = price + 0.3
		candles[i].Close = price
		candles[i].High = price + 0.4
		candles[i].Low = price - 0.4
		candles[i].Volume = int64(1_000_000 - int64(steps)*80_000)
	}
	last := &candles[n-1]
	prevClose := candles[n-2].Close
	last.Open = prevClose - 0.3
	last.Low = prevClose - 2.0
	last.Close = prevClose + 0.3
	last.High = last.Close + 0.1
	last.Volume = 300_000
	return candles
}

func TestTrendPullbackSynthetic(t *testing.T) {
	usParams := DefaultScreenParams(market.US)

	candlesShort := generateBaseCandles(50)
	if hit := TrendPullback(candlesShort, usParams); hit != nil {
		t.Fatalf("expected nil for < 60 candles")
	}

	candles := buildTrendPullbackCandles()
	hit := TrendPullback(candles, usParams)
	if hit == nil {
		t.Fatalf("expected TrendPullback hit, got nil")
	}
	if hit.Name != "trend_pullback" || hit.DaysAgo != 0 {
		t.Errorf("unexpected hit details: %+v", hit)
	}
	if !CheckTrendPullbackExact(candles, usParams) {
		t.Fatalf("expected CheckTrendPullbackExact true on full fixture")
	}

	t.Run("fails without established uptrend", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		for i := 0; i < len(c); i++ {
			c[i].Close = 100 // flat: MA60 never slopes up
			c[i].Open = 100
			c[i].High = 100.5
			c[i].Low = 99.5
		}
		if CheckTrendPullbackExact(c, usParams) {
			t.Errorf("expected false without an established uptrend")
		}
	})

	t.Run("fails when pulled back too far from MA20", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		c[len(c)-1].Close -= 20 // way beyond PullbackMA20DevPct
		c[len(c)-1].Low = c[len(c)-1].Close - 1
		if CheckTrendPullbackExact(c, usParams) {
			t.Errorf("expected false when deviation from MA20 exceeds threshold")
		}
	})

	t.Run("fails without volume dry-up", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		c[len(c)-1].Volume = 3_000_000 // spike instead of dry-up
		if CheckTrendPullbackExact(c, usParams) {
			t.Errorf("expected false without volume dry-up")
		}
	})

	t.Run("fails without reversal bar", func(t *testing.T) {
		c := append([]data.Candle(nil), candles...)
		last := &c[len(c)-1]
		prev := c[len(c)-2]
		last.Low = prev.Close - 0.2 // no long lower wick
		last.Open = prev.Close - 0.1
		last.Close = prev.Close - 0.15 // small down bar, no engulfing/breakout
		last.High = prev.Close
		if CheckTrendPullbackExact(c, usParams) {
			t.Errorf("expected false without a bullish reversal bar")
		}
	})

	det := NewDetector(i18n.ZH)
	sig1, state1 := det.CheckTrendPullback("TEST", candles, "")
	if sig1 == nil || state1 != "hit" {
		t.Fatalf("expected signal and state='hit', got sig=%v, state=%s", sig1, state1)
	}
	sig2, state2 := det.CheckTrendPullback("TEST", candles, "hit")
	if sig2 != nil || state2 != "hit" {
		t.Fatalf("expected nil signal on repeat hit, got sig=%v, state=%s", sig2, state2)
	}
	sig3, state3 := det.CheckTrendPullback("TEST", generateBaseCandles(100), "hit")
	if sig3 != nil || state3 != "" {
		t.Fatalf("expected cleared state '', got sig=%v, state=%s", sig3, state3)
	}
}

// gapCandles builds 80 flat bars at 100 on 1M volume, then replaces the last
// one with a gap-up bar: opens gapPct above the prior close, trades on
// volMult x the baseline, and closes at closePx. Every 網 6 condition is
// satisfied by default, so each test below can break exactly one.
func gapCandles(gapPct, volMult, closePx, high, low float64) []data.Candle {
	candles := make([]data.Candle, 80)
	now := time.Now()
	for i := 0; i < 80; i++ {
		candles[i] = data.Candle{
			Date:   now.AddDate(0, 0, -80+i),
			Open:   100.0,
			High:   100.0,
			Low:    100.0,
			Close:  100.0,
			Volume: 1_000_000,
		}
	}
	open := 100.0 * (1 + gapPct/100.0)
	candles[79] = data.Candle{
		Date:   now,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closePx,
		Volume: int64(1_000_000 * volMult),
	}
	return candles
}

func TestPostGapDriftSynthetic(t *testing.T) {
	p := DefaultScreenParams(market.US)

	// A clean +7% gap on 4x volume closing near the high, at a new high.
	if !CheckPostGapDriftExact(gapCandles(7, 4, 108, 109, 106), p) {
		t.Fatal("clean gap-up did not trigger")
	}

	// The gap has to clear GapUpPct (US 5.0).
	if CheckPostGapDriftExact(gapCandles(3, 4, 104, 105, 102), p) {
		t.Error("a +3% gap triggered, want no trigger below GapUpPct")
	}

	// ...and be on real volume, not a thin print.
	if CheckPostGapDriftExact(gapCandles(7, 1.5, 108, 109, 106), p) {
		t.Error("1.5x volume triggered, want no trigger below GapVolMult")
	}

	if hit := PostGapDrift(gapCandles(7, 4, 108, 109, 106), p); hit == nil || hit.Name != "post_gap_drift" {
		t.Errorf("PostGapDrift = %+v, want a post_gap_drift hit", hit)
	}
	if hit := PostGapDrift(generateBaseCandles(50), p); hit != nil {
		t.Error("triggered on 50 candles, want nil below the 60-bar minimum")
	}
}

// 好消息出盡: gapped +7% and was sold all the way back down to the low. The
// news is being sold into and the drift runs the other way, so condition 3
// (close in the upper half of the range) has to reject it — otherwise this
// screen is long a stock the tape just told you to be short.
func TestPostGapDriftRejectsASoldGap(t *testing.T) {
	p := DefaultScreenParams(market.US)
	// Opens at 107, runs to 110, closes at 101.5 — below the 105.75 midpoint.
	if CheckPostGapDriftExact(gapCandles(7, 4, 101.5, 110, 101.5), p) {
		t.Error("a gap sold back to the low triggered, want no trigger (好消息出盡)")
	}
}

// A bounce inside a downtrend is not drift. Same gap, same volume, same
// strong close — but the stock is far below where it traded two months ago,
// so condition 4 (new high) has to reject it.
func TestPostGapDriftRejectsABounceInADowntrend(t *testing.T) {
	p := DefaultScreenParams(market.US)
	candles := gapCandles(7, 4, 108, 109, 106)
	// Lift bars 20..40 well above the gap bar's close. They sit inside
	// IsNewHigh's 60-bar window (closes[20:80] for 80 candles), so this is
	// still a +7% gap off yesterday but nowhere near a 60-day high.
	for i := 20; i < 40; i++ {
		candles[i].Open, candles[i].High, candles[i].Low, candles[i].Close = 200, 200, 200, 200
	}
	if CheckPostGapDriftExact(candles, p) {
		t.Error("a gap far below the 60-day high triggered, want no trigger (下跌反彈)")
	}
}

// A TW limit-up locks High == Low == Close at the open. That is the strongest
// possible "held the gap", so the midpoint test must pass it rather than
// divide by a zero range and reject.
func TestPostGapDriftAcceptsALockedLimitUpBar(t *testing.T) {
	p := DefaultScreenParams(market.TW)
	if !CheckPostGapDriftExact(gapCandles(9, 4, 109, 109, 109), p) {
		t.Error("a locked limit-up bar did not trigger, want a trigger")
	}
}

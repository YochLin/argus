package signals

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/market"
)

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

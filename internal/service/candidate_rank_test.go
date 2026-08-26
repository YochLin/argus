package service

import (
	"math"
	"testing"
	"time"

	"argus/internal/data"
)

func TestNormalize01(t *testing.T) {
	if got := Normalize01(nil); len(got) != 0 {
		t.Errorf("Normalize01(nil) = %v, want empty", got)
	}
	equal := Normalize01([]float64{5, 5, 5})
	for _, v := range equal {
		if v != 0.5 {
			t.Errorf("Normalize01(equal) = %v, want all 0.5", equal)
			break
		}
	}
	spread := Normalize01([]float64{10, 20, 30})
	if spread[0] != 0 || spread[2] != 1 || spread[1] != 0.5 {
		t.Errorf("Normalize01(spread) = %v, want [0, 0.5, 1]", spread)
	}
}

// flatCandles builds n days of flat OHLCV at the given close/volume, oldest
// first — enough bars for RelativeStrength(lookback=63) and PriceLevels to
// run without erroring, without needing real market data.
func flatCandles(n int, close float64, volume int64) []data.Candle {
	out := make([]data.Candle, n)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out[i] = data.Candle{
			Date: start.AddDate(0, 0, i),
			Open: close, High: close, Low: close, Close: close,
			Volume: volume,
		}
	}
	return out
}

// rankHistoryStub is a RiskHistoryReader stub keyed by ticker for
// TestRankAndTruncateCandidates below.
type rankHistoryStub struct {
	byTicker map[string][]data.Candle
}

func (s rankHistoryStub) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	return s.byTicker[ticker], nil
}

func TestRankAndTruncateCandidates(t *testing.T) {
	t.Run("no-op under the cap", func(t *testing.T) {
		got := RankAndTruncateCandidates(rankHistoryStub{}, []string{"AAA", "BBB"}, "SPY", 5)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (no-op)", len(got))
		}
	})

	t.Run("truncates to n and keeps only known tickers", func(t *testing.T) {
		bench := flatCandles(120, 100, 1_000_000)
		strong := flatCandles(120, 100, 5_000_000)
		for i := 60; i < len(strong); i++ {
			strong[i].Close = 100 + float64(i-59) // steadily rallies vs. a flat SPY
			strong[i].High, strong[i].Low, strong[i].Open = strong[i].Close, strong[i].Close, strong[i].Close
		}
		weak := flatCandles(120, 50, 10_000) // thin, no momentum

		history := rankHistoryStub{byTicker: map[string][]data.Candle{
			"SPY":    bench,
			"STRONG": strong,
			"WEAK":   weak,
		}}
		got := RankAndTruncateCandidates(history, []string{"STRONG", "WEAK"}, "SPY", 1)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0] != "STRONG" {
			t.Errorf("kept %v, want [STRONG] (higher relative strength + liquidity)", got)
		}
	})

	t.Run("unscoreable ticker (fetch failure) survives with a neutral score, not dropped outright", func(t *testing.T) {
		history := rankHistoryStub{byTicker: map[string][]data.Candle{
			"KNOWN": flatCandles(120, 100, 1_000_000),
			// "MISSING" has no entry -> GetHistory returns nil, nil (unscoreable)
		}}
		got := RankAndTruncateCandidates(history, []string{"KNOWN", "MISSING"}, "SPY", 2)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (both kept, under cap)", len(got))
		}
	})
}

// zigzagCandles oscillates between 90 and 110 on a 20-bar cycle, which gives
// signals.PriceLevels two clean clusters to find (support ~90, resistance
// ~110) with enough touches to survive minLevelTouches.
func zigzagCandles(n int) []data.Candle {
	out := make([]data.Candle, n)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		phase := i % 20
		c := 90 + float64(phase)*2 // 90 -> 108 on the way up
		if phase >= 10 {
			c = 110 - float64(phase-10)*2 // 110 -> 92 on the way down
		}
		out[i] = data.Candle{
			Date: start.AddDate(0, 0, i),
			Open: c, High: c, Low: c, Close: c,
			Volume: 1_000_000,
		}
	}
	return out
}

// A close parked just under overhead resistance must not score as though it
// were sitting on support. Before 2026-08-26 this measured math.Abs to the
// nearest level of either side, so the 5-point gap up to resistance at 110
// (4.8%) won over the 15-point drop to support at 90 (14.3%) — the two
// opposite readings were indistinguishable to the ranker.
func TestNearestSupportDistIgnoresResistance(t *testing.T) {
	candles := zigzagCandles(120)
	for i := len(candles) - 5; i < len(candles); i++ {
		candles[i].Open, candles[i].High, candles[i].Low, candles[i].Close = 105, 105, 105, 105
	}

	got, ok := nearestSupportDist(candles)
	if !ok {
		t.Fatal("ok = false, want the support cluster at ~90 to be found")
	}
	if want := (105.0 - 90.0) / 105.0; math.Abs(got-want) > 0.02 {
		t.Errorf("dist = %.4f, want ~%.4f (distance down to support, not up to resistance at 110)", got, want)
	}
}

// No level below the close is missing information, not a bad level: the
// caller scores ok=false the same neutral 0.5 as an unscoreable ticker.
func TestNearestSupportDistNoSupportBelow(t *testing.T) {
	candles := zigzagCandles(120)
	for i := len(candles) - 5; i < len(candles); i++ {
		candles[i].Open, candles[i].High, candles[i].Low, candles[i].Close = 60, 60, 60, 60
	}
	if _, ok := nearestSupportDist(candles); ok {
		t.Error("ok = true, want false — every level sits above a close of 60")
	}
	if _, ok := nearestSupportDist(nil); ok {
		t.Error("ok = true for no candles, want false")
	}
}

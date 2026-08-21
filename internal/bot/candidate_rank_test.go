package bot

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/market"
)

func TestNormalize01(t *testing.T) {
	if got := normalize01(nil); len(got) != 0 {
		t.Errorf("normalize01(nil) = %v, want empty", got)
	}
	equal := normalize01([]float64{5, 5, 5})
	for _, v := range equal {
		if v != 0.5 {
			t.Errorf("normalize01(equal) = %v, want all 0.5", equal)
			break
		}
	}
	spread := normalize01([]float64{10, 20, 30})
	if spread[0] != 0 || spread[2] != 1 || spread[1] != 0.5 {
		t.Errorf("normalize01(spread) = %v, want [0, 0.5, 1]", spread)
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

// rankHistoryStub is a data.HistoryProvider stub keyed by ticker for
// TestRankAndTruncateCandidates below.
type rankHistoryStub struct {
	byTicker map[string][]data.Candle
}

func (s rankHistoryStub) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	return s.byTicker[ticker], nil
}

func TestRankAndTruncateCandidates(t *testing.T) {
	b := &Bot{}

	t.Run("no-op under the cap", func(t *testing.T) {
		b.history = rankHistoryStub{}
		got := b.rankAndTruncateCandidates([]string{"AAA", "BBB"}, market.US, 5)
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

		b.history = rankHistoryStub{byTicker: map[string][]data.Candle{
			"SPY":    bench,
			"STRONG": strong,
			"WEAK":   weak,
		}}
		got := b.rankAndTruncateCandidates([]string{"STRONG", "WEAK"}, market.US, 1)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0] != "STRONG" {
			t.Errorf("kept %v, want [STRONG] (higher relative strength + liquidity)", got)
		}
	})

	t.Run("unscoreable ticker (fetch failure) survives with a neutral score, not dropped outright", func(t *testing.T) {
		b.history = rankHistoryStub{byTicker: map[string][]data.Candle{
			"KNOWN": flatCandles(120, 100, 1_000_000),
			// "MISSING" has no entry -> GetHistory returns nil, nil (unscoreable)
		}}
		got := b.rankAndTruncateCandidates([]string{"KNOWN", "MISSING"}, market.US, 2)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (both kept, under cap)", len(got))
		}
	})
}

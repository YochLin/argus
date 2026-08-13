package signals

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/market"
)

func genTrustCandles(n int) []data.Candle {
	candles := make([]data.Candle, n)
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = data.Candle{
			Date: base.AddDate(0, 0, i),
			Open: 100, High: 101, Low: 99, Close: 100,
			Volume: 1_000_000,
		}
	}
	return candles
}

// TestCheckTrustAccumulationExact covers docs/phase-15-trust-follow.md
// §4.6's synthetic cases — the dormant-segment check (case 3) is the one
// that stands in for the textbook's unavailable holding-ratio condition, so
// it gets its own case rather than being folded into the others.
func TestCheckTrustAccumulationExact(t *testing.T) {
	p := DefaultScreenParams(market.TW) // TrustLookback 60, ConsecMin/Max 3/8, VolPctMin 1.0, DormantPctMax 0.5

	t.Run("dormant then 4-day buying streak at ~2% of volume hits", func(t *testing.T) {
		candles := genTrustCandles(60)
		trustNet := make([]int64, 60)
		for i := 56; i < 60; i++ {
			trustNet[i] = 300_000 // 4 days x 300k = 1.2M / 60M volume = 2%
		}
		if !CheckTrustAccumulationExact(candles, trustNet, p) {
			t.Fatal("expected a hit: dormant lookback + fresh buying streak within magnitude")
		}
	})

	t.Run("15-day streak exceeds TrustConsecMax, no hit", func(t *testing.T) {
		candles := genTrustCandles(60)
		trustNet := make([]int64, 60)
		for i := 45; i < 60; i++ {
			trustNet[i] = 300_000 // 15 consecutive days
		}
		if CheckTrustAccumulationExact(candles, trustNet, p) {
			t.Fatal("expected no hit: streak longer than TrustConsecMax means already accumulating, not first buy")
		}
	})

	t.Run("dormant segment already at 3pct of volume, no hit", func(t *testing.T) {
		candles := genTrustCandles(60)
		trustNet := make([]int64, 60)
		trustNet[10] = 1_560_000 // single dormant-window buy = 3% of the 52-day dormant window's volume
		for i := 56; i < 60; i++ {
			trustNet[i] = 300_000 // fresh 4-day streak, otherwise passes
		}
		if CheckTrustAccumulationExact(candles, trustNet, p) {
			t.Fatal("expected no hit: dormant segment already shows accumulation, not a first-time build")
		}
	})

	t.Run("streak hits but price already extended off MA20, no hit", func(t *testing.T) {
		candles := genTrustCandles(60)
		candles[59].Close = 130
		candles[59].High = 131
		trustNet := make([]int64, 60)
		for i := 56; i < 60; i++ {
			trustNet[i] = 300_000
		}
		if CheckTrustAccumulationExact(candles, trustNet, p) {
			t.Fatal("expected no hit: price too far above MA20 means already pumped, not a fresh entry")
		}
	})
}

// TestAlignTrustNet covers §4.6's two most important cases: alignment is
// silent when it's wrong, so a mismatch here doesn't error, it just
// quietly changes the backtest numbers.
func TestAlignTrustNet(t *testing.T) {
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("rows missing 3 days: output length matches candles, missing days are 0", func(t *testing.T) {
		candles := make([]data.Candle, 10)
		for i := 0; i < 10; i++ {
			candles[i] = data.Candle{Date: base.AddDate(0, 0, i)}
		}
		var rows []data.TrustNetDay
		for i := 0; i < 10; i++ {
			if i == 2 || i == 5 || i == 8 { // 3 missing days
				continue
			}
			rows = append(rows, data.TrustNetDay{Date: base.AddDate(0, 0, i), Net: int64(i + 1)})
		}

		got := AlignTrustNet(candles, rows)
		if len(got) != len(candles) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(candles))
		}
		for _, missing := range []int{2, 5, 8} {
			if got[missing] != 0 {
				t.Errorf("got[%d] = %d, want 0 for a day with no row", missing, got[missing])
			}
		}
		if got[0] != 1 || got[9] != 10 {
			t.Errorf("present days = %v, want got[0]=1 got[9]=10", got)
		}
	})

	t.Run("rows have a date candles don't: ignored, no shift", func(t *testing.T) {
		candles := make([]data.Candle, 5)
		for i := 0; i < 5; i++ {
			candles[i] = data.Candle{Date: base.AddDate(0, 0, i)}
		}
		rows := []data.TrustNetDay{
			{Date: base.AddDate(0, 0, 1), Net: 100},
			{Date: base.AddDate(0, 0, 3), Net: 300},
			{Date: base.AddDate(0, 0, 99), Net: 999}, // not a candle date at all
		}

		got := AlignTrustNet(candles, rows)
		want := []int64{0, 100, 0, 300, 0}
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %d, want %d (extra row must not shift indices)", i, got[i], want[i])
			}
		}
	})
}

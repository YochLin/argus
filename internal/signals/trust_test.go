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

// genTrustCandlesTrending is a mild linear uptrend (unlike genTrustCandles'
// flat price) so MA60 actually slopes up — CheckTrustFollowExact's trend
// filter (condition 4) needs that, and a flat series makes ma60Today ==
// ma60Past, which fails it by construction.
func genTrustCandlesTrending(n int) []data.Candle {
	candles := make([]data.Candle, n)
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		price := 80 + float64(i)*0.2
		candles[i] = data.Candle{
			Date: base.AddDate(0, 0, i),
			Open: price, High: price + 1, Low: price - 1, Close: price,
			Volume: 1_000_000,
		}
	}
	return candles
}

// TestCheckTrustFollowExact covers Phase 15 v2's short-window redesign
// (docs/phase-15-trust-follow.md) — each case isolates one of the six
// conditions in CheckTrustFollowExact so a regression in one doesn't hide
// behind another passing by coincidence.
func TestCheckTrustFollowExact(t *testing.T) {
	p := DefaultScreenParams(market.TW) // MinAvgVolume5d 1e6, TrustNetVolPctMin 3.0, ForeignSellVolPctMax -3.0, MaxMA20DevPct 12.0

	t.Run("3-day buy streak at 5pct intensity in an uptrend hits", func(t *testing.T) {
		candles := genTrustCandlesTrending(70)
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		for i := 67; i < 70; i++ {
			trustNet[i] = 50_000 // 3 x 50k = 150k / 3M volume = 5%
		}
		if !CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected a hit: 3-day buy window + intensity + uptrend all pass")
		}
	})

	t.Run("no trust buying, no hit", func(t *testing.T) {
		candles := genTrustCandlesTrending(70)
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		if CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected no hit: neither the 3-day sum nor the 5-day count condition is met")
		}
	})

	t.Run("3-day sum negative but 3-of-5 days positive falls back to the 5-day window", func(t *testing.T) {
		candles := genTrustCandlesTrending(70)
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		trustNet[65] = 400_000  // day -5, positive
		trustNet[66] = 400_000  // day -4, positive
		trustNet[67] = -100_000 // day -3
		trustNet[68] = -100_000 // day -2
		trustNet[69] = 50_000   // day -1, positive -> 3 of last 5 positive
		// trailing 3-day sum = -100k -100k +50k = -150k <= 0, so the 3-day
		// condition alone must not trigger this.
		if !CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected a hit via the 5-day 3-of-5 alternate condition")
		}
	})

	t.Run("flat price: MA60 not sloping up, no hit", func(t *testing.T) {
		candles := genTrustCandles(70) // flat, ma60Today == ma60Past
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		for i := 67; i < 70; i++ {
			trustNet[i] = 50_000
		}
		if CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected no hit: trend filter requires MA60 sloping up, flat price doesn't qualify")
		}
	})

	t.Run("price extended too far off MA20, no hit", func(t *testing.T) {
		candles := genTrustCandlesTrending(70)
		candles[69].Close = 130
		candles[69].High = 131
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		for i := 67; i < 70; i++ {
			trustNet[i] = 50_000
		}
		if CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected no hit: deviation from MA20 exceeds MaxMA20DevPct")
		}
	})

	t.Run("土洋對作: heavy foreign same-day sell excludes an otherwise-valid hit", func(t *testing.T) {
		candles := genTrustCandlesTrending(70)
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		for i := 67; i < 70; i++ {
			trustNet[i] = 50_000
		}
		foreignNet[69] = -50_000 // -5% of today's 1M volume, breaches -3% floor
		if CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected no hit: foreign investors selling heavily into the same tape")
		}
	})

	// pullbackCandles is a run that accelerated (0.5/bar from bar 50) and has
	// since been pulling back for the last few bars: MA60 still up, close
	// still above MA20 and MA60, but MA5 falling. A plain linear trend cannot
	// express this — there MA20 sits at the midpoint of its window, so "close
	// below its own 5 bars ago" and "close above MA20" are near-exclusive and
	// a fixture built that way fails on the MA20 leg instead of the one under
	// test.
	pullbackCandles := func(n, pullbackBars int) []data.Candle {
		candles := genTrustCandlesTrending(n)
		price := candles[49].Close
		for i := 50; i < n; i++ {
			if i < n-pullbackBars {
				price += 0.5
			} else {
				price -= 0.4
			}
			candles[i].Close, candles[i].Open = price, price
			candles[i].High, candles[i].Low = price+1, price-1
		}
		return candles
	}

	t.Run("above MA60 but short MA falling, no hit", func(t *testing.T) {
		candles := pullbackCandles(70, 5)
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		for i := 67; i < 70; i++ {
			trustNet[i] = 50_000
		}
		if CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected no hit: MA5/MA10 falling means the entry is mid-pullback")
		}
	})

	// The whole point of the episode loop: a rejected buying run must STAY
	// rejected, not slip through on the next bar whose short MA ticks up.
	// Without it the filter postpones the entry instead of dropping it, which
	// measured worse than not filtering at all — see CheckTrustFollowExact.
	t.Run("rejected episode does not re-enter once the short MA ticks up", func(t *testing.T) {
		candles := pullbackCandles(72, 7)
		// Last two bars turn back up, so the evaluated bar on its own looks
		// fine — but the episode began during the pullback.
		for i := 70; i < 72; i++ {
			price := candles[69].Close + float64(i-69)*1.2
			candles[i].Close, candles[i].Open = price, price
			candles[i].High, candles[i].Low = price+1, price-1
		}
		trustNet := make([]int64, 72)
		foreignNet := make([]int64, 72)
		for i := 66; i < 72; i++ { // 投信 buying straight through the pullback
			trustNet[i] = 50_000
		}
		if !shortMARising(data.Closes(candles)) {
			t.Fatal("fixture is wrong: the evaluated bar itself must look rising, or this proves nothing")
		}
		if CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected no hit: the buying episode started mid-pullback, so it stays rejected")
		}
	})

	t.Run("thin liquidity, no hit", func(t *testing.T) {
		candles := genTrustCandlesTrending(70)
		for i := 65; i < 70; i++ {
			candles[i].Volume = 500_000 // below MinAvgVolume5d
		}
		trustNet := make([]int64, 70)
		foreignNet := make([]int64, 70)
		for i := 67; i < 70; i++ {
			trustNet[i] = 50_000
		}
		if CheckTrustFollowExact(candles, trustNet, foreignNet, p) {
			t.Fatal("expected no hit: 5-day avg volume below MinAvgVolume5d")
		}
	})
}

// TestAlignTrustNet covers alignment's two most important cases: alignment
// is silent when it's wrong, so a mismatch here doesn't error, it just
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

// TestAlignForeignNet confirms AlignForeignNet reads ForeignNet (not Net) —
// the one thing that's easy to get backwards after the alignByDate refactor.
func TestAlignForeignNet(t *testing.T) {
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]data.Candle, 3)
	for i := 0; i < 3; i++ {
		candles[i] = data.Candle{Date: base.AddDate(0, 0, i)}
	}
	rows := []data.TrustNetDay{
		{Date: base.AddDate(0, 0, 0), Net: 100, ForeignNet: -900},
		{Date: base.AddDate(0, 0, 1), Net: 200, ForeignNet: -800},
		{Date: base.AddDate(0, 0, 2), Net: 300, ForeignNet: -700},
	}

	got := AlignForeignNet(candles, rows)
	want := []int64{-900, -800, -700}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

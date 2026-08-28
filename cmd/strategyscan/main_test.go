package main

import (
	"math"
	"reflect"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/paper"
	"argus/internal/signals"
)

var defaultExitCfg = paper.Config{
	StopATRMult:       2,
	StopLossPct:       10,
	TrailingPct:       15,
	TrailingATRMult:   0,
	TakeProfitATRMult: 0,
	Market:            market.US,
	FeeDiscount:       1.0,
}

// flatThenCandles builds a flat run of n days at flatClose (so ATR(14) stays
// 0 through entry, exercising simulateTrade's/paper.Account's fixed-%
// fallback path deterministically), then appends extra day-close values.
func flatThenCandles(n int, flatClose float64, extraCloses ...float64) []data.Candle {
	out := make([]data.Candle, 0, n+len(extraCloses))
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out = append(out, data.Candle{Date: start.AddDate(0, 0, i), Open: flatClose, High: flatClose, Low: flatClose, Close: flatClose, Volume: 1000})
	}
	for i, c := range extraCloses {
		out = append(out, data.Candle{Date: start.AddDate(0, 0, n+i), Open: c, High: c, Low: c, Close: c, Volume: 1000})
	}
	return out
}

func TestSimulateTrade_Stop(t *testing.T) {
	entryIdx := 19
	candles := flatThenCandles(entryIdx+1, 100, 85) // flat @100 fallback stop = 90, then a drop to 85
	out, ok := simulateTrade(candles, entryIdx, defaultExitCfg, 0, 60)
	if !ok {
		t.Fatal("simulateTrade returned ok=false, want a trade")
	}
	if out.ExitReason != "stop" {
		t.Errorf("ExitReason = %q, want %q", out.ExitReason, "stop")
	}
	if out.HoldDays != 1 {
		t.Errorf("HoldDays = %d, want 1", out.HoldDays)
	}
	// raw price return is exactly -15%; US has no modeled commission and 0
	// slippage was passed, so friction should be exactly zero here.
	if out.ExitRet != -15.0 {
		t.Errorf("ExitRet = %.4f, want exactly -15.0 (US: no commission, no slippage)", out.ExitRet)
	}
}

func TestSimulateTrade_TWFeesWidenCost(t *testing.T) {
	entryIdx := 19
	candles := flatThenCandles(entryIdx+1, 100, 85)
	twCfg := defaultExitCfg
	twCfg.Market = market.TW
	out, ok := simulateTrade(candles, entryIdx, twCfg, 0, 60)
	if !ok {
		t.Fatal("simulateTrade returned ok=false, want a trade")
	}
	// TW charges commission on both legs plus a 0.3% sell-side tax — must
	// make the realized return worse than the raw -15% price move.
	if out.ExitRet >= -15.0 {
		t.Errorf("ExitRet = %.4f, want < -15.0 (TW commission+tax must worsen the raw -15%% move)", out.ExitRet)
	}
}

func TestSimulateTrade_Trailing(t *testing.T) {
	entryIdx := 19
	candles := flatThenCandles(entryIdx+1, 100, 120, 102) // rally to a 120 peak, then a 15% drawdown to 102
	out, ok := simulateTrade(candles, entryIdx, defaultExitCfg, 0, 60)
	if !ok {
		t.Fatal("simulateTrade returned ok=false, want a trade")
	}
	if out.ExitReason != "trailing" {
		t.Errorf("ExitReason = %q, want %q", out.ExitReason, "trailing")
	}
	if out.HoldDays != 2 {
		t.Errorf("HoldDays = %d, want 2", out.HoldDays)
	}
}

func TestSimulateTrade_Timeout(t *testing.T) {
	entryIdx := 19
	candles := flatThenCandles(entryIdx+1, 100, 101, 102, 103) // drifts up slightly, never triggers stop/target/trailing
	out, ok := simulateTrade(candles, entryIdx, defaultExitCfg, 0, 3)
	if !ok {
		t.Fatal("simulateTrade returned ok=false, want a trade")
	}
	if out.ExitReason != "timeout" {
		t.Errorf("ExitReason = %q, want %q", out.ExitReason, "timeout")
	}
	if out.HoldDays != 3 {
		t.Errorf("HoldDays = %d, want 3 (maxHoldDays)", out.HoldDays)
	}
}

func TestSimulateTrade_SlippageWidensCost(t *testing.T) {
	entryIdx := 19
	candles := flatThenCandles(entryIdx+1, 100, 85)
	noSlip, _ := simulateTrade(candles, entryIdx, defaultExitCfg, 0, 60)
	withSlip, _ := simulateTrade(candles, entryIdx, defaultExitCfg, 0.5, 60)
	if withSlip.ExitRet >= noSlip.ExitRet {
		t.Errorf("ExitRet with 0.5%% slippage/side (%.4f) should be lower than with none (%.4f)", withSlip.ExitRet, noSlip.ExitRet)
	}
}

func TestParseDevSweep(t *testing.T) {
	base := signals.DefaultScreenParams(market.US) // MaxMA20DevPct 15
	got, err := parseDevSweep(" 8, 10 ,", base)
	if err != nil {
		t.Fatalf("parseDevSweep: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 variants, got %d", len(got))
	}
	if got[0].name != "trend_breakout_dev8" || got[0].params.MaxMA20DevPct != 8 {
		t.Errorf("variant 0 = %+v", got[0])
	}
	// Every other knob must be inherited, not reset — a variant that silently
	// dropped e.g. MinAvgVolume5d would screen a different population and its
	// excess would be uninterpretable.
	if got[1].params.MinAvgVolume5d != base.MinAvgVolume5d || got[1].params.BreakoutVolMA20 != base.BreakoutVolMA20 {
		t.Errorf("variant 1 lost inherited params: %+v", got[1].params)
	}
	if v, err := parseDevSweep("", base); err != nil || v != nil {
		t.Errorf("empty sweep: got %v, %v", v, err)
	}
	if _, err := parseDevSweep("abc", base); err == nil {
		t.Error("want error on non-numeric sweep value")
	}
	if _, err := parseDevSweep("0", base); err == nil {
		t.Error("want error on non-positive sweep value")
	}
}

func TestSplitRegime(t *testing.T) {
	bull, bear := splitRegime([]TriggerRecord{
		{MarketRegime: "bull"}, {MarketRegime: "bear"}, {MarketRegime: "bull"},
	})
	if len(bull) != 2 || len(bear) != 1 {
		t.Fatalf("want 2 bull / 1 bear, got %d / %d", len(bull), len(bear))
	}
}

func TestParseHoldSweep(t *testing.T) {
	got, err := parseHoldSweep(" 20, 60 ,20, 10 ")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if want := []int{20, 60, 10}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (input order kept, duplicate dropped)", got, want)
	}
	if got, err := parseHoldSweep(""); got != nil || err != nil {
		t.Errorf("empty = (%v, %v), want (nil, nil) — the sweep is off by default", got, err)
	}
	for _, bad := range []string{"0", "-5", "20,abc"} {
		if _, err := parseHoldSweep(bad); err == nil {
			t.Errorf("parseHoldSweep(%q) = nil error, want one", bad)
		}
	}
}

// sweepCandles builds a deterministic path from per-day percentage moves.
func sweepCandles(moves []float64) []data.Candle {
	price := 100.0
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]data.Candle, 0, len(moves)+1)
	out = append(out, data.Candle{Date: start, Open: price, High: price, Low: price, Close: price, Volume: 1_000_000})
	for i, mv := range moves {
		price *= 1 + mv/100
		out = append(out, data.Candle{
			Date: start.AddDate(0, 0, i+1),
			Open: price, High: price, Low: price, Close: price,
			Volume: 1_000_000,
		})
	}
	return out
}

// The multi-horizon replay must agree exactly with the single-horizon one it
// replaced, for every horizon and on both branches (a rule exit fired, and a
// forced close at the horizon). One replay serving many horizons is only
// worth having if it is the same replay.
func TestSimulateTradeHorizonsMatchesSingleHorizon(t *testing.T) {
	cfg := paper.Config{StopATRMult: 2, StopLossPct: 10, TrailingPct: 18, Market: market.US}
	holds := []int{5, 10, 20, 40, 60}

	paths := map[string][]float64{
		"grind up (forced close at every horizon)": repeatMove(+0.4, 80),
		"straight down (stop fires early)":         repeatMove(-1.5, 80),
		"up then collapse (trailing fires late)":   append(repeatMove(+1.2, 40), repeatMove(-2.0, 40)...),
	}

	for name, moves := range paths {
		t.Run(name, func(t *testing.T) {
			candles := sweepCandles(moves)
			multi := simulateTradeHorizons(candles, 0, cfg, 0.1, holds)
			for _, h := range holds {
				want, wantOK := simulateTrade(candles, 0, cfg, 0.1, h)
				got, gotOK := multi[h]
				if gotOK != wantOK {
					t.Fatalf("h=%d: ok = %v, want %v", h, gotOK, wantOK)
				}
				if !wantOK {
					continue
				}
				if math.Abs(got.ExitRet-want.ExitRet) > 1e-9 || got.ExitReason != want.ExitReason || got.HoldDays != want.HoldDays {
					t.Errorf("h=%d: got %+v, want %+v", h, got, want)
				}
			}
		})
	}
}

func repeatMove(pct float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = pct
	}
	return out
}

func TestParseFloatList(t *testing.T) {
	got, err := parseFloatList(" 1, 2.5 ,4 ")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if want := []float64{1, 2.5, 4}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, err := parseFloatList(""); got != nil || err != nil {
		t.Errorf("empty = (%v, %v), want (nil, nil)", got, err)
	}
	for _, bad := range []string{"0", "-1", "2,x"} {
		if _, err := parseFloatList(bad); err == nil {
			t.Errorf("parseFloatList(%q) = nil error, want one", bad)
		}
	}
}

// The stop-width study divides by (Entry-Stop), so a replay that reports a
// stop it didn't actually use would silently produce plausible R values
// off the wrong denominator. Pin that the reported stop is the one the ATR
// multiple asks for, and that it scales with the multiple.
func TestSimulateTradeReportsTheStopItUsed(t *testing.T) {
	candles := sweepCandles(append(repeatMove(+1.0, 30), repeatMove(-1.2, 50)...))

	var prevDist float64
	for _, mult := range []float64{1, 2, 3, 4} {
		cfg := paper.Config{StopATRMult: mult, StopLossPct: 10, TrailingPct: 18, Market: market.US}
		o, ok := simulateTrade(candles, 20, cfg, 0.1, 60)
		if !ok {
			t.Fatalf("mult=%g: no trade", mult)
		}
		if o.Entry <= 0 || o.Stop <= 0 || o.Stop >= o.Entry {
			t.Fatalf("mult=%g: entry=%g stop=%g, want 0 < stop < entry", mult, o.Entry, o.Stop)
		}
		dist := o.Entry - o.Stop
		if prevDist > 0 {
			// Each step here is +1 ATR, so the gap grows by a constant.
			if step := dist - prevDist; step <= 0 {
				t.Errorf("mult=%g: stop distance %g did not widen from %g", mult, dist, prevDist)
			}
		}
		prevDist = dist
	}
}

// A stopped-out trade must lose at least its full unit of risk: R <= -1
// before friction, and strictly worse after. If this ever passes with
// R > -1 the replay is exiting above its own stop.
func TestStoppedOutTradeLosesAtLeastOneR(t *testing.T) {
	candles := sweepCandles(repeatMove(-1.5, 60))
	cfg := paper.Config{StopATRMult: 2, StopLossPct: 10, TrailingPct: 18, Market: market.US}

	o, ok := simulateTrade(candles, 20, cfg, 0.1, 60)
	if !ok {
		t.Fatal("no trade")
	}
	if o.ExitReason != "stop" {
		t.Fatalf("ExitReason = %q, want stop on a straight decline", o.ExitReason)
	}
	stopPct := (o.Entry - o.Stop) / o.Entry * 100
	if r := o.ExitRet / stopPct; r > -1 {
		t.Errorf("R = %.3f, want <= -1 (a stop-out loses its risk unit plus friction)", r)
	}
}

// The T+1 variant's whole point is that it is scored on the bar it actually
// entered. fillForwardReturns is what makes that true, so check it moves with
// idx rather than silently reusing the signal bar — and that it reports
// "no data" instead of a fake 0 when the horizon runs off the end.
func TestFillForwardReturnsMovesWithTheEntryBar(t *testing.T) {
	// 70 bars rising 1% a day, so a 5-day forward return is ~5% from any bar
	// and the entry price differs between consecutive bars.
	candles := make([]data.Candle, 70)
	px := 100.0
	for i := range candles {
		candles[i] = data.Candle{
			Date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: px, High: px, Low: px, Close: px, Volume: 1_000_000,
		}
		px *= 1.01
	}
	idxByDate := map[string]int{}
	for i, c := range candles {
		idxByDate[c.Date.Format("2006-01-02")] = i
	}

	var atSignal, atNextBar TriggerRecord
	fillForwardReturns(&atSignal, 60, candles, candles, idxByDate)
	fillForwardReturns(&atNextBar, 61, candles, candles, idxByDate)
	if !atSignal.Has5d || !atNextBar.Has5d {
		t.Fatal("Has5d = false, want both bars to have a 5-day forward return")
	}
	if atSignal.Ret5d == atNextBar.Ret5d {
		// Equal here would mean idx is being ignored — the exact bug the T+1
		// variant would hide, since it would silently score T+0's returns.
		t.Errorf("both bars returned %.4f%%, want different bars to differ", atSignal.Ret5d)
	}

	// 20 days past bar 60 runs off the end of a 70-bar series.
	if atSignal.Has20d {
		t.Error("Has20d = true past the end of the data, want false (not a 0 return)")
	}
}

// TestUniverseTickers checks each embedded universe parses to a plausible
// ticker count (guards against a truncated/corrupt embed) and that an
// unrecognized -universe value falls back to the S&P 500 default, matching
// universeTickers' documented contract (validation of "must be a known
// value" lives at the main() call site, not here).
func TestUniverseTickers(t *testing.T) {
	cases := []struct {
		universe string
		wantMin  int
		wantMax  int
	}{
		{"", 490, 510},      // S&P 500 default
		{"bogus", 490, 510}, // unrecognized -> same S&P 500 default
		{"sp400", 390, 410}, // S&P 400 mid-cap
		{"sp600", 550, 650}, // S&P 600 small-cap (SPSM live count: 602)
	}
	for _, c := range cases {
		got := universeTickers(c.universe)
		if len(got) < c.wantMin || len(got) > c.wantMax {
			t.Errorf("universeTickers(%q) = %d tickers, want between %d and %d", c.universe, len(got), c.wantMin, c.wantMax)
		}
	}
}

// TestPullbackEntryIdx covers §8.4①'s legal outcomes: confirms the very
// next bar when price breaches the signal low (and MA5 agrees), never
// confirms with a zero-length window, and never confirms across a
// monotonic uptrend that touches neither condition.
func TestPullbackEntryIdx(t *testing.T) {
	// Five flat 100-closes so MA5 is well-defined at the signal bar, then a
	// signal bar (idx 5, close=100, low=99), then a close (98) that breaches
	// both the signal low and MA5(~99.6) on the very next bar.
	candles := flatThenCandles(0, 0)
	closes := []float64{100, 100, 100, 100, 100, 100 /* signal, low=99 */, 98}
	for i, c := range closes {
		low := c
		if i == 5 {
			low = 99
		}
		candles = append(candles, data.Candle{
			Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: c, High: c, Low: low, Close: c, Volume: 1000,
		})
	}
	signalIdx := 5

	if idx, ok := pullbackEntryIdx(candles, signalIdx, 10); !ok || idx != signalIdx+1 {
		t.Errorf("confirms next bar: got (%d, %v), want (%d, true)", idx, ok, signalIdx+1)
	}
	if idx, ok := pullbackEntryIdx(candles, signalIdx, 0); ok {
		t.Errorf("zero-length window: got (%d, true), want ok=false", idx)
	}

	// A steady uptrend never pulls back to MA5 or the signal low (both
	// always below the current close in a monotonic rise).
	upCandles := flatThenCandles(0, 0)
	for i := 0; i < 12; i++ {
		c := 100.0 + float64(i)
		upCandles = append(upCandles, data.Candle{
			Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 1000,
		})
	}
	if idx, ok := pullbackEntryIdx(upCandles, 4, 6); ok {
		t.Errorf("monotonic uptrend, no pullback: got (%d, true), want ok=false", idx)
	}
}

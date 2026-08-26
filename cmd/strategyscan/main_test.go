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

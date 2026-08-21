package main

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/paper"
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

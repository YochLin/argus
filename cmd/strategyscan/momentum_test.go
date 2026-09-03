package main

import (
	"testing"

	"argus/internal/market"
)

// buildMomentumPool returns n tickers of length days, all sharing the same
// calendar (sweepCandles always starts from the same date). "WIN" grinds up
// the whole way (strong positive 12-1 momentum); the rest grind down
// slightly (negative momentum) — a pool small enough (momentumDecile=10)
// that the top decile is exactly one ticker, so a correct implementation has
// no ambiguity about which one it must pick.
func buildMomentumPool(days int) map[string]tickerHist {
	hists := map[string]tickerHist{
		"WIN": newTickerHist(sweepCandles(repeatMove(0.5, days))),
	}
	for i := 0; i < 9; i++ {
		hists[string(rune('A'+i))] = newTickerHist(sweepCandles(repeatMove(-0.1, days)))
	}
	return hists
}

func TestMomentumAt_RequiresFullLookback(t *testing.T) {
	h := newTickerHist(sweepCandles(repeatMove(0.5, 300)))
	if _, ok := momentumAt(h, momentumLookback-1); ok {
		t.Error("momentumAt with idx just under momentumLookback: want ok=false")
	}
	if _, ok := momentumAt(h, momentumLookback); !ok {
		t.Error("momentumAt with idx == momentumLookback: want ok=true")
	}
}

// TestRebalanceTargets_PicksTopDecile pins §8.5's ranking mechanism: given a
// 10-ticker pool where exactly one ticker has a strong uptrend and the rest
// grind down, rebalanceTargets' top decile (n=1) must be that ticker, and
// only once it has momentumLookback days of history — before that, the pool
// is entirely ineligible (nil, a legal "stay in cash" result).
func TestRebalanceTargets_PicksTopDecile(t *testing.T) {
	hists := buildMomentumPool(400)
	winCandles := hists["WIN"].candles

	tooEarly := winCandles[momentumLookback-1].Date.Format("2006-01-02")
	if got := rebalanceTargets(hists, tooEarly); got != nil {
		t.Errorf("rebalanceTargets before any ticker has full history = %v, want nil", got)
	}

	rebalanceDate := winCandles[300].Date.Format("2006-01-02")
	got := rebalanceTargets(hists, rebalanceDate)
	if len(got) != 1 || got[0] != "WIN" {
		t.Fatalf("rebalanceTargets(%s) = %v, want exactly [WIN]", rebalanceDate, got)
	}
}

// TestRunMomentumBacktest_HoldsTheWinner is a smoke test for the full
// monthly-rebalance replay: once the warm-up period passes, the strategy
// should end up holding (and profiting from) the pool's one uptrending
// ticker, while the equal-weight buy-and-hold control (no ranking, no
// warm-up) stays invested in the whole pool from day one.
func TestRunMomentumBacktest_HoldsTheWinner(t *testing.T) {
	hists := buildMomentumPool(500)
	bench := hists["WIN"].candles

	strat := runMomentumBacktest(hists, bench, 100000, market.US, "", "")
	ctrl := runEqualWeightBuyHold(hists, bench, 100000, market.US, "", "")

	if len(strat.Curve) != len(bench) || len(ctrl.Curve) != len(bench) {
		t.Fatalf("curve length: strat=%d ctrl=%d, want %d (every bench day walked)",
			len(strat.Curve), len(ctrl.Curve), len(bench))
	}

	last := strat.Curve[len(strat.Curve)-1]
	if last.PositionsHeld != 1 {
		t.Errorf("final PositionsHeld = %d, want 1 (should be holding just WIN post warm-up)", last.PositionsHeld)
	}
	if strat.FinalEquity <= 100000 {
		t.Errorf("strategy FinalEquity = %.2f, want > 100000 (WIN grinds up the whole path)", strat.FinalEquity)
	}

	firstCtrl := ctrl.Curve[0]
	if firstCtrl.PositionsHeld != 10 {
		t.Errorf("control PositionsHeld on day 1 = %d, want 10 (buys the whole pool immediately, no warm-up)", firstCtrl.PositionsHeld)
	}
}

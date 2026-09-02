package main

import (
	"math"
	"testing"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/paper"
)

// benchPath builds a synthetic SPY-like series: calm days of tiny
// alternating moves, then volatile days of large alternating moves — enough
// history for volMultiplierAt to see a trailing-1y pool with a clear
// low-vol/high-vol split, so tests below exercise a genuinely non-neutral
// multiplier instead of trivially getting 1.0 from too little history.
func benchPath(calm, volatile int) []data.Candle {
	moves := make([]float64, 0, calm+volatile)
	for i := 0; i < calm; i++ {
		if i%2 == 0 {
			moves = append(moves, 0.05)
		} else {
			moves = append(moves, -0.05)
		}
	}
	for i := 0; i < volatile; i++ {
		if i%2 == 0 {
			moves = append(moves, 3.0)
		} else {
			moves = append(moves, -3.0)
		}
	}
	return sweepCandles(moves)
}

func TestVolMultiplierAt_BoundsAndNeutralDefault(t *testing.T) {
	bench := benchPath(280, 40)
	rv20 := computeRV20(data.Closes(bench))

	if m := volMultiplierAt(rv20, 100); m != 1.0 {
		t.Errorf("with < volPctLookback history, want neutral 1.0, got %v", m)
	}
	if m := volMultiplierAt(rv20, -1); m != 1.0 {
		t.Errorf("out-of-range index, want neutral 1.0, got %v", m)
	}

	// Deep into the volatile tail, recent vol should rank at/near the top of
	// its trailing pool, so the multiplier should sit at/near the floor.
	last := len(bench) - 1
	if m := volMultiplierAt(rv20, last); m >= 1.0 || m < volMultFloor-1e-9 {
		t.Errorf("volMultiplierAt at the volatile tail = %v, want in [%v, 1.0)", m, volMultFloor)
	}

	// Every in-range value must stay inside the pre-registered band.
	for i := range rv20 {
		if m := volMultiplierAt(rv20, i); m < volMultFloor-1e-9 || m > volMultCeil+1e-9 {
			t.Fatalf("volMultiplierAt(%d) = %v, out of [%v, %v]", i, m, volMultFloor, volMultCeil)
		}
	}
}

// TestPortfolioBacktest_VolTargetInvariant pins docs/phase-25 §3.5's
// pre-registered invariant: flipping -vol-target must never change WHICH
// trades happen, their tickers, their entry/exit dates, or their %% return —
// only position size. If any of those move, the overlay has leaked into
// something other than sizing, which the spec calls a bug, not a finding.
func TestPortfolioBacktest_VolTargetInvariant(t *testing.T) {
	bench := benchPath(280, 60)

	// Ticker A: a long, gentle uptrend (tight ATR-based stop stays out of the
	// way) followed by a sharp decline steep enough to blow through the
	// stop almost immediately from any of the three entries below.
	tickerA := sweepCandles(append(repeatMove(0.3, 300), repeatMove(-1.5, 40)...))
	// Ticker B: a gentle downtrend, then a rally, then a crash steep enough
	// to trip the trailing stop on the entry taken during the rally.
	tickerB := sweepCandles(append(append(repeatMove(-0.2, 300), repeatMove(2.0, 20)...), repeatMove(-3.0, 20)...))

	hists := map[string]tickerHist{
		"A": newTickerHist(tickerA),
		"B": newTickerHist(tickerB),
	}
	entries := map[string][]string{}
	for _, idx := range []int{285, 300, 310} {
		d := tickerA[idx].Date.Format("2006-01-02")
		entries[d] = append(entries[d], "A")
	}
	for _, idx := range []int{290, 305} {
		d := tickerB[idx].Date.Format("2006-01-02")
		entries[d] = append(entries[d], "B")
	}

	// MaxPositionPct left off (0) here on purpose: this test isolates the
	// vol-target multiplier's effect on RiskPct, uncomplicated by the
	// position cap potentially binding differently once equity has already
	// diverged between the two runs (that path is covered separately by
	// TestPortfolioBacktest_MaxPositionPctCaps).
	cfg := paper.Config{StopATRMult: 2, StopLossPct: 10, TrailingPct: 18, Market: market.US, RiskPct: 1.0}

	off := runPortfolioBacktest(hists, entries, bench, cfg, 100000, false, 0, "", "")
	on := runPortfolioBacktest(hists, entries, bench, cfg, 100000, true, 0, "", "")

	if len(off.Trades) == 0 {
		t.Fatal("test setup produced no closed trades — widen the entry/price paths")
	}
	if len(off.Trades) != len(on.Trades) {
		t.Fatalf("trade count differs: off=%d on=%d — vol-target must not skip or add trades",
			len(off.Trades), len(on.Trades))
	}

	sizeDiffered := false
	for i := range off.Trades {
		o, n := off.Trades[i], on.Trades[i]
		if o.Ticker != n.Ticker || o.EntryDate != n.EntryDate || o.ExitDate != n.ExitDate {
			t.Fatalf("trade %d: ticker/date differs: off=%+v on=%+v", i, o, n)
		}
		if math.Abs(o.ReturnPct-n.ReturnPct) > 1e-9 {
			t.Errorf("trade %d (%s %s): %%return differs off=%.6f on=%.6f — vol-target must only change size",
				i, o.Ticker, o.EntryDate, o.ReturnPct, n.ReturnPct)
		}
		if o.Shares != n.Shares {
			sizeDiffered = true
		}
	}
	if !sizeDiffered {
		t.Error("no trade's Shares differed between vol-target off/on — test isn't exercising the overlay")
	}
}

// TestPortfolioBacktest_MaxPositionPctCaps checks Part A's other headline
// claim: MaxPositionPct is now actually enforced (unlike simulateTrade's
// disposable account, which zeroes it to guarantee fills). A single large
// entry sized by RiskPct alone would exceed the cap; this asserts the filled
// notional never does.
func TestPortfolioBacktest_MaxPositionPctCaps(t *testing.T) {
	bench := benchPath(60, 0)
	ticker := sweepCandles(repeatMove(0.1, 60))
	hists := map[string]tickerHist{"A": newTickerHist(ticker)}
	entryDate := ticker[40].Date.Format("2006-01-02")
	entries := map[string][]string{entryDate: {"A"}}

	const capPct = 5.0
	cfg := paper.Config{StopATRMult: 2, StopLossPct: 10, TrailingPct: 18, Market: market.US, RiskPct: 50, MaxPositionPct: capPct}
	result := runPortfolioBacktest(hists, entries, bench, cfg, 100000, false, 0, "", "")

	// Find the day the position was opened and check its notional against
	// equity that same day.
	var openDayEquity float64
	for _, p := range result.Curve {
		if p.Date == entryDate {
			openDayEquity = p.Equity
		}
	}
	if openDayEquity <= 0 {
		t.Fatal("no equity recorded on the entry date")
	}
	entryPrice := ticker[40].Close
	// Shares actually bought = held notional / price, read back from the
	// equity curve's cash delta (cash spent = shares*price, fee-free in US).
	var cashAfter float64
	for _, p := range result.Curve {
		if p.Date == entryDate {
			cashAfter = p.Cash
		}
	}
	spent := 100000 - cashAfter
	notionalPct := spent / openDayEquity * 100
	if notionalPct > capPct+1e-6 {
		t.Errorf("position notional = %.2f%% of equity, want <= %.2f%% (RiskPct=50 uncapped would ask for far more)", notionalPct, capPct)
	}
	if spent <= 0 {
		t.Fatal("no position was opened — test setup produced no fill")
	}
	_ = entryPrice
}

// TestPortfolioBacktest_MaxCorrelationAtEntry pins §8.6's diversification
// gate: a BUY candidate whose trailing 60-day return correlation with an
// already-held position is >= the threshold must not fill, while the exact
// same setup with the gate off (0) fills normally.
func TestPortfolioBacktest_MaxCorrelationAtEntry(t *testing.T) {
	bench := benchPath(150, 0)
	// A and B trace the IDENTICAL move sequence (sweepCandles always starts
	// from the same date/price), so their trailing-return correlation is
	// exactly 1.0 by construction — as correlated as two tickers can get.
	moves := repeatMove(0.3, 150)
	tickerA := sweepCandles(moves)
	tickerB := sweepCandles(moves)
	hists := map[string]tickerHist{
		"A": newTickerHist(tickerA),
		"B": newTickerHist(tickerB),
	}
	entryDateA := tickerA[10].Date.Format("2006-01-02")
	// B's signal lands well past correlationWindow (60), with A still open
	// (a smooth uptrend never trips a trailing stop).
	entryDateB := tickerA[100].Date.Format("2006-01-02")
	entries := map[string][]string{
		entryDateA: {"A"},
		entryDateB: {"B"},
	}

	cfg := paper.Config{StopATRMult: 2, StopLossPct: 10, TrailingPct: 18, Market: market.US, RiskPct: 1.0, MaxPositionPct: 40}

	gated := runPortfolioBacktest(hists, entries, bench, cfg, 100000, false, 0.9, "", "")
	ungated := runPortfolioBacktest(hists, entries, bench, cfg, 100000, false, 0, "", "")

	heldAt := func(result portfolioResult, date string) int {
		for _, p := range result.Curve {
			if p.Date == date {
				return p.PositionsHeld
			}
		}
		return -1
	}

	if got := heldAt(gated, entryDateB); got != 1 {
		t.Errorf("gate on (threshold=0.9): positions held on B's signal date = %d, want 1 (B's perfectly-correlated entry must be skipped)", got)
	}
	if got := heldAt(ungated, entryDateB); got != 2 {
		t.Errorf("gate off (threshold=0): positions held on B's signal date = %d, want 2 (B must fill normally)", got)
	}
}

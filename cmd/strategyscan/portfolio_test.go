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

	off := runPortfolioBacktest(hists, entries, bench, cfg, 100000, false, "", "")
	on := runPortfolioBacktest(hists, entries, bench, cfg, 100000, true, "", "")

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
	result := runPortfolioBacktest(hists, entries, bench, cfg, 100000, false, "", "")

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

// TestPortfolioBacktest_PartialExitAtR pins §8.4②'s portfolio-layer wiring:
// once cfg.PartialExitAtR fires, the position must stay open (not appear in
// result.Trades, which is only round trips — see the `stillOpen` skip this
// added to runPortfolioBacktest's sell-pairing loop) with its cash proceeds
// landing in the curve, and the same holding must eventually close for real
// once the rally reverses.
func TestPortfolioBacktest_PartialExitAtR(t *testing.T) {
	bench := benchPath(90, 0)
	ticker := sweepCandles(append(repeatMove(0.5, 40), repeatMove(-5, 20)...))
	hists := map[string]tickerHist{"A": newTickerHist(ticker)}
	entryDate := ticker[5].Date.Format("2006-01-02")
	entries := map[string][]string{entryDate: {"A"}}

	cfg := paper.Config{StopATRMult: 2, StopLossPct: 10, TrailingPct: 18, Market: market.US, RiskPct: 1.0, MaxPositionPct: 40, PartialExitAtR: 1}
	result := runPortfolioBacktest(hists, entries, bench, cfg, 100000, false, "", "")

	var cashAfterEntry float64
	for _, p := range result.Curve {
		if p.Date == entryDate {
			cashAfterEntry = p.Cash
			break
		}
	}
	if cashAfterEntry <= 0 {
		t.Fatal("no cash recorded on the entry date")
	}

	var midCash float64
	for _, p := range result.Curve {
		if p.Date > entryDate && p.Cash > midCash {
			midCash = p.Cash
		}
	}
	if midCash <= cashAfterEntry+1e-6 {
		t.Fatalf("expected cash to rise above %.2f after the partial exit's proceeds, got max %.2f", cashAfterEntry, midCash)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("expected exactly one closed round trip (the eventual full exit after the crash), got %d: %+v", len(result.Trades), result.Trades)
	}
	if result.Trades[0].EntryDate != entryDate {
		t.Errorf("closed trade's EntryDate = %s, want %s (the partial fill must not have been paired off as its own round trip)", result.Trades[0].EntryDate, entryDate)
	}
}

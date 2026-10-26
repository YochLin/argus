package signals

import (
	"math"

	"argus/internal/data"
)

// The momentum-breakout watch (2026-10-26): a momentum leader that ran up,
// then tightened into a base above its moving averages, breaking above the
// base's high. TypeMomentumSetup is a stock whose base is complete and is
// waiting for that break; TypeMomentumBreakout is one that broke it.
//
// # Result: measured, did NOT pass — observation only
//
// cmd/strategyscan -mb-backtest (breakout.go) specified every rule with the
// user before the first run; excess per-trade return over a random entry
// with the same exits, month-clustered bootstrap, split 2021-11-01:
//
//	pool                 ADR floor   n     early σ   late σ
//	S&P 400 + 600        5%          261   1.2       0.0
//	S&P 500              5%          59    1.3       1.0
//	S&P 500 + 400 + 600  5%          288   1.3       0.5
//	S&P 600              5%          195   1.8       0.3
//	S&P 500              3%          232   -0.3      1.3
//
// No pool clears 1 SE in both slices. The edge follows whichever group led
// the market (small caps early, large caps late), which no fixed index pool
// captures. The user asked to watch S&P 500 at a 3% ADR floor anyway, so
// service.ScanMomentumBreakout pushes it, labelled unvalidated, and nothing
// (ranking, the LLM prompt) consumes it.
const (
	TypeMomentumSetup    = "strategy_momentum_setup"
	TypeMomentumBreakout = "strategy_momentum_breakout"
)

const (
	momentumBaseMax       = 40 // bars searched for the base's high
	momentumBaseMin       = 5  // a younger "base" is a stock still making highs
	momentumPriorRunBars  = 63
	momentumPriorRunPct   = 30.0
	momentumAboveMA20Frac = 0.8
)

// ADRPct is the average daily range, (high/low - 1) in %, over the last n
// candles; 0 when there are fewer than n.
func ADRPct(candles []data.Candle, n int) float64 {
	if len(candles) < n {
		return 0
	}
	var sum float64
	for _, c := range candles[len(candles)-n:] {
		if c.Low > 0 {
			sum += c.High/c.Low - 1
		}
	}
	return sum / float64(n) * 100
}

// MomentumBase is a completed base: Pivot is its high (the breakout level),
// Days its length in bars, the high's bar included.
type MomentumBase struct {
	Pivot float64
	Days  int
}

// DetectMomentumBase reports whether the candles end in a base: the highest
// high of the last 40 bars starts it and the last candle ends it, >= 5 bars
// long; the 63 bars up to that high rose >= 30% low-to-high; the base's
// second half has a higher low than its first; the last 5 bars' average
// range is below the first half's; every close is above MA50 and >= 80%
// are above MA20.
func DetectMomentumBase(candles []data.Candle) (MomentumBase, bool) {
	e := len(candles) - 1
	lo := e - momentumBaseMax + 1
	if lo-momentumPriorRunBars < 0 || lo < 49 {
		return MomentumBase{}, false
	}
	st := lo
	for j := lo; j <= e; j++ {
		if candles[j].High > candles[st].High {
			st = j
		}
	}
	n := e - st + 1
	if n < momentumBaseMin {
		return MomentumBase{}, false
	}
	minLow := math.Inf(1)
	for j := st - momentumPriorRunBars; j <= st; j++ {
		minLow = math.Min(minLow, candles[j].Low)
	}
	if minLow <= 0 || (candles[st].High/minLow-1)*100 < momentumPriorRunPct {
		return MomentumBase{}, false
	}
	closes := data.Closes(candles)
	half := st + n/2
	low1, low2, rng1 := math.Inf(1), math.Inf(1), 0.0
	above20 := 0
	for j := st; j <= e; j++ {
		k := candles[j]
		if k.Low <= 0 {
			return MomentumBase{}, false
		}
		if j < half {
			low1 = math.Min(low1, k.Low)
			rng1 += k.High/k.Low - 1
		} else {
			low2 = math.Min(low2, k.Low)
		}
		if k.Close <= MA(closes[:j+1], 50) {
			return MomentumBase{}, false
		}
		if k.Close > MA(closes[:j+1], 20) {
			above20++
		}
	}
	if low2 <= low1 || float64(above20) < momentumAboveMA20Frac*float64(n) {
		return MomentumBase{}, false
	}
	if ADRPct(candles, 5)/100 >= rng1/float64(half-st) {
		return MomentumBase{}, false
	}
	return MomentumBase{Pivot: candles[st].High, Days: n}, true
}

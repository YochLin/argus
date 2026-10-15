package main

import (
	"argus/internal/data"
	"argus/internal/paper"
	"argus/internal/signals"
)

// The 2026-09-19 request's exit half: stop at the support low under the
// entry, take profit when the close loses the MA20. Neither rule exists in
// internal/paper — the live engine's stop is an ATR multiple off the entry
// price and it has no moving-average exit at all — so this is a replay local
// to the research tool rather than an addition to the production rulebook,
// which would be adding a live trading rule to measure whether it is worth
// having.
//
// It is NOT a second copy of simulateTrade's exit logic: the two rules below
// are the entire difference, and everything they share (friction, the
// forced-close-at-maxHold convention, TradeOutcome's meaning) is either
// called from internal/paper or trivially identical, so the two cannot drift
// on anything that matters to a comparison between them.
//
// -support-ma-exit applies to the random-entry CONTROL as well as to every
// screen, which is the only reason a number produced under it can be read
// against a number produced under paper.DefaultExits.
//
// Measured 2026-09-19 and NOT adopted: replayed over identical control
// entries it is worse than paper.DefaultExits in all four market x split
// samples, by -0.54 to -2.04pp per trade (up to 5.5 sigma). The numbers and
// the reading are in signals.CheckKDJAboveMA100Exact's doc comment; the code
// stays so the result reproduces.
//
// ponytail: exits evaluate on the close only, no intraday gap-through
// simulation — same simplification MarkClose documents, and it has to be the
// same one or the two exit configs would not be comparable.

// supportStop is the support level under an entry at entryIdx: the lowest LOW
// of the trailing lookback bars, the signal bar included.
//
// The low, not the close (signals.LowestClose) — 「找低點作為支撐位」 is a
// line a chart reader draws under the wicks, and a close-based floor sits
// above it by an entire bar's lower shadow, which on this data would make
// every stop meaningfully tighter than the one the rule describes.
//
// ok is false when the support is at or above the entry price, which is
// possible only when the entry bar closes exactly on its own low: there is
// no room for a stop there and the trade is skipped rather than silently
// given the ATR stop of a different rule.
func supportStop(candles []data.Candle, entryIdx, lookback int) (float64, bool) {
	from := entryIdx - lookback + 1
	if from < 0 {
		from = 0
	}
	low := candles[from].Low
	for _, c := range candles[from+1 : entryIdx+1] {
		if c.Low < low {
			low = c.Low
		}
	}
	entry := candles[entryIdx].Close
	if low <= 0 || low >= entry {
		return 0, false
	}
	return low, true
}

// maExitPeriod is the moving average the take-profit rule watches. Fixed at
// the 20 the request named; see CheckKDJAboveMA100Exact on why the numbers
// under test are constants and not flags.
const maExitPeriod = 20

// simulateTradeSupportMA replays entryIdx forward under the request's own
// exit rules, returning the same TradeOutcome shape simulateTrade does so
// every downstream summary and CSV reads it unchanged.
//
// Each bar after the entry, in order:
//
//  1. close at or below the support stop -> exit "stop". Checked first, the
//     same precedence MarkClose gives the fixed stop.
//  2. otherwise, 「KD 跌破 X 且跌破 MA20」 -> exit "ma20". BOTH legs, as the
//     request states them: K below kdExit AND the close below its MA20.
//     Either one alone fires far too easily to be the rule described (K
//     oscillates through any level several times inside a normal hold).
//
// The MA20 leg ARMS on the first close above the MA20 and only then can
// fire, on a close back below it. 「跌破」 is a break of a level from above,
// and rule ①'s entry — a KD turn out of oversold — usually fires on a bar
// that is still UNDER its MA20, so without arming the exit would trip the
// next day for most trades and what got measured would be a one-bar hold.
//
// kdExit is the KD level under test (20 for the oversold-turn entry, 50 for
// the pullback entry) and comes from -support-ma-exit, so one run measures
// one exit rule, for the screens AND for the control.
func simulateTradeSupportMA(candles []data.Candle, entryIdx int, cfg paper.Config, slippagePct float64, maxHoldDays, supportLookback int, kdExit float64) (TradeOutcome, bool) {
	entry := candles[entryIdx].Close
	if entry <= 0 {
		return TradeOutcome{}, false
	}
	stop, ok := supportStop(candles, entryIdx, supportLookback)
	if !ok {
		return TradeOutcome{}, false
	}
	closes := data.Closes(candles)

	// Fees are percentages of notional, so the scale is arbitrary — except
	// for TW's twMinFee floor, which binds only on a small fill. A large
	// notional keeps FeeFor on its statutory percentage, which is the same
	// place simulateTrade's 1e15 account converges to.
	const notional = 1e12
	shares := notional / entry
	buyFee := paper.FeeFor(cfg.Market, "BUY", notional, cfg.FeeDiscount)
	slippageRoundTripPct := 2 * slippagePct

	exit := func(idx int, reason string, days int) (TradeOutcome, bool) {
		px := candles[idx].Close
		sellFee := paper.FeeFor(cfg.Market, "SELL", px*shares, cfg.FeeDiscount)
		feePct := (buyFee + sellFee) / notional * 100.0
		return TradeOutcome{
			ExitRet:    (px-entry)/entry*100.0 - feePct - slippageRoundTripPct,
			ExitReason: reason,
			HoldDays:   days,
			Entry:      entry,
			Stop:       stop,
			ExitDate:   candles[idx].Date.Format("2006-01-02"),
			ExitPrice:  px,
		}, true
	}

	// The KD the exit watches, over the bars this replay can reach. Computed
	// once per trade against a bounded window rather than per bar against
	// the whole series — see signals.KDSeries.
	end := entryIdx + maxHoldDays
	if end >= len(candles) {
		end = len(candles) - 1
	}
	k, _ := signals.KDSeries(candles[:end+1], end-entryIdx+1) // k[i] is candles[entryIdx+i]

	armed := false
	last := entryIdx
	for i := 1; i <= maxHoldDays; i++ {
		idx := entryIdx + i
		if idx >= len(candles) {
			break
		}
		last = idx
		close := candles[idx].Close
		if close <= 0 {
			continue
		}
		if close <= stop {
			return exit(idx, "stop", i)
		}
		ma := signals.MA(closes[:idx+1], maExitPeriod)
		if ma == 0 {
			continue
		}
		if !armed {
			if close > ma {
				armed = true
			}
			continue
		}
		if close < ma && i < len(k) && k[i] < kdExit {
			return exit(idx, "ma20", i)
		}
	}
	if last == entryIdx {
		return TradeOutcome{}, false
	}
	return exit(last, "timeout", last-entryIdx)
}

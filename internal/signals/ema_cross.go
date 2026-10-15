package signals

import "argus/internal/data"

// emaCrossPeriod is the EMA length CheckEMACrossExact tests. Fixed, not a
// ScreenParams knob, for the same reason CheckMTFCrossExact takes no
// parameters: the number IS the hypothesis under test, and a knob would
// invite re-tuning it against the same two splits the answer came from.
const emaCrossPeriod = 10

// emaRisingLookback is how far back CheckEMACrossUpExact measures the EMA10's
// slope: 5 trading days, i.e. one week, matching the dedup window every
// alert-once check in this codebase already uses. Pre-registered, not swept —
// see CheckEMACrossExact on why these are not ScreenParams knobs.
//
// It cannot be 1. See CheckEMACrossUpExact's doc comment: at a lookback of 1
// the condition is not merely weak, it is unsatisfiable.
const emaRisingLookback = 5

// CheckEMACrossExact evaluates candles' last bar against "close just
// reclaimed the EMA10": yesterday's close at or below yesterday's EMA10,
// today's close above today's EMA10.
//
// It is an EVENT, not a state — the "just" is in the rule itself, so unlike
// CheckMTFCrossExact this does not lean on the 5-bar dedup above it for its
// alert semantics (the dedup still applies, and only ever suppresses a
// second cross inside a week — a whipsaw, which is the thing to suppress).
//
// Deliberately bare: no volume gate, no trend filter, no RSI cap. The
// question being asked is whether the cross by itself times an entry better
// than a random day does, and every added condition would make a negative
// answer unattributable. cmd/strategyscan applies its -min-avg-volume floor
// to the control population too, so the liquidity screen is common to both
// sides rather than an edge handed to this one.
//
// The exit side is paper.DefaultExits unchanged — a 2-ATR initial stop and
// an 18% trailing stop — which is already the ATR-based exit the request
// asked for and, more importantly, the exit the random-entry control is
// replayed through. Changing the exit changes both sides together, so it
// cannot manufacture an excess.
//
// # Result: measured 2026-09-19 — indistinguishable from a random entry day, at full power
//
// Per-trade excess exit return vs. the random-entry control replayed through
// identical exit rules in the same run, date-clustered bootstrap (400
// resamples of dates), split 2021-11-01, same caches and same four slices
// every other screen here is scored on:
//
//	exit config                      US in-samp   US holdout   TW in-samp   TW holdout
//	2 ATR stop + 18% trail (default)  +0.27 0.6o   -0.22 0.5o   -0.44 0.7o   +0.14 0.3o
//	2 ATR stop + 3 ATR trail          +0.20 0.5o   -0.12 0.3o   -0.43 0.7o   +0.20 0.4o
//
// No slice clears §4.4's 1 SE in either exit configuration, and the sign
// flips between splits in both markets.
//
// CheckEMACrossUpExact — the same cross gated on the EMA10 having risen over
// the prior week — does not rescue it and is directionally worse, on the same
// default-exit runs:
//
//	                      US in-samp   US holdout   TW in-samp   TW holdout
//	ema10_cross            +0.27 0.6o   -0.22 0.5o   -0.44 0.7o   +0.14 0.3o
//	ema10_cross_up         +0.04 0.1o   -0.61 1.3o   -0.89 1.3o   +0.19 0.3o
//
// The filter keeps ~38% of the triggers and, splitting the base screen's own
// trades by whether the filter kept them, it keeps the WORSE half in three of
// four slices (kept minus dropped, mean exit return: -0.46pp US in-samp,
// -0.46pp US holdout, -0.82pp TW in-samp, +0.08pp TW holdout). Reading: a
// reclaim after the EMA10 has been falling for a week is a dip being bought,
// and over a sample whose base rate is positive drift that beats buying a
// cross that arrives when the average is already climbing. Requiring the
// trend to be established first is paying up for confirmation.
//
// Unlike CheckPostGapDriftExact — which measured the right sign in all four
// samples and failed only on trigger count — this is NOT a power problem and
// re-running it on a wider universe will not change the answer. The screen
// fires 36k-44k times per slice, roughly every 29 trading days per ticker
// after the 5-bar dedup, which buys the smallest standard error of any screen
// in this study (0.40-0.68pp): the measurement can resolve half a point of
// edge and there is not half a point of edge to resolve. An EMA10 reclaim is
// close to a coin flip on when it happens, and it happens constantly.
//
// The exit swap is reported because the request named an ATR exit
// specifically, and it is worth being explicit that it changes nothing about
// the entry question: the control pays the same exit, so the two rows above
// differ only in noise. It does change ABSOLUTE behavior — a 3-ATR trail
// closes trades ~7 days sooner and moves the dominant exit reason from stop
// (26.1k of 43.8k US holdout trades) to trailing (20.9k) — but the mean
// return moves with the control, not against it. Consistent with
// paper.DefaultExits' own trailing-stop calibration and with the 2026-08-26
// finding that TRAILING_STOP_ATR_MULT=3 was negative in all four slices.
//
// Reproduce (same caches as CheckTrendPullbackExact's doc comment):
//
//	strategyscan -market=us -range=10y -history-file=us_daily.csv            -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=us -range=10y -history-file=us_daily.csv            -date-from=2021-11-01                     -dump-trades=dump.csv
//	strategyscan -market=tw           -history-file=tw_daily.csv -skip-trust -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=tw           -history-file=tw_daily.csv -skip-trust -date-from=2021-11-01                     -dump-trades=dump.csv
//	python3 pead_study.py "LABEL=strategyscan_results_<market>.csv,dump.csv" ...  (ema10_cross row, "overall" columns)
//
// Add -trailing-pct=0 -trailing-atr=3 to each for the pure-ATR exit row.
//
// Not wired into service.CheckStatefulSignals, i18n, or the recommendation
// prompt — it exists in this package only so the result stays reproducible.
func CheckEMACrossExact(candles []data.Candle) bool { return emaCross(candles, false) }

// CheckEMACrossUpExact is CheckEMACrossExact plus one condition: the EMA10
// must have been RISING over the week going into the signal bar —
// yesterday's EMA10 above its own value emaRisingLookback bars earlier.
//
// The thesis is that a reclaim while the average is still sinking is a bounce
// inside a downtrend, not a trend resuming, and the two should not be counted
// as the same event. It is a separate screen rather than an edit to the one
// above so both are scored against the SAME random-entry control in the SAME
// run — the filter's contribution is then read directly off two rows, with no
// cross-run join.
//
// # Why the slope is measured over a week and not over the last bar
//
// On an EMA — unlike an SMA — the average's slope sign and the price's
// position relative to it are THE SAME FACT, one bar apart:
//
//	EMA_t = (1-k)*EMA_{t-1} + k*C_t, so
//	  EMA_t > EMA_{t-1}  <=>  k*(C_t - EMA_{t-1}) > 0     <=>  C_t > EMA_{t-1}
//	  C_t   > EMA_t      <=>  (1-k)*(C_t - EMA_{t-1}) > 0 <=>  C_t > EMA_{t-1}
//
// A close above the EMA necessarily drags the EMA up on that same bar. (The
// intuition that these are independent conditions is imported from SMAs,
// where the average also moves with the bar dropping out of the window, so an
// SMA genuinely can fall on an up-close.) Two consequences, both of which
// would have silently produced a meaningless screen:
//
//   - Gating on the SIGNAL bar's own slope (EMA_t > EMA_{t-1}) filters
//     nothing: it is implied by the cross. The variant would have measured
//     the unfiltered screen twice under two names.
//   - Gating on the PREVIOUS bar's slope (EMA_{t-1} > EMA_{t-2}) filters
//     everything: the cross requires C_{t-1} <= EMA_{t-1}, which by the same
//     identity IS "EMA_{t-1} was falling". The two are mutually exclusive and
//     the screen could never fire at all.
//
// Only a multi-bar lookback carries information the cross does not already
// contain, which is why emaRisingLookback is 5 and why it is a constant with
// a floor rather than a knob that could be turned down to 1.
//
// # Result: measured 2026-09-19 — see the table on CheckEMACrossExact
func CheckEMACrossUpExact(candles []data.Candle) bool { return emaCross(candles, true) }

// emaCross is the shared body: a close crossing above its EMA10, optionally
// gated on the EMA10 having risen over the preceding emaRisingLookback bars.
func emaCross(candles []data.Candle, requireRising bool) bool {
	n := len(candles)
	if n < emaCrossPeriod+emaRisingLookback+2 {
		return false
	}
	closes := data.Closes(candles)

	today := EMA(closes, emaCrossPeriod)
	prev := EMA(closes[:n-1], emaCrossPeriod)
	if today == 0 || prev == 0 {
		return false
	}
	if closes[n-2] > prev || closes[n-1] <= today {
		return false
	}
	if requireRising {
		past := EMA(closes[:n-1-emaRisingLookback], emaCrossPeriod)
		if past == 0 || prev <= past {
			return false
		}
	}
	return true
}

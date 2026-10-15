package signals

import "argus/internal/data"

// CheckPostGapDriftExact evaluates candles' last bar (網 6【財報後漂移】):
// a stock that gapped up hard on heavy volume, held the gap into the close,
// and did it at a new high. Phase 25 §2.
//
// The premise is post-earnings-announcement drift: after a genuine positive
// surprise a stock keeps drifting the same way for weeks, because analysts
// anchor on stale estimates, institutional accumulation is spread over days,
// and not everyone reads the filing on the day. That is an
// information-diffusion story, not a "the market is dumb" story, which is why
// it has survived 60 years and every out-of-sample retest thrown at it.
//
// Note what this screen does NOT do: look up an earnings date, or an analyst
// estimate. The gap itself is the surprise proxy — for an index constituent
// the four earnings days a year dominate the list of largest overnight moves,
// and the market's own reaction is a fresher aggregate of the news than a
// consensus estimate that may be weeks stale. "Earnings-day return" is a
// standard surprise proxy in the literature. The practical consequence is
// that this screen needs price and volume only, so it backtests against the
// existing OHLCV cache with no new data source (Phase 23 PR8's finding that
// Finnhub's free tier caps at 4 quarters is what makes that matter).
//
// Conditions 3 and 4 are the two that stop this from being "buy anything that
// gapped", and neither is optional:
//
//   - Without 3 (close in the upper half of the day's range), a stock that
//     gapped +7% and was sold all the way back to the low qualifies. That is
//     the news being sold into, and it drifts DOWN — the opposite trade.
//   - Without 4 (new high), a sharp bounce inside a downtrend qualifies. A
//     gap off a base is not drift; drift is a repricing to a new level.
//
// # Result: measured, did NOT pass, deliberately NOT wired to the bot
//
// Phase 25 §2.5 pre-registered the bar before any of this was run: excess
// return over the random-entry control under identical exit rules had to
// clear 1 SE in BOTH time slices of the primary sample (S&P 400 mid-caps,
// where §2.6 argued PEAD should be strongest because attention is scarcest).
// It did not. Date-clustered bootstrap, 400 resamples of dates (not trades —
// same-day entries co-move), split at 2021-11-01, the earlier slice being the
// holdout:
//
//	sample                    n     excess     SE   sigma
//	SP400 holdout  [primary] 208     +2.30%   2.54    0.9
//	SP400 in-samp  [primary] 270     +0.72%   1.13    0.6
//	SP500 holdout            288     +0.85%   0.91    0.9
//	SP500 in-samp            323     +1.94%   1.43    1.4
//
// So this screen exists for cmd/strategyscan to keep measuring, and for the
// record above to be reproducible. It is NOT in i18n, NOT in
// service.CheckStatefulSignals, and must not be added there on the strength
// of the numbers above — §4.4's whole point is that the bar does not move
// after seeing the result.
//
// What is genuinely interesting, and why this is "underpowered" rather than
// "disproven": it is the only screen here whose excess is positive-signed in
// all four samples. Every other screen flips sign between universes. Four of
// four is p≈0.06 under a coin-flip null, which is suggestive and not
// conclusive — and the four are not independent, since SP400 and SP500 span
// the same calendar. The blocker is sample size, not effect size: the screen
// fires ~200-320 times per slice because a >=5% gap on >=3x volume at a
// 60-day high is rare. Resolving it needs more names (Russell-2000 scale),
// not more tuning of the thresholds, and re-tuning them against these same
// slices would just fit the noise.
//
// # Re-verified 2026-09-19 on a wider TW sample: verdict UNCHANGED, and the
// "underpowered, not disproven" reading above is now weaker than it was
//
// Running the same screen against the whole-market TW cache (2,103 tickers,
// point-in-time daily quotes, so far less survivorship-biased than the
// back-filled US index lists) and S&P 500 produced numbers that clear §4.4's
// arithmetic on their face — TW +4.18%/1.9o in-sample and +5.80%/3.0o
// holdout, SP500 +1.59%/1.3o and +2.57%/1.6o, all four positive. They do not
// survive the follow-up checks, and this entry exists so nobody re-runs that
// table and reads it as a pass:
//
//   - Primary sample still fails. Re-run on S&P 400, the sample §2.5
//     pre-registered, under the current exit defaults: in-sample -0.90%/0.4o
//     (NEGATIVE), holdout +1.06%/0.9o. The 2026-08-27 verdict reproduces.
//   - The mean is a thin right tail. Dropping the best 5% of triggers turns
//     every slice negative: TW +5.80 -> -1.94, TW in-samp +4.18 -> -5.39
//     (5.6o negative), SP500 +2.57 -> -2.14 (2.7o), SP500 in-samp +1.59 ->
//     -1.12. ~31 of 619 trades carry the whole TW holdout result.
//   - The MEDIAN excess is negative in all four, and TW's win rate is BELOW
//     its control's (25.3% vs 33.8% in-sample, 30.2% vs 28.7% holdout). The
//     typical trigger is worse than a random entry; only the extreme tail is
//     better. A bootstrap SE on a distribution this skewed reads far more
//     confident than the evidence is.
//   - TW's holdout edge is recency-concentrated: +1.86pp over 2021-11..2024-02
//     versus +8.16pp from 2024-03 on.
//
// Not concentrated in a few names, at least (470 distinct tickers across 619
// TW holdout trades), so it is a trade-level tail rather than a name-selection
// artifact. Still not a basis for shipping. Reading four universes x two
// splits and taking the cells that pass is exactly the search §4.4 exists to
// prevent; the primary sample was named in advance and it is the one that
// answers.
//
// Two secondary findings from the same runs:
//
//   - Entry timing is not the problem. The T+1 variant (same signal, entered
//     one bar later — see gapDriftT1Strategy in cmd/strategyscan) tracks T+0
//     within 0.6pp in every sample, so the effect is not a one-session
//     continuation that a real fill would miss.
//   - §2.6's slippage worry does not apply to the excess. Slippage is a
//     constant charge per round trip, it does not move the exit, and the
//     control pays it too — so tripling it to 0.3%/side leaves every excess
//     figure above identical to the cent. (The sigmas wobble by ~0.03
//     between those two runs only because the bootstrap RNG has advanced;
//     the underlying returns differ by a constant.) What it does cost is
//     0.40pp of ABSOLUTE return (SP400 holdout +5.47% -> +5.07%), small only
//     because the average hold is ~37 days. Slippage can make the trade
//     unprofitable; it cannot manufacture or destroy an edge over a control
//     that trades the same way. Worth remembering before running that
//     sensitivity again on any screen measured against this control.
//
// # Update — post_gap_drift_confirmed (Phase 25 §8.1), also measured, also NOT shipped
//
// §8.1 asked whether the gap-based surprise proxy is being diluted by gaps
// that have nothing to do with an actual earnings release — i.e. does
// requiring the signal day to fall within ±1 trading day of a real SEC 10-Q/
// 10-K filed date (data.SEC.GetFilingDates, cmd/strategyscan's
// -earnings-dates-file cache) sharpen the edge. Same S&P 400 universe, same
// two time slices, same random-entry control, same bootstrap:
//
//	sample                         n     excess     SE   sigma
//	SP400 holdout, confirmed      88     +2.23%   2.39    0.9
//	SP400 in-samp, confirmed     155     +2.22%   1.58    1.4
//	(for reference, unconfirmed) 208     +2.30%   2.54    0.9
//	(for reference, unconfirmed) 270     +0.72%   1.22    0.6
//
// Pre-registered bar was the same as above: >1 SE in BOTH splits. Holdout
// stayed at 0.9 sigma — did not clear. Still NOT wired into i18n or
// service.CheckStatefulSignals, same as the unconfirmed screen.
//
// The pre-registered extra requirement (trigger count strictly lower than
// unconfirmed) held in both slices: 88 < 208 (holdout), 155 < 270 (in-sample)
// — confirming against a live filing calendar does cut the sample roughly in
// half, as expected (not every >=5% gap on 3x volume is an earnings gap).
//
// What's actually informative here: excess return on the holdout slice is
// UNCHANGED to within 0.07pp (+2.30% -> +2.23%) despite the sample nearly
// halving. Per §8.1.4's own pre-registered instruction for exactly this
// pattern ("excess didn't rise AND the sample shrank -> the answer is
// universe expansion, not loosening the window"), that is the honest
// reading — not that confirmation adds nothing, but that it isn't the lever
// this screen needs. The in-sample slice's jump to 1.4 sigma is the kind of
// one-good-one-bad split split §4.4's two-slice rule exists to keep from
// being reported as a win.
//
// §4.6's required question: did this invalidate anything? No. If anything it
// reinforces §2.2's original premise — the raw gap is already most of the
// earnings-surprise signal, since restricting to gaps provably tied to a
// real SEC filing barely moves the per-trade excess. The bottleneck named in
// the original result (sample size, not signal quality) stands unchanged;
// this update is one more data point for it, not a revision of it.
//
// # Update — S&P 600 small-cap re-run (2026-09-01), the prescribed fix for the sample-size bottleneck
//
// PEAD literature says the effect is strongest in small-caps (thinner
// analyst coverage, slower information diffusion), and S&P 600 (~600 more US
// small-caps, added for this) gives ~50% more names than SP400 to test that
// against. Same split/bootstrap, same random-entry control:
//
//	sample                    n     excess     SE   sigma
//	SP600 in-samp     [T0]   235     +0.97%   2.01    0.5
//	SP600 holdout     [T0]   342     -0.91%   1.10    0.8  (negative)
//	SP600 in-samp     [T1]   235     +2.03%   2.52    0.8
//	SP600 holdout     [T1]   342     -0.92%   1.12    0.8  (negative)
//
// Still doesn't clear 1 SE in either slice or variant — expected, given the
// SP400/SP500 pattern above. What IS new: the holdout excess is negative,
// for both T0 and T1. That breaks the "positive-signed in all four samples"
// pattern the original result flagged as suggestive (§ above) — it's now
// positive in in-sample slices only, which is the same "flips sign between
// universes" shape every OTHER screen here already had. Small-caps being the
// literature's strongest case for PEAD and still landing negative on holdout
// is evidence against "just needs more names," not for it — sample size
// alone doesn't explain a sign flip, only a wide SE. Read together with the
// unchanged SP400/SP500 numbers, this closes the "underpowered, not
// disproven" framing rather than resolving it in the screen's favor; nothing
// here argues for revisiting the "NOT wired" status above.
func CheckPostGapDriftExact(candles []data.Candle, p ScreenParams) bool {
	n := len(candles)
	if n < 60 {
		return false
	}
	closes := data.Closes(candles)
	volumes := data.Volumes(candles)
	bar := candles[n-1]

	// 1. The event: an opening gap up of at least GapUpPct.
	prevClose := closes[n-2]
	if prevClose <= 0 || (bar.Open-prevClose)/prevClose*100.0 < p.GapUpPct {
		return false
	}

	// 2. Volume confirms news, not a thin print on a quiet tape.
	volRatio := VolumeRatio(volumes, 20)
	if volRatio == 0 || volRatio < p.GapVolMult {
		return false
	}

	// 3. Held the gap into the close. No guard for High == Low is needed: a
	// bar that never moved (a TW limit-up locked at the open, the strongest
	// hold there is) has Close == mid, which passes.
	if bar.Close < (bar.High+bar.Low)/2 {
		return false
	}

	// 4. Repricing, not a bounce: the gap has to make a new high.
	if !IsNewHigh(closes, p.NewHighLookback) {
		return false
	}

	// 5. Liquidity: same gate, same window as the other four screens.
	window5v := volumes[n-6 : n-1]
	var sumV int64
	for _, v := range window5v {
		sumV += v
	}
	if float64(sumV)/5.0 < p.MinAvgVolume5d {
		return false
	}

	return true
}

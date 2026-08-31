package paper

import "argus/internal/market"

// ExitDefaults are the calibrated exit-rule parameters for one market.
//
// This package's whole reason to exist is that the backtest and the live
// paper account run the SAME engine, so their behavior cannot diverge (see
// the package doc). That guarantee only ever covered the rules, never the
// numbers fed into them — and the numbers promptly diverged: the exit study
// of 2026-08-26 moved the live US trailing stop from 15 to 18 while
// cmd/server/backtest.go and cmd/strategyscan both kept defaulting to 15, so
// the tools whose banner says "live-aligned" would have gone on measuring
// the configuration that was just retired. A study of the wrong parameters
// is worse than no study, because it still produces confident numbers.
//
// So the calibrated values live here, once, and all three callers read them:
// internal/app/config.go as its env fallback, and the two offline tools as their
// flag defaults. Changing a number here changes what the bot runs AND what
// the next study measures, together.
//
// The same drift is why StopLossPct is market-scoped here even though both
// tools used to hardcode 10: live TW has been 12 since Phase 13 §7 (TW's
// ±10% daily limit makes a 10% stop trip on an ordinary limit move), so
// every TW study so far modelled a stop 2 points tighter than the real one.
type ExitDefaults struct {
	StopATRMult       float64
	StopLossPct       float64
	TrailingPct       float64
	TrailingATRMult   float64
	TakeProfitATRMult float64
}

// DefaultExits returns m's calibrated exit parameters.
//
// TrailingPct is 18 for both markets as of 2026-08-26. Replaying the
// random-entry control through this engine at 8/12/15/18/25 improves
// monotonically the looser it gets, in all four samples (US/TW x
// in/out-of-sample); a paired date-clustered bootstrap on identical entries
// puts 15 -> 18 at 2.3-4.0 sigma everywhere. 25 measures better still and is
// deliberately not taken — see .env.example for why the metric the backtest
// maximizes is not what this setting is for.
//
// TakeProfitATRMult stays 0 on evidence rather than by default: every value
// tested (2/3/4/6 ATR) costs return in all four samples, and the tighter the
// worse. 2 ATR lifts the US out-of-sample win rate from 41.6% to 59.6% while
// halving mean return — it buys the appearance of winning often and sells
// the returns.
//
// There is deliberately no maximum-holding-period field: the live account
// holds until a stop, trailing stop, or target fires. cmd/strategyscan's
// -hold-sweep replayed the control at 5 through 120 days and annualized
// return is flat from 20 days out in both time slices, so a time stop in
// that range buys nothing, and no screen's preferred horizon replicated
// across the split. Only very short caps (5-10 days) move the number, and
// they move it down. See that flag's doc comment for the tables.
//
// StopATRMult = 2 is calibrated as of 2026-08-27, and the answer was to
// leave it where it was.
//
// The comparison has to be made in R — (exit-entry)/(entry-stop) — because
// SuggestShares sizes a position so a stop-out loses a fixed share of
// equity, so a wider stop buys fewer shares and the same price move is a
// different amount of money. But R alone inverts the answer, and that is
// the whole point of this entry: uncapped, R rises monotonically as the
// stop tightens (US control, ~59k trades per slice: 0.505 at 1 ATR down to
// 0.251 at 5). Live sizing is NOT uncapped. At RISK_PCT_PER_TRADE's
// effective 1% and PAPER_MAX_POSITION_PCT's 25%, risk-based sizing asks for
// 100*risk/stopWidth percent of the account — 39% at a 2.5% stop — and the
// cap truncates it. A truncated position earns proportionally less, and
// once that is applied the curve turns over:
//
//	ATR mult   1.0    1.5    2.0    2.5    3.0    4.0    5.0
//	holdout  0.311  0.369  0.375  0.359  0.337  0.287  0.247
//	in-samp  0.148  0.156  0.155  0.146  0.137  0.119  0.106
//
// Both slices peak at 1.5-2.0 and 2.0 is inside the noise of the top in
// both, so it stands.
//
// The mechanism is worth stating because it couples two settings that look
// independent: the cap stops binding at a stop width of RISK_PCT/
// MAX_POSITION_PCT = 4%, which is about 1.6 ATR on this data — precisely
// where the peak sits. Below it you pay a higher stop-out rate (70.6% at 1
// ATR vs 53.2% at 2) without getting the larger position that would justify
// it; above it, sizing works and the wider stop only dilutes R. So MOVING
// PAPER_MAX_POSITION_PCT MOVES THE OPTIMAL STOP WIDTH. Re-run
// cmd/strategyscan -stop-sweep with the new cap before assuming 2 still
// holds.
//
// Win rate, for the third time in these studies, points the other way: 28.1%
// at 1 ATR climbing to 58.0% at 5, while capped R peaks in the middle.
//
// StopLossPct is still uncalibrated — it is only the fallback for when ATR
// is unavailable, so it is rare enough that the sweep above says nothing
// about it.
func DefaultExits(m market.MarketID) ExitDefaults {
	d := ExitDefaults{
		StopATRMult: 2,
		StopLossPct: 10,
		TrailingPct: 18,
	}
	if m == market.TW {
		d.StopLossPct = 12
	}
	return d
}

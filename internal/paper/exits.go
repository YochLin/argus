package paper

import "argus/internal/market"

// ExitDefaults are the calibrated exit-rule parameters for one market.
//
// This package's whole reason to exist is that the backtest and the live
// paper account run the SAME engine, so their behavior cannot diverge (see
// the package doc). That guarantee only ever covered the rules, never the
// numbers fed into them — and the numbers promptly diverged: the exit study
// of 2026-08-26 moved the live US trailing stop from 15 to 18 while
// cmd/bot/backtest.go and cmd/strategyscan both kept defaulting to 15, so
// the tools whose banner says "live-aligned" would have gone on measuring
// the configuration that was just retired. A study of the wrong parameters
// is worse than no study, because it still produces confident numbers.
//
// So the calibrated values live here, once, and all three callers read them:
// cmd/bot/config.go as its env fallback, and the two offline tools as their
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
// StopATRMult/StopLossPct are NOT calibrated: changing the stop distance
// changes SuggestShares' position size, so two stop widths produce different
// notional exposure and their percentage returns are not comparable without
// R-normalizing first. These are the historical values, pending that study.
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

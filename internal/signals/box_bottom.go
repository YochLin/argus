package signals

import (
	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

// BoxBottomRebound evaluates candles for Box Bottom Rebound triggers within the last strategyLookbackDays.
// Returns the most recent hit (smallest DaysAgo) or nil if none triggered.
func BoxBottomRebound(candles []data.Candle, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < 60 {
			break
		}
		sub := candles[:evalIdx+1]
		if CheckBoxBottomReboundExact(sub, p) {
			return &StrategyHit{
				Name:    "box_bottom",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

// CheckBoxBottomReboundExact evaluates candles' last bar (網 2【箱底反彈】).
//
// # Result: measured (Phase 25 §4.7, dedicated run), downgraded
//
// §4.6 (PR #176) found, incidentally, that on S&P 400 mid-caps 網 2's excess
// return over the random-entry control was negative in both pre-registered
// time slices and significant, versus noise on S&P 500 — but that finding
// fell out of a study pre-registered for 網 6, not for 網 2, so §4.4's
// downgrade was deliberately NOT applied on the strength of it. This is the
// dedicated re-run that decision was waiting on, pre-registered before
// running: >1 SE in BOTH time splits on the SAME universe stays validated;
// one split short of 1 SE, a significant negative split, or negative in
// both, downgrades to briefing-only (same bar 網 3 was downgraded under, not
// a delist either way).
//
// Same date-clustered bootstrap as 網 3/網 6 (cmd/strategyscan/pead_study.py,
// 400 resamples of dates, split 2021-11-01, earlier slice the holdout),
// against the random-entry control from the SAME run, both universes:
//
//	sample                     n     excess     SE   sigma
//	SP500 holdout [large-cap] 260     +0.75%   0.81    0.9
//	SP500 in-samp [large-cap] 274     -1.12%   0.84    1.3  (negative, significant)
//	SP400 holdout [mid-cap]   167     -3.35%   1.48    2.3  (negative, significant)
//	SP400 in-samp [mid-cap]   160     -2.47%   0.82    3.0  (negative, significant)
//
// Neither universe clears the bar. S&P 400 replicates §4.6's incidental
// numbers almost exactly (-3.35%/2.3σ holdout, -2.45%/2.9σ in-sample there vs
// -3.35%/2.3σ, -2.47%/3.0σ here — same data, same bootstrap, now run and
// labeled as its own study rather than a side effect of 網 6's). S&P 500 —
// the large-cap universe this screen was originally tuned against, and the
// closer match to the bot's actual live watchlist — does NOT hold up under
// this protocol either: holdout doesn't clear 1 SE and in-sample is a
// significant NEGATIVE excess, so "noise in one split, real negative in the
// other" fails the bar the same way mid-cap's "negative in both" does. That
// settles the question this run exists to answer: there is no universe
// where 網 2 clears the bar, so scoping the downgrade to mid-caps only would
// be wrong, not merely unnecessary. Downgraded unconditionally, exactly like
// 網 3 (internal/service/scan.go's DecorateStrategyHits,
// i18n.KeyStrategyUnvalidatedBoxBottom) — still emitted, never delisted.
//
// TW (2026-08-28 follow-up): run once the operator brought up a live
// Shioaji daemon (`shioaji server start`, see defaultShioajiSocket's doc
// comment). Whole-market point-in-time OHLCV cache via -build-history
// (~15 min for the full 2016-08..2026-08 window — Shioaji has no TWSE-T86-
// style WAF, unlike CheckTrustFollowExact's data source), tw150 baseline,
// same split/bootstrap:
//
//	sample            n     excess     SE   sigma
//	TW in-samp       151     -1.28%   1.94    0.7
//	TW holdout       117     +3.11%   2.18    1.4
//
// Neither split clears 1 SE in the required direction together — holdout is
// positive but in-sample is negative and short of 1 SE, the same "one split
// short" failure mode US large-cap hit. TW does not overturn the
// unconditional downgrade above; it's the third universe now confirming the
// same answer, not a dissenting one.
//
// S&P 600 small-cap (2026-09-01 follow-up, Phase 25 §8's universe-expansion
// pass — same run also covers 網 1/網 6, see their own doc comments): same
// split/bootstrap, same protocol.
//
//	sample                     n     excess     SE   sigma
//	SP600 in-samp             187     -2.28%   1.02    2.2  (negative, significant)
//	SP600 holdout             248     -0.44%   0.93    0.5
//
// Fourth universe (after SP500/SP400/TW above), same answer: does not clear
// the bar, in-sample negative and significant again. Does not overturn the
// unconditional downgrade; adds to it.
//
// Reproduce (US path: CACHE built once per universe via -build-history from
// Yahoo, ~1-2 min per universe; TW path: whole-market point-in-time via
// -build-history from a live Shioaji daemon, ~15 min, -skip-trust since this
// screen doesn't use 網 5's trust-net data):
//
//	strategyscan -market=us -range=10y -build-history=us_daily.csv
//	strategyscan -market=us -range=10y -build-history=sp400_daily.csv -universe=sp400
//	strategyscan -market=tw -build-history=tw_daily.csv -date-from=2016-08-01 -date-to=2026-08-27
//	strategyscan -market=us -range=10y -history-file=us_daily.csv    -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=us -range=10y -history-file=us_daily.csv    -date-from=2021-11-01                     -dump-trades=dump.csv
//	strategyscan -market=us -range=10y -history-file=sp400_daily.csv -universe=sp400 -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=us -range=10y -history-file=sp400_daily.csv -universe=sp400 -date-from=2021-11-01                     -dump-trades=dump.csv
//	strategyscan -market=tw -history-file=tw_daily.csv -skip-trust -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=tw -history-file=tw_daily.csv -skip-trust -date-from=2021-11-01                     -dump-trades=dump.csv
//	python3 pead_study.py "LABEL=strategyscan_results_<market>.csv,dump.csv" ...  (repeat per run; box_bottom row, "overall"/first n-excess-SE-sigma columns is the pre-registered number, not "matched")
func CheckBoxBottomReboundExact(candles []data.Candle, p ScreenParams) bool {
	n := len(candles)
	if n < 60+p.MA60SlopeLookback {
		return false
	}
	highs := data.Highs(candles)
	lows := data.Lows(candles)
	closes := data.Closes(candles)

	// 0. Trend pre-filter (2026-08-12 audit, §11.7/§11.7): box-bottom dip
	// buying with no regard for trend is a repeatable 3~4σ loser vs baseline
	// (docs/phase-14-strategy-screens-2.md §11.5/§11.7). Reuses 網 4's exact MA60
	// gate rather than inventing a new one.
	ma60Today := MA(closes, 60)
	ma60Past := MA(closes[:n-p.MA60SlopeLookback], 60)
	if ma60Today == 0 || ma60Past == 0 || ma60Today <= ma60Past {
		return false
	}
	if closes[n-1] <= ma60Today {
		return false
	}

	// 1. Box shape: 30-day close range <= p.BoxMaxRangePct
	boxCloses := closes[n-boxWindowDays:]
	minC := boxCloses[0]
	maxC := boxCloses[0]
	for _, c := range boxCloses {
		if c < minC {
			minC = c
		}
		if c > maxC {
			maxC = c
		}
	}
	if minC <= 0 || (maxC-minC)/minC*100.0 > p.BoxMaxRangePct {
		return false
	}

	// 2. Floor: eval close distance to 30-day min close <= p.BoxFloorPct
	evalClose := closes[n-1]
	if (evalClose-minC)/minC*100.0 > p.BoxFloorPct {
		return false
	}

	// 3. KD: K < 25 AND (golden cross OR hook up)
	kSeries, dSeries := StochasticSeries(highs, lows, closes, 9, 3)
	if kSeries == nil || dSeries == nil || len(kSeries) < 2 {
		return false
	}
	kToday := kSeries[n-1]
	dToday := dSeries[n-1]
	kPrev := kSeries[n-2]
	dPrev := dSeries[n-2]

	if kToday >= kdOversold {
		return false
	}
	goldenCross := kPrev <= dPrev && kToday > dToday
	hookUp := kPrev < kdOversold && kToday > kPrev
	if !goldenCross && !hookUp {
		return false
	}

	// 4. RSI: within last rsiReboundLookback (3) days RSI was < 30 AND RSI today > RSI yesterday
	rsiToday := RSI(closes, 14)
	rsiPrev := RSI(closes[:n-1], 14)
	if rsiToday <= rsiPrev {
		return false
	}
	wasOversold := false
	for i := n - rsiReboundLookback; i < n; i++ {
		if RSI(closes[:i+1], 14) < rsiReboundLevel {
			wasOversold = true
			break
		}
	}
	if !wasOversold {
		return false
	}

	return true
}

func (d *Detector) CheckBoxBottom(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
	hit := BoxBottomRebound(candles, DefaultScreenParams(market.Of(ticker)))
	if hit == nil {
		return nil, ""
	}
	newState = "hit"
	if prevState == "hit" {
		return nil, newState
	}

	daysAgoStr := i18n.T(d.lang, i18n.KeyDaysAgoToday)
	if hit.DaysAgo > 0 {
		daysAgoStr = i18n.T(d.lang, i18n.KeyDaysAgoN, hit.DaysAgo)
	}

	return &Signal{
		Ticker:  ticker,
		Type:    TypeBoxBottom,
		Message: i18n.T(d.lang, i18n.KeyStrategyBoxBottom, ticker, daysAgoStr),
	}, newState
}

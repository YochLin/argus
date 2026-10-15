package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

// TrendPullback evaluates candles for Trend Pullback (網 4) triggers within
// the last strategyLookbackDays. Returns the most recent hit (smallest
// DaysAgo) or nil if none triggered.
func TrendPullback(candles []data.Candle, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < 60 {
			break
		}
		sub := candles[:evalIdx+1]
		if CheckTrendPullbackExact(sub, p) {
			return &StrategyHit{
				Name:    "trend_pullback",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

// TrendBreakout evaluates candles for Trend Breakout (網 3) triggers within
// the last strategyLookbackDays. Returns the most recent hit (smallest
// DaysAgo) or nil if none triggered.
func TrendBreakout(candles []data.Candle, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < 60 {
			break
		}
		sub := candles[:evalIdx+1]
		if CheckTrendBreakoutExact(sub, p) {
			return &StrategyHit{
				Name:    "trend_breakout",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

// CheckTrendPullbackExact evaluates candles' last bar (網 4【趨勢回檔】):
// only buy a pullback inside an established uptrend, never a falling knife.
// See docs/phase-14-strategy-screens-2.md §4.1 for the condition rationale.
//
// # Result: measured (Phase 25 §4.4, dedicated run, 2026-09-01), downgraded
//
// A Phase 14 follow-up had already looked at this screen's regime split
// (TW ~1.5-1.8σ, US ~0.8-1.7σ) and landed on "keep firing in both markets,
// don't delist" — but that judgment never got a user-visible §4.4 annotation
// the way 網 2/網 3/網 5 did, the same gap PLAN.md's 網1／網4 專屬降級評估 item
// flagged for 網 1. This is that item's dedicated run for 網 4: plain-US
// (S&P 500) and TW, both pre-registered splits, same date-clustered
// bootstrap protocol as 網 1/網 2/網 3/網 6 (cmd/strategyscan/pead_study.py,
// 400 resamples of dates, split 2021-11-01), against the random-entry
// control from the SAME run:
//
//	sample                     n     excess     SE   sigma
//	SP500 in-samp [large-cap] 931     +0.54%   0.66    0.8
//	SP500 holdout [large-cap] 860     -0.75%   0.73    1.0  (negative)
//	TW in-samp                1740    +0.09%   1.12    0.1
//	TW holdout                1962    +0.93%   0.86    1.1
//
// No universe clears 1 SE in BOTH splits with the same sign: US is short in
// in-sample and negative-but-borderline in holdout, TW is essentially zero
// in in-sample despite holdout clearing 1 SE. Downgraded unconditionally to
// briefing material under §4.4, closing the annotation gap the Phase 14
// follow-up left open — same treatment as 網 1/網 2/網 3/網 5 (see
// signals.TypeTrendPullback, service.DecorateStrategyHits): still emitted,
// never delisted.
//
// Reproduce (same caches/commands as CheckSqueezeBreakoutExact's doc
// comment, trend_pullback row instead):
//
//	strategyscan -market=us -range=10y -history-file=us_daily.csv -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=us -range=10y -history-file=us_daily.csv -date-from=2021-11-01                     -dump-trades=dump.csv
//	strategyscan -market=tw -history-file=tw_daily.csv -skip-trust -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=tw -history-file=tw_daily.csv -skip-trust -date-from=2021-11-01                     -dump-trades=dump.csv
//	python3 pead_study.py "LABEL=strategyscan_results_<market>.csv,dump.csv" ...  (repeat per run; trend_pullback row, "overall" columns are the pre-registered number, not "matched")
func CheckTrendPullbackExact(candles []data.Candle, p ScreenParams) bool {
	n := len(candles)
	if n < 60+p.MA60SlopeLookback {
		return false
	}
	closes := data.Closes(candles)
	volumes := data.Volumes(candles)
	highs := data.Highs(candles)
	lows := data.Lows(candles)

	// 1. Trend established: MA60 sloping up and close above MA60
	ma60Today := MA(closes, 60)
	ma60Past := MA(closes[:n-p.MA60SlopeLookback], 60)
	if ma60Today == 0 || ma60Past == 0 || ma60Today <= ma60Past {
		return false
	}
	evalClose := closes[n-1]
	if evalClose <= ma60Today {
		return false
	}

	// 2. Pulled back to MA20: |deviation| <= PullbackMA20DevPct
	ma20 := MA(closes, 20)
	dev, ok := DeviationPct(evalClose, ma20)
	if !ok || math.Abs(dev) > p.PullbackMA20DevPct {
		return false
	}

	// 3. Volume dry-up: today's volume ratio < PullbackVolRatio
	volRatio := VolumeRatio(volumes, 20)
	if volRatio == 0 || volRatio >= p.PullbackVolRatio {
		return false
	}

	// 4. KD oversold turn: K < PullbackKDLevel AND (golden cross OR hook up)
	kSeries, dSeries := StochasticSeries(highs, lows, closes, 9, 3)
	if kSeries == nil || dSeries == nil || len(kSeries) < 2 {
		return false
	}
	kToday := kSeries[n-1]
	dToday := dSeries[n-1]
	kPrev := kSeries[n-2]
	dPrev := dSeries[n-2]
	if kToday >= p.PullbackKDLevel {
		return false
	}
	goldenCross := kPrev <= dPrev && kToday > dToday
	hookUp := kPrev < p.PullbackKDLevel && kToday > kPrev
	if !goldenCross && !hookUp {
		return false
	}

	// 5. Reversal bar confirming the bounce
	if !IsBullishReversalBar(candles) {
		return false
	}

	return true
}

// CheckTrendBreakoutExact evaluates candles' last bar (網 3【趨勢突破】):
// new high + bullish MA alignment + attack volume, gated by a deviation cap
// to filter out exhaustion spikes. See
// docs/phase-14-strategy-screens-2.md §4.2 for the condition rationale.
func CheckTrendBreakoutExact(candles []data.Candle, p ScreenParams) bool {
	n := len(candles)
	if n < 60 {
		return false
	}
	closes := data.Closes(candles)
	volumes := data.Volumes(candles)

	// 1. Liquidity: avg volume of preceding 5 days > p.MinAvgVolume5d
	window5v := volumes[n-6 : n-1]
	var sumV int64
	for _, v := range window5v {
		sumV += v
	}
	avgV5 := float64(sumV) / 5.0
	if avgV5 < p.MinAvgVolume5d {
		return false
	}

	// 2. New high
	if !IsNewHigh(closes, p.NewHighLookback) {
		return false
	}

	// 3. Bullish MA alignment: MA5 > MA20 > MA60
	if MAAlignment(closes) != StateBullish {
		return false
	}

	// 4. Attack volume: today's volume ratio >= BreakoutVolMA20
	volRatio := VolumeRatio(volumes, 20)
	if volRatio == 0 || volRatio < p.BreakoutVolMA20 {
		return false
	}

	// 5. Deviation gate: not too extended above MA20 — the most important
	// false-breakout filter (docs/phase-14-strategy-screens-2.md §4.2)
	ma20 := MA(closes, 20)
	dev, ok := DeviationPct(closes[n-1], ma20)
	if !ok || dev > p.MaxMA20DevPct {
		return false
	}

	// 6. Solid bull bar: small upper wick relative to body
	if !IsSolidBullBar(candles[n-1], p.MaxUpperWickRatio) {
		return false
	}

	return true
}

func (d *Detector) CheckTrendPullback(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
	hit := TrendPullback(candles, DefaultScreenParams(market.Of(ticker)))
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
		Type:    TypeTrendPullback,
		Message: i18n.T(d.lang, i18n.KeyStrategyTrendPullback, ticker, daysAgoStr),
	}, newState
}

// CheckTrendBreakout screens 網 3【趨勢突破】. Phase 23's calibration study
// (cmd/strategyscan) found this screen's excess return over a random entry
// under identical exit rules to be NEGATIVE at ~4 sigma in three independent
// out-of-sample slices — US time-slice, US S&P 400 mid-caps, and TW
// time-slice — so it is downgraded under the pre-registration's §4.4 rule to
// briefing material: still emitted (§4.4 是「不下架」), but internal/bot tags
// it with i18n.KeyStrategyUnvalidated so it never reads as an entry
// trigger. Reproduce with:
//
//	strategyscan -market=us -range=10y -date-from=2016-11-01 -date-to=2021-10-31
//	strategyscan -market=us -range=10y -universe=sp400
//	strategyscan -market=tw -range=10y -date-from=2016-11-01 -date-to=2021-10-31 -skip-trust
func (d *Detector) CheckTrendBreakout(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
	hit := TrendBreakout(candles, DefaultScreenParams(market.Of(ticker)))
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
		Type:    TypeTrendBreakout,
		Message: i18n.T(d.lang, i18n.KeyStrategyTrendBreakout, ticker, daysAgoStr),
	}, newState
}

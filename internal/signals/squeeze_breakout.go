package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

// SqueezeBreakout evaluates candles for Squeeze Breakout triggers within the last strategyLookbackDays.
// Returns the most recent hit (smallest DaysAgo) or nil if none triggered.
func SqueezeBreakout(candles []data.Candle, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < 60 {
			break
		}
		sub := candles[:evalIdx+1]
		if CheckSqueezeBreakoutExact(sub, p) {
			return &StrategyHit{
				Name:    "squeeze_breakout",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

// CheckSqueezeBreakoutExact evaluates candles' last bar (網 1【擠壓突破】).
//
// # Result: measured (Phase 25 §4.4, dedicated run, 2026-09-01), downgraded
//
// An incidental S&P 600 small-cap number (2026-09-01, from the
// universe-expansion study) came in one split short of §4.4's bar:
//
//	sample            n     excess     SE   sigma
//	SP600 in-samp    207     +4.31%   2.63    1.6
//	SP600 holdout    249     -0.99%   1.27    0.8  (negative)
//
// but per the precedent 網 2's downgrade set (PR #181 — a downgrade needs a
// run pre-registered FOR that screen, not an incidental number from one
// registered for something else), that alone was explicitly not a decision.
// This is the dedicated run PLAN.md's 網1／網4 專屬降級評估 item asked for:
// plain-US (S&P 500, this screen's original tuning universe) and TW, both
// pre-registered splits, same date-clustered bootstrap protocol as 網 2/網
// 3/網 6 (cmd/strategyscan/pead_study.py, 400 resamples of dates, split
// 2021-11-01), against the random-entry control from the SAME run:
//
//	sample                     n     excess     SE   sigma
//	SP500 in-samp [large-cap] 395     -0.50%   0.87    0.6
//	SP500 holdout [large-cap] 340     +1.34%   1.61    0.8
//	TW in-samp                1527    +0.48%   1.04    0.5
//	TW holdout                1518    +0.12%   0.77    0.2
//
// Neither universe clears 1 SE in even one split, let alone both — weaker
// than the sp600 incidental number, not stronger. Downgraded unconditionally
// to briefing material under §4.4, the same treatment as 網 2/網 3/網 5 (see
// signals.TypeSqueezeBreakout, service.DecorateStrategyHits): still emitted,
// never delisted.
//
// Reproduce (US: default -universe is S&P 500; TW: -skip-trust since this
// screen doesn't use 網 5's trust-net data; CACHE built once per market via
// -build-history, same caches 網 2's/網 6's doc comments reuse):
//
//	strategyscan -market=us -range=10y -history-file=us_daily.csv -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=us -range=10y -history-file=us_daily.csv -date-from=2021-11-01                     -dump-trades=dump.csv
//	strategyscan -market=tw -history-file=tw_daily.csv -skip-trust -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=tw -history-file=tw_daily.csv -skip-trust -date-from=2021-11-01                     -dump-trades=dump.csv
//	python3 pead_study.py "LABEL=strategyscan_results_<market>.csv,dump.csv" ...  (repeat per run; squeeze_breakout row, "overall" columns are the pre-registered number, not "matched")
func CheckSqueezeBreakoutExact(candles []data.Candle, p ScreenParams) bool {
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

	// 2. Squeeze: min bandwidth in trailing 5 bars == min bandwidth in trailing 20 bars
	bwSeries := BollingerBandwidthSeries(closes, 20, 2.0)
	if len(bwSeries) < 20 {
		return false
	}
	window20bw := bwSeries[len(bwSeries)-squeezeBWWindow:]
	min20bw := window20bw[0]
	for _, bw := range window20bw {
		if bw < min20bw {
			min20bw = bw
		}
	}
	window5bw := bwSeries[len(bwSeries)-5:]
	min5bw := window5bw[0]
	for _, bw := range window5bw {
		if bw < min5bw {
			min5bw = bw
		}
	}
	if math.Abs(min5bw-min20bw) > 1e-9 {
		return false
	}

	// 3. Breakout: close > Bollinger Upper (20, 2.0) and today vol >= 2.0 * avgV5
	mean, upper, _, ok := bollingerBands(closes, 20, 2.0)
	if !ok || mean == 0 {
		return false
	}
	evalClose := closes[n-1]
	evalVol := float64(volumes[n-1])
	if evalClose <= upper || evalVol < p.BreakoutVolMult*avgV5 {
		return false
	}

	// 4. Confirmation: MACD histogram today > 0 and yesterday <= 0
	_, _, histToday := MACD(closes)
	_, _, histPrev := MACD(closes[:n-1])
	if histToday <= 0 || histPrev > 0 {
		return false
	}

	return true
}

func (d *Detector) CheckSqueezeBreakout(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string) {
	hit := SqueezeBreakout(candles, DefaultScreenParams(market.Of(ticker)))
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
		Type:    TypeSqueezeBreakout,
		Message: i18n.T(d.lang, i18n.KeyStrategySqueezeBreakout, ticker, daysAgoStr),
	}, newState
}

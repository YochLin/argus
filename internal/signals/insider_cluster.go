package signals

import (
	"math"
	"time"

	"argus/internal/data"
)

// insiderLookbackDays is the trailing window Phase 25 §8.2.3's cluster-buy
// conditions count within — CALENDAR days, not trading days like
// boxWindowDays: SEC filings land on whatever day the filer submits, not on
// a trading calendar.
const insiderLookbackDays = 90

// CheckInsiderClusterBuyExact evaluates candles/txs' last bar (Phase 25
// §8.2 "內部人群聚買"): multiple distinct filers making open-market buys
// (SEC Form 4 code "P") within a trailing 90-calendar-day window, with
// meaningful aggregate notional and no offsetting wave of insider selling
// in the same window — plus the same MA60 trend pre-filter 網 2/網 4 use
// (a cluster buy with no regard for trend is exactly the failure mode
// CheckBoxBottomReboundExact's downgrade already demonstrated once) and the
// standard liquidity gate.
//
// txs is the FULL set of a ticker's transactions the caller has (no
// pre-filtering by date expected) — this function does its own trailing-
// window filtering off candles' last bar, since a backtest replays the same
// evaluation day many times across a scan but Finnhub's history is fetched
// once per ticker (see cmd/strategyscan/insider_cache.go).
//
// # Result: measured, does NOT clear the bar — negative and significant in
// # BOTH splits (worst possible outcome, not just "noise")
//
// Same date-clustered bootstrap/split as every other Phase 25 item
// (cmd/strategyscan/pead_study.py, 400 resamples, split 2021-11-01),
// against the random-entry control from the same run, S&P 500, cache built
// 2026-08-29 (FINNHUB_API_KEY newly configured that day — see
// data.Finnhub.GetInsiderTransactionsRange's doc comment for the coverage
// check that unblocked this):
//
//	sample       n     excess     SE   sigma
//	in-samp     856     -1.55%   0.65    2.4  (negative, significant)
//	holdout     742     -1.41%   0.58    2.4  (negative, significant)
//
// This isn't "no edge found" (noise, the common outcome elsewhere in this
// batch) — it's a significant NEGATIVE excess in both splits, the same
// clean failure mode as CheckTrendBreakoutExact's downgrade. The literature
// mechanism (open-market insider buying is informative) may still be real;
// what this specific screen measured is that gating it behind the MA60
// uptrend pre-filter (borrowed from 網 2/網 4 on the assumption reusing an
// existing gate beats inventing a new one) selects for insiders buying INTO
// an already-extended move — structurally the same "bought the top" problem
// CheckTrendBreakoutExact's own doc comment describes for its own MaxMA20DevPct
// sweep. Not investigated further: re-threshold-hunting after seeing this
// result would be exactly the "test many, keep the winner" pattern §4.4
// exists to prevent. Not wired into checkStatefulSignals or the live bot.
// InsiderMinFilers=2/InsiderMinNotional=250_000 were never swept — the
// negative sign showed up before threshold calibration was reached, so
// there's nothing to calibrate yet.
//
// Reproduce:
//
//	strategyscan -market=us -insider-tx=insider_us.csv -date-from=2016-08-01 -date-to=2026-08-27   (needs FINNHUB_API_KEY; ~503 tickers x 1.1s/call, ~10 min)
//	strategyscan -market=us -range=10y -build-history=us_daily.csv
//	strategyscan -market=us -range=10y -history-file=us_daily.csv -insider-tx-file=insider_us.csv -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=us -range=10y -history-file=us_daily.csv -insider-tx-file=insider_us.csv -date-from=2021-11-01                     -dump-trades=dump.csv
//	python3 pead_study.py "LABEL=strategyscan_results_us.csv,dump.csv" ...  (repeat per run; insider_cluster_buy row, "overall" column is the pre-registered number)
func CheckInsiderClusterBuyExact(candles []data.Candle, txs []data.InsiderTransaction, p ScreenParams) bool {
	n := len(candles)
	if n < 60+p.MA60SlopeLookback {
		return false
	}
	closes := data.Closes(candles)

	// 0. Trend pre-filter — same MA60 gate as 網 2/網 4.
	ma60Today := MA(closes, 60)
	ma60Past := MA(closes[:n-p.MA60SlopeLookback], 60)
	if ma60Today == 0 || ma60Past == 0 || ma60Today <= ma60Past {
		return false
	}
	if closes[n-1] <= ma60Today {
		return false
	}

	// 1. Liquidity: avg volume of preceding 5 days >= p.MinAvgVolume5d
	volumes := data.Volumes(candles)
	window5v := volumes[n-5:]
	var sumV int64
	for _, v := range window5v {
		sumV += v
	}
	if float64(sumV)/5.0 < p.MinAvgVolume5d {
		return false
	}

	// 2-4. Cluster/scale/net-direction over the trailing insiderLookbackDays.
	evalDate := candles[n-1].Date
	windowStart := evalDate.AddDate(0, 0, -insiderLookbackDays)
	filers := make(map[string]bool)
	var buyNotional, sellNotional float64
	for _, tx := range txs {
		d, err := time.Parse("2006-01-02", tx.TransactionDate)
		if err != nil || d.Before(windowStart) || d.After(evalDate) {
			continue
		}
		notional := math.Abs(float64(tx.Change)) * tx.TransactionPrice
		switch tx.TransactionCode {
		case "P":
			filers[tx.Name] = true
			buyNotional += notional
		case "S":
			sellNotional += notional
		}
	}

	if len(filers) < p.InsiderMinFilers {
		return false
	}
	if buyNotional < p.InsiderMinNotional {
		return false
	}
	if sellNotional >= buyNotional {
		return false
	}
	return true
}

package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

// alignByDate maps rows onto candles by date, oldest-first, len ==
// len(candles). A missing date (T86 doesn't list a ticker on a day with no
// institutional trade at all) means net == 0 — that's a real "no activity"
// reading, not missing data, so it's never dropped or interpolated. Do NOT
// align by trailing-end slicing: Yahoo candles and FinMind trading days
// aren't guaranteed to be the same length or even cover the same calendar
// range.
func alignByDate(candles []data.Candle, rows []data.TrustNetDay, field func(data.TrustNetDay) int64) []int64 {
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date.Format("2006-01-02")] = field(r)
	}
	out := make([]int64, len(candles))
	for i, c := range candles {
		out[i] = byDate[c.Date.Format("2006-01-02")]
	}
	return out
}

// AlignTrustNet maps rows' 投信 net onto candles by date. See alignByDate.
func AlignTrustNet(candles []data.Candle, rows []data.TrustNetDay) []int64 {
	return alignByDate(candles, rows, func(r data.TrustNetDay) int64 { return r.Net })
}

// AlignForeignNet maps rows' 外資 net onto candles by date. See alignByDate.
func AlignForeignNet(candles []data.Candle, rows []data.TrustNetDay) []int64 {
	return alignByDate(candles, rows, func(r data.TrustNetDay) int64 { return r.ForeignNet })
}

// TrustNetSum totals the last n days' net buy/sell.
func TrustNetSum(trustNet []int64, n int) int64 {
	total := len(trustNet)
	if n > total {
		n = total
	}
	var sum int64
	for _, v := range trustNet[total-n:] {
		sum += v
	}
	return sum
}

// trustBuyWindow returns 3 if the trailing 3-day net sum is positive
// (primary condition), else 5 if at least 3 of the trailing 5 days were net
// buys (alternate condition — catches a steady-but-not-every-day buyer the
// 3-day sum can miss), else 0 (neither condition met).
func trustBuyWindow(trustNet []int64) int {
	n := len(trustNet)
	if n >= 3 && TrustNetSum(trustNet, 3) > 0 {
		return 3
	}
	if n >= 5 {
		buyDays := 0
		for _, v := range trustNet[n-5:] {
			if v > 0 {
				buyDays++
			}
		}
		if buyDays >= 3 {
			return 5
		}
	}
	return 0
}

// TrustNetVolPct is (net buy in [from,to] / total volume in [from,to]) x
// 100 — the scale-free "投信買超強度" measure. from/to are inclusive
// candles/trustNet indices.
func TrustNetVolPct(candles []data.Candle, trustNet []int64, from, to int) float64 {
	if from < 0 || to >= len(candles) || to >= len(trustNet) || from > to {
		return 0
	}
	var netSum int64
	var volSum int64
	for i := from; i <= to; i++ {
		netSum += trustNet[i]
		volSum += candles[i].Volume
	}
	if volSum == 0 {
		return 0
	}
	return float64(netSum) / float64(volSum) * 100.0
}

// TrustFollowTechnicalGate checks CheckTrustFollowExact's candle-only
// conditions (liquidity + MA20/MA60 trend + deviation) — the cheap half of
// the screen. Exported so internal/bot can run it against the whole daily
// TW universe before spending a FinMind request on only the tickers that
// already qualify, the same short-circuit shape bot.revenueGrowthOK uses for
// 網 3's fundamentals gate.
func TrustFollowTechnicalGate(candles []data.Candle, p ScreenParams) bool {
	n := len(candles)
	if n < 60+p.MA60SlopeLookback {
		return false
	}

	// 1. Liquidity: avg volume of preceding 5 days >= p.MinAvgVolume5d
	volumes := data.Volumes(candles)
	window5v := volumes[n-5:]
	var sumV int64
	for _, v := range window5v {
		sumV += v
	}
	avgV5 := float64(sumV) / 5.0
	if avgV5 < p.MinAvgVolume5d {
		return false
	}

	// 2. Trend: close above MA20 and MA60, MA60 sloping up
	closes := data.Closes(candles)
	ma20 := MA(closes, 20)
	ma60Today := MA(closes, 60)
	ma60Past := MA(closes[:n-p.MA60SlopeLookback], 60)
	evalClose := closes[n-1]
	if ma20 == 0 || ma60Today == 0 || ma60Past == 0 || ma60Today <= ma60Past {
		return false
	}
	if evalClose <= ma20 || evalClose <= ma60Today {
		return false
	}

	// 3. Exclude already-pumped: deviation from MA20 within range
	dev, ok := DeviationPct(evalClose, ma20)
	if !ok || math.Abs(dev) >= p.MaxMA20DevPct {
		return false
	}

	return true
}

// CheckTrustFollowExact evaluates candles/trustNet/foreignNet's last bar (網
// 5【主力跟單】v2): a short-window 投信 buying streak with real intensity,
// inside an established MA60 uptrend, excluding days where 外資 is heavily
// selling into the same tape ("土洋對作"). Replaces Phase 15 v1's 60-day
// dormant-segment approximation — see docs/phase-15-trust-follow.md — which
// backtested at -1.4pp excess and was never wired.
//
// Phase 25 §4.4 (2026-08-27): trustNet/foreignNet's DATA SOURCE moved from
// FinMind (a secondhand mirror) to TWSE's own T86 report — the same report
// FinMind's own TaiwanStockInstitutionalInvestorsBuySellWide is itself
// sourced from — via internal/data/twse_t86.go. This function's conditions
// were left untouched (per §4's "swap the source first, treat threshold
// changes as a separate follow-up" instruction), so this is purely a
// provenance/plumbing change: canonical instead of secondhand, free/keyless
// instead of needing FINMIND_TOKEN (which, notably, was never actually
// configured with a real value in this deployment — 網5 was silently
// disabled in production before this change, not merely running on
// secondhand data).
//
// # Result (2026-08-29): measured, does NOT clear the bar — downgraded
//
// The §4.4-mandated retest finished. Building the T86 side-cache took ~10
// hours of real wall-clock time (started 2026-08-27 23:35, finished
// 2026-08-29 00:32) — much longer than the ~3h estimate below, because the
// WAF's occasional `context deadline exceeded`/`connection reset by peer`
// timeouts (not full IP blocks this run — those retried and succeeded) added
// up over a 2,457-trading-day crawl. Same date-clustered bootstrap as every
// other Phase 25 item (cmd/strategyscan/t86_study.py, 400 resamples, split
// 2021-11-01), tw150 universe, against the random-entry control from the
// same run:
//
//	sample       n     excess     SE   sigma
//	in-samp   1562     -0.72%   0.69    1.0  (negative, at the bar)
//	holdout   2371     +0.44%   0.80    0.5  (positive, short of 1 SE)
//
// Neither split clears >1 SE positive — in-sample is negative (right at the
// 1 SE line), holdout is positive but well short. Same "one split short of
// 1 SE" failure pattern box_bottom's US large-cap slice hit. This settles
// the §4.4 question the source swap left open: T86 did not turn 網5 into a
// validated screen. Downgraded to briefing-only, same treatment as 網2/網3
// (internal/service/scan.go's DecorateStrategyHits,
// i18n.KeyStrategyUnvalidatedTrustFollow) — still emitted, never delisted.
// This also closes the loop PR #178 opened: 網5 went live unconditionally
// on the T86 swap with zero measurement (FINMIND_TOKEN had never actually
// been configured, so it was silently dead in production before that PR) —
// the honest state, now that it's actually been measured, is "measured and
// downgraded," not "measured and validated."
//
// Universe note: T86 is TWSE's own report — TSE (上市) listings only,
// structurally no TPEx (上櫃/OTC) coverage. 9 of tw150's 118 tickers never
// appeared anywhere in the 10-year whole-market crawl (0 rows each) despite
// having real Yahoo/Shioaji price history — a permanent scope gap, not a
// crawl failure. cmd/strategyscan/main.go now drops those 9 from the
// universe up front (right after loading -t86-file, before the fetch loop)
// rather than let them trip the existing 5%-fetch-error FATAL guard, which
// exists to catch real cache problems, not this structural exclusion.
//
// Reproduce (T86 cache build is the slow part — ~10h in practice; TW price
// history via a live Shioaji daemon, `shioaji server start`, ~15 min for
// the whole market, or omit -history-file to fetch just tw150 from Yahoo
// as this run did):
//
//	strategyscan -market=tw -build-t86-cache=t86.csv -date-from=2016-08-01 -date-to=2026-08-27
//	strategyscan -market=tw -range=10y -t86-file=t86.csv -date-from=2016-11-01 -date-to=2021-10-31 -dump-trades=dump.csv
//	strategyscan -market=tw -range=10y -t86-file=t86.csv -date-from=2021-11-01                     -dump-trades=dump.csv
//	python3 t86_study.py "LABEL=strategyscan_results_tw.csv,dump.csv" ...  (repeat per run; "overall" column is the pre-registered number)
func CheckTrustFollowExact(candles []data.Candle, trustNet, foreignNet []int64, p ScreenParams) bool {
	n := len(candles)
	if len(trustNet) != n || len(foreignNet) != n || !TrustFollowTechnicalGate(candles, p) {
		return false
	}

	// 4. Trust momentum: 3-day net sum > 0, or >=3 of the last 5 days net buy
	window := trustBuyWindow(trustNet)
	if window == 0 {
		return false
	}

	// 5. Intensity: net buy / volume over that same window >= TrustNetVolPctMin
	if TrustNetVolPct(candles, trustNet, n-window, n-1) < p.TrustNetVolPctMin {
		return false
	}

	// 6. 土洋對作: today's 外資 net sell isn't overwhelming the 投信 buy
	volumes := data.Volumes(candles)
	foreignPct := float64(foreignNet[n-1]) / float64(volumes[n-1]) * 100.0
	if foreignPct <= p.TrustForeignSellVolPctMax {
		return false
	}

	return true
}

// TrustFollow evaluates candles/trustNet/foreignNet for Trust Follow (網 5)
// triggers within the last strategyLookbackDays. All three slices must
// already be the same length (post AlignTrustNet/AlignForeignNet) — each
// offset slices all three at the same index, the easiest line in this file
// to get wrong.
func TrustFollow(candles []data.Candle, trustNet, foreignNet []int64, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx+1 < 60+p.MA60SlopeLookback {
			break
		}
		if CheckTrustFollowExact(candles[:evalIdx+1], trustNet[:evalIdx+1], foreignNet[:evalIdx+1], p) {
			return &StrategyHit{
				Name:    "trust_follow",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

// TypeTrustFollow is 網 5's Signal.Type. Exported for the same reason as
// TypeBoxBottom/TypeTrendBreakout (internal/signals/strategies.go) — see
// CheckTrustFollowExact's doc comment for the numbers behind its §4.4
// downgrade.
const TypeTrustFollow = "strategy_trust_follow"

func (d *Detector) CheckTrustFollow(ticker string, candles []data.Candle, trustNet, foreignNet []int64, prevState string) (sig *Signal, newState string) {
	p := DefaultScreenParams(market.Of(ticker))
	hit := TrustFollow(candles, trustNet, foreignNet, p)
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

	evalIdx := len(candles) - 1 - hit.DaysAgo
	subTrust := trustNet[:evalIdx+1]
	window := trustBuyWindow(subTrust)
	volPct := TrustNetVolPct(candles[:evalIdx+1], subTrust, evalIdx+1-window, evalIdx)

	return &Signal{
		Ticker:  ticker,
		Type:    TypeTrustFollow,
		Message: i18n.T(d.lang, i18n.KeyStrategyTrustFollow, ticker, daysAgoStr, window, volPct),
	}, newState
}

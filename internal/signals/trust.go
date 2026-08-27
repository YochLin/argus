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
// The §4.4-mandated RETEST — rerun the pre-registered two-split backtest
// against the new source to separate "data source changed the numbers" from
// "conditions changed the numbers" — was NOT completed. Building the T86
// side-cache this retest needs (cmd/strategyscan -build-t86-cache) requires
// walking every calendar day of the 10-year window one HTTP request at a
// time (T86 has no ranged query — see twse_t86.go). Live-verified 2026-08-27
// while building this exact cache: TWSE's rwd/T86 endpoint enforces an
// aggressive anti-scraping WAF — a burst of ~50 requests within ~20s
// triggered a sustained IP-level block (still active >20 minutes later,
// and it blocked EVERY date including "today," not just the ones scraped —
// confirmed not host-wide, since openapi.twse.com.tw stayed reachable
// throughout). A 2s/request pace held clean through 20+ consecutive fresh
// dates with no re-trigger and is what -build-t86-cache now uses (see
// t86_cache.go), but at that pace a full 10-year, whole-market crawl takes
// roughly 3+ hours of continuous wall-clock time — confirmed by timing a
// live partial run (30 minutes covered 2016-08-01..2018-03, ~16% of the
// 2016-08..2026-08 window). That is a real, evidenced wall-clock cost, not
// a design gap, and it did not finish within this session.
//
// So: NO excess-return numbers exist yet for 網5 against T86, in either
// direction. Do not infer anything about whether the source swap helped,
// hurt, or was neutral — nothing was measured. What IS verified: the T86
// client parses real historical responses correctly across TWSE's pre-2017
// (16-column) and post-2017 (19-column) report layouts (see
// twse_t86_test.go, built from live-captured fixtures), the day-major cache
// builder/loader round-trips correctly (t86_cache_test.go), and a live
// partial build (2016-08-01..2018-03, ~41k rows, 118 tw150 tickers) ran
// end-to-end against the real endpoint without a parsing or alignment
// error. To finish the retest: let `strategyscan -market=tw
// -build-t86-cache=t86.csv -date-from=2016-08-01 -date-to=2026-08-27` run to
// completion (~3h), then run both pre-registered slices with
// `-t86-file=t86.csv -dump-trades=...` and feed the results to
// cmd/strategyscan/t86_study.py — same date-clustered bootstrap method as
// pead_study.py, same >1SE bar as every other Phase 25 workstream. Until
// then, the last real numbers for 網5 remain whatever Phase 23 PR1 measured
// against FinMind (see docs/phase-15-trust-follow.md) — this change neither
// confirms nor overturns that.
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
		Type:    "strategy_trust_follow",
		Message: i18n.T(d.lang, i18n.KeyStrategyTrustFollow, ticker, daysAgoStr, window, volPct),
	}, newState
}

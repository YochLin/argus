package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

// AlignTrustNet maps rows onto candles by date, oldest-first, len ==
// len(candles). A missing date (T86 doesn't list a ticker on a day with no
// institutional trade at all) means net == 0 — that's a real "no activity"
// reading, not missing data, so it's never dropped or interpolated. Do NOT
// align by trailing-end slicing: Yahoo candles and FinMind trading days
// aren't guaranteed to be the same length or even cover the same calendar
// range.
func AlignTrustNet(candles []data.Candle, rows []data.TrustNetDay) []int64 {
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date.Format("2006-01-02")] = r.Net
	}
	out := make([]int64, len(candles))
	for i, c := range candles {
		out[i] = byDate[c.Date.Format("2006-01-02")]
	}
	return out
}

// TrustConsecutiveBuyDays counts trailing days with a positive net buy.
func TrustConsecutiveBuyDays(trustNet []int64) int {
	days := 0
	for i := len(trustNet) - 1; i >= 0; i-- {
		if trustNet[i] <= 0 {
			break
		}
		days++
	}
	return days
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

// TrustNetVolPct is (net buy in [from,to] / total volume in [from,to]) x
// 100 — the scale-free "主力吃貨佔量" proxy that stands in for holding
// ratio (docs/phase-15-trust-follow.md §3.1). from/to are inclusive
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

// CheckTrustAccumulationExact evaluates candles/trustNet's last bar (網
// 5【主力跟單】): a trust-buying streak that follows a dormant period,
// approximating "just started accumulating" without the holding-ratio data
// that isn't available for free (docs/phase-15-trust-follow.md §3). The
// dormant-segment check (condition 5) is the core of this screen — it's
// what stands in for the textbook's "持股比例 2~8%" condition and must not
// be loosened before the backtest in cmd/strategyscan says otherwise.
func CheckTrustAccumulationExact(candles []data.Candle, trustNet []int64, p ScreenParams) bool {
	n := len(candles)
	if n < p.TrustLookback || len(trustNet) != n {
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

	// 2. Consecutive buying streak within [TrustConsecMin, TrustConsecMax]
	consec := TrustConsecutiveBuyDays(trustNet)
	if consec < p.TrustConsecMin || consec > p.TrustConsecMax {
		return false
	}

	lookbackFrom := n - p.TrustLookback

	// 3. Magnitude: net buy / volume over the full lookback window
	if TrustNetVolPct(candles, trustNet, lookbackFrom, n-1) < p.TrustNetVolPctMin {
		return false
	}

	// 4. Dormant segment: the lookback window minus the trailing
	// TrustConsecMax days must show near-zero accumulation, or this isn't a
	// first-time build — it's a streak partway through an existing one.
	dormantTo := n - 1 - p.TrustConsecMax
	if dormantTo < lookbackFrom {
		return false
	}
	if TrustNetVolPct(candles, trustNet, lookbackFrom, dormantTo) >= p.TrustDormantPctMax {
		return false
	}

	// 5. Exclude already-pumped: deviation from MA20 within range
	closes := data.Closes(candles)
	ma20 := MA(closes, 20)
	dev, ok := DeviationPct(closes[n-1], ma20)
	if !ok || math.Abs(dev) >= p.MaxMA20DevPct {
		return false
	}

	return true
}

// TrustAccumulation evaluates candles/trustNet for Trust Accumulation (網
// 5) triggers within the last strategyLookbackDays. candles and trustNet
// must already be the same length (post AlignTrustNet) — each offset slices
// both at the same index, the easiest line in this file to get wrong.
func TrustAccumulation(candles []data.Candle, trustNet []int64, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx+1 < p.TrustLookback {
			break
		}
		if CheckTrustAccumulationExact(candles[:evalIdx+1], trustNet[:evalIdx+1], p) {
			return &StrategyHit{
				Name:    "trust_accumulation",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

func (d *Detector) CheckTrustAccumulation(ticker string, candles []data.Candle, trustNet []int64, prevState string) (sig *Signal, newState string) {
	p := DefaultScreenParams(market.Of(ticker))
	hit := TrustAccumulation(candles, trustNet, p)
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
	consec := TrustConsecutiveBuyDays(subTrust)
	volPct := TrustNetVolPct(candles[:evalIdx+1], subTrust, evalIdx+1-p.TrustLookback, evalIdx)

	return &Signal{
		Ticker:  ticker,
		Type:    "strategy_trust_accumulation",
		Message: i18n.T(d.lang, i18n.KeyStrategyTrustAccumulation, ticker, daysAgoStr, consec, volPct),
	}, newState
}

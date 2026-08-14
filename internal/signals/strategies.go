package signals

import (
	"math"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/market"
)

const (
	strategyLookbackDays = 5    // 「近 N 日內曾觸發」窗口;原為配合 universe scan 5 天輪掃,改為每日全掃後留作漏掃緩衝(假日/bot 停機/掃描中斷),重複觸發由 signal_states 去重
	squeezeBWWindow      = 20   // 帶寬新低的回看窗
	boxWindowDays        = 30   // 網 2 箱型窗口
	kdOversold           = 25.0 // K 低檔門檻
	rsiReboundLevel      = 30.0 // RSI 超賣線
	rsiReboundLookback   = 3    // RSI 近 N 日內曾 < 30

	FamilyStrategySqueeze = "strategy_squeeze"
	FamilyStrategyBox     = "strategy_box"

	FamilyStrategyPullback = "strategy_pullback"
	FamilyStrategyBreakout = "strategy_breakout"

	FamilyStrategyTrust = "strategy_trust"
)

// ScreenParams holds the market-calibrated thresholds CheckSqueezeBreakoutExact/
// CheckBoxBottomReboundExact screen against (Phase 13 §7) — a struct param
// rather than fields on Detector because SqueezeBreakout/BoxBottomRebound are
// pure functions called directly by computeTechnicals, not methods on
// Detector. minAvgVolume5d/boxMaxRangePct are the two knobs that actually
// differ per market (see DefaultScreenParams); boxFloorPct/breakoutVolMult
// are relative values with no institutional-rule dependency, so both markets
// share one constant for those instead of duplicating them per-market.
// ponytail: MinAvgVolume5d is share-count, not notional (avgV5 x price) — a
// single market-scale threshold on notional would let both markets share one
// number instead of a per-market share-count constant, but that's a bigger
// change than this pass calls for (§7's "本輪不做"). Upgrade path: swap the
// liquidity check in CheckSqueezeBreakoutExact for a notional comparison if
// per-market recalibration ever proves too fiddly.
type ScreenParams struct {
	MinAvgVolume5d  float64 // 網 1 流動性門檻(股)
	BoxMaxRangePct  float64 // 箱型高低差上限 %
	BoxFloorPct     float64 // 距箱底 %
	BreakoutVolMult float64 // 突破日量 >= N x 前 5 日均量

	// 網 3 趨勢突破（Phase 14）
	NewHighLookback   int     // 創新高回看窗（日），60
	BreakoutVolMA20   float64 // 突破日量 >= N x MA20 均量，1.5
	MaxMA20DevPct     float64 // 距 MA20 乖離上限 %，US 15 / TW 12（台股 ±10% 漲跌幅，波動結構較緊）
	MaxUpperWickRatio float64 // 上影線 / 實體 上限，0.5

	// 網 4 趨勢回檔（Phase 14）
	PullbackMA20DevPct float64 // 距 MA20 ±N%，2.0
	PullbackVolRatio   float64 // 拉回量縮門檻，VolumeRatio(20) < N
	PullbackKDLevel    float64 // K 低檔門檻，30
	MA60SlopeLookback  int     // MA60 上彎的比較窗（日），10

	// 基本面短路求值開關（bot 層依此決定是否打 FinMind/Finnhub，見 internal/bot）
	RequireRevenueGrowth bool    // TW true / US false
	MinRevenueGrowthPct  float64 // 10.0

	// 網 5 主力跟單（Phase 15 v2，TW only）
	TrustNetVolPctMin         float64 // 短窗（3或5日）投信買超 / 該窗成交量 下限 %，3.0
	TrustForeignSellVolPctMax float64 // 當日外資賣超 / 當日成交量 下限（負值，跌破即排除「土洋對作」），-3.0
	RequireTrustData          bool    // TW true / US false —— bot 層據此決定要不要打 FinMind
}

// DefaultScreenParams returns m's calibrated ScreenParams. TW's
// minAvgVolume5d/boxMaxRangePct are wider than US's starting point per
// docs/tw-us-parity.md §7 — TW's tw150 universe skews toward larger caps
// (1,000 張 filters out thin mid-caps an SP500-scale US threshold wouldn't
// catch), and TW's ±10% daily limit makes a 15% "consolidation" box
// virtually untriggerable, so it's widened to 18%. These are calibration
// starting points, not backtested values — see §7's calibration-script note
// (deliberately not committed to this codebase). Phase 14 adds 網 3/網 4's
// thresholds under the same "starting point, not backtested" caveat — TW's
// MaxMA20DevPct is tighter (12% vs US 15%) for the same ±10% daily-limit
// reason BoxMaxRangePct is wider; RequireRevenueGrowth is TW-only since
// Finnhub has no monthly-revenue concept (data.Fundamentals.MonthRevenueYoYPct
// is TW-only, see that field's doc comment).
func DefaultScreenParams(m market.MarketID) ScreenParams {
	if m == market.TW {
		return ScreenParams{
			MinAvgVolume5d: 1_000_000, BoxMaxRangePct: 18.0, BoxFloorPct: 2.0, BreakoutVolMult: 2.0,
			NewHighLookback: 60, BreakoutVolMA20: 1.5, MaxMA20DevPct: 12.0, MaxUpperWickRatio: 0.5,
			PullbackMA20DevPct: 2.0, PullbackVolRatio: 0.8, PullbackKDLevel: 30.0, MA60SlopeLookback: 10,
			RequireRevenueGrowth: true, MinRevenueGrowthPct: 10.0,
			TrustNetVolPctMin: 3.0, TrustForeignSellVolPctMax: -3.0, RequireTrustData: true,
		}
	}
	return ScreenParams{
		MinAvgVolume5d: 500_000, BoxMaxRangePct: 15.0, BoxFloorPct: 2.0, BreakoutVolMult: 2.0,
		NewHighLookback: 60, BreakoutVolMA20: 1.5, MaxMA20DevPct: 15.0, MaxUpperWickRatio: 0.5,
		PullbackMA20DevPct: 2.0, PullbackVolRatio: 0.8, PullbackKDLevel: 30.0, MA60SlopeLookback: 10,
		RequireRevenueGrowth: false, MinRevenueGrowthPct: 10.0,
		RequireTrustData: false,
	}
}

type StrategyHit struct {
	Name    string // "squeeze_breakout" / "box_bottom"
	DaysAgo int    // 0 = 今天觸發
}

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
		Type:    "strategy_squeeze_breakout",
		Message: i18n.T(d.lang, i18n.KeyStrategySqueezeBreakout, ticker, daysAgoStr),
	}, newState
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
		Type:    "strategy_box_bottom",
		Message: i18n.T(d.lang, i18n.KeyStrategyBoxBottom, ticker, daysAgoStr),
	}, newState
}

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
		Type:    "strategy_trend_pullback",
		Message: i18n.T(d.lang, i18n.KeyStrategyTrendPullback, ticker, daysAgoStr),
	}, newState
}

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
		Type:    "strategy_trend_breakout",
		Message: i18n.T(d.lang, i18n.KeyStrategyTrendBreakout, ticker, daysAgoStr),
	}, newState
}

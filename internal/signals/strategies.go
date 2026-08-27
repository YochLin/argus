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

// TypeTrendBreakout is 網 3's Signal.Type. Exported (unlike the other
// four screens' types, which nothing outside this package matches on)
// because internal/bot has to single it out for the §4.4 downgrade
// notice — see CheckTrendBreakout's doc comment.
const TypeTrendBreakout = "strategy_trend_breakout"

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

	// 網 6 財報後漂移（Phase 25 §2）
	GapUpPct   float64 // 跳空開高門檻 %，US 5.0 / TW 7.0（台股 ±10% 漲跌幅，5% 太鬆）
	GapVolMult float64 // 跳空日量 >= N x MA20 均量，3.0

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
// MaxMA20DevPct is the one knob here that HAS since been backtested, and the
// result was to leave it alone. The "don't chase an extended breakout"
// intuition says to tighten it; cmd/strategyscan's -tb-dev-sweep re-screened
// 網 3 at 12/10/8/6 against a random-entry control under identical exit rules,
// and every tightening made things monotonically worse in all four samples.
// Excess return vs that control (US in-sample / US OOS / S&P 400 mid-cap /
// TW OOS; TW's own default is 12, so it has no 15 row):
//
//	cap  US IS    US OOS   SP400    TW OOS
//	15   -0.20%   -0.95%   -0.95%      n/a
//	12   -0.26%   -1.05%   -1.12%   -1.69%
//	10   -0.65%   -1.18%   -1.15%   -1.79%
//	 8   -0.66%   -1.23%   -1.37%   -1.83%
//	 6   -0.83%   -1.19%   -1.16%   -2.35%
//
// The extended breakouts are the ones that carry the screen; capping them out
// removes more signal than noise. Don't re-tighten this without re-running
// that sweep — and note the whole screen is downgraded anyway (see
// Detector.CheckTrendBreakout).
func DefaultScreenParams(m market.MarketID) ScreenParams {
	if m == market.TW {
		return ScreenParams{
			MinAvgVolume5d: 1_000_000, BoxMaxRangePct: 18.0, BoxFloorPct: 2.0, BreakoutVolMult: 2.0,
			NewHighLookback: 60, BreakoutVolMA20: 1.5, MaxMA20DevPct: 12.0, MaxUpperWickRatio: 0.5,
			PullbackMA20DevPct: 2.0, PullbackVolRatio: 0.8, PullbackKDLevel: 30.0, MA60SlopeLookback: 10,
			GapUpPct: 7.0, GapVolMult: 3.0,
			RequireRevenueGrowth: true, MinRevenueGrowthPct: 10.0,
			TrustNetVolPctMin: 3.0, TrustForeignSellVolPctMax: -3.0, RequireTrustData: true,
		}
	}
	return ScreenParams{
		MinAvgVolume5d: 500_000, BoxMaxRangePct: 15.0, BoxFloorPct: 2.0, BreakoutVolMult: 2.0,
		NewHighLookback: 60, BreakoutVolMA20: 1.5, MaxMA20DevPct: 15.0, MaxUpperWickRatio: 0.5,
		PullbackMA20DevPct: 2.0, PullbackVolRatio: 0.8, PullbackKDLevel: 30.0, MA60SlopeLookback: 10,
		GapUpPct: 5.0, GapVolMult: 3.0,
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

// CheckBoxBottomReboundExact evaluates candles' last bar (網 2【箱底反彈】).
//
// Phase 25 §4.6 note — this is not 網 2's own study, it is what fell out of
// the 網 6 runs, which measured all five screens against the same control on
// a universe 網 2 had not been checked against before. On S&P 400 mid-caps
// 網 2's excess return is negative in BOTH time slices and significant in
// both: -3.35% at 2.3 sigma (holdout) and -2.45% at 2.9 sigma (in-sample),
// the strongest negative of any screen in those runs. On S&P 500 large caps
// it is +0.66%/-1.03%, i.e. noise, which is why this never showed up before.
//
// Deliberately NOT acted on here: a §4.4 downgrade is a separate decision
// from shipping 網 6, and folding it into that PR would mean changing a live
// screen on the strength of a study that was pre-registered for something
// else. The honest next step is a run whose stated purpose is 網 2 on
// mid-caps. Until then, treat this as a flag, not a verdict.
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

// PostGapDrift evaluates candles for Post-Earnings-Announcement-Drift (網 6)
// triggers within the last strategyLookbackDays. Returns the most recent hit
// (smallest DaysAgo) or nil if none triggered.
func PostGapDrift(candles []data.Candle, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx < 60 {
			break
		}
		sub := candles[:evalIdx+1]
		if CheckPostGapDriftExact(sub, p) {
			return &StrategyHit{
				Name:    "post_gap_drift",
				DaysAgo: offset,
			}
		}
	}
	return nil
}

// CheckPostGapDriftExact evaluates candles' last bar (網 6【財報後漂移】):
// a stock that gapped up hard on heavy volume, held the gap into the close,
// and did it at a new high. Phase 25 §2.
//
// The premise is post-earnings-announcement drift: after a genuine positive
// surprise a stock keeps drifting the same way for weeks, because analysts
// anchor on stale estimates, institutional accumulation is spread over days,
// and not everyone reads the filing on the day. That is an
// information-diffusion story, not a "the market is dumb" story, which is why
// it has survived 60 years and every out-of-sample retest thrown at it.
//
// Note what this screen does NOT do: look up an earnings date, or an analyst
// estimate. The gap itself is the surprise proxy — for an index constituent
// the four earnings days a year dominate the list of largest overnight moves,
// and the market's own reaction is a fresher aggregate of the news than a
// consensus estimate that may be weeks stale. "Earnings-day return" is a
// standard surprise proxy in the literature. The practical consequence is
// that this screen needs price and volume only, so it backtests against the
// existing OHLCV cache with no new data source (Phase 23 PR8's finding that
// Finnhub's free tier caps at 4 quarters is what makes that matter).
//
// Conditions 3 and 4 are the two that stop this from being "buy anything that
// gapped", and neither is optional:
//
//   - Without 3 (close in the upper half of the day's range), a stock that
//     gapped +7% and was sold all the way back to the low qualifies. That is
//     the news being sold into, and it drifts DOWN — the opposite trade.
//   - Without 4 (new high), a sharp bounce inside a downtrend qualifies. A
//     gap off a base is not drift; drift is a repricing to a new level.
//
// # Result: measured, did NOT pass, deliberately NOT wired to the bot
//
// Phase 25 §2.5 pre-registered the bar before any of this was run: excess
// return over the random-entry control under identical exit rules had to
// clear 1 SE in BOTH time slices of the primary sample (S&P 400 mid-caps,
// where §2.6 argued PEAD should be strongest because attention is scarcest).
// It did not. Date-clustered bootstrap, 400 resamples of dates (not trades —
// same-day entries co-move), split at 2021-11-01, the earlier slice being the
// holdout:
//
//	sample                    n     excess     SE   sigma
//	SP400 holdout  [primary] 208     +2.30%   2.54    0.9
//	SP400 in-samp  [primary] 270     +0.72%   1.13    0.6
//	SP500 holdout            288     +0.85%   0.91    0.9
//	SP500 in-samp            323     +1.94%   1.43    1.4
//
// So this screen exists for cmd/strategyscan to keep measuring, and for the
// record above to be reproducible. It is NOT in i18n, NOT in
// service.CheckStatefulSignals, and must not be added there on the strength
// of the numbers above — §4.4's whole point is that the bar does not move
// after seeing the result.
//
// What is genuinely interesting, and why this is "underpowered" rather than
// "disproven": it is the only screen here whose excess is positive-signed in
// all four samples. Every other screen flips sign between universes. Four of
// four is p≈0.06 under a coin-flip null, which is suggestive and not
// conclusive — and the four are not independent, since SP400 and SP500 span
// the same calendar. The blocker is sample size, not effect size: the screen
// fires ~200-320 times per slice because a >=5% gap on >=3x volume at a
// 60-day high is rare. Resolving it needs more names (Russell-2000 scale),
// not more tuning of the thresholds, and re-tuning them against these same
// slices would just fit the noise.
//
// Two secondary findings from the same runs:
//
//   - Entry timing is not the problem. The T+1 variant (same signal, entered
//     one bar later — see gapDriftT1Strategy in cmd/strategyscan) tracks T+0
//     within 0.6pp in every sample, so the effect is not a one-session
//     continuation that a real fill would miss.
//   - §2.6's slippage worry does not apply to the excess. Slippage is a
//     constant charge per round trip, it does not move the exit, and the
//     control pays it too — so tripling it to 0.3%/side leaves every excess
//     figure above identical to the cent. (The sigmas wobble by ~0.03
//     between those two runs only because the bootstrap RNG has advanced;
//     the underlying returns differ by a constant.) What it does cost is
//     0.40pp of ABSOLUTE return (SP400 holdout +5.47% -> +5.07%), small only
//     because the average hold is ~37 days. Slippage can make the trade
//     unprofitable; it cannot manufacture or destroy an edge over a control
//     that trades the same way. Worth remembering before running that
//     sensitivity again on any screen measured against this control.
//
// # Update — post_gap_drift_confirmed (Phase 25 §8.1), also measured, also NOT shipped
//
// §8.1 asked whether the gap-based surprise proxy is being diluted by gaps
// that have nothing to do with an actual earnings release — i.e. does
// requiring the signal day to fall within ±1 trading day of a real SEC 10-Q/
// 10-K filed date (data.SEC.GetFilingDates, cmd/strategyscan's
// -earnings-dates-file cache) sharpen the edge. Same S&P 400 universe, same
// two time slices, same random-entry control, same bootstrap:
//
//	sample                         n     excess     SE   sigma
//	SP400 holdout, confirmed      88     +2.23%   2.39    0.9
//	SP400 in-samp, confirmed     155     +2.22%   1.58    1.4
//	(for reference, unconfirmed) 208     +2.30%   2.54    0.9
//	(for reference, unconfirmed) 270     +0.72%   1.22    0.6
//
// Pre-registered bar was the same as above: >1 SE in BOTH splits. Holdout
// stayed at 0.9 sigma — did not clear. Still NOT wired into i18n or
// service.CheckStatefulSignals, same as the unconfirmed screen.
//
// The pre-registered extra requirement (trigger count strictly lower than
// unconfirmed) held in both slices: 88 < 208 (holdout), 155 < 270 (in-sample)
// — confirming against a live filing calendar does cut the sample roughly in
// half, as expected (not every >=5% gap on 3x volume is an earnings gap).
//
// What's actually informative here: excess return on the holdout slice is
// UNCHANGED to within 0.07pp (+2.30% -> +2.23%) despite the sample nearly
// halving. Per §8.1.4's own pre-registered instruction for exactly this
// pattern ("excess didn't rise AND the sample shrank -> the answer is
// universe expansion, not loosening the window"), that is the honest
// reading — not that confirmation adds nothing, but that it isn't the lever
// this screen needs. The in-sample slice's jump to 1.4 sigma is the kind of
// one-good-one-bad split split §4.4's two-slice rule exists to keep from
// being reported as a win.
//
// §4.6's required question: did this invalidate anything? No. If anything it
// reinforces §2.2's original premise — the raw gap is already most of the
// earnings-surprise signal, since restricting to gaps provably tied to a
// real SEC filing barely moves the per-trade excess. The bottleneck named in
// the original result (sample size, not signal quality) stands unchanged;
// this update is one more data point for it, not a revision of it.
func CheckPostGapDriftExact(candles []data.Candle, p ScreenParams) bool {
	n := len(candles)
	if n < 60 {
		return false
	}
	closes := data.Closes(candles)
	volumes := data.Volumes(candles)
	bar := candles[n-1]

	// 1. The event: an opening gap up of at least GapUpPct.
	prevClose := closes[n-2]
	if prevClose <= 0 || (bar.Open-prevClose)/prevClose*100.0 < p.GapUpPct {
		return false
	}

	// 2. Volume confirms news, not a thin print on a quiet tape.
	volRatio := VolumeRatio(volumes, 20)
	if volRatio == 0 || volRatio < p.GapVolMult {
		return false
	}

	// 3. Held the gap into the close. No guard for High == Low is needed: a
	// bar that never moved (a TW limit-up locked at the open, the strongest
	// hold there is) has Close == mid, which passes.
	if bar.Close < (bar.High+bar.Low)/2 {
		return false
	}

	// 4. Repricing, not a bounce: the gap has to make a new high.
	if !IsNewHigh(closes, p.NewHighLookback) {
		return false
	}

	// 5. Liquidity: same gate, same window as the other four screens.
	window5v := volumes[n-6 : n-1]
	var sumV int64
	for _, v := range window5v {
		sumV += v
	}
	if float64(sumV)/5.0 < p.MinAvgVolume5d {
		return false
	}

	return true
}

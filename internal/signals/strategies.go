package signals

import (
	"math"
	"time"

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

// TypeBoxBottom is 網 2's Signal.Type. Exported for the same reason as
// TypeTrendBreakout — see CheckBoxBottomReboundExact's doc comment for the
// §4.4 downgrade this backs.
const TypeBoxBottom = "strategy_box_bottom"

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

	// 內部人群聚買（Phase 25 §8.2，US only — TW 無 Form 4 對應揭露規範）
	InsiderMinFilers   int     // 過去 InsiderLookbackDays 天內，不同董監自掏腰包（P 碼）買進的人數下限，2
	InsiderMinNotional float64 // 同期 P 碼合計金額（股數 x 價格）下限，美元，250_000
}

// insiderLookbackDays is the trailing window Phase 25 §8.2.3's cluster-buy
// conditions count within — CALENDAR days, not trading days like
// boxWindowDays: SEC filings land on whatever day the filer submits, not on
// a trading calendar.
const insiderLookbackDays = 90

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
		InsiderMinFilers: 2, InsiderMinNotional: 250_000,
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

// CheckSqueezeBreakoutExact evaluates candles' last bar (網 1【擠壓突破】).
//
// # Status: not yet formally evaluated against §4.4's bar — no dedicated
// pre-registered run exists for THIS screen specifically. PLAN.md's
// 2026-08-30 "網 1／網 4 的專屬降級評估" item covers that; per the rule that
// decision established for 網 2 (a downgrade needs a run pre-registered FOR
// that screen, not an incidental number from a study registered for
// another), do not add a downgrade annotation here on the strength of the
// data point below alone.
//
// One data point toward that pending evaluation — S&P 600 small-cap,
// 2026-09-01 (Phase 25 §8's universe-expansion pass, same run as 網 2/網 6's
// updates above/below):
//
//	sample            n     excess     SE   sigma
//	SP600 in-samp    207     +4.31%   2.63    1.6
//	SP600 holdout    249     -0.99%   1.27    0.8  (negative)
//
// In-sample clears 1 SE; holdout doesn't and flips negative — one split
// short, the same failure shape 網 2's SP500 leg hit before its downgrade.
// Not a decision on its own (see above): the dedicated run still needs a
// plain-US split and a TW split before §4.4 applies.
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
		Type:    TypeBoxBottom,
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
//
// # Update — S&P 600 small-cap re-run (2026-09-01), the prescribed fix for the sample-size bottleneck
//
// PEAD literature says the effect is strongest in small-caps (thinner
// analyst coverage, slower information diffusion), and S&P 600 (~600 more US
// small-caps, added for this) gives ~50% more names than SP400 to test that
// against. Same split/bootstrap, same random-entry control:
//
//	sample                    n     excess     SE   sigma
//	SP600 in-samp     [T0]   235     +0.97%   2.01    0.5
//	SP600 holdout     [T0]   342     -0.91%   1.10    0.8  (negative)
//	SP600 in-samp     [T1]   235     +2.03%   2.52    0.8
//	SP600 holdout     [T1]   342     -0.92%   1.12    0.8  (negative)
//
// Still doesn't clear 1 SE in either slice or variant — expected, given the
// SP400/SP500 pattern above. What IS new: the holdout excess is negative,
// for both T0 and T1. That breaks the "positive-signed in all four samples"
// pattern the original result flagged as suggestive (§ above) — it's now
// positive in in-sample slices only, which is the same "flips sign between
// universes" shape every OTHER screen here already had. Small-caps being the
// literature's strongest case for PEAD and still landing negative on holdout
// is evidence against "just needs more names," not for it — sample size
// alone doesn't explain a sign flip, only a wide SE. Read together with the
// unchanged SP400/SP500 numbers, this closes the "underpowered, not
// disproven" framing rather than resolving it in the screen's favor; nothing
// here argues for revisiting the "NOT wired" status above.
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

// InsiderClusterBuy evaluates candles/txs for Insider Cluster Buy (Phase 25
// §8.2) triggers within the last strategyLookbackDays. txs is passed
// unsliced at every offset — unlike TrustFollow's trustNet, insider filings
// are sparse events, not one row per trading day, so
// CheckInsiderClusterBuyExact does its own date-window filtering off
// candles' last bar rather than needing a pre-aligned same-length slice.
func InsiderClusterBuy(candles []data.Candle, txs []data.InsiderTransaction, p ScreenParams) *StrategyHit {
	n := len(candles)
	for offset := 0; offset < strategyLookbackDays; offset++ {
		evalIdx := n - 1 - offset
		if evalIdx+1 < 60+p.MA60SlopeLookback {
			break
		}
		if CheckInsiderClusterBuyExact(candles[:evalIdx+1], txs, p) {
			return &StrategyHit{Name: "insider_cluster_buy", DaysAgo: offset}
		}
	}
	return nil
}

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

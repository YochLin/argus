package signals

import (
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

// TypeSqueezeBreakout is 網 1's Signal.Type, and TypeTrendPullback is 網 4's.
// Exported for the same reason as TypeTrendBreakout/TypeBoxBottom — see
// CheckSqueezeBreakoutExact's/CheckTrendPullbackExact's doc comments for the
// §4.4 downgrade this backs (2026-09-01 dedicated run, PLAN.md's 網1／網4
// 專屬降級評估 item).
const (
	TypeSqueezeBreakout = "strategy_squeeze_breakout"
	TypeTrendPullback   = "strategy_trend_pullback"
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

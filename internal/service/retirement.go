package service

import (
	"math"

	"argus/internal/assets"
)

// Retirement projection (Phase 9 波次3 PR7, docs/phase-9-asset-platform.md
// §8.7/§10.2②) — a pure function, no DB/network, same "health metrics never
// go through the LLM" rule as the rest of this file's wealth math.
//
// Returns are quoted in real (inflation-adjusted) terms, so spending is
// expressed in today's dollars and there's no separate inflation parameter to
// misread as nominal. Both returns are blended from the retirement pool's
// own mix (RetirementMix); 3.0%/1.0% are the fallbacks when there's nothing
// to blend.
//
// These are the plain-constant defaults, not env-tunable, even though
// §10.2②'s doc text suggests "情境參數走 env 可調": every other wealth
// threshold actually shipped in this codebase (GoalStatus's 5pp band,
// /w/alloc's drift bands) is a plain constant too — this stays consistent
// with what was actually built rather than the doc's unimplemented
// aspiration. /w/retire's settings drawer does let a user override the
// return/withdrawal/horizon figures per RetirementInputs below, but that's
// a page-local, unpersisted "what-if" query param (internal/web/wealth_retire.go),
// not an env var — these constants stay the resolved default whenever no
// override is given.
const (
	RetirementPreReturnReal          = 0.03 // default real return, still contributing
	RetirementPostReturnReal         = 0.01 // default real return, drawing down
	RetirementWithdrawalMultiple     = 25   // 4% rule: need = annual spend * 25
	RetirementWithdrawalRateReal     = 0.04 // same 4% rule as a rate rather than a multiple — RetirementInputs.WithdrawalRate takes this form
	RetirementScenarioCrashPct       = 0.30 // one-off shock, sequence-of-returns scenario
	RetirementScenarioCrashLeadYears = 5    // years before retirement the shock lands
	RetirementScenarioLowReturnDelta = 0.015
)

// RetirementReturnClass is one asset class of the retirement pool's return
// table (/w/retire's 報酬率假設). Movable classes can be rebalanced at will
// and so take part in the post-retirement mix and the page's trial mix;
// policies, pension, property and crypto stay as they are.
type RetirementReturnClass struct {
	Class   string
	Return  float64 // assumed long-run real return, a fraction
	Movable bool
}

// RetirementReturnClasses is the design's table, in display order. The
// returns are conservative long-run real assumptions, not forecasts —
// overstating them understates the gap. Checked 2026-11 against: 1-year TWD
// deposits ~1.7-1.9% vs CPI 1.66% (2025) to 2.18% (2024); TWD participating
// policies' declared rates ~2.75-2.9% (declared, not IRR); 勞退新制's 5-year
// average ~4.8%, a bull-market window.
var RetirementReturnClasses = []RetirementReturnClass{
	{"etf", 0.045, true},
	{"bond", 0.015, true},
	{"gold", 0.005, true},
	{"cash", -0.003, true},
	{"policy", 0.003, false},
	{"pension", 0.015, false},
	{"estate", 0.01, false},
	{"crypto", 0, false},
}

// RetirementPostMix is the movable classes' shares after retiring ("偏保守",
// the page's default post-retirement mix). The page's other presets live in
// the frontend, which sends the resulting return back as an override.
var RetirementPostMix = map[string]float64{"etf": 0.30, "bond": 0.50, "gold": 0.05, "cash": 0.15}

// RetirementReturnClassOf maps an assets.type onto RetirementReturnClasses.
// ETFs are recorded as "fund" in the wealth account; "equity" only comes from
// the trading account, which can't be earmarked, but maps the same way.
// Anything unrecognised ("other") is treated as cash, the class that assumes
// the least.
func RetirementReturnClassOf(assetType string) string {
	switch assetType {
	case "fund", "equity":
		return "etf"
	case "insurance":
		return "policy"
	case "bond", "gold", "pension", "estate", "crypto":
		return assetType
	default:
		return "cash"
	}
}

// RetirementReturnMix is one class of the retirement pool: its TWD amount,
// the return assumed for it, and whether it can be rebalanced.
type RetirementReturnMix struct {
	Class     string  `json:"class"`
	Amount    float64 `json:"amount"`
	ReturnPct float64 `json:"returnPct"`
	Movable   bool    `json:"movable"`
}

// RetirementMix sums each earmarked item's TWD value into
// RetirementReturnClasses and derives the two blended returns: pre, the pool
// as it is today; post, its movable part re-spread over RetirementPostMix
// (the fixed classes unchanged; with nothing movable, post equals pre). ok
// is false when nothing has a positive value — the caller then keeps its flat
// defaults. Every class is listed, amount 0 when absent, so the page can
// price a trial mix into a class the pool doesn't hold yet. Returns are
// rounded to 0.1pt, the slider's step, so the figure shown is the one
// projected with.
func RetirementMix(items []GoalAssetItem, typeOf map[int64]string) (mix []RetirementReturnMix, pre, post float64, ok bool) {
	amt := map[string]float64{}
	var total, movable float64
	for _, it := range items {
		if it.Value == nil || *it.Value <= 0 {
			continue
		}
		amt[RetirementReturnClassOf(typeOf[it.AssetID])] += *it.Value
		total += *it.Value
	}
	if total <= 0 {
		return nil, 0, 0, false
	}
	for _, c := range RetirementReturnClasses {
		a := amt[c.Class]
		mix = append(mix, RetirementReturnMix{Class: c.Class, Amount: a, ReturnPct: math.Round(c.Return*1000) / 10, Movable: c.Movable})
		pre += c.Return * a / total
		if c.Movable {
			movable += a
		}
	}
	if movable == 0 {
		post = pre
	} else {
		for _, c := range RetirementReturnClasses {
			a := amt[c.Class]
			if c.Movable {
				a = movable * RetirementPostMix[c.Class]
			}
			post += c.Return * a / total
		}
	}
	return mix, math.Round(pre*1000) / 1000, math.Round(post*1000) / 1000, true
}

// RetirementHorizonAge is the default simulate-depletion-out-to age, and so
// the latest retirement age that can be saved (assets.ValidateRetirement).
const RetirementHorizonAge = assets.RetirementHorizonAge

// RetirementInputs is the caller-resolved state. YearsToRetirement and
// YearsPostRetirement are already clamped to >= 0 by the caller (derived
// from the user's birth year + chosen retirement age, and from
// RetirementHorizonAge - retirement age respectively) — this package knows
// nothing about calendars or birth years.
type RetirementInputs struct {
	YearsToRetirement   int
	YearsPostRetirement int
	Pool                float64 // today's TWD value of retirement-earmarked assets
	MonthlyContribution float64
	MonthlySpend        float64 // today's dollars
	// PreReturn/PostReturn/WithdrawalRate are caller-resolved fractions
	// (e.g. 0.04 for 4%) — this package no longer defaults them internally,
	// same "caller resolves, package stays a pure function" rule as
	// YearsToRetirement/YearsPostRetirement above. A caller with no custom
	// value passes RetirementPreReturnReal/RetirementPostReturnReal/
	// RetirementWithdrawalRateReal.
	PreReturn      float64
	PostReturn     float64
	WithdrawalRate float64
}

// RetirementPathPoint is one yearly balance sample for the projection chart.
// YearsFromNow 0 is today; YearsFromNow == the input's YearsToRetirement is
// the retirement date itself.
type RetirementPathPoint struct {
	YearsFromNow int
	Balance      float64
}

// RetirementProjection is one scenario's output.
type RetirementProjection struct {
	ProjectedAtRetirement float64
	Need                  float64
	AchievementPct        float64 // atRetirement/need*100, capped at 999 (matches the template)
	GapAmount             float64 // Need - ProjectedAtRetirement; <= 0 means funded
	Funded                bool
	// DepletionYearsAfterRetirement is nil when the balance never reaches
	// zero within YearsPostRetirement — the template's "92 歲後仍有結餘".
	DepletionYearsAfterRetirement *int
	Path                          []RetirementPathPoint
}

// simulate runs one deterministic path: annual compounding pre-retirement
// with a fixed annual contribution, then annual drawdown post-retirement —
// mirrors retireModel()'s `bal = bal*g + contrib` / `bal = bal*gr - spend*12`
// loops exactly, just year-indexed rather than by literal age.
// crashAtYear >= 0 applies a one-off multiplicative shock that many years
// from now, before that year's growth (the sequence-of-returns scenario);
// a negative value disables it.
func simulate(in RetirementInputs, preReturn, postReturn float64, crashAtYear int) (path []RetirementPathPoint, atRetirement float64, depletionYears *int) {
	bal := in.Pool
	annualContribution := in.MonthlyContribution * 12
	annualSpend := in.MonthlySpend * 12
	path = append(path, RetirementPathPoint{YearsFromNow: 0, Balance: bal})

	for y := 1; y <= in.YearsToRetirement; y++ {
		if crashAtYear >= 0 && y-1 == crashAtYear {
			bal *= 1 - RetirementScenarioCrashPct
		}
		bal = bal*(1+preReturn) + annualContribution
		path = append(path, RetirementPathPoint{YearsFromNow: y, Balance: bal})
	}
	atRetirement = bal

	for y := 1; y <= in.YearsPostRetirement; y++ {
		bal = bal*(1+postReturn) - annualSpend
		if bal < 0 {
			bal = 0
		}
		path = append(path, RetirementPathPoint{YearsFromNow: in.YearsToRetirement + y, Balance: bal})
		if bal <= 0 && depletionYears == nil {
			yy := y
			depletionYears = &yy
		}
	}
	return path, atRetirement, depletionYears
}

func project(in RetirementInputs, preReturn, postReturn float64, crashAtYear int) RetirementProjection {
	path, atRet, depletion := simulate(in, preReturn, postReturn, crashAtYear)
	need := in.MonthlySpend * 12 / in.WithdrawalRate
	achievement := 999.0
	if need > 0 {
		achievement = atRet / need * 100
		if achievement > 999 {
			achievement = 999
		}
	}
	return RetirementProjection{
		ProjectedAtRetirement:         atRet,
		Need:                          need,
		AchievementPct:                achievement,
		GapAmount:                     need - atRet,
		Funded:                        atRet >= need,
		DepletionYearsAfterRetirement: depletion,
		Path:                          path,
	}
}

// ComputeRetirementProjection is the baseline scenario.
func ComputeRetirementProjection(in RetirementInputs) RetirementProjection {
	return project(in, in.PreReturn, in.PostReturn, -1)
}

// ComputeRetirementScenarios runs the three §10.2② scenarios off the same
// inputs: baseline, a market crash RetirementScenarioCrashLeadYears years
// before retirement (clamped to "now" if retirement is already closer than
// that), and a long-term real return permanently RetirementScenarioLowReturnDelta
// lower on both legs. Deliberately not Monte Carlo — the doc's own reasoning
// is that a single-user tool gets more decision value from three named paths
// than from a simulated "78% success" figure that invites false precision.
func ComputeRetirementScenarios(in RetirementInputs) (baseline, crash, lowReturn RetirementProjection) {
	baseline = ComputeRetirementProjection(in)

	crashAtYear := in.YearsToRetirement - RetirementScenarioCrashLeadYears
	if crashAtYear < 0 {
		crashAtYear = 0
	}
	crash = project(in, in.PreReturn, in.PostReturn, crashAtYear)

	lowReturn = project(in, in.PreReturn-RetirementScenarioLowReturnDelta, in.PostReturn-RetirementScenarioLowReturnDelta, -1)
	return baseline, crash, lowReturn
}

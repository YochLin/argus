package service

// Retirement projection (Phase 9 波次3 PR7, docs/phase-9-asset-platform.md
// §8.7/§10.2②) — a pure function, no DB/network, same "health metrics never
// go through the LLM" rule as the rest of this file's wealth math.
//
// The model matches the design template's retireModel() exactly: returns are
// quoted in real (inflation-adjusted) terms — 3.0% while still contributing,
// 1.0% after — so spending is expressed in today's dollars and there's no
// separate inflation parameter to misread as nominal.
//
// These are the plain-constant defaults, not env-tunable, even though
// §10.2②'s doc text suggests "情境參數走 env 可調": every other wealth
// threshold actually shipped in this codebase (goalStatus's 5pp band,
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
	RetirementHorizonAge             = 92   // default simulate-depletion-out-to age
	RetirementScenarioCrashPct       = 0.30 // one-off shock, sequence-of-returns scenario
	RetirementScenarioCrashLeadYears = 5    // years before retirement the shock lands
	RetirementScenarioLowReturnDelta = 0.015
)

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

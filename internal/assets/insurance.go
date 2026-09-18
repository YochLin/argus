package assets

// Insurance need-side formula (Phase 9 波次3 PR8, docs/phase-9-asset-platform.md
// §8.6/§8.16.1) — a simplified survivor-needs-method (遺族需要法), picked over
// the "double-ten rule" (need = 10x annual income) because it actually uses
// the household facts this platform has (debt, expenses, liquid assets,
// dependents), not just salary. §8.16.1 sorts the six coverage kinds' need
// into three input categories: platform-computed (TotalDebt/AnnualExpense/
// LiquidAssets), asked-once (Dependents/YoungestChildAge/SpouseHasIncome —
// the §8.16.2 "個人參數"), and constants (below). Like every other health
// metric in this package, `need` is a pure function — never routed through
// the LLM (§2 point 6).
//
// ⚠️ This is a household-planning estimate, not licensed financial/insurance
// advice — the coefficients below are modeling choices, not a regulatory
// formula. Documented so a future reviewer can second-guess them, not
// because they're authoritative.

// ChildIndependenceAge/SpouseIncomeOffset are the two modeling constants the
// formula needs: dependencyYears = max(0, ChildIndependenceAge -
// youngestChildAge) is how many more years of income replacement the
// household needs; SpouseIncomeOffset halves that (and the disability leg)
// when the spouse has independent income, since the household isn't solely
// dependent on the insured's income in that case.
const (
	ChildIndependenceAge = 22
	SpouseIncomeOffset   = 0.5
)

// Constants for the two coverage kinds §8.16.1 could find no per-user
// formula for (critical illness/cancer lump sums, hospital daily rate) —
// round estimates grounded in public consumer-insurance guidance, not an
// official government statistics API (§8.16.1 explicitly notes these are
// "人讀的報告不是 API"). Re-verify periodically; not derived from any live
// data source in this codebase.
//
// HospitalDailyBaselineTWD: a private/single-room upgrade's out-of-pocket
// daily rate difference typically runs NT$1,500–5,000/day depending on room
// class (Roo.Cash's 2026 hospital ward-fee roundup,
// https://roo.cash/blog/expense-of-hospital-ward/); NT$3,000/day is the
// lower end of a private-room upgrade, used as a conservative baseline.
// Retrieved 2026-09-18.
const HospitalDailyBaselineTWD = 3000

// CriticalIllnessLumpSumTWD/CancerLumpSumTWD: consumer guidance cites a
// floor of "at least NT$1,000,000" for 重大傷病險 coverage (SmartBeb's 2026
// guide, https://www.smartbeb.com.tw/article/Catastrophic_illness/id/1053)
// against high-cost targeted cancer therapy running roughly NT$200,000/month
// (My83's 2026 cancer-insurance guide, https://my83.com.tw/blogs?p=1358).
// NT$2,000,000 approximates that floor plus roughly a year of such
// treatment; cancer's own lump sum is set lower since it's a narrower
// diagnosis category than the broader 重大疾病/傷病 bucket. Retrieved
// 2026-09-18.
const (
	CriticalIllnessLumpSumTWD = 2000000
	CancerLumpSumTWD          = 1500000
)

// InsuranceNeedInputs bundles the three input categories. HasYoungestChildAge
// distinguishes "no dependents" from "dependents but age not provided yet" —
// both degrade dependencyYears to 0, but only the latter is a data gap
// worth surfacing to the caller if it ever needs to (this package doesn't
// track that distinction further; the web handler does, via its own
// "hasProfile" flag).
type InsuranceNeedInputs struct {
	TotalDebt           float64
	AnnualExpense       float64
	LiquidAssets        float64
	Dependents          int
	YoungestChildAge    int
	HasYoungestChildAge bool
	SpouseHasIncome     bool
}

func spouseFactor(spouseHasIncome bool) float64 {
	if spouseHasIncome {
		return SpouseIncomeOffset
	}
	return 1
}

// dependencyYears is how many more years of income replacement the
// household needs — 0 when there are no dependents, or a dependent count is
// set but the youngest child's age wasn't (never guess an age).
func dependencyYears(in InsuranceNeedInputs) float64 {
	if in.Dependents <= 0 || !in.HasYoungestChildAge {
		return 0
	}
	y := float64(ChildIndependenceAge - in.YoungestChildAge)
	if y < 0 {
		return 0
	}
	return y
}

// LifeInsuranceNeed: clear existing debt, replace living expenses for the
// years until the youngest dependent is self-sufficient (halved when the
// spouse has independent income), minus the liquid-asset buffer already on
// hand. Floored at 0 — a household with no debt and ample liquid assets has
// no *additional* need, not a negative one.
func LifeInsuranceNeed(in InsuranceNeedInputs) float64 {
	need := in.TotalDebt + dependencyYears(in)*in.AnnualExpense*spouseFactor(in.SpouseHasIncome) - in.LiquidAssets
	if need < 0 {
		return 0
	}
	return need
}

// AccidentInsuranceNeed mirrors life insurance's need — Taiwan LNA practice
// conventionally covers accidental death at the same level as natural-cause
// life insurance.
func AccidentInsuranceNeed(in InsuranceNeedInputs) float64 {
	return LifeInsuranceNeed(in)
}

// DisabilityInsuranceNeed is a *monthly* figure (disability benefits are
// per-period, not lump sum): the household's monthly expense, halved when
// the spouse has independent income — same spouse offset as the life leg,
// since a disabled earner's household still has monthly living costs to
// cover, not a one-off debt/replacement need.
func DisabilityInsuranceNeed(in InsuranceNeedInputs) float64 {
	return in.AnnualExpense / 12 * spouseFactor(in.SpouseHasIncome)
}

package service

import "testing"

// TestComputeRetirementProjectionFundedVsGap pins the baseline arithmetic
// against a hand-computed two-year example: pool 1,000,000, no contribution,
// 3%/1% real returns, spend low enough to be funded on one input and a spend
// high enough to leave a gap on another.
func TestComputeRetirementProjectionFundedVsGap(t *testing.T) {
	in := RetirementInputs{YearsToRetirement: 2, YearsPostRetirement: 3, Pool: 1000000, MonthlySpend: 1000}
	got := ComputeRetirementProjection(in)
	wantAtRet := 1000000 * 1.03 * 1.03
	if diff := got.ProjectedAtRetirement - wantAtRet; diff > 1 || diff < -1 {
		t.Errorf("ProjectedAtRetirement = %v, want ~%v", got.ProjectedAtRetirement, wantAtRet)
	}
	wantNeed := 1000.0 * 12 * RetirementWithdrawalMultiple
	if got.Need != wantNeed {
		t.Errorf("Need = %v, want %v", got.Need, wantNeed)
	}
	if !got.Funded || got.GapAmount > 0 {
		t.Errorf("expected funded with pool far exceeding a 300k need, got Funded=%v GapAmount=%v", got.Funded, got.GapAmount)
	}

	underfunded := ComputeRetirementProjection(RetirementInputs{YearsToRetirement: 0, YearsPostRetirement: 30, Pool: 100000, MonthlySpend: 90000})
	if underfunded.Funded {
		t.Errorf("expected an underfunded plan, got Funded=true")
	}
	if underfunded.DepletionYearsAfterRetirement == nil {
		t.Errorf("expected the pool to deplete within 30 years, got nil (never depletes)")
	}
}

// TestComputeRetirementScenariosCrashHurtsBaseline pins that the
// sequence-of-returns and low-return scenarios never do better than
// baseline, and that a plan with money left over at the horizon reports
// DepletionYearsAfterRetirement == nil in every scenario.
func TestComputeRetirementScenariosCrashHurtsBaseline(t *testing.T) {
	in := RetirementInputs{YearsToRetirement: 10, YearsPostRetirement: 30, Pool: 20000000, MonthlyContribution: 30000, MonthlySpend: 50000}
	baseline, crash, lowReturn := ComputeRetirementScenarios(in)

	if crash.ProjectedAtRetirement >= baseline.ProjectedAtRetirement {
		t.Errorf("crash scenario projected %v, want less than baseline %v", crash.ProjectedAtRetirement, baseline.ProjectedAtRetirement)
	}
	if lowReturn.ProjectedAtRetirement >= baseline.ProjectedAtRetirement {
		t.Errorf("low-return scenario projected %v, want less than baseline %v", lowReturn.ProjectedAtRetirement, baseline.ProjectedAtRetirement)
	}
	for name, p := range map[string]RetirementProjection{"baseline": baseline, "crash": crash, "lowReturn": lowReturn} {
		if p.DepletionYearsAfterRetirement != nil {
			t.Errorf("%s: expected no depletion with a well-funded pool, got depletion at year %d", name, *p.DepletionYearsAfterRetirement)
		}
	}
}

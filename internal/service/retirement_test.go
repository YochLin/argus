package service

import (
	"slices"
	"testing"

	"argus/internal/assets"
)

// TestComputeRetirementProjectionFundedVsGap pins the baseline arithmetic
// against a hand-computed two-year example: pool 1,000,000, no contribution,
// 3%/1% real returns, spend low enough to be funded on one input and a spend
// high enough to leave a gap on another.
func TestComputeRetirementProjectionFundedVsGap(t *testing.T) {
	in := RetirementInputs{
		YearsToRetirement: 2, YearsPostRetirement: 3, Pool: 1000000, MonthlySpend: 1000,
		PreReturn: RetirementPreReturnReal, PostReturn: RetirementPostReturnReal, WithdrawalRate: RetirementWithdrawalRateReal,
	}
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

	underfunded := ComputeRetirementProjection(RetirementInputs{
		YearsToRetirement: 0, YearsPostRetirement: 30, Pool: 100000, MonthlySpend: 90000,
		PreReturn: RetirementPreReturnReal, PostReturn: RetirementPostReturnReal, WithdrawalRate: RetirementWithdrawalRateReal,
	})
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
	in := RetirementInputs{
		YearsToRetirement: 10, YearsPostRetirement: 30, Pool: 20000000, MonthlyContribution: 30000, MonthlySpend: 50000,
		PreReturn: RetirementPreReturnReal, PostReturn: RetirementPostReturnReal, WithdrawalRate: RetirementWithdrawalRateReal,
	}
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

func TestBlendedPreReturn(t *testing.T) {
	v := func(f float64) *float64 { return &f }
	typeOf := map[int64]string{1: "deposit", 2: "fund", 3: "pension", 4: "other"}
	// 200 deposit (0%) + 600 fund (4%) + 200 pension (2%) = 2.8%; a valueless row is skipped.
	ret, mix, ok := BlendedPreReturn([]GoalAssetItem{
		{AssetID: 1, Value: v(200)}, {AssetID: 2, Value: v(600)}, {AssetID: 3, Value: v(200)}, {AssetID: 4},
	}, typeOf)
	if !ok || ret != 0.028 {
		t.Fatalf("ret = %v ok = %v, want 0.028 true", ret, ok)
	}
	want := []RetirementReturnMix{{"cash", 20, 0}, {"fund", 60, 4}, {"pension", 20, 2}}
	if len(mix) != len(want) {
		t.Fatalf("mix = %+v, want %+v", mix, want)
	}
	for i := range want {
		if mix[i] != want[i] {
			t.Errorf("mix[%d] = %+v, want %+v", i, mix[i], want[i])
		}
	}
	if _, _, ok := BlendedPreReturn([]GoalAssetItem{{AssetID: 4}}, typeOf); ok {
		t.Error("ok = true with nothing valued, want the caller's fallback")
	}
	for c := range RetirementCategoryReturnReal {
		if !slices.Contains(assets.AllocCategories, c) {
			t.Errorf("return table has unknown category %q", c)
		}
	}
	for _, c := range assets.AllocCategories {
		if _, has := RetirementCategoryReturnReal[c]; !has {
			t.Errorf("category %q has no assumed return", c)
		}
	}
}

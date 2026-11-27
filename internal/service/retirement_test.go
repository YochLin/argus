package service

import (
	"math"
	"testing"

	"argus/internal/db"
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

func TestRetirementMix(t *testing.T) {
	v := func(f float64) *float64 { return &f }
	typeOf := map[int64]string{1: "deposit", 2: "fund", 3: "pension", 4: "other", 5: "insurance"}
	// 200 cash (−0.3%) + 600 etf (4.5%) + 200 pension (1.5%); a valueless row is skipped.
	mix, pre, post, ok := RetirementMix([]GoalAssetItem{
		{AssetID: 1, Value: v(200)}, {AssetID: 2, Value: v(600)}, {AssetID: 3, Value: v(200)}, {AssetID: 4},
	}, typeOf)
	if !ok {
		t.Fatal("ok = false")
	}
	// pre = 0.2×−0.3 + 0.6×4.5 + 0.2×1.5 = 2.94% → 2.9%.
	if pre != 0.029 {
		t.Errorf("pre = %v, want 0.029", pre)
	}
	// post: the 800 movable re-spread 30/50/5/15 over etf/bond/gold/cash, pension kept:
	// (240×4.5 + 400×1.5 + 40×0.5 + 120×−0.3 + 200×1.5) / 1000 = 1.964% → 2.0%.
	if post != 0.02 {
		t.Errorf("post = %v, want 0.02", post)
	}
	if len(mix) != len(RetirementReturnClasses) || mix[0] != (RetirementReturnMix{"etf", 600, 4.5, true}) || mix[5] != (RetirementReturnMix{"pension", 200, 1.5, false}) {
		t.Errorf("mix = %+v", mix)
	}

	// Nothing movable: post stays at pre.
	_, pre, post, _ = RetirementMix([]GoalAssetItem{{AssetID: 5, Value: v(100)}}, typeOf)
	if pre != 0.003 || post != pre {
		t.Errorf("policy only: pre = %v post = %v, want 0.003 both", pre, post)
	}
	if _, _, _, ok := RetirementMix([]GoalAssetItem{{AssetID: 4}}, typeOf); ok {
		t.Error("ok = true with nothing valued, want the caller's fallback")
	}
	var sum float64
	for _, w := range RetirementPostMix {
		sum += w
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Errorf("RetirementPostMix sums to %v, want 1", sum)
	}
}

func TestRetirementContributionTWD(t *testing.T) {
	fund, dep, other := int64(1), int64(2), int64(3)
	flows := []db.RecurringCashflow{
		{Direction: "out", Category: "sip", Amount: 10000, Currency: "TWD", AssetID: &fund, Active: true},
		{Direction: "out", Category: "sip", Amount: 4000, Currency: "TWD", AssetID: &dep, Active: true},   // half earmarked
		{Direction: "out", Category: "sip", Amount: 9000, Currency: "TWD", AssetID: &other, Active: true}, // not earmarked
		{Direction: "out", Category: "sip", Amount: 9000, Currency: "TWD", AssetID: &fund, Active: false}, // paused
		{Direction: "out", Category: "rent", Amount: 20000, Currency: "TWD", AssetID: &fund, Active: true},
		{Direction: "in", Category: "sip", Amount: 9000, Currency: "TWD", AssetID: &fund, Active: true},
	}
	earmarks := []db.GoalAsset{{GoalID: 1, AssetID: fund, Ratio: 1}, {GoalID: 1, AssetID: dep, Ratio: 0.5}}
	if got := RetirementContributionTWD(flows, earmarks, "2026-11-27", &fakeWealthQuotes{}, &fakeWealthFX{}); got != 12000 {
		t.Errorf("contribution = %v, want 12000", got)
	}
	if got := livingSpendTWD(flows, "2026-11-27", &fakeWealthQuotes{}, &fakeWealthFX{}); got == nil || *got != 20000 {
		t.Errorf("living spend = %v, want 20000 (定期定額 left out)", got)
	}
}

// TestRequiredMonthlyContribution feeds the back-solved amount into the
// projection and expects it to land on 100% funded.
func TestRequiredMonthlyContribution(t *testing.T) {
	for _, r := range []float64{0.03, 0} {
		in := RetirementInputs{YearsToRetirement: 20, YearsPostRetirement: 30, Pool: 1e6, MonthlySpend: 50000, PreReturn: r, PostReturn: 0.01, WithdrawalRate: 0.04}
		need := ComputeRetirementProjection(in).Need
		c := RequiredMonthlyContribution(in, need)
		if c == nil {
			t.Fatalf("r=%v: nil", r)
		}
		in.MonthlyContribution = *c
		if got := ComputeRetirementProjection(in).AchievementPct; math.Abs(got-100) > 1e-6 {
			t.Errorf("r=%v: funded %v%% with %v/mo, want 100", r, got, *c)
		}
	}
	rich := RetirementInputs{YearsToRetirement: 5, Pool: 1e9, MonthlySpend: 1000, PreReturn: 0.03, WithdrawalRate: 0.04}
	if c := RequiredMonthlyContribution(rich, 300000); c == nil || *c != 0 {
		t.Errorf("already funded = %v, want 0", c)
	}
	if c := RequiredMonthlyContribution(RetirementInputs{}, 1); c != nil {
		t.Errorf("no years left = %v, want nil", *c)
	}
}

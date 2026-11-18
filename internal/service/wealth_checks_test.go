package service

import (
	"reflect"
	"testing"
)

func codes(items []CheckItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Code)
	}
	return out
}

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

// healthy passes every rule and asks for nothing.
func healthy() CheckInputs {
	return CheckInputs{
		AssetCount: 5, Dependents: ip(0), HasSalary: true, HasAge: true,
		LiquidityMonths: fp(8), MonthlyExpense: fp(50000), DebtRatioPct: fp(20),
		CoverageKind: "life", CoveragePct: 95,
		RetirementPct: fp(110),
		ModelPicked:   true, Rebalance: fp(0),
		SavingsRatePct: fp(30),
	}
}

func TestEvaluateWealthChecksAllClear(t *testing.T) {
	if got := EvaluateWealthChecks(healthy()); len(got) != 0 {
		t.Errorf("healthy inputs: got %v, want no items", codes(got))
	}
}

func TestEvaluateWealthChecksOrder(t *testing.T) {
	in := healthy()
	in.SavingsRatePct = fp(10)                     // tip
	in.Stale = []string{"a", "b", "c", "d"}        // data
	in.CardBalance, in.CardName = fp(12000), "玉山卡" // do
	in.GoalsBehind = 2                             // tip
	in.LiquidityMonths = fp(2)                     // do, ahead of the card in rule order
	got := EvaluateWealthChecks(in)
	want := []string{"emergency", "card", "goals", "savings", "stale"}
	if !reflect.DeepEqual(codes(got), want) {
		t.Fatalf("order = %v, want %v (severity first, rule order within)", codes(got), want)
	}
	if g := got[0].Gap; g == nil || *g != 200000 {
		t.Errorf("emergency gap = %v, want (6-2)*50000", g)
	}
	if n := got[4].Names; len(n) != 3 || got[4].Count != 4 {
		t.Errorf("stale = %v/%d, want three names and a count of 4", n, got[4].Count)
	}
}

func TestEvaluateWealthChecksThresholds(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*CheckInputs)
		want []string
	}{
		{"dependents raise the emergency fund to 9 months", func(in *CheckInputs) { in.Dependents = ip(2) }, []string{"emergency"}},
		{"debt ratio at 40% is too high", func(in *CheckInputs) { in.DebtRatioPct = fp(40) }, []string{"debtRatio"}},
		{"coverage under 80%", func(in *CheckInputs) { in.CoveragePct = 79.9 }, []string{"coverage"}},
		{"no measurable coverage is no item", func(in *CheckInputs) { in.CoverageKind, in.CoveragePct = "", 0 }, []string{}},
		{"retirement under 80%", func(in *CheckInputs) { in.RetirementPct = fp(60) }, []string{"retirement"}},
		{"savings rate at 20% passes", func(in *CheckInputs) { in.SavingsRatePct = fp(20) }, []string{}},
		{"drift is ignored with no model picked", func(in *CheckInputs) { in.ModelPicked, in.OffTarget, in.Rebalance = false, 2, fp(9000) }, []string{}},
		{"drift is ignored under 3 assets", func(in *CheckInputs) { in.AssetCount, in.OffTarget, in.Rebalance = 2, 2, fp(9000) }, []string{}},
		{"drift", func(in *CheckInputs) { in.OffTarget, in.Rebalance = 2, fp(9000) }, []string{"alloc"}},
		{"missing profile facts are asked for", func(in *CheckInputs) { in.Dependents, in.HasAge, in.HasSalary = nil, false, false }, []string{"dependents", "age", "salary"}},
	}
	for _, c := range cases {
		in := healthy()
		c.mod(&in)
		if got := codes(EvaluateWealthChecks(in)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

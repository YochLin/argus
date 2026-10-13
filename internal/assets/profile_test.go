package assets

import (
	"slices"
	"testing"
)

func TestProfileDraftProblems(t *testing.T) {
	i := func(v int) *int { return &v }
	f := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name    string
		d       ProfileDraft
		stored  *int
		missing []string
		bad     bool
	}{
		{name: "empty", d: ProfileDraft{}, bad: true},
		{name: "salary only", d: ProfileDraft{AnnualSalary: f(1e6)}},
		{name: "age needs spend", d: ProfileDraft{RetirementAge: i(60)}, stored: i(1990), missing: []string{"retirement_monthly_spend"}},
		{name: "spend needs age", d: ProfileDraft{RetirementMonthlySpend: f(4e4)}, stored: i(1990), missing: []string{"retirement_age"}},
		{name: "age needs a birth year", d: ProfileDraft{RetirementAge: i(60), RetirementMonthlySpend: f(4e4)}, missing: []string{"birth_year"}},
		{name: "draft's birth year counts", d: ProfileDraft{BirthYear: i(1990), RetirementAge: i(60), RetirementMonthlySpend: f(4e4)}},
		{name: "already past it", d: ProfileDraft{RetirementAge: i(30), RetirementMonthlySpend: f(4e4)}, stored: i(1990), bad: true},
		{name: "past the horizon", d: ProfileDraft{RetirementAge: i(95), RetirementMonthlySpend: f(4e4)}, stored: i(1990), bad: true},
		{name: "birth year in the future", d: ProfileDraft{BirthYear: i(2030)}, bad: true},
	} {
		var missing []string
		bad := false
		for _, p := range tc.d.Problems(tc.stored, 2026) {
			if p.Missing {
				missing = append(missing, p.Field)
			} else {
				bad = true
			}
		}
		if bad != tc.bad || !slices.Equal(missing, tc.missing) {
			t.Errorf("%s: bad=%v missing=%v, want bad=%v missing=%v", tc.name, bad, missing, tc.bad, tc.missing)
		}
	}
}

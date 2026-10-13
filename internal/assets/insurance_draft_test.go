package assets

import (
	"reflect"
	"testing"
)

func TestInsurancePerPeriod(t *testing.T) {
	for kind, want := range map[string]string{"life": "", "accident": "", "ci": "", "cancer": "", "disability": "month", "hospital": "day"} {
		if got, ok := InsurancePerPeriod(kind); !ok || got != want {
			t.Errorf("InsurancePerPeriod(%q) = %q, %v; want %q, true", kind, got, ok, want)
		}
	}
	if _, ok := InsurancePerPeriod("savings"); ok {
		t.Error("InsurancePerPeriod(savings) ok = true, want false")
	}
}

func TestInsuranceDraftProblems(t *testing.T) {
	amt, zero, prem, negYears := 2000000.0, 0.0, 36000.0, int64(-1)
	ok := InsuranceDraft{Name: "富邦終身壽險", Insurer: "富邦人壽", Kind: "life", Amount: &amt, AnnualPremium: &prem}
	cases := []struct {
		name string
		edit func(*InsuranceDraft)
		want []string
	}{
		{"complete", func(*InsuranceDraft) {}, []string{}},
		{"no premium given", func(d *InsuranceDraft) { d.AnnualPremium = nil }, []string{}},
		{"nothing filled in", func(d *InsuranceDraft) { *d = InsuranceDraft{} }, []string{"insurer", "name", "kind", "amount"}},
		{"bad values", func(d *InsuranceDraft) { d.Kind, d.Amount, d.PremiumYears = "savings", &zero, &negYears }, []string{"kind!", "amount!", "premium_years!"}},
	}
	for _, c := range cases {
		d := ok
		c.edit(&d)
		if got := fields(d.Problems()); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

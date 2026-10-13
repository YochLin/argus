package assets

import (
	"reflect"
	"testing"
)

func TestCashflowDraftProblems(t *testing.T) {
	amt, zero, day, badDay := 32000.0, 0.0, int64(5), int64(32)
	ok := CashflowDraft{Direction: "out", Name: "房貸扣款", Amount: &amt, Currency: "TWD", DayOfMonth: &day, Category: "mortgage"}
	cases := []struct {
		name string
		edit func(*CashflowDraft)
		want []string
	}{
		{"complete", func(*CashflowDraft) {}, []string{}},
		{"no fixed day", func(c *CashflowDraft) { c.DayOfMonth = nil }, []string{}},
		{"income", func(c *CashflowDraft) { c.Direction, c.Category = "in", "salary" }, []string{}},
		{"nothing filled in", func(c *CashflowDraft) { *c = CashflowDraft{} }, []string{"direction", "name", "amount", "currency", "category"}},
		{"bad values", func(c *CashflowDraft) { c.Direction, c.Amount, c.DayOfMonth = "both", &zero, &badDay }, []string{"direction!", "amount!", "day_of_month!"}},
		{"category of the other direction", func(c *CashflowDraft) { c.Category = "salary" }, []string{"category!"}},
	}
	for _, c := range cases {
		d := ok
		c.edit(&d)
		if got := fields(d.Problems()); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

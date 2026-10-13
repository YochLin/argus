package assets

import (
	"fmt"
	"slices"
	"strings"
)

// IncomeCategories and ExpenseCategories are the /w/cash form's 收入五項 and
// 支出六項 (docs/phase-9-asset-platform.md §8.4). The column is free text,
// but these are what the page groups and labels.
var (
	IncomeCategories  = []string{"salary", "rent", "dividend", "bond_interest", "fund_dividend"}
	ExpenseCategories = []string{"living", "mortgage", "sip", "loan", "insurance", "tax"}
)

// NewCashflow is the part of a recurring cash flow that has rules: what the
// /w/cash forms and the assistant's drafts validate. Amount nil = not given.
// Callers trim first.
type NewCashflow struct {
	Direction, Name string
	Amount          *float64
	DayOfMonth      *int64
}

// ValidateCashflow returns every problem with c, in field order.
func ValidateCashflow(c NewCashflow) []Problem {
	var ps []Problem
	switch c.Direction {
	case "":
		ps = append(ps, missing("direction"))
	case "in", "out":
	default:
		ps = append(ps, Problem{Field: "direction", Msg: `direction must be "in" or "out": ` + c.Direction})
	}
	if c.Name == "" {
		ps = append(ps, missing("name"))
	}
	switch {
	case c.Amount == nil:
		ps = append(ps, missing("amount"))
	case *c.Amount <= 0:
		ps = append(ps, Problem{Field: "amount", Msg: "amount must be positive"})
	}
	if c.DayOfMonth != nil && (*c.DayOfMonth < 1 || *c.DayOfMonth > 31) {
		ps = append(ps, Problem{Field: "day_of_month", Msg: fmt.Sprintf("day of month must be between 1 and 31: %d", *c.DayOfMonth)})
	}
	return ps
}

// CashflowDraft is a proposed recurring cash flow — wealth_drafts.payload for
// kind "cashflow", and the propose_cashflow tool's input. It is never linked
// to an asset (§5.2's simplification): that link is made on /w/cash.
type CashflowDraft struct {
	Direction  string   `json:"direction" jsonschema:"in for income, out for an expense or payment"`
	Name       string   `json:"name" jsonschema:"what the user calls it, e.g. 薪水 or 房貸扣款"`
	Amount     *float64 `json:"amount,omitempty" jsonschema:"the monthly amount, positive. Omit if the user hasn't said"`
	Currency   string   `json:"currency" jsonschema:"ISO code, e.g. TWD or USD"`
	DayOfMonth *int64   `json:"day_of_month,omitempty" jsonschema:"1-31, the day it is paid or debited; omit if it has no fixed day"`
	Category   string   `json:"category" jsonschema:"for in: salary, rent, dividend, bond_interest or fund_dividend; for out: living, mortgage, sip (a fund DCA), loan, insurance or tax"`
}

// Normalize trims every text field and folds the enum-like ones' case.
func (d *CashflowDraft) Normalize() {
	d.Name = strings.TrimSpace(d.Name)
	d.Direction = strings.ToLower(strings.TrimSpace(d.Direction))
	d.Category = strings.ToLower(strings.TrimSpace(d.Category))
	d.Currency = strings.ToUpper(strings.TrimSpace(d.Currency))
}

// Problems is ValidateCashflow plus what only a draft needs: a currency and a
// category from the direction's list (the form always picks one; a draft
// without one would land in /w/cash ungrouped).
func (d CashflowDraft) Problems() []Problem {
	ps := ValidateCashflow(NewCashflow{Direction: d.Direction, Name: d.Name, Amount: d.Amount, DayOfMonth: d.DayOfMonth})
	if d.Currency == "" {
		ps = append(ps, missing("currency"))
	}
	cats := ExpenseCategories
	if d.Direction == "in" {
		cats = IncomeCategories
	}
	switch {
	case d.Category == "":
		ps = append(ps, missing("category"))
	case (d.Direction == "in" || d.Direction == "out") && !slices.Contains(cats, d.Category):
		ps = append(ps, Problem{Field: "category", Msg: "category must be one of " + strings.Join(cats, ", ") + ": " + d.Category})
	}
	return ps
}

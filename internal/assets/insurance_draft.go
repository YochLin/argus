package assets

import (
	"strings"
)

// InsuranceKind is a coverage kind and its benefit unit: "" a lump sum,
// "month" or "day". The unit is derived from the kind server-side, never
// trusted from a client, so a coverage can't be filed under the wrong unit
// and corrupt the have-side sums (§8.6: per-month/per-day benefits are never
// added to lump sums).
type InsuranceKind struct{ Kind, PerPeriod string }

// InsuranceKinds are the six coverage kinds, in the design template's order
// (Argus Trading WebUI.dc.html insureModel()).
var InsuranceKinds = []InsuranceKind{
	{"life", ""}, {"accident", ""}, {"ci", ""}, {"cancer", ""},
	{"disability", "month"}, {"hospital", "day"},
}

// InsurancePerPeriod returns kind's benefit unit; ok is false for a kind
// that isn't one of the six.
func InsurancePerPeriod(kind string) (perPeriod string, ok bool) {
	for _, k := range InsuranceKinds {
		if k.Kind == kind {
			return k.PerPeriod, true
		}
	}
	return "", false
}

func insuranceKindNames() string {
	names := make([]string, len(InsuranceKinds))
	for i, k := range InsuranceKinds {
		names[i] = k.Kind
	}
	return strings.Join(names, ", ")
}

// NewPolicy is the part of a new insurance policy that has rules: what the
// /w/insure form and the assistant's drafts validate. A policy is one asset
// with one coverage (the form's shape). Nil numbers = not given; callers
// trim first.
type NewPolicy struct {
	Insurer, Name, Kind string
	Amount              *float64
	AnnualPremium       *float64
	PremiumYears        *int64
}

// ValidatePolicy returns every problem with p, in field order.
func ValidatePolicy(p NewPolicy) []Problem {
	var ps []Problem
	if p.Insurer == "" {
		ps = append(ps, missing("insurer"))
	}
	if p.Name == "" {
		ps = append(ps, missing("name"))
	}
	if p.Kind == "" {
		ps = append(ps, missing("kind"))
	} else if _, ok := InsurancePerPeriod(p.Kind); !ok {
		ps = append(ps, bad("kind", "KindInvalid", "kind must be one of "+insuranceKindNames()+": "+p.Kind, p.Kind))
	}
	switch {
	case p.Amount == nil:
		ps = append(ps, missing("amount"))
	case *p.Amount <= 0:
		ps = append(ps, bad("amount", "AmountPositive", "amount must be positive"))
	}
	if p.AnnualPremium != nil && *p.AnnualPremium < 0 {
		ps = append(ps, bad("annual_premium", "PremiumNegative", "annual premium can't be negative"))
	}
	if p.PremiumYears != nil && *p.PremiumYears < 0 {
		ps = append(ps, bad("premium_years", "PremiumYearsNegative", "premium years can't be negative"))
	}
	return ps
}

// InsuranceDraft is a proposed policy — wealth_drafts.payload for kind
// "insurance", and the propose_insurance tool's input. Like the /w/insure
// form it carries one coverage, in TWD; a policy with several coverages is
// finished on /w/insure.
type InsuranceDraft struct {
	Name          string   `json:"name" jsonschema:"the policy's name, e.g. 富邦終身壽險"`
	Insurer       string   `json:"insurer" jsonschema:"the insurance company, e.g. 富邦人壽"`
	Insured       string   `json:"insured,omitempty" jsonschema:"who it covers, e.g. 本人 or 配偶; omit if not said"`
	Kind          string   `json:"kind" jsonschema:"the main coverage: life, accident, ci (critical illness), cancer, disability (paid per month) or hospital (paid per day)"`
	Amount        *float64 `json:"amount,omitempty" jsonschema:"the coverage amount in TWD: the sum insured, or for disability the monthly benefit, for hospital the daily benefit. Omit if the user hasn't said"`
	AnnualPremium *float64 `json:"annual_premium,omitempty" jsonschema:"premium paid per year in TWD"`
	PremiumYears  *int64   `json:"premium_years,omitempty" jsonschema:"how many years premiums are paid, e.g. 20"`
}

// Normalize trims every text field and folds the kind's case.
func (d *InsuranceDraft) Normalize() {
	d.Name = strings.TrimSpace(d.Name)
	d.Insurer = strings.TrimSpace(d.Insurer)
	d.Insured = strings.TrimSpace(d.Insured)
	d.Kind = strings.ToLower(strings.TrimSpace(d.Kind))
}

// Problems is ValidatePolicy over the draft.
func (d InsuranceDraft) Problems() []Problem {
	return ValidatePolicy(NewPolicy{Insurer: d.Insurer, Name: d.Name, Kind: d.Kind, Amount: d.Amount, AnnualPremium: d.AnnualPremium, PremiumYears: d.PremiumYears})
}

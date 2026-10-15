package assets

import (
	"fmt"
	"strconv"
	"strings"
)

// RetirementHorizonAge is the age the retirement projection simulates out
// to, and so the latest retirement age that can be saved.
const RetirementHorizonAge = 92

// Profile is the household settings /api/wealth/profile saves (§9.3,
// service.WealthProfile). Nil = leave as it is.
type Profile struct {
	AnnualSalary                  *float64
	BirthYear                     *int
	RetirementMonthlyContribution *float64
	Dependents                    *int
	YoungestChildAge              *int
	SpouseHasIncome               *bool
}

// ValidateProfile returns every problem with the fields p sets.
func ValidateProfile(p Profile, thisYear int) []Problem {
	var ps []Problem
	if p.AnnualSalary != nil && *p.AnnualSalary < 0 {
		ps = append(ps, bad("annual_salary", "SalaryNegative", "annual salary must not be negative"))
	}
	if p.BirthYear != nil && (*p.BirthYear < 1900 || *p.BirthYear > thisYear) {
		ps = append(ps, bad("birth_year", "BirthYearRange", fmt.Sprintf("birth year must be between 1900 and %d", thisYear), strconv.Itoa(thisYear)))
	}
	if p.RetirementMonthlyContribution != nil && *p.RetirementMonthlyContribution < 0 {
		ps = append(ps, bad("retirement_monthly_contribution", "RetireContribNegative", "monthly retirement contribution must not be negative"))
	}
	if p.Dependents != nil && *p.Dependents < 0 {
		ps = append(ps, bad("dependents", "DependentsNegative", "dependents must not be negative"))
	}
	if p.YoungestChildAge != nil && (*p.YoungestChildAge < 0 || *p.YoungestChildAge > 30) {
		ps = append(ps, bad("youngest_child_age", "ChildAgeRange", "youngest child's age must be between 0 and 30"))
	}
	return ps
}

// ValidateRetirement checks a retirement plan — what /w/retire's quick
// switch saves into the retirement goal. Age and spend go together (the goal
// stores a date and an amount derived from both), and the age needs a birth
// year to become a date. Neither given = nothing to check.
func ValidateRetirement(age *int, monthlySpend *float64, birthYear *int, thisYear int) []Problem {
	if age == nil && monthlySpend == nil {
		return nil
	}
	var ps []Problem
	if age == nil {
		ps = append(ps, missing("retirement_age"))
	} else if birthYear == nil {
		ps = append(ps, missing("birth_year"))
	} else if *age <= thisYear-*birthYear || *age > RetirementHorizonAge {
		ps = append(ps, bad("retirement_age", "RetireAgeRange", fmt.Sprintf("retirement age must be above the current age and at most %d", RetirementHorizonAge), strconv.Itoa(RetirementHorizonAge)))
	}
	if monthlySpend == nil {
		ps = append(ps, missing("retirement_monthly_spend"))
	} else if *monthlySpend <= 0 {
		ps = append(ps, bad("retirement_monthly_spend", "RetireSpendPositive", "monthly spending in retirement must be positive"))
	}
	return ps
}

// ProfileDraft is proposed changes to the household settings and the
// retirement plan — wealth_drafts.payload for kind "profile". Only the
// fields given change.
type ProfileDraft struct {
	AnnualSalary                  *float64 `json:"annual_salary,omitempty" jsonschema:"yearly salary before tax, TWD"`
	BirthYear                     *int     `json:"birth_year,omitempty" jsonschema:"the user's birth year, e.g. 1990"`
	RetirementMonthlyContribution *float64 `json:"retirement_monthly_contribution,omitempty" jsonschema:"what the user puts aside for retirement each month, TWD"`
	Dependents                    *int     `json:"dependents,omitempty" jsonschema:"number of people who depend on the user's income"`
	YoungestChildAge              *int     `json:"youngest_child_age,omitempty" jsonschema:"age of the youngest child, if any"`
	SpouseHasIncome               *bool    `json:"spouse_has_income,omitempty"`
	RetirementAge                 *int     `json:"retirement_age,omitempty" jsonschema:"planned retirement age; goes with retirement_monthly_spend"`
	RetirementMonthlySpend        *float64 `json:"retirement_monthly_spend,omitempty" jsonschema:"monthly spending in retirement in today's TWD; goes with retirement_age"`
}

func (d ProfileDraft) Profile() Profile {
	return Profile{
		AnnualSalary: d.AnnualSalary, BirthYear: d.BirthYear, RetirementMonthlyContribution: d.RetirementMonthlyContribution,
		Dependents: d.Dependents, YoungestChildAge: d.YoungestChildAge, SpouseHasIncome: d.SpouseHasIncome,
	}
}

// Problems checks the draft against the stored birth year when it doesn't
// carry its own.
func (d ProfileDraft) Problems(storedBirthYear *int, thisYear int) []Problem {
	if d == (ProfileDraft{}) {
		return []Problem{bad("profile", "ProfileEmpty", "the draft changes nothing")}
	}
	ps := ValidateProfile(d.Profile(), thisYear)
	by := d.BirthYear
	if by == nil {
		by = storedBirthYear
	}
	return append(ps, ValidateRetirement(d.RetirementAge, d.RetirementMonthlySpend, by, thisYear)...)
}

// NewGoal is the part of a savings goal that has rules: what /w/goals'
// drawer and the assistant's drafts validate. Nil = not given.
type NewGoal struct {
	Name                string
	TargetAmount        *float64
	SavedAmount         *float64
	MonthlyContribution *float64
	StartYear           int
}

// ValidateGoal returns every problem with g, in field order.
func ValidateGoal(g NewGoal) []Problem {
	var ps []Problem
	if g.Name == "" {
		ps = append(ps, missing("name"))
	}
	if g.TargetAmount == nil {
		ps = append(ps, missing("target_amount"))
	} else if *g.TargetAmount <= 0 {
		ps = append(ps, bad("target_amount", "TargetPositive", "target amount must be positive"))
	}
	if g.SavedAmount != nil && *g.SavedAmount < 0 {
		ps = append(ps, bad("saved_amount", "SavedNegative", "saved amount must not be negative"))
	}
	if g.MonthlyContribution != nil && *g.MonthlyContribution < 0 {
		ps = append(ps, bad("monthly_contribution", "MonthlyNegative", "monthly contribution must not be negative"))
	}
	if g.StartYear < 0 {
		ps = append(ps, bad("start_year", "StartYearNegative", "start year must not be negative"))
	}
	return ps
}

// GoalDraft is a proposed savings goal — wealth_drafts.payload for kind
// "goal". Always a general goal in TWD, like /w/goals' drawer; retirement is
// the profile draft's retirement_age/retirement_monthly_spend.
type GoalDraft struct {
	Name                string   `json:"name" jsonschema:"what the goal is for, e.g. 子女教育金 or 購屋頭期款"`
	TargetAmount        *float64 `json:"target_amount,omitempty" jsonschema:"amount to reach, TWD"`
	TargetYear          *int     `json:"target_year,omitempty" jsonschema:"year the user wants to reach it"`
	SavedAmount         *float64 `json:"saved_amount,omitempty" jsonschema:"already saved towards it, TWD"`
	MonthlyContribution *float64 `json:"monthly_contribution,omitempty" jsonschema:"put aside for it each month, TWD"`
	Note                string   `json:"note,omitempty"`
}

func (d *GoalDraft) Normalize() {
	d.Name, d.Note = strings.TrimSpace(d.Name), strings.TrimSpace(d.Note)
}

func (d GoalDraft) Problems(thisYear int) []Problem {
	ps := ValidateGoal(NewGoal{Name: d.Name, TargetAmount: d.TargetAmount, SavedAmount: d.SavedAmount, MonthlyContribution: d.MonthlyContribution})
	if d.TargetYear != nil && (*d.TargetYear < thisYear || *d.TargetYear > thisYear+100) {
		ps = append(ps, bad("target_year", "TargetYearRange", fmt.Sprintf("target year must be between %d and %d", thisYear, thisYear+100), strconv.Itoa(thisYear), strconv.Itoa(thisYear+100)))
	}
	return ps
}

// TargetDate is the goal's date as /w/goals' drawer stores a target year.
func (d GoalDraft) TargetDate() string {
	if d.TargetYear == nil {
		return ""
	}
	return fmt.Sprintf("%04d-12-31", *d.TargetYear)
}

package assets

import (
	"math"
	"sort"
)

// Loan is one debt-payoff simulation input. Balance and MinPayment are
// always positive. MinPayment normally comes from AmortizedMinPayment
// (there's no recurring_cashflows table yet to read a real minimum from —
// see docs/phase-9-asset-platform.md §10.2①).
type Loan struct {
	Name       string
	Balance    float64
	RatePct    float64
	MinPayment float64
}

// AmortizedMinPayment is the standard fixed-payment (等額本息) formula: the
// level monthly payment that clears balance over remainingMonths at ratePct
// annual interest. This is the fallback §10.2① calls for when no
// recurring_cashflows override exists. ok=false when balance or
// remainingMonths isn't usable.
func AmortizedMinPayment(balance, ratePct float64, remainingMonths int) (float64, bool) {
	if balance <= 0 || remainingMonths <= 0 {
		return 0, false
	}
	monthlyRate := ratePct / 100 / 12
	if monthlyRate <= 0 {
		return balance / float64(remainingMonths), true
	}
	factor := math.Pow(1+monthlyRate, float64(remainingMonths))
	return balance * monthlyRate * factor / (factor - 1), true
}

// DebtPayoffPlan is one strategy's simulated outcome against a given extra
// monthly payment.
type DebtPayoffPlan struct {
	Order         []string // loan names, in the order they get fully paid off
	Months        int
	TotalInterest float64
	MonthsSaved   int // vs the same strategy's minimums-only baseline (extraMonthly=0)
}

// maxPayoffMonths caps the simulation loop.
// ponytail: a loan set that wouldn't clear in 50 years under its own stated
// minimums is almost certainly a bad MinPayment input, not a real plan —
// bail out rather than loop forever.
const maxPayoffMonths = 600

// ComputeDebtPayoff runs both the debt snowball (smallest balance first)
// and debt avalanche (highest rate first) strategies against the same
// loans and extra monthly payment, per §10.2①: output both numbers, never
// pick one for the user — snowball wins on psychological momentum,
// avalanche wins on total interest, that's a preference, not an
// optimization the platform should make.
func ComputeDebtPayoff(loans []Loan, extraMonthly float64) (snowball, avalanche DebtPayoffPlan) {
	snowballPriority := sortedIndices(loans, func(a, b Loan) bool { return a.Balance < b.Balance })
	avalanchePriority := sortedIndices(loans, func(a, b Loan) bool { return a.RatePct > b.RatePct })
	return runPlan(loans, extraMonthly, snowballPriority), runPlan(loans, extraMonthly, avalanchePriority)
}

func runPlan(loans []Loan, extraMonthly float64, priority []int) DebtPayoffPlan {
	months, interest, order := simulatePayoff(loans, extraMonthly, priority)
	baseMonths, _, _ := simulatePayoff(loans, 0, priority)
	return DebtPayoffPlan{Order: order, Months: months, TotalInterest: interest, MonthsSaved: baseMonths - months}
}

// simulatePayoff pays every open loan's own minimum each month, then
// redirects whatever's left over (extraMonthly, plus the minimums of
// already-cleared loans — the actual "snowball") at the highest-priority
// still-open loan, cascading down the priority order within the same month
// if that loan clears with budget still unspent.
func simulatePayoff(loans []Loan, extraMonthly float64, priority []int) (months int, totalInterest float64, order []string) {
	n := len(loans)
	balances := make([]float64, n)
	for i, l := range loans {
		balances[i] = l.Balance
	}
	cleared := make([]bool, n)
	nCleared := 0

	for months = 0; months < maxPayoffMonths && nCleared < n; months++ {
		for i := range loans {
			if cleared[i] {
				continue
			}
			interest := balances[i] * loans[i].RatePct / 100 / 12
			totalInterest += interest
			balances[i] += interest
		}

		freed := extraMonthly
		for i, c := range cleared {
			if c {
				freed += loans[i].MinPayment
			}
		}
		for i := range loans {
			if cleared[i] {
				continue
			}
			pay := math.Min(loans[i].MinPayment, balances[i])
			balances[i] -= pay
			freed += loans[i].MinPayment - pay
			if balances[i] <= 0.005 {
				cleared[i] = true
				nCleared++
				order = append(order, loans[i].Name)
			}
		}

		for _, idx := range priority {
			if cleared[idx] || freed <= 0.005 {
				continue
			}
			pay := math.Min(freed, balances[idx])
			balances[idx] -= pay
			freed -= pay
			if balances[idx] <= 0.005 {
				cleared[idx] = true
				nCleared++
				order = append(order, loans[idx].Name)
			}
		}
	}
	return months, totalInterest, order
}

func sortedIndices(loans []Loan, less func(a, b Loan) bool) []int {
	idx := make([]int, len(loans))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return less(loans[idx[i]], loans[idx[j]]) })
	return idx
}

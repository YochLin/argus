package service

import (
	"sort"

	"argus/internal/db"
)

// The wealth checks are the home page's "本月該做的事": fixed rules of thumb,
// checked against numbers the wealth pages already compute, each failing one
// becoming an action item. The thresholds are the rule, deliberately not
// settings. An item carries a code and figures, never text: the frontend
// words it, the same way draft problems carry a Code.
const (
	EmergencyMonths           = 6.0  // liquid assets / monthly spending
	EmergencyMonthsDependents = 9.0  // the same with anyone depending on the user's income
	DebtRatioMaxPct           = 40.0 // liabilities / assets
	CoverageMinPct            = 80.0 // the worst lump-sum cover, % of need
	RetirementMinPct          = 80.0 // baseline projection, % of need
	SavingsRateMinPct         = 20.0 // (income - spending) / income, recurring cash flows
	RebalanceMinAssets        = 3    // fewer assets than this and drift isn't measured
)

// Check severities, most urgent first.
const (
	SeverityDo   = "do"
	SeverityTip  = "tip"
	SeverityData = "data"
)

// CheckItem is one action. Value/Target are the figure the rule compared and
// its threshold; Gap is the amount still short (TWD), nil when there isn't
// one. Kind and Names say which coverage / which records.
type CheckItem struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Link     string   `json:"link"`
	Value    *float64 `json:"value,omitempty"`
	Target   *float64 `json:"target,omitempty"`
	Gap      *float64 `json:"gap,omitempty"`
	Count    int      `json:"count,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Names    []string `json:"names,omitempty"`
}

// WealthChecks is the whole result. New means nothing has been recorded yet:
// the page shows its setup guide instead, so Items stays empty.
type WealthChecks struct {
	AsOf  string      `json:"asOf"`
	New   bool        `json:"new"`
	Rules int         `json:"rules"`
	Items []CheckItem `json:"items"`
}

// CheckInputs is what the rules look at. A nil pointer is "can't be computed
// yet", and that rule is skipped (or, for the profile fields, asked for).
type CheckInputs struct {
	AssetCount int

	Dependents *int
	HasSalary  bool
	HasAge     bool

	LiquidityMonths *float64
	MonthlyExpense  *float64
	DebtRatioPct    *float64

	CardBalance *float64 // the largest credit card balance, TWD
	CardRatePct *float64
	CardName    string

	CoverageKind string // the worst lump-sum cover; "" when none can be measured
	CoveragePct  float64
	CoverageGap  float64

	RetirementPct *float64 // nil without a birth year or a need
	RetirementGap float64

	GoalsBehind int

	ModelPicked bool
	Rebalance   *float64 // total of the rebalance orders; nil when drift isn't measured
	OffTarget   int      // categories outside the band

	SavingsRatePct *float64

	Stale []string // records not updated for more than 90 days
}

// CheckRuleCount is how many rules EvaluateWealthChecks applies — the
// "依 N 項規則檢查" line. The profile asks aren't rules, they unlock them.
const CheckRuleCount = 9

// EvaluateWealthChecks applies the rules. Items come back most severe first,
// and in rule order (the usual financial-planning order) within a severity.
func EvaluateWealthChecks(in CheckInputs) []CheckItem {
	items := []CheckItem{}
	add := func(it CheckItem) { items = append(items, it) }
	f := func(v float64) *float64 { return &v }

	need := EmergencyMonths
	if in.Dependents != nil && *in.Dependents > 0 {
		need = EmergencyMonthsDependents
	}
	if in.LiquidityMonths != nil && *in.LiquidityMonths < need {
		it := CheckItem{Code: "emergency", Severity: SeverityDo, Link: "/w/balance", Value: in.LiquidityMonths, Target: f(need)}
		if in.MonthlyExpense != nil {
			it.Gap = f((need - *in.LiquidityMonths) * *in.MonthlyExpense)
		}
		add(it)
	}
	// No monthly spending yet with a salary saved means there is no deposit
	// balance from a month ago to estimate it from: say so rather than skip
	// the rule. Without a salary the salary ask below covers it.
	if in.LiquidityMonths == nil && in.MonthlyExpense == nil && in.HasSalary {
		add(CheckItem{Code: "emergencyData", Severity: SeverityData, Link: "/w/balance"})
	}
	if in.DebtRatioPct != nil && *in.DebtRatioPct >= DebtRatioMaxPct {
		add(CheckItem{Code: "debtRatio", Severity: SeverityDo, Link: "/w/balance", Value: in.DebtRatioPct, Target: f(DebtRatioMaxPct)})
	}
	if in.CardBalance != nil && *in.CardBalance > 0 {
		add(CheckItem{Code: "card", Severity: SeverityDo, Link: "/w/balance", Value: in.CardRatePct, Gap: in.CardBalance, Names: []string{in.CardName}})
	}
	if in.CoverageKind != "" && in.CoveragePct < CoverageMinPct {
		add(CheckItem{Code: "coverage", Severity: SeverityDo, Link: "/w/insure", Kind: in.CoverageKind, Value: f(in.CoveragePct), Target: f(CoverageMinPct), Gap: f(in.CoverageGap)})
	}
	if in.RetirementPct != nil && *in.RetirementPct < RetirementMinPct {
		add(CheckItem{Code: "retirement", Severity: SeverityTip, Link: "/w/retire", Value: in.RetirementPct, Target: f(RetirementMinPct), Gap: f(in.RetirementGap)})
	}
	if in.GoalsBehind > 0 {
		add(CheckItem{Code: "goals", Severity: SeverityTip, Link: "/w/goals", Count: in.GoalsBehind})
	}
	if in.ModelPicked && in.AssetCount >= RebalanceMinAssets && in.OffTarget > 0 && in.Rebalance != nil {
		add(CheckItem{Code: "alloc", Severity: SeverityTip, Link: "/w/alloc", Count: in.OffTarget, Gap: in.Rebalance})
	}
	if in.SavingsRatePct != nil && *in.SavingsRatePct < SavingsRateMinPct {
		add(CheckItem{Code: "savings", Severity: SeverityTip, Link: "/w/cash", Value: in.SavingsRatePct, Target: f(SavingsRateMinPct)})
	}
	if in.Dependents == nil {
		add(CheckItem{Code: "dependents", Severity: SeverityData, Link: "/w/balance"})
	}
	if !in.HasAge {
		add(CheckItem{Code: "age", Severity: SeverityData, Link: "/w/balance"})
	}
	if !in.HasSalary {
		add(CheckItem{Code: "salary", Severity: SeverityData, Link: "/w/balance"})
	}
	if len(in.Stale) > 0 {
		add(CheckItem{Code: "stale", Severity: SeverityData, Link: "/w/balance", Count: len(in.Stale), Names: firstN(in.Stale, 3)})
	}

	rank := map[string]int{SeverityDo: 0, SeverityTip: 1, SeverityData: 2}
	sort.SliceStable(items, func(i, j int) bool { return rank[items[i].Severity] < rank[items[j].Severity] })
	return items
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// CashflowTotalsTWD sums the active recurring cash flows by direction, in
// TWD. ok=false when any of them can't be priced.
func CashflowTotalsTWD(list []db.RecurringCashflow, date string, quotes QuoteReader, fx WealthFXStore) (in, out float64, ok bool) {
	for _, c := range list {
		if !c.Active {
			continue
		}
		rate, rok := RateToTWD(c.Currency, date, true, quotes, fx)
		if !rok {
			return 0, 0, false
		}
		if c.Direction == "in" {
			in += c.Amount * rate
		} else {
			out += c.Amount * rate
		}
	}
	return in, out, true
}

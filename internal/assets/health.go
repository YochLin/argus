package assets

// MonthlyExpense derives §2 point 4's "本月支出 ≈ 薪資入帳 − 存款餘額變化" — the
// only expense figure this platform can ever produce, since it deliberately
// does not do transaction-level bookkeeping (docs/phase-9-asset-platform.md
// §2 point 4). depositNow/depositPrevMonth are the deposit-type assets'
// total value (already converted to one currency) at two points a month
// apart.
//
// ponytail: this conflates "spent" with "any deposit outflow not matched by
// salary" — a one-off transfer to a brokerage account reads as expense.
// Known, accepted limitation per the spec; no fix planned short of real
// transaction data.
func MonthlyExpense(monthlySalary, depositNow, depositPrevMonth float64) float64 {
	return monthlySalary - (depositNow - depositPrevMonth)
}

// DebtRatio is total liabilities as a percentage of total assets. ok=false
// when totalAssets<=0 — nothing to divide by, not a real 0%.
func DebtRatio(totalAssets, totalLiabilities float64) (float64, bool) {
	if totalAssets <= 0 {
		return 0, false
	}
	return totalLiabilities / totalAssets * 100, true
}

// SavingsRate is the share of monthly salary not spent. ok=false when
// monthlySalary<=0 (profile.annual_salary not set yet, §9.3).
func SavingsRate(monthlySalary, monthlyExpense float64) (float64, bool) {
	if monthlySalary <= 0 {
		return 0, false
	}
	return (monthlySalary - monthlyExpense) / monthlySalary * 100, true
}

// ExpenseRatio is the share of monthly salary spent — SavingsRate's
// complement, kept as its own function since the platform's health-metric
// trio (負債比/收支比/儲蓄率, doc §1) displays both as separate numbers.
func ExpenseRatio(monthlySalary, monthlyExpense float64) (float64, bool) {
	if monthlySalary <= 0 {
		return 0, false
	}
	return monthlyExpense / monthlySalary * 100, true
}

// LiquidityMonths is how many months of expenses the liquid-group total
// covers (現金 ÷ 月支出, doc §8.3-1). ok=false when monthlyExpense<=0 —
// nothing meaningful to divide by (and a negative expense, i.e. deposits
// grew faster than salary alone explains, would otherwise produce a
// misleadingly large or negative number).
func LiquidityMonths(liquidAssets, monthlyExpense float64) (float64, bool) {
	if monthlyExpense <= 0 {
		return 0, false
	}
	return liquidAssets / monthlyExpense, true
}

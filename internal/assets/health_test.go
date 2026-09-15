package assets

import "testing"

func TestMonthlyExpense(t *testing.T) {
	got := MonthlyExpense(60000, 120000, 100000) // salary 60k, deposits grew 20k
	want := 40000.0
	if !approxEqual(got, want) {
		t.Errorf("MonthlyExpense() = %v, want %v", got, want)
	}
}

func TestDebtRatio(t *testing.T) {
	if got, ok := DebtRatio(0, 100); ok {
		t.Errorf("DebtRatio(0, 100) ok = true, want false; got %v", got)
	}
	got, ok := DebtRatio(1000, 300)
	if !ok || !approxEqual(got, 30) {
		t.Errorf("DebtRatio(1000, 300) = %v, %v; want 30, true", got, ok)
	}
}

func TestSavingsRateAndExpenseRatio(t *testing.T) {
	if _, ok := SavingsRate(0, 100); ok {
		t.Error("SavingsRate(0, ...) ok = true, want false")
	}
	sr, ok := SavingsRate(60000, 40000)
	if !ok || !approxEqual(sr, 33.33333333333333) {
		t.Errorf("SavingsRate(60000, 40000) = %v, %v", sr, ok)
	}
	er, ok := ExpenseRatio(60000, 40000)
	if !ok || !approxEqual(er, 66.66666666666667) {
		t.Errorf("ExpenseRatio(60000, 40000) = %v, %v", er, ok)
	}
	if !approxEqual(sr+er, 100) {
		t.Errorf("SavingsRate + ExpenseRatio = %v, want 100", sr+er)
	}
}

func TestLiquidityMonths(t *testing.T) {
	if _, ok := LiquidityMonths(1000, 0); ok {
		t.Error("LiquidityMonths(..., 0) ok = true, want false")
	}
	got, ok := LiquidityMonths(120000, 40000)
	if !ok || !approxEqual(got, 3) {
		t.Errorf("LiquidityMonths(120000, 40000) = %v, %v; want 3, true", got, ok)
	}
}

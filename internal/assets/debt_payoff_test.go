package assets

import (
	"math"
	"testing"
)

func TestAmortizedMinPaymentZeroRate(t *testing.T) {
	got, ok := AmortizedMinPayment(12000, 0, 12)
	if !ok || !approxEqual(got, 1000) {
		t.Errorf("AmortizedMinPayment(12000, 0, 12) = %v, %v; want 1000, true", got, ok)
	}
}

func TestAmortizedMinPaymentInvalidInputs(t *testing.T) {
	if _, ok := AmortizedMinPayment(0, 5, 12); ok {
		t.Error("AmortizedMinPayment(0, ...) ok = true, want false")
	}
	if _, ok := AmortizedMinPayment(1000, 5, 0); ok {
		t.Error("AmortizedMinPayment(..., 0) ok = true, want false")
	}
}

func TestAmortizedMinPaymentPositiveRate(t *testing.T) {
	// A 12-month, 6%/yr loan's level payment must fully amortize the
	// balance: simulating it down with that payment should land at ~0.
	payment, ok := AmortizedMinPayment(120000, 6, 12)
	if !ok {
		t.Fatal("AmortizedMinPayment ok = false")
	}
	balance := 120000.0
	monthlyRate := 0.06 / 12
	for i := 0; i < 12; i++ {
		balance += balance * monthlyRate
		balance -= payment
	}
	if math.Abs(balance) > 1e-6 {
		t.Errorf("amortization schedule leaves balance = %v, want ~0 (payment=%v)", balance, payment)
	}
}

func TestComputeDebtPayoffAvalancheNeverWorseThanSnowball(t *testing.T) {
	loans := []Loan{
		{Name: "small", Balance: 500, RatePct: 5, MinPayment: 30},
		{Name: "big", Balance: 5000, RatePct: 20, MinPayment: 150},
	}
	snowball, avalanche := ComputeDebtPayoff(loans, 100)

	if avalanche.TotalInterest > snowball.TotalInterest+1e-6 {
		t.Errorf("avalanche interest %v > snowball interest %v; avalanche should never pay more total interest",
			avalanche.TotalInterest, snowball.TotalInterest)
	}
	if len(snowball.Order) != len(loans) || len(avalanche.Order) != len(loans) {
		t.Errorf("Order lengths = %d, %d; want both %d", len(snowball.Order), len(avalanche.Order), len(loans))
	}
	if snowball.Months <= 0 || avalanche.Months <= 0 {
		t.Errorf("Months = %d, %d; want both > 0", snowball.Months, avalanche.Months)
	}
	if snowball.MonthsSaved <= 0 || avalanche.MonthsSaved <= 0 {
		t.Errorf("MonthsSaved = %d, %d; want both > 0 given a positive extra payment",
			snowball.MonthsSaved, avalanche.MonthsSaved)
	}
}

func TestComputeDebtPayoffNoExtraPaymentSavesNothing(t *testing.T) {
	loans := []Loan{{Name: "only", Balance: 1000, RatePct: 5, MinPayment: 50}}
	snowball, avalanche := ComputeDebtPayoff(loans, 0)
	if snowball.MonthsSaved != 0 || avalanche.MonthsSaved != 0 {
		t.Errorf("MonthsSaved = %d, %d with no extra payment; want both 0", snowball.MonthsSaved, avalanche.MonthsSaved)
	}
}

func TestComputeDebtPayoffEmptyLoans(t *testing.T) {
	snowball, avalanche := ComputeDebtPayoff(nil, 500)
	if snowball.Months != 0 || snowball.TotalInterest != 0 || snowball.Order != nil {
		t.Errorf("snowball with no loans = %+v, want zero value", snowball)
	}
	if avalanche.Months != 0 || avalanche.TotalInterest != 0 || avalanche.Order != nil {
		t.Errorf("avalanche with no loans = %+v, want zero value", avalanche)
	}
}

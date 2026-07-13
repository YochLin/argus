package render

import (
	"strings"
	"testing"

	"argus/internal/data"
	"argus/internal/i18n"
)

func TestFundamentals(t *testing.T) {
	fd := &data.Fundamentals{
		Ticker:           "AAPL",
		PE:               28.5,
		MarketCapMillion: 3200000,
	}
	out := Fundamentals(i18n.EN, fd)
	if !strings.Contains(out, "3,200,000") {
		t.Errorf("Fundamentals() = %q, want market cap formatted with thousands separators", out)
	}
}

func TestFinancialStatement(t *testing.T) {
	st := &data.FinancialStatement{
		Form:       "10-K",
		FiscalYear: 2025,
		PeriodEnd:  "2025-09-27",
		Revenue:    391035000000,
		NetIncome:  93736000000,
	}
	out := FinancialStatement(i18n.EN, st)
	if !strings.Contains(out, "10-K") || !strings.Contains(out, "2025-09-27") {
		t.Errorf("FinancialStatement() = %q, want it to contain form and period end", out)
	}
	// Revenue/NetIncome are formatted in millions with thousands separators.
	if !strings.Contains(out, "391,035") {
		t.Errorf("FinancialStatement() = %q, want revenue formatted in millions", out)
	}
}

func TestCommaf(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{4321020, "4,321,020"},
		{1234567.89, "1,234,568"}, // rounds to nearest integer
		{-1234567, "-1,234,567"},
		{-500, "-500"},
		{100, "100"},
	}
	for _, tt := range tests {
		if got := Commaf(tt.in); got != tt.want {
			t.Errorf("Commaf(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

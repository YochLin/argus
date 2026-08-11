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

// TestFinancialStatementSkipsEmptySections covers a TW filing (FinMind,
// Phase 6 PR3): TotalAssets/TotalLiabilities/TotalEquity/OperatingCashFlow/
// CapEx/FreeCashFlow are all genuinely unavailable (0), and both sections
// must be omitted entirely rather than rendering a misleading "$0M" trio.
func TestFinancialStatementSkipsEmptySections(t *testing.T) {
	st := &data.FinancialStatement{
		Form:       "Q1",
		FiscalYear: 2026,
		PeriodEnd:  "2026-03-31",
		Revenue:    1134103440000,
		DilutedEPS: 22.08,
	}
	out := FinancialStatement(i18n.EN, st)
	if strings.Contains(out, "Balance Sheet") {
		t.Errorf("FinancialStatement() = %q, want no Balance Sheet section when all its fields are 0", out)
	}
	if strings.Contains(out, "Cash Flow") {
		t.Errorf("FinancialStatement() = %q, want no Cash Flow section when all its fields are 0", out)
	}
	if !strings.Contains(out, "22.08") {
		t.Errorf("FinancialStatement() = %q, want the diluted EPS still rendered", out)
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

// TestMoney covers the US-vs-TW split: US keeps cents (a whole-dollar
// round like Commaf's would misrepresent a per-share price/fee, e.g.
// $205.50 rounding to "$206"), TW stays whole-unit as it's quoted.
func TestMoney(t *testing.T) {
	tests := []struct {
		ticker string
		in     float64
		want   string
	}{
		{"AAPL", 205.50, "$205.50"},
		{"AAPL", 1234.5, "$1,234.50"},
		{"AAPL", 0.99, "$0.99"},
		{"AAPL", -205.50, "-$205.50"},
		{"2330", 1234, "NT$1,234"},
		{"2330", -20, "-NT$20"},
	}
	for _, tt := range tests {
		if got := Money(tt.ticker, tt.in); got != tt.want {
			t.Errorf("Money(%q, %v) = %q, want %q", tt.ticker, tt.in, got, tt.want)
		}
	}
}

package bot

import (
	"strings"
	"testing"

	"argus/internal/data"
	"argus/internal/i18n"
)

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
		if got := commaf(tt.in); got != tt.want {
			t.Errorf("commaf(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDedup(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []string
	}{
		{
			name: "removes overlap",
			a:    []string{"AAPL", "MSFT", "NVDA"},
			b:    []string{"MSFT"},
			want: []string{"AAPL", "NVDA"},
		},
		{
			name: "no overlap returns a unchanged",
			a:    []string{"AAPL", "MSFT"},
			b:    []string{"TSLA"},
			want: []string{"AAPL", "MSFT"},
		},
		{
			name: "everything overlaps returns nil",
			a:    []string{"AAPL", "MSFT"},
			b:    []string{"AAPL", "MSFT"},
			want: nil,
		},
		{
			name: "empty a returns nil",
			a:    nil,
			b:    []string{"AAPL"},
			want: nil,
		},
		{
			name: "empty b returns a unchanged",
			a:    []string{"AAPL", "MSFT"},
			b:    nil,
			want: []string{"AAPL", "MSFT"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedup(tt.a, tt.b)
			if len(got) != len(tt.want) {
				t.Fatalf("dedup(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("dedup(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
				}
			}
		})
	}
}

func TestFormatQuote(t *testing.T) {
	t.Run("positive change shows up arrow", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", Price: 200, ChangePercent: 1.5, Open: 198, High: 201, Low: 197}
		out := formatQuote(i18n.EN, q)
		if !strings.Contains(out, "▲") {
			t.Errorf("formatQuote() = %q, want it to contain up arrow", out)
		}
		if !strings.Contains(out, "AAPL") {
			t.Errorf("formatQuote() = %q, want it to contain ticker", out)
		}
	})

	t.Run("negative change shows down arrow", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", Price: 200, ChangePercent: -1.5, Open: 198, High: 201, Low: 197}
		out := formatQuote(i18n.EN, q)
		if !strings.Contains(out, "▼") {
			t.Errorf("formatQuote() = %q, want it to contain down arrow", out)
		}
	})
}

func TestFormatFundamentals(t *testing.T) {
	fd := &data.Fundamentals{
		Ticker:           "AAPL",
		PE:               28.5,
		MarketCapMillion: 3200000,
	}
	out := formatFundamentals(i18n.EN, fd)
	if !strings.Contains(out, "3,200,000") {
		t.Errorf("formatFundamentals() = %q, want market cap formatted with thousands separators", out)
	}
}

func TestFormatFinancialStatement(t *testing.T) {
	st := &data.FinancialStatement{
		Form:       "10-K",
		FiscalYear: 2025,
		PeriodEnd:  "2025-09-27",
		Revenue:    391035000000,
		NetIncome:  93736000000,
	}
	out := formatFinancialStatement(i18n.EN, st)
	if !strings.Contains(out, "10-K") || !strings.Contains(out, "2025-09-27") {
		t.Errorf("formatFinancialStatement() = %q, want it to contain form and period end", out)
	}
	// Revenue/NetIncome are formatted in millions with thousands separators.
	if !strings.Contains(out, "391,035") {
		t.Errorf("formatFinancialStatement() = %q, want revenue formatted in millions", out)
	}
}

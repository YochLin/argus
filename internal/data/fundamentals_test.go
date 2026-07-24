package data

import "testing"

// fakeFundamentalsProvider is a minimal FundamentalsProvider stub for
// exercising FundamentalsRouter's dispatch logic without hitting either
// real backend.
type fakeFundamentalsProvider struct {
	name string
}

func (f fakeFundamentalsProvider) GetFundamentals(ticker string) (*Fundamentals, error) {
	return &Fundamentals{Ticker: f.name + ":" + ticker}, nil
}

func (f fakeFundamentalsProvider) GetFinancialStatements(ticker, freq string) (*FinancialStatement, error) {
	return &FinancialStatement{Ticker: f.name + ":" + ticker}, nil
}

func TestFundamentalsRouter(t *testing.T) {
	router := &FundamentalsRouter{
		US: fakeFundamentalsProvider{name: "us"},
		TW: fakeFundamentalsProvider{name: "tw"},
	}

	fd, err := router.GetFundamentals("AAPL")
	if err != nil || fd.Ticker != "us:AAPL" {
		t.Errorf("GetFundamentals(AAPL) = %+v, %v, want routed to US", fd, err)
	}
	fd, err = router.GetFundamentals("2330")
	if err != nil || fd.Ticker != "tw:2330" {
		t.Errorf("GetFundamentals(2330) = %+v, %v, want routed to TW", fd, err)
	}

	st, err := router.GetFinancialStatements("AAPL", "annual")
	if err != nil || st.Ticker != "us:AAPL" {
		t.Errorf("GetFinancialStatements(AAPL) = %+v, %v, want routed to US", st, err)
	}
	st, err = router.GetFinancialStatements("2330", "quarterly")
	if err != nil || st.Ticker != "tw:2330" {
		t.Errorf("GetFinancialStatements(2330) = %+v, %v, want routed to TW", st, err)
	}
}

func TestFundamentalsRouterMissingProvider(t *testing.T) {
	usOnly := &FundamentalsRouter{US: fakeFundamentalsProvider{name: "us"}}
	if _, err := usOnly.GetFundamentals("2330"); err == nil {
		t.Error("GetFundamentals(2330) with no TW provider = nil error, want an error")
	}
	if _, err := usOnly.GetFundamentals("AAPL"); err != nil {
		t.Errorf("GetFundamentals(AAPL) with US provider configured = %v, want nil error", err)
	}

	twOnly := &FundamentalsRouter{TW: fakeFundamentalsProvider{name: "tw"}}
	if _, err := twOnly.GetFundamentals("AAPL"); err == nil {
		t.Error("GetFundamentals(AAPL) with no US provider = nil error, want an error")
	}

	empty := &FundamentalsRouter{}
	if _, err := empty.GetFundamentals("AAPL"); err == nil {
		t.Error("GetFundamentals(AAPL) on an empty router = nil error, want an error")
	}
}

func TestPeriodEndDate(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2025-09-27 00:00:00", "2025-09-27"},
		{"2025-09-27", "2025-09-27"},
		{"", ""},
		{"2025", "2025"},
	}
	for _, tt := range tests {
		if got := periodEndDate(tt.in); got != tt.want {
			t.Errorf("periodEndDate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

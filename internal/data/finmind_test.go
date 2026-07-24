package data

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFinMindTWGuard confirms both FinMind methods reject a non-TW ticker
// before making any HTTP request — mirrors finnhub_test.go's
// TestFinnhubTWGuard (market swapped): baseURL is left at the real API's
// address, so a request that slipped past the guard would fail loudly
// (network error) rather than silently, not return errNotTWTicker.
func TestFinMindTWGuard(t *testing.T) {
	f := NewFinMind("")

	if _, err := f.GetFundamentals("AAPL"); err != errNotTWTicker {
		t.Errorf("GetFundamentals(AAPL) error = %v, want errNotTWTicker", err)
	}
	if _, err := f.GetFinancialStatements("AAPL", "quarterly"); err != errNotTWTicker {
		t.Errorf("GetFinancialStatements(AAPL) error = %v, want errNotTWTicker", err)
	}
}

// finmindServer serves recorded 2330 (TSMC) responses — live-curled from
// api.finmindtrade.com on 2026-07-24 (see docs/phase-6-tw-market.md's PR3
// completion notes) and trimmed to the rows each test needs — keyed by the
// dataset query param, same request-shape mock as yahoo_test.go's
// twChartServer.
func finmindServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("dataset") {
		case "TaiwanStockPER":
			w.Write([]byte(`{"msg":"success","status":200,"data":[
				{"date":"2026-07-22","stock_id":"2330","dividend_yield":0.92,"PER":32.27,"PBR":10.56},
				{"date":"2026-07-23","stock_id":"2330","dividend_yield":0.91,"PER":32.33,"PBR":10.59}
			]}`))
		case "TaiwanStockFinancialStatements":
			w.Write([]byte(`{"msg":"success","status":200,"data":[
				{"date":"2025-12-31","stock_id":"2330","type":"Revenue","value":900000000000.0,"origin_name":"營業收入"},
				{"date":"2025-12-31","stock_id":"2330","type":"GrossProfit","value":500000000000.0,"origin_name":"營業毛利"},
				{"date":"2025-12-31","stock_id":"2330","type":"OperatingIncome","value":450000000000.0,"origin_name":"營業利益"},
				{"date":"2025-12-31","stock_id":"2330","type":"IncomeAfterTaxes","value":400000000000.0,"origin_name":"本期淨利"},
				{"date":"2025-12-31","stock_id":"2330","type":"EPS","value":15.0,"origin_name":"基本每股盈餘"},
				{"date":"2026-03-31","stock_id":"2330","type":"Revenue","value":1134103440000.0,"origin_name":"營業收入"},
				{"date":"2026-03-31","stock_id":"2330","type":"GrossProfit","value":751295421000.0,"origin_name":"營業毛利"},
				{"date":"2026-03-31","stock_id":"2330","type":"OperatingIncome","value":658966142000.0,"origin_name":"營業利益"},
				{"date":"2026-03-31","stock_id":"2330","type":"IncomeAfterTaxes","value":572801304000.0,"origin_name":"本期淨利"},
				{"date":"2026-03-31","stock_id":"2330","type":"EPS","value":22.08,"origin_name":"基本每股盈餘"}
			]}`))
		case "TaiwanStockMonthRevenue":
			w.Write([]byte(`{"msg":"success","status":200,"data":[
				{"date":"2025-07-01","stock_id":"2330","country":"Taiwan","revenue":263708978000,"revenue_month":6,"revenue_year":2025,"create_time":"2025-07-10"},
				{"date":"2026-07-01","stock_id":"2330","country":"Taiwan","revenue":442679969000,"revenue_month":6,"revenue_year":2026,"create_time":"2026-07-13"}
			]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFinMindGetFundamentals(t *testing.T) {
	srv := finmindServer(t)
	defer srv.Close()

	f := NewFinMind("")
	f.baseURL = srv.URL

	fd, err := f.GetFundamentals("2330")
	if err != nil {
		t.Fatalf("GetFundamentals: %v", err)
	}
	if fd.PE != 32.33 || fd.PB != 10.59 {
		t.Errorf("PE/PB = %v/%v, want the latest (2026-07-23) row 32.33/10.59", fd.PE, fd.PB)
	}
	if fd.DividendYieldPct != 0.91 {
		t.Errorf("DividendYieldPct = %v, want 0.91", fd.DividendYieldPct)
	}
	if fd.EPS != 22.08 {
		t.Errorf("EPS = %v, want the latest quarter's 22.08", fd.EPS)
	}
	wantMargin := 751295421000.0 / 1134103440000.0 * 100
	if fd.GrossMarginPct < wantMargin-0.01 || fd.GrossMarginPct > wantMargin+0.01 {
		t.Errorf("GrossMarginPct = %v, want ~%v", fd.GrossMarginPct, wantMargin)
	}
	wantYoY := (442679969000.0 - 263708978000.0) / 263708978000.0 * 100
	if fd.MonthRevenueYoYPct < wantYoY-0.01 || fd.MonthRevenueYoYPct > wantYoY+0.01 {
		t.Errorf("MonthRevenueYoYPct = %v, want ~%v", fd.MonthRevenueYoYPct, wantYoY)
	}
	// Fields FinMind's free datasets don't carry stay at the zero value.
	if fd.ROE != 0 || fd.Beta != 0 {
		t.Errorf("ROE/Beta = %v/%v, want 0 (not available from FinMind)", fd.ROE, fd.Beta)
	}
}

func TestFinMindGetFinancialStatements(t *testing.T) {
	srv := finmindServer(t)
	defer srv.Close()

	f := NewFinMind("")
	f.baseURL = srv.URL

	st, err := f.GetFinancialStatements("2330", "quarterly")
	if err != nil {
		t.Fatalf("GetFinancialStatements: %v", err)
	}
	if st.PeriodEnd != "2026-03-31" || st.FiscalYear != 2026 || st.Form != "Q1" {
		t.Errorf("PeriodEnd/FiscalYear/Form = %q/%d/%q, want 2026-03-31/2026/Q1", st.PeriodEnd, st.FiscalYear, st.Form)
	}
	if st.Revenue != 1134103440000.0 || st.DilutedEPS != 22.08 {
		t.Errorf("Revenue/DilutedEPS = %v/%v, want the latest (2026-03-31) quarter's values", st.Revenue, st.DilutedEPS)
	}
	// No balance sheet / cash flow data in this dataset — must stay 0.
	if st.TotalAssets != 0 || st.OperatingCashFlow != 0 {
		t.Errorf("TotalAssets/OperatingCashFlow = %v/%v, want 0 (dataset has no such fields)", st.TotalAssets, st.OperatingCashFlow)
	}
}

func TestQuarterLabel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2026-03-31", "Q1"},
		{"2026-06-30", "Q2"},
		{"2026-09-30", "Q3"},
		{"2025-12-31", "Q4"},
		{"not-a-date", "TW"},
	}
	for _, tt := range tests {
		if got := quarterLabel(tt.in); got != tt.want {
			t.Errorf("quarterLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

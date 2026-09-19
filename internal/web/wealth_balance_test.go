package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"argus/internal/db"
)

func newWealthBalanceTestServer(fake *fakeDB, wealthDB *fakeWealthDB, password string) *Server {
	s := &Server{db: fake, wealthDB: wealthDB, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}, password: password}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/wealth/balance", s.handleWealthBalance)
	s.mux.HandleFunc("GET /api/wealth/debt-payoff", s.handleWealthDebtPayoff)
	s.mux.HandleFunc("GET /api/wealth/profile", s.handleWealthProfileGet)
	s.mux.HandleFunc("POST /api/wealth/profile", s.requireWritable(s.requireAuth(s.handleWealthProfileUpdate)))
	return s
}

func TestHandleWealthBalanceGroupsAndTotals(t *testing.T) {
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(100000)},
		{Asset: db.Asset{ID: 2, Side: "liability", Type: "loan", Name: "房貸", AssetGroup: "hard", Currency: "TWD"}, Value: floatPtr(40000)},
		{Asset: db.Asset{ID: 3, Side: "asset", Type: "other", Name: "未估價", AssetGroup: "hard", Currency: "TWD"}}, // no snapshot, must not fabricate
	}}
	s := newWealthBalanceTestServer(fake, &fakeWealthDB{}, "")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/balance", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got balanceSheetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TotalAssets == nil || *got.TotalAssets != 100000 {
		t.Errorf("TotalAssets = %v, want 100000 (unvalued asset contributes 0)", got.TotalAssets)
	}
	if got.TotalLiabilities == nil || *got.TotalLiabilities != 40000 {
		t.Errorf("TotalLiabilities = %v, want 40000", got.TotalLiabilities)
	}
	if got.NetWorth == nil || *got.NetWorth != 60000 {
		t.Errorf("NetWorth = %v, want 60000", got.NetWorth)
	}
	if got.DebtRatioPct == nil || *got.DebtRatioPct != 40 {
		t.Errorf("DebtRatioPct = %v, want 40 (40000/100000)", got.DebtRatioPct)
	}
	if len(got.Liabilities) != 1 || got.Liabilities[0].Name != "房貸" {
		t.Errorf("Liabilities = %+v, want one row 房貸", got.Liabilities)
	}
	if got.MonthlySalary != nil {
		t.Errorf("MonthlySalary = %v, want nil (profile.annual_salary not set)", got.MonthlySalary)
	}
	if got.SavingsRatePct != nil || got.LiquidityMonths != nil {
		t.Errorf("SavingsRatePct=%v LiquidityMonths=%v, want both nil without a salary", got.SavingsRatePct, got.LiquidityMonths)
	}
	if len(got.QuarterlyTrend) != quarterlyTrendQuarters {
		t.Errorf("len(QuarterlyTrend) = %d, want %d", len(got.QuarterlyTrend), quarterlyTrendQuarters)
	}
}

// TestHandleWealthBalanceNoLiabilitiesRendersEmptySlice pins the
// nil-vs-empty-slice convention (same as wealth_goals_test.go's
// TestHandleWealthGoalsListEmptyRendersEmptySlices) for the one field on this
// endpoint that used to violate it: with zero liability-type assets, the
// loop that builds `liabilities` never appends anything, so an
// un-initialized `var liabilities []liabilityDetail` serializes as JSON
// `null` — which crashed the frontend's `sheet.liabilities.length` on a
// real account with no debt at all (live-verified on production 2026-09-19).
func TestHandleWealthBalanceNoLiabilitiesRendersEmptySlice(t *testing.T) {
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(100000)},
	}}
	s := newWealthBalanceTestServer(fake, &fakeWealthDB{}, "")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/balance", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"liabilities":null`) {
		t.Errorf("liabilities serialized as null, want []: %s", rec.Body.String())
	}
	var got balanceSheetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Liabilities == nil || len(got.Liabilities) != 0 {
		t.Errorf("Liabilities = %#v, want non-nil empty slice", got.Liabilities)
	}
}

func TestHandleWealthBalanceHealthMetricsWithSalaryAndDepositHistory(t *testing.T) {
	monthAgo := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(150000)},
		},
		wealthAssetsAsOf: map[string][]db.AssetWithValue{
			monthAgo: {{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(100000)}},
		},
		settings: map[string]string{annualSalarySettingKey: "720000"}, // 60000/month
	}
	s := newWealthBalanceTestServer(fake, &fakeWealthDB{}, "")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/balance", nil))
	var got balanceSheetResponse
	json.Unmarshal(rec.Body.Bytes(), &got)

	if got.MonthlySalary == nil || *got.MonthlySalary != 60000 {
		t.Fatalf("MonthlySalary = %v, want 60000", got.MonthlySalary)
	}
	// expense = salary(60000) - (depositNow(150000) - depositPrev(100000)) = 10000
	if got.SavingsRatePct == nil {
		t.Fatal("SavingsRatePct = nil, want a value (salary + two deposit points are both present)")
	}
	wantSavings := (60000.0 - 10000.0) / 60000.0 * 100
	if diff := *got.SavingsRatePct - wantSavings; diff < -1e-6 || diff > 1e-6 {
		t.Errorf("SavingsRatePct = %v, want %v", *got.SavingsRatePct, wantSavings)
	}
	if got.LiquidityMonths == nil {
		t.Fatal("LiquidityMonths = nil, want a value")
	}
	wantLiquidity := 150000.0 / 10000.0
	if *got.LiquidityMonths != wantLiquidity {
		t.Errorf("LiquidityMonths = %v, want %v", *got.LiquidityMonths, wantLiquidity)
	}
}

// TestHandleWealthBalanceColdStartDepositDegrades pins §8.17.1's rule: a
// deposit asset that exists today but has no snapshot a month ago (e.g. it
// was only just created) must not be read as "balance was 0 a month ago" —
// that would manufacture a huge fake expense/income swing. The ratios that
// depend on it must degrade to nil instead.
func TestHandleWealthBalanceColdStartDepositDegrades(t *testing.T) {
	monthAgo := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	depositAsset := db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid", Currency: "TWD"}
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{{Asset: depositAsset, Value: floatPtr(300000)}},
		wealthAssetsAsOf: map[string][]db.AssetWithValue{
			// mirrors what ListAssetsValueAsOf's real LEFT JOIN returns for a
			// date before the asset's first snapshot: the spine row is still
			// there, Value is just nil — not "the row doesn't exist."
			monthAgo: {{Asset: depositAsset}},
		},
		settings: map[string]string{annualSalarySettingKey: "900000"},
	}
	s := newWealthBalanceTestServer(fake, &fakeWealthDB{}, "")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/balance", nil))
	var got balanceSheetResponse
	json.Unmarshal(rec.Body.Bytes(), &got)

	if got.MonthlySalary == nil || *got.MonthlySalary != 75000 {
		t.Fatalf("MonthlySalary = %v, want 75000", got.MonthlySalary)
	}
	if got.SavingsRatePct != nil || got.ExpenseRatioPct != nil || got.LiquidityMonths != nil {
		t.Errorf("SavingsRatePct=%v ExpenseRatioPct=%v LiquidityMonths=%v, want all nil (no deposit history a month back)",
			got.SavingsRatePct, got.ExpenseRatioPct, got.LiquidityMonths)
	}
}

func TestHandleWealthProfileGetAndUpdate(t *testing.T) {
	fake := &fakeDB{}
	wealthDB := &fakeWealthDB{}
	s := newWealthBalanceTestServer(fake, wealthDB, "secret")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/profile", nil))
	var got wealthProfileResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.AnnualSalary != nil {
		t.Errorf("AnnualSalary = %v, want nil (never set)", got.AnnualSalary)
	}

	cookie := loginAndGetCookie(t, s, "secret")
	body, _ := json.Marshal(wealthProfileUpdateRequest{AnnualSalary: floatPtr(900000)})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/profile", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastSettingKey != annualSalarySettingKey || wealthDB.lastSettingValue != "900000.00" {
		t.Errorf("SetSetting() got key=%q value=%q", wealthDB.lastSettingKey, wealthDB.lastSettingValue)
	}
}

func TestHandleWealthProfileUpdateRequiresAuth(t *testing.T) {
	s := newWealthBalanceTestServer(&fakeDB{}, &fakeWealthDB{}, "secret")
	body, _ := json.Marshal(wealthProfileUpdateRequest{AnnualSalary: floatPtr(100)})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/profile", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (no auth cookie)", rec.Code)
	}
}

// TestHandleWealthDebtPayoffAvalancheNeverWorse mirrors the pure-function
// test in internal/assets but through the HTTP handler, pinning that a loan
// missing rate/term is silently excluded rather than guessed at.
func TestHandleWealthDebtPayoffAvalancheNeverWorse(t *testing.T) {
	rate5, rate20 := 5.0, 20.0
	months12, months60 := int64(12), int64(60)
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "liability", Type: "loan", Name: "small"}, Value: floatPtr(500)},
			{Asset: db.Asset{ID: 2, Side: "liability", Type: "loan", Name: "big"}, Value: floatPtr(5000)},
			{Asset: db.Asset{ID: 3, Side: "liability", Type: "loan", Name: "no-rate"}, Value: floatPtr(1000)}, // excluded: no loan_details
			{Asset: db.Asset{ID: 4, Side: "asset", Type: "deposit", Name: "not-a-liability"}, Value: floatPtr(9999)},
		},
		loanDetails: map[int64]*db.LoanDetails{
			1: {RatePct: &rate5, RemainingMonths: &months12},
			2: {RatePct: &rate20, RemainingMonths: &months60},
		},
	}
	s := newWealthBalanceTestServer(fake, &fakeWealthDB{}, "")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/debt-payoff?extra=100", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got debtPayoffResponse
	json.Unmarshal(rec.Body.Bytes(), &got)

	if len(got.Loans) != 2 {
		t.Fatalf("Loans = %v, want exactly [small big] (no-rate/asset excluded)", got.Loans)
	}
	if got.Avalanche.TotalInterest > got.Snowball.TotalInterest+1e-6 {
		t.Errorf("avalanche interest %v > snowball interest %v", got.Avalanche.TotalInterest, got.Snowball.TotalInterest)
	}
	if got.InterestDifference < 0 {
		t.Errorf("InterestDifference = %v, want >= 0", got.InterestDifference)
	}
}

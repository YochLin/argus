package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"argus/internal/db"
	"argus/internal/market"
	"argus/internal/service"
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

// TestHandleWealthBalanceFilesEquityUnderGrowth pins that the US/TW equity
// virtual rows (net-worth snapshots, never real assets rows — §9.1 rule 3)
// sit in the growth group on the balance sheet.
func TestHandleWealthBalanceFilesEquityUnderGrowth(t *testing.T) {
	fake := &fakeDB{netWorthOnOrBeforeFn: func(_ string, m market.MarketID) (float64, bool, error) {
		return 5000, m == market.TW, nil
	}}
	s := newWealthBalanceTestServer(fake, &fakeWealthDB{}, "")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/balance", nil))
	var got balanceSheetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body %s)", err, rec.Body.String())
	}
	for _, g := range got.AssetGroups {
		if g.Group != "growth" {
			if len(g.Assets) != 0 {
				t.Errorf("%s holds %+v, want only growth to hold the equity row", g.Group, g.Assets)
			}
			continue
		}
		if g.MarketValue != 5000 || len(g.Assets) != 1 || g.Assets[0].Type != "equity_tw" {
			t.Errorf("growth = %+v, want the one equity_tw row worth 5000", g)
		}
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
	if len(got.LiabGroups) != 2 || got.LiabGroups[0].Kind != "short" || got.LiabGroups[1].Kind != "long" {
		t.Errorf("LiabGroups = %+v, want [short, long] both present (even empty)", got.LiabGroups)
	}
}

// TestBuildLiabGroups pins the short/long split (over longTermRemainingMonths
// of term left is the only "long" case; a term-less loan and a credit card
// both default short), the value-desc sort within each group, and the
// %-of-total column — buildLiabGroups is pure so this exercises it directly
// rather than through the full handler.
func TestBuildLiabGroups(t *testing.T) {
	months264, months36 := int64(264), int64(36)
	liabs := []liabilityDetail{
		{Name: "信用卡", Type: "credit_card", ValueTWD: 20000},
		{Name: "房貸", Type: "loan", RemainingMonths: &months264, ValueTWD: 6000000},
		{Name: "車貸", Type: "loan", RemainingMonths: &months36, ValueTWD: 300000},
	}
	got := buildLiabGroups(liabs, 6320000)
	if len(got) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(got))
	}
	short, long := got[0], got[1]
	if short.Kind != "short" || long.Kind != "long" {
		t.Fatalf("kinds = %s, %s, want short, long", short.Kind, long.Kind)
	}
	if len(short.Liabilities) != 2 || short.Liabilities[0].Name != "車貸" || short.Liabilities[1].Name != "信用卡" {
		t.Errorf("short.Liabilities = %+v, want [車貸, 信用卡] (value desc)", short.Liabilities)
	}
	if short.MarketValue != 320000 {
		t.Errorf("short.MarketValue = %v, want 320000", short.MarketValue)
	}
	if len(long.Liabilities) != 1 || long.Liabilities[0].Name != "房貸" {
		t.Errorf("long.Liabilities = %+v, want [房貸]", long.Liabilities)
	}
	if long.PctOfLiab == nil || math.Abs(*long.PctOfLiab-94.9367) > 0.01 {
		t.Errorf("long.PctOfLiab = %v, want ~94.94%%", long.PctOfLiab)
	}
}

// TestBuildLiabGroupsAlwaysBothKinds pins the "always both, even empty"
// contract the frontend relies on to render two headers unconditionally.
func TestBuildLiabGroupsAlwaysBothKinds(t *testing.T) {
	got := buildLiabGroups(nil, 0)
	if len(got) != 2 || got[0].Liabilities == nil || got[1].Liabilities == nil {
		t.Errorf("buildLiabGroups(nil, 0) = %+v, want two groups with non-nil empty Liabilities", got)
	}
	if got[0].PctOfLiab != nil || got[1].PctOfLiab != nil {
		t.Errorf("PctOfLiab = %v, %v, want nil when totalLiabilities is 0", got[0].PctOfLiab, got[1].PctOfLiab)
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
		settings: map[string]string{service.AnnualSalarySettingKey: "720000"}, // 60000/month
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
		settings: map[string]string{service.AnnualSalarySettingKey: "900000"},
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
	var got service.WealthProfile
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
	if wealthDB.lastSettingKey != service.AnnualSalarySettingKey || wealthDB.lastSettingValue != "900000.00" {
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

// TestBucketAssets pins the FX-conversion/grouping core /w/balance and
// /w/alloc share: an asset in a bucket nobody asked for still counts toward
// the total, liabilities never do, the US/TW equity rows land in the
// equity bucket, and an asset whose currency can't be priced clears fxOK
// on its own (no equity row there to do it for it).
func TestBucketAssets(t *testing.T) {
	today := "2026-10-11"
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	keys := []string{"liquid", "growth"}
	byGroup := func(a db.AssetWithValue) string { return a.AssetGroup }

	s := &Server{quotes: &fakeQuotes{}, fxDB: &fakeFXDB{rates: map[string]float64{fxKey(today, "USDTWD"): 32}}}
	list := []db.AssetWithValue{
		{Asset: db.Asset{ID: 1, Type: "deposit", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(100)},
		{Asset: db.Asset{ID: 2, Type: "gold", AssetGroup: "elsewhere", Currency: "TWD"}, Value: floatPtr(50)},
		{Asset: db.Asset{ID: 3, Side: "liability", Type: "loan", AssetGroup: "liquid", Currency: "TWD"}, Value: floatPtr(999)},
	}
	buckets, byCurrency, total, fxOK := s.bucketAssets(list, today, now, 10, true, 1000, true, keys, byGroup, "growth")
	if !fxOK || total != 100+50+10*32+1000 {
		t.Errorf("fxOK = %v, total = %v; want true, 1470 (the unbucketed 50 counts, the liability doesn't)", fxOK, total)
	}
	if byCurrency["TWD"] != 1150 || byCurrency["USD"] != 320 {
		t.Errorf("byCurrency = %v, want TWD 1150 and USD 320 (TWD-valued)", byCurrency)
	}
	if b := buckets["liquid"]; b.MarketValue != 100 || len(b.Assets) != 1 || b.Assets[0].AssetID != 1 {
		t.Errorf("liquid = %+v, want just asset 1 at 100", b)
	}
	growth := buckets["growth"]
	if growth.MarketValue != 1320 || len(growth.Assets) != 2 {
		t.Fatalf("growth = %+v, want the two equity rows worth 1320", growth)
	}
	types := map[string]bool{}
	for _, it := range growth.Assets {
		types[it.Type] = it.Category == "equity" && it.Source == "sync"
	}
	if !types["equity_us"] || !types["equity_tw"] {
		t.Errorf("growth rows = %+v, want an equity_us and an equity_tw row", growth.Assets)
	}

	broke := &Server{quotes: &fakeQuotes{err: map[string]error{"USDTWD=X": errors.New("no fx")}}, fxDB: &fakeFXDB{}}
	usd := []db.AssetWithValue{{Asset: db.Asset{ID: 4, Type: "deposit", AssetGroup: "liquid", Currency: "USD"}, Value: floatPtr(10)}}
	buckets, _, total, fxOK = broke.bucketAssets(usd, today, now, 0, false, 0, false, keys, byGroup, "growth")
	if fxOK || total != 0 || len(buckets["liquid"].Assets) != 0 {
		t.Errorf("unpriceable USD asset: fxOK = %v, total = %v, liquid = %+v; want false, 0, empty", fxOK, total, buckets["liquid"])
	}
}

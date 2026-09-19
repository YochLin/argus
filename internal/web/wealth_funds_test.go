package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"argus/internal/db"
)

func newWealthFundsTestServer(fake *fakeDB) *Server {
	s := &Server{db: fake, password: "", quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/wealth/funds", s.handleWealthFundsGet)
	return s
}

// TestHandleWealthFundsGetBasic pins the KPI row and per-row math for two
// TWD funds with full cost data, plus the schedule-merge behavior when both
// funds share the same NextContributionDate.
func TestHandleWealthFundsGetBasic(t *testing.T) {
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "fund", Name: "元大台灣50"}, Value: floatPtr(640000), Cost: floatPtr(520000)},
			{Asset: db.Asset{ID: 2, Side: "asset", Type: "fund", Name: "國泰永續高股息"}, Value: floatPtr(398000), Cost: floatPtr(356000)},
		},
		fundDetails: map[int64]*db.FundDetails{
			1: {Code: "0050", Platform: "券商", MonthlyAmount: floatPtr(12000), NextContributionDate: "2026-10-06"},
			2: {Code: "00878", Platform: "券商", MonthlyAmount: floatPtr(8000), NextContributionDate: "2026-10-06"},
		},
	}
	s := newWealthFundsTestServer(fake)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/funds", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var resp wealthFundsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.MarketValue == nil || *resp.MarketValue != 1038000 {
		t.Errorf("MarketValue = %v, want 1038000", resp.MarketValue)
	}
	if resp.Cost == nil || *resp.Cost != 876000 {
		t.Errorf("Cost = %v, want 876000", resp.Cost)
	}
	if resp.PnL == nil || *resp.PnL != 162000 {
		t.Errorf("PnL = %v, want 162000", resp.PnL)
	}
	if resp.ReturnPct == nil {
		t.Fatal("ReturnPct = nil, want a value")
	}
	if got, want := *resp.ReturnPct, 162000.0/876000.0*100; got < want-0.01 || got > want+0.01 {
		t.Errorf("ReturnPct = %v, want %v", got, want)
	}
	if resp.MonthlyTotal != 20000 {
		t.Errorf("MonthlyTotal = %v, want 20000", resp.MonthlyTotal)
	}

	if len(resp.Schedule) != 1 {
		t.Fatalf("len(Schedule) = %d, want 1 (both funds share the same next date)", len(resp.Schedule))
	}
	if resp.Schedule[0].Date != "2026-10-06" || resp.Schedule[0].Amount != 20000 {
		t.Errorf("Schedule[0] = %+v, want date=2026-10-06 amount=20000", resp.Schedule[0])
	}
	if resp.Schedule[0].Names != "元大台灣50 · 國泰永續高股息" {
		t.Errorf("Schedule[0].Names = %q, want joined names", resp.Schedule[0].Names)
	}

	if len(resp.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(resp.Rows))
	}
	if len(resp.Chart) != 24 {
		t.Fatalf("len(Chart) = %d, want 24", len(resp.Chart))
	}
	if resp.Chart[23].Date != time.Now().Format("2006-01-02") {
		t.Errorf("Chart[23].Date = %q, want today", resp.Chart[23].Date)
	}
}

// TestHandleWealthFundsGetStoppedFund pins the "已停扣" degrade: a fund with
// no MonthlyAmount (lump-sum-only) still contributes to MV/cost/pnl but is
// excluded from MonthlyTotal and the schedule.
func TestHandleWealthFundsGetStoppedFund(t *testing.T) {
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "fund", Name: "富邦科技（單筆）"}, Value: floatPtr(411000), Cost: floatPtr(300000)},
		},
		fundDetails: map[int64]*db.FundDetails{
			1: {Code: "0052", Platform: "券商"},
		},
	}
	s := newWealthFundsTestServer(fake)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/funds", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var resp wealthFundsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.MonthlyTotal != 0 {
		t.Errorf("MonthlyTotal = %v, want 0 (stopped)", resp.MonthlyTotal)
	}
	if len(resp.Schedule) != 0 {
		t.Errorf("len(Schedule) = %d, want 0 (stopped fund excluded)", len(resp.Schedule))
	}
	if len(resp.Rows) != 1 || resp.Rows[0].MonthlyAmount != nil {
		t.Errorf("Rows[0].MonthlyAmount = %v, want nil", resp.Rows[0].MonthlyAmount)
	}
	if resp.Rows[0].Value == nil || *resp.Rows[0].Value != 411000 {
		t.Errorf("Rows[0].Value = %v, want 411000", resp.Rows[0].Value)
	}
}

// TestHandleWealthFundsGetUnpriceableCurrencyDegrades pins §8.17.1's
// whole-metric rule for the KPI row: a fund whose currency can't be priced
// to TWD nulls out the entire KPI row (MarketValue included, not just
// Cost/PnL/ReturnPct — see wealthFundsResponse's doc comment), while that
// same fund's own table row independently shows nil for the figures it
// couldn't compute — a per-row degrade, not a page-wide failure.
func TestHandleWealthFundsGetUnpriceableCurrencyDegrades(t *testing.T) {
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "fund", Name: "Vanguard Total World", Currency: "USD"}, Value: floatPtr(16000), Cost: floatPtr(13000)},
		},
		fundDetails: map[int64]*db.FundDetails{1: {Code: "VT"}},
	}
	s := newWealthFundsTestServer(fake)
	s.quotes = &fakeQuotes{err: map[string]error{"USDTWD=X": errors.New("no quote")}}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/funds", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var resp wealthFundsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Cost != nil {
		t.Errorf("Cost = %v, want nil (USD unpriceable — fakeQuotes has no rate)", resp.Cost)
	}
	if resp.PnL != nil {
		t.Errorf("PnL = %v, want nil", resp.PnL)
	}
	if resp.MarketValue != nil {
		t.Errorf("MarketValue = %v, want nil (the one fund couldn't be priced)", resp.MarketValue)
	}
	if len(resp.Rows) != 1 || resp.Rows[0].Value != nil {
		t.Errorf("Rows[0].Value = %v, want nil (same unpriceable currency)", resp.Rows[0].Value)
	}
}

func TestFundsChartDatesLength(t *testing.T) {
	today := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	dates := fundsChartDates(today)
	if len(dates) != 24 {
		t.Fatalf("len(dates) = %d, want 24", len(dates))
	}
	if dates[23] != "2026-09-19" {
		t.Errorf("dates[23] = %q, want 2026-09-19 (today, not the current month's last day)", dates[23])
	}
	if dates[22] != "2026-08-31" {
		t.Errorf("dates[22] = %q, want 2026-08-31 (last full month's last day)", dates[22])
	}
	if dates[0] != "2024-10-31" {
		t.Errorf("dates[0] = %q, want 2024-10-31 (23 months before September 2026)", dates[0])
	}
}

func TestMergeFundSchedule(t *testing.T) {
	items := []fundScheduleItem{
		{Date: "2026-09-20", Names: "AGHY", Amount: 4000},
		{Date: "2026-09-06", Names: "0050", Amount: 12000},
		{Date: "2026-09-06", Names: "00878", Amount: 8000},
	}
	merged := mergeFundSchedule(items)
	if len(merged) != 2 {
		t.Fatalf("len(merged) = %d, want 2", len(merged))
	}
	if merged[0].Date != "2026-09-06" || merged[0].Amount != 20000 || merged[0].Names != "0050 · 00878" {
		t.Errorf("merged[0] = %+v, want date=2026-09-06 amount=20000 names=\"0050 · 00878\"", merged[0])
	}
	if merged[1].Date != "2026-09-20" {
		t.Errorf("merged[1].Date = %q, want 2026-09-20 (sorted after 09-06)", merged[1].Date)
	}
}

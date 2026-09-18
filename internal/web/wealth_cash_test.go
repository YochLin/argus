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

func newWealthCashTestServer(password string, fake *fakeDB, wealthDB wealthWriter) *Server {
	s := &Server{db: fake, wealthDB: wealthDB, password: password, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/wealth/cash", s.handleWealthCashList)
	s.mux.HandleFunc("POST /api/wealth/cash", s.requireWritable(s.requireAuth(s.handleWealthCashCreate)))
	s.mux.HandleFunc("POST /api/wealth/cash/deactivate", s.requireWritable(s.requireAuth(s.handleWealthCashDeactivate)))
	return s
}

func int64p(v int64) *int64 { return &v }

// TestHandleWealthCashListEmptyRendersEmptySlices pins the same convention
// wealth_alloc_test.go's TestHandleWealthAllocNoDataRendersEmpty does: with
// no recurring flows and no option positions, events must serialize as [],
// not null — a nil Go slice round-trips through json.Unmarshal identically
// to an empty one, so only the raw body catches a frontend `.length`-on-null
// crash (this exact bug: recurringCashflowEvents/optionExpiryEvents return a
// nil slice when they append nothing).
func TestHandleWealthCashListEmptyRendersEmptySlices(t *testing.T) {
	s := newWealthCashTestServer("", &fakeDB{}, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/cash", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, field := range []string{"items", "events"} {
		if strings.Contains(body, `"`+field+`":null`) {
			t.Errorf("%s serialized as null, want []: %s", field, body)
		}
	}
}

// TestHandleWealthCashList pins §8.4/§9.4 PR5: active flows sum into a
// monthly total, a paused (inactive) flow still shows in the item list but
// doesn't count toward the total, and a day_of_month flow produces a 90-day
// event with its linked asset's venue attached.
func TestHandleWealthCashList(t *testing.T) {
	today := time.Now()
	dueDay := int64(today.Day())

	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "fund", Name: "0050 定期定額", Venue: "國泰證券", Currency: "TWD"}},
		},
		recurringCashflows: []db.RecurringCashflow{
			{ID: 1, Direction: "in", Name: "薪資", Amount: 80000, Currency: "TWD", Category: "salary", Active: true},
			{ID: 2, Direction: "out", Name: "房貸", Amount: 35000, Currency: "TWD", Category: "mortgage", Active: true, DayOfMonth: &dueDay},
			{ID: 3, Direction: "out", Name: "0050 定期定額", Amount: 15000, Currency: "TWD", Category: "sip", Active: true, DayOfMonth: &dueDay, AssetID: int64p(1)},
			{ID: 4, Direction: "out", Name: "已停用的訂閱", Amount: 500, Currency: "TWD", Category: "living", Active: false},
		},
	}
	s := newWealthCashTestServer("", fake, &fakeWealthDB{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/cash", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got cashResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Items) != 4 {
		t.Fatalf("len(Items) = %d, want 4 (including the paused one)", len(got.Items))
	}
	if got.MonthlyIn == nil || *got.MonthlyIn != 80000 {
		t.Errorf("MonthlyIn = %v, want 80000", got.MonthlyIn)
	}
	// 房貸(35000) + 0050 定期定額(15000); the paused 已停用的訂閱 must not count.
	if got.MonthlyOut == nil || *got.MonthlyOut != 50000 {
		t.Errorf("MonthlyOut = %v, want 50000 (paused flow excluded)", got.MonthlyOut)
	}
	if got.MonthlyNet == nil || *got.MonthlyNet != 30000 {
		t.Errorf("MonthlyNet = %v, want 30000", got.MonthlyNet)
	}
	// SaveRatePct/DcaSharePct/FixedSharePct are all percentages of MonthlyIn
	// (80000): net 30000 -> 37.5%, sip 15000 -> 18.75%, mortgage 35000 (a
	// "fixed" category) -> 43.75%.
	if got.SaveRatePct == nil || *got.SaveRatePct != 37.5 {
		t.Errorf("SaveRatePct = %v, want 37.5", got.SaveRatePct)
	}
	if got.DcaSharePct == nil || *got.DcaSharePct != 18.75 {
		t.Errorf("DcaSharePct = %v, want 18.75", got.DcaSharePct)
	}
	if got.FixedSharePct == nil || *got.FixedSharePct != 43.75 {
		t.Errorf("FixedSharePct = %v, want 43.75", got.FixedSharePct)
	}
	if got.AnnualNet == nil || *got.AnnualNet != 360000 {
		t.Errorf("AnnualNet = %v, want 360000 (flat MonthlyNet*12)", got.AnnualNet)
	}
	// Each active item's ValueTwd is its own TWD amount; the paused item
	// (id 4) has none since it's excluded from every total.
	byID := map[int64]cashflowItem{}
	for _, it := range got.Items {
		byID[it.ID] = it
	}
	if v := byID[1].ValueTwd; v == nil || *v != 80000 {
		t.Errorf("item 1 ValueTwd = %v, want 80000", v)
	}
	if v := byID[4].ValueTwd; v != nil {
		t.Errorf("paused item 4 ValueTwd = %v, want nil", v)
	}
	// EventsNet sums the (TWD, all-in-out here) event amounts with sign —
	// checked against the events list itself rather than a hardcoded number,
	// since how many monthly occurrences land within the 90-day window
	// depends on today's date.
	var wantEventsNet float64
	for _, e := range got.Events {
		if e.Amount == nil {
			continue
		}
		if e.Direction == "out" {
			wantEventsNet -= *e.Amount
		} else {
			wantEventsNet += *e.Amount
		}
	}
	if got.EventsNet == nil || *got.EventsNet != wantEventsNet {
		t.Errorf("EventsNet = %v, want %v", got.EventsNet, wantEventsNet)
	}

	// Both day_of_month flows fall due today, so at least one occurrence of
	// each shows up in the 90-day event table, and the asset-linked one
	// carries its asset's venue.
	var sawMortgage, sawSIPWithVenue bool
	for _, e := range got.Events {
		if e.Item == "房貸" {
			sawMortgage = true
		}
		if e.Item == "0050 定期定額" && e.Venue == "國泰證券" {
			sawSIPWithVenue = true
		}
	}
	if !sawMortgage {
		t.Errorf("Events = %+v, want a 房貸 occurrence", got.Events)
	}
	if !sawSIPWithVenue {
		t.Errorf("Events = %+v, want a 0050 定期定額 occurrence with venue 國泰證券", got.Events)
	}
}

// TestHandleWealthCashCreateAndDeactivate pins the write paths: create
// requires a valid direction/name/amount, and deactivate is the only way to
// remove a flow from the active total (no hard delete, §9.1's soft-delete
// convention applied to recurring_cashflows).
func TestHandleWealthCashCreateAndDeactivate(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthCashTestServer("secret", &fakeDB{}, wealthDB)
	cookie := loginAndGetCookie(t, s, "secret")

	body, _ := json.Marshal(map[string]any{
		"direction": "out", "name": "保費", "amount": 3000, "currency": "TWD", "dayOfMonth": 5, "category": "insurance",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/cash", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastNewCashflow.Name != "保費" || wealthDB.lastNewCashflow.Direction != "out" || wealthDB.lastNewCashflow.Amount != 3000 {
		t.Errorf("lastNewCashflow = %+v, want 保費/out/3000", wealthDB.lastNewCashflow)
	}

	// Missing/invalid direction is rejected before it reaches the DB.
	badBody, _ := json.Marshal(map[string]any{"direction": "sideways", "name": "x", "amount": 1})
	badReq := httptest.NewRequest(http.MethodPost, "/api/wealth/cash", bytes.NewReader(badBody))
	badReq.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec2, badReq)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("bad direction status = %d, want 400", rec2.Code)
	}

	deactivateBody, _ := json.Marshal(map[string]any{"id": 7})
	deactivateReq := httptest.NewRequest(http.MethodPost, "/api/wealth/cash/deactivate", bytes.NewReader(deactivateBody))
	deactivateReq.AddCookie(cookie)
	rec3 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec3, deactivateReq)
	if rec3.Code != http.StatusOK {
		t.Fatalf("deactivate status = %d, body = %s", rec3.Code, rec3.Body.String())
	}
	if wealthDB.lastDeactivateID != 7 {
		t.Errorf("lastDeactivateID = %d, want 7", wealthDB.lastDeactivateID)
	}
}

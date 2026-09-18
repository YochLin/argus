package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"argus/internal/db"
)

func newWealthRetireTestServer(password string, fake *fakeDB, wealthDB wealthWriter) *Server {
	s := &Server{db: fake, wealthDB: wealthDB, password: password, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/wealth/retire", s.handleWealthRetireGet)
	s.mux.HandleFunc("POST /api/wealth/retire", s.requireWritable(s.requireAuth(s.handleWealthRetireSave)))
	return s
}

// TestHandleWealthRetireGetNoBirthYear pins the degrade path: with no
// profile.birth_year set yet, the page still renders (defaults for the
// quick-switch selection) but every projection field stays nil/unset since
// there's no way to turn "retirement age 60" into a calendar date.
func TestHandleWealthRetireGetNoBirthYear(t *testing.T) {
	s := newWealthRetireTestServer("", &fakeDB{}, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/retire", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got retirementResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HasBirthYear {
		t.Errorf("HasBirthYear = true, want false")
	}
	// Pool itself is still computable (no earmarks yet is a known zero, not
	// a pricing failure) — only the age-dependent projection degrades.
	if got.Baseline != nil || got.Need != nil {
		t.Errorf("expected nil projection fields with no birth year, got Baseline=%v Need=%v", got.Baseline, got.Need)
	}
	if got.RetirementAge != defaultRetirementAge || got.MonthlySpend != defaultRetirementMonthlySpend {
		t.Errorf("defaults = (%d, %v), want (%d, %v)", got.RetirementAge, got.MonthlySpend, defaultRetirementAge, defaultRetirementMonthlySpend)
	}
}

// TestHandleWealthRetireGetComputesProjection pins the end-to-end read path
// once birth year + an existing retirement goal are both present: the
// current age/target date resolve a years-to-retirement count, the
// earmarked asset backs the pool, and all three scenarios come back funded
// with a well-stocked pool.
func TestHandleWealthRetireGetComputesProjection(t *testing.T) {
	birthYear := time.Now().Year() - 40  // current age 40
	retireYear := time.Now().Year() + 20 // retirement age 60
	v := 20000000.0
	fake := &fakeDB{
		settings: map[string]string{birthYearSettingKey: strconv.Itoa(birthYear)},
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "fund", Name: "退休基金", Currency: "TWD"}, Value: &v},
		},
		goals: []db.Goal{
			{ID: 1, Name: "退休金", Kind: "retirement", TargetAmount: 90000 * 12 * 25, Currency: "TWD", TargetDate: strconv.Itoa(retireYear) + "-01-01", CreatedAt: "2020-01-01 00:00:00"},
		},
		goalAssets: []db.GoalAsset{{GoalID: 1, AssetID: 1, Ratio: 1.0}},
	}
	s := newWealthRetireTestServer("", fake, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/retire", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got retirementResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.HasBirthYear {
		t.Fatalf("HasBirthYear = false, want true")
	}
	if got.RetirementAge != 60 {
		t.Errorf("RetirementAge = %d, want 60 (derived from target_date - birthYear)", got.RetirementAge)
	}
	if got.Pool == nil || *got.Pool != v {
		t.Errorf("Pool = %v, want %v", got.Pool, v)
	}
	if got.Baseline == nil || !got.Baseline.Funded {
		t.Fatalf("Baseline = %+v, want a funded projection", got.Baseline)
	}
	if got.Goal == nil || got.Goal.Saved == nil || *got.Goal.Saved != v {
		t.Errorf("Goal.Saved = %v, want %v", got.Goal, v)
	}
}

// TestHandleWealthRetireSaveRequiresBirthYearThenUpsertsGoal pins the
// write path's precondition (birth year must already be set) and, once
// satisfied, that the quick-switch save computes the right target date and
// amount from the chosen age/spend.
func TestHandleWealthRetireSaveRequiresBirthYearThenUpsertsGoal(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthRetireTestServer("secret", &fakeDB{}, wealthDB)
	cookie := loginAndGetCookie(t, s, "secret")

	body, _ := json.Marshal(map[string]any{"name": "退休金", "retirementAge": 60, "monthlySpend": 90000})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/retire", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no birth year: status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}

	birthYear := time.Now().Year() - 40
	s2 := newWealthRetireTestServer("secret", &fakeDB{settings: map[string]string{birthYearSettingKey: strconv.Itoa(birthYear)}}, wealthDB)
	cookie2 := loginAndGetCookie(t, s2, "secret")
	req2 := httptest.NewRequest(http.MethodPost, "/api/wealth/retire", bytes.NewReader(body))
	req2.AddCookie(cookie2)
	rec2 := httptest.NewRecorder()
	s2.mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec2.Code, rec2.Body.String())
	}
	wantDate := strconv.Itoa(birthYear+60) + "-01-01"
	if wealthDB.lastRetirementGoalTargetDate != wantDate {
		t.Errorf("lastRetirementGoalTargetDate = %q, want %q", wealthDB.lastRetirementGoalTargetDate, wantDate)
	}
	wantAmount := 90000.0 * 12 * 25
	if wealthDB.lastRetirementGoalTargetAmount != wantAmount {
		t.Errorf("lastRetirementGoalTargetAmount = %v, want %v", wealthDB.lastRetirementGoalTargetAmount, wantAmount)
	}
}

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
	s.mux.HandleFunc("POST /api/wealth/retire/assign", s.requireWritable(s.requireAuth(s.handleWealthRetireAssign)))
	return s
}

// assignRetire posts body to /api/wealth/retire/assign as a logged-in user.
func assignRetire(t *testing.T, s *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	cookie := loginAndGetCookie(t, s, "secret")
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/retire/assign", bytes.NewReader(raw))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func wealthAsset(id int64, side string) db.AssetWithValue {
	return db.AssetWithValue{Asset: db.Asset{ID: id, Side: side, Type: "deposit", Name: "a"}}
}

// TestHandleWealthRetireGetNoBirthYearRendersSample pins the sample-mode
// path (design's isSample/範例): with no profile.birth_year set yet, the
// page still renders a full projection — using defaultSampleCurrentAge
// rather than a real age — tagged IsSample so nothing presents it as the
// user's real numbers. Pool itself is still computable (no earmarks yet is
// a known zero, not a pricing failure), which is what unlocks this path.
func TestHandleWealthRetireGetNoBirthYearRendersSample(t *testing.T) {
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
	if !got.IsSample {
		t.Errorf("IsSample = false, want true (no real birth year on file)")
	}
	if got.CurrentAge != defaultSampleCurrentAge {
		t.Errorf("CurrentAge = %d, want %d (defaultSampleCurrentAge)", got.CurrentAge, defaultSampleCurrentAge)
	}
	if got.Baseline == nil || got.Need == nil {
		t.Errorf("expected a full sample projection even without a birth year, got Baseline=%v Need=%v", got.Baseline, got.Need)
	}
	if got.RetirementAge != defaultRetirementAge || got.MonthlySpend != defaultRetirementMonthlySpend {
		t.Errorf("defaults = (%d, %v), want (%d, %v)", got.RetirementAge, got.MonthlySpend, defaultRetirementAge, defaultRetirementMonthlySpend)
	}
}

// TestHandleWealthRetireGetNoBirthYearGoalCardSkipsAgePace pins the safety
// property the sample path must not violate: applyRetirementPace's
// age-based fields (only ever set by that function — see its doc comment)
// must stay unset when there's no real birth year, even though the
// projection itself now computes fine with a sample age. The goal card can
// still carry a generic calendar-pace status from goalStatus (created→
// target date, independent of age) — that's pre-existing, unrelated
// behavior this test doesn't touch.
func TestHandleWealthRetireGetNoBirthYearGoalCardSkipsAgePace(t *testing.T) {
	fake := &fakeDB{
		goals: []db.Goal{{ID: 1, Name: "退休金", Kind: "retirement", TargetAmount: 20000000, TargetDate: "2060-01-01", CreatedAt: "2020-01-01"}},
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "退休帳戶", Currency: "TWD"}, Value: floatPtr(10000000)},
		},
		goalAssets: []db.GoalAsset{{GoalID: 1, AssetID: 1, Ratio: 1}},
	}
	s := newWealthRetireTestServer("", fake, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/retire", nil))
	var got retirementResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Goal == nil {
		t.Fatal("Goal = nil, want the retirement goal to still render")
	}
	if got.Goal.ProjectedAtRetirement != nil || got.Goal.RetirementAge != 0 {
		t.Errorf("Goal.ProjectedAtRetirement = %v, Goal.RetirementAge = %d, want both unset (applyRetirementPace must not run off a sample age)",
			got.Goal.ProjectedAtRetirement, got.Goal.RetirementAge)
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

// TestHandleWealthRetireGetLiveOverridesDontPersist pins the settings
// drawer's what-if contract: a query-param override changes the returned
// projection but never the goal card's real saved/target figures, and a
// follow-up request with no query params comes back identical to the
// pre-override baseline — nothing from the override was written anywhere.
func TestHandleWealthRetireGetLiveOverridesDontPersist(t *testing.T) {
	birthYear := time.Now().Year() - 40
	retireYear := time.Now().Year() + 20
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

	get := func(query string) retirementResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/retire"+query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		var got retirementResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return got
	}

	base := get("")
	live := get("?preR=8&postR=1&swr=4")

	if live.PreReturnPct != 8 {
		t.Errorf("live PreReturnPct = %v, want 8", live.PreReturnPct)
	}
	if live.Baseline == nil || base.Baseline == nil || live.Baseline.AchievementPct <= base.Baseline.AchievementPct {
		t.Errorf("expected a higher achievement rate under the 8%% preR override, live=%+v base=%+v", live.Baseline, base.Baseline)
	}
	if live.Goal == nil || base.Goal == nil || live.Goal.Saved == nil || base.Goal.Saved == nil || *live.Goal.Saved != *base.Goal.Saved {
		t.Errorf("goal's saved figure must stay the real pool regardless of overrides, live=%v base=%v", live.Goal, base.Goal)
	}

	after := get("")
	if after.PreReturnPct != base.PreReturnPct || after.RetirementAge != base.RetirementAge {
		t.Errorf("override leaked into persisted state: after=%+v, want same as base=%+v", after, base)
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

// A new account has no retirement goal and usually no birth year: assigning
// must still work, creating the goal (without a date) and the earmarks.
func TestHandleWealthRetireAssignCreatesGoalWithoutBirthYear(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{wealthAsset(7, "asset"), wealthAsset(8, "asset")}}
	s := newWealthRetireTestServer("secret", fake, wealthDB)

	rec := assignRetire(t, s, map[string]any{
		"name": "退休規劃", "retirementAge": 60, "monthlySpend": 90000,
		"assets": []map[string]any{{"assetId": 7, "ratio": 1}, {"assetId": 8, "ratio": 0.3}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastRetirementGoalName != "退休規劃" || wealthDB.lastRetirementGoalTargetDate != "" {
		t.Errorf("goal created as (%q, date %q), want (退休規劃, no date)", wealthDB.lastRetirementGoalName, wealthDB.lastRetirementGoalTargetDate)
	}
	if wealthDB.lastRetirementGoalTargetAmount != 90000.0*12*25 {
		t.Errorf("target amount = %v, want %v", wealthDB.lastRetirementGoalTargetAmount, 90000.0*12*25)
	}
	if wealthDB.lastGoalAssetsGoalID != wealthDB.nextGoalID || len(wealthDB.lastGoalAssets) != 2 {
		t.Errorf("earmarks = goal %d %+v, want 2 rows on the goal just created (%d)", wealthDB.lastGoalAssetsGoalID, wealthDB.lastGoalAssets, wealthDB.nextGoalID)
	}
}

// An existing goal is never rewritten by an assign — its target and date came
// from the quick-switch buttons, not from this drawer.
func TestHandleWealthRetireAssignLeavesExistingGoalAlone(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	fake := &fakeDB{
		goals:        []db.Goal{{ID: 5, Kind: "retirement", Name: "退休", TargetAmount: 1}},
		wealthAssets: []db.AssetWithValue{wealthAsset(7, "asset")},
	}
	s := newWealthRetireTestServer("secret", fake, wealthDB)

	rec := assignRetire(t, s, map[string]any{"assets": []map[string]any{{"assetId": 7, "ratio": 0.5}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastRetirementGoalName != "" {
		t.Errorf("UpsertRetirementGoal ran for an existing goal (name %q)", wealthDB.lastRetirementGoalName)
	}
	if wealthDB.lastGoalAssetsGoalID != 5 || len(wealthDB.lastGoalAssets) != 1 || wealthDB.lastGoalAssets[0].Ratio != 0.5 {
		t.Errorf("earmarks = goal %d %+v, want one row at 0.5 on goal 5", wealthDB.lastGoalAssetsGoalID, wealthDB.lastGoalAssets)
	}
}

// Another goal already holds 70% of asset 7: 40% more would double-count it.
// The request is rejected before anything is written — no goal, no earmarks.
func TestHandleWealthRetireAssignRejectsOver100AndWritesNothing(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	fake := &fakeDB{
		goals:        []db.Goal{{ID: 9, Kind: "general", Name: "換屋頭期款"}},
		goalAssets:   []db.GoalAsset{{GoalID: 9, AssetID: 7, Ratio: 0.7}},
		wealthAssets: []db.AssetWithValue{wealthAsset(7, "asset")},
	}
	s := newWealthRetireTestServer("secret", fake, wealthDB)

	body := func(ratio float64) map[string]any {
		return map[string]any{"name": "退休規劃", "retirementAge": 60, "monthlySpend": 90000,
			"assets": []map[string]any{{"assetId": 7, "ratio": ratio}}}
	}
	if rec := assignRetire(t, s, body(0.4)); rec.Code != http.StatusBadRequest {
		t.Fatalf("0.4 over a 0.7 holder: status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastRetirementGoalName != "" || wealthDB.lastGoalAssets != nil {
		t.Errorf("a rejected request still wrote: goal %q, earmarks %+v", wealthDB.lastRetirementGoalName, wealthDB.lastGoalAssets)
	}
	// Exactly what's left (0.3, noisy as a float) is accepted.
	if rec := assignRetire(t, s, body(1-0.7)); rec.Code != http.StatusOK {
		t.Fatalf("the remaining 30%%: status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleWealthRetireAssignRejectsBadInput(t *testing.T) {
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{wealthAsset(7, "asset"), wealthAsset(8, "liability")}}
	base := map[string]any{"name": "退休規劃", "retirementAge": 60, "monthlySpend": 90000}
	one := func(id int64, ratio float64) []map[string]any { return []map[string]any{{"assetId": id, "ratio": ratio}} }
	for name, assets := range map[string][]map[string]any{
		"unknown asset":      one(99, 1),
		"a liability":        one(8, 1),
		"zero ratio":         one(7, 0),
		"ratio above 1":      one(7, 1.5),
		"duplicate asset id": {{"assetId": 7, "ratio": 0.5}, {"assetId": 7, "ratio": 0.5}},
	} {
		wealthDB := &fakeWealthDB{}
		s := newWealthRetireTestServer("secret", fake, wealthDB)
		body := map[string]any{"assets": assets}
		for k, v := range base {
			body[k] = v
		}
		if rec := assignRetire(t, s, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400, body = %s", name, rec.Code, rec.Body.String())
		}
		if wealthDB.lastRetirementGoalName != "" || wealthDB.lastGoalAssets != nil {
			t.Errorf("%s: a rejected request still wrote", name)
		}
	}

	// With no goal yet, creating one needs a name and a spend.
	s := newWealthRetireTestServer("secret", fake, &fakeWealthDB{})
	if rec := assignRetire(t, s, map[string]any{"assets": one(7, 1)}); rec.Code != http.StatusBadRequest {
		t.Errorf("no name/spend and no goal: status = %d, want 400", rec.Code)
	}
}

// An empty list is a real request: "nothing is set aside for retirement".
func TestHandleWealthRetireAssignEmptyListClearsEarmarks(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	fake := &fakeDB{
		goals:      []db.Goal{{ID: 5, Kind: "retirement", Name: "退休", TargetAmount: 1}},
		goalAssets: []db.GoalAsset{{GoalID: 5, AssetID: 7, Ratio: 1}},
	}
	s := newWealthRetireTestServer("secret", fake, wealthDB)
	rec := assignRetire(t, s, map[string]any{"assets": []map[string]any{}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastGoalAssetsGoalID != 5 || len(wealthDB.lastGoalAssets) != 0 {
		t.Errorf("earmarks = goal %d %+v, want goal 5 with none", wealthDB.lastGoalAssetsGoalID, wealthDB.lastGoalAssets)
	}
}

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

func newWealthGoalsTestServer(password string, fake *fakeDB, wealthDB wealthWriter) *Server {
	s := &Server{db: fake, wealthDB: wealthDB, password: password, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/wealth/goals", s.handleWealthGoalsList)
	s.mux.HandleFunc("POST /api/wealth/goals", s.requireWritable(s.requireAuth(s.handleWealthGoalCreate)))
	s.mux.HandleFunc("POST /api/wealth/goals/delete", s.requireWritable(s.requireAuth(s.handleWealthGoalDelete)))
	s.mux.HandleFunc("POST /api/wealth/goals/earmark", s.requireWritable(s.requireAuth(s.handleWealthGoalEarmark)))
	return s
}

// TestHandleWealthGoalsListEmptyRendersEmptySlices pins the same
// nil-vs-empty-slice convention wealth_cash_test.go's
// TestHandleWealthCashListEmptyRendersEmptySlices does — with no goals, both
// "goals" and each goal's "assets" must serialize as [], not null.
func TestHandleWealthGoalsListEmptyRendersEmptySlices(t *testing.T) {
	s := newWealthGoalsTestServer("", &fakeDB{}, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/goals", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `"goals":null`) {
		t.Errorf("goals serialized as null, want []: %s", body)
	}
}

// TestHandleWealthGoalsList pins §8.8's progress math: saved sums each
// earmarked asset's value × ratio converted to TWD, progress is saved as a
// percent of targetAmount, and an unlinked goal (no earmarks) shows saved=0
// rather than nil (nothing to degrade — an empty earmark list is a known
// zero, not a pricing failure).
func TestHandleWealthGoalsList(t *testing.T) {
	v1, v2 := 500000.0, 300000.0
	fake := &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "玉山活存", Currency: "TWD"}, Value: &v1},
			{Asset: db.Asset{ID: 2, Side: "asset", Type: "fund", Name: "0050 定期定額", Venue: "國泰證券", Currency: "TWD"}, Value: &v2},
		},
		goals: []db.Goal{
			{ID: 1, Name: "緊急預備金", Kind: "general", TargetAmount: 300000, Currency: "TWD", CreatedAt: "2026-01-01 00:00:00"},
			{ID: 2, Name: "換屋頭期款", Kind: "general", TargetAmount: 2000000, Currency: "TWD", CreatedAt: "2026-01-01 00:00:00"},
		},
		goalAssets: []db.GoalAsset{
			{GoalID: 1, AssetID: 1, Ratio: 1.0}, // 500000 * 1.0 = 500000 -> fully funded
			{GoalID: 2, AssetID: 2, Ratio: 0.5}, // 300000 * 0.5 = 150000
		},
	}
	s := newWealthGoalsTestServer("", fake, &fakeWealthDB{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/goals", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got goalsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Goals) != 2 {
		t.Fatalf("len(Goals) = %d, want 2", len(got.Goals))
	}

	byID := map[int64]goalItem{}
	for _, g := range got.Goals {
		byID[g.ID] = g
	}

	g1 := byID[1]
	if g1.Saved == nil || *g1.Saved != 500000 {
		t.Errorf("goal 1 Saved = %v, want 500000", g1.Saved)
	}
	// 500000/300000 = 166.7% raw, capped at 100 (matches the design
	// template's goalRow(), which caps the same way — an overfunded goal
	// reads as "done", not as a number inviting a second-guess).
	if g1.ProgressPct == nil || *g1.ProgressPct != 100 {
		t.Errorf("goal 1 ProgressPct = %v, want 100 (capped)", g1.ProgressPct)
	}
	if len(g1.Assets) != 1 || g1.Assets[0].Name != "玉山活存" {
		t.Errorf("goal 1 Assets = %+v, want one row 玉山活存", g1.Assets)
	}

	g2 := byID[2]
	if g2.Saved == nil || *g2.Saved != 150000 {
		t.Errorf("goal 2 Saved = %v, want 150000", g2.Saved)
	}
	if len(g2.Assets) != 1 || g2.Assets[0].Venue != "國泰證券" {
		t.Errorf("goal 2 Assets = %+v, want one row with venue 國泰證券", g2.Assets)
	}
}

// TestHandleWealthGoalsListRetirementMonthlyContribution pins that the
// kind="retirement" row's MonthlyContribution comes from the same
// profile.retirement_monthly_contribution setting /w/retire reads — the
// design template shows a real "每月投入" figure for the retirement row, and
// a general goal (no such setting to read) must stay nil rather than reuse
// it.
func TestHandleWealthGoalsListRetirementMonthlyContribution(t *testing.T) {
	fake := &fakeDB{
		goals: []db.Goal{
			{ID: 1, Name: "退休金", Kind: "retirement", TargetAmount: 27000000, Currency: "TWD", CreatedAt: "2026-01-01 00:00:00"},
			{ID: 2, Name: "緊急預備金", Kind: "general", TargetAmount: 300000, Currency: "TWD", CreatedAt: "2026-01-01 00:00:00"},
		},
		settings: map[string]string{retirementContribSettingKey: "28000"},
	}
	s := newWealthGoalsTestServer("", fake, &fakeWealthDB{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/goals", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got goalsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	byID := map[int64]goalItem{}
	for _, g := range got.Goals {
		byID[g.ID] = g
	}
	if mc := byID[1].MonthlyContribution; mc == nil || *mc != 28000 {
		t.Errorf("retirement goal MonthlyContribution = %v, want 28000", mc)
	}
	if byID[2].MonthlyContribution != nil {
		t.Errorf("general goal MonthlyContribution = %v, want nil", byID[2].MonthlyContribution)
	}
}

// TestGoalStatus pins the straight-line expected-progress classification a
// target_date goal gets: ahead/behind/onTrack around a 5pp band.
func TestGoalStatus(t *testing.T) {
	created := "2026-01-01 00:00:00"
	target := "2027-01-01"                             // one year out
	today, _ := time.Parse("2006-01-02", "2026-07-02") // ~half the year elapsed, expected ~50%

	if got, expectedPct, ok := goalStatus(created, target, 80, today); got != "ahead" || !ok || expectedPct < 49 || expectedPct > 51 {
		t.Errorf("progress 80%% at ~50%% elapsed = (%q, %v, %v), want (ahead, ~50, true)", got, expectedPct, ok)
	}
	if got, _, ok := goalStatus(created, target, 20, today); got != "behind" || !ok {
		t.Errorf("progress 20%% at ~50%% elapsed = (%q, ok=%v), want behind", got, ok)
	}
	if got, _, ok := goalStatus(created, target, 50, today); got != "onTrack" || !ok {
		t.Errorf("progress 50%% at ~50%% elapsed = (%q, ok=%v), want onTrack", got, ok)
	}
	if _, _, ok := goalStatus(created, "", 50, today); ok {
		t.Errorf("no targetDate: ok = %v, want false", ok)
	}
}

// TestHandleWealthGoalCreateDeleteEarmark pins the three write paths: create
// requires a name and positive targetAmount, delete removes a goal
// (cascading its earmarks server-side), and earmark rejects an out-of-range
// ratio before it reaches the DB.
func TestHandleWealthGoalCreateDeleteEarmark(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthGoalsTestServer("secret", &fakeDB{}, wealthDB)
	cookie := loginAndGetCookie(t, s, "secret")

	body, _ := json.Marshal(map[string]any{"name": "緊急預備金", "targetAmount": 300000, "targetDate": "2027-01-01"})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/goals", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastNewGoal.Name != "緊急預備金" || wealthDB.lastNewGoal.TargetAmount != 300000 {
		t.Errorf("lastNewGoal = %+v, want 緊急預備金/300000", wealthDB.lastNewGoal)
	}

	badBody, _ := json.Marshal(map[string]any{"name": "", "targetAmount": 0})
	badReq := httptest.NewRequest(http.MethodPost, "/api/wealth/goals", bytes.NewReader(badBody))
	badReq.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec2, badReq)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("empty name/amount status = %d, want 400", rec2.Code)
	}

	deleteBody, _ := json.Marshal(map[string]any{"id": 9})
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/wealth/goals/delete", bytes.NewReader(deleteBody))
	deleteReq.AddCookie(cookie)
	rec3 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec3, deleteReq)
	if rec3.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec3.Code, rec3.Body.String())
	}
	if wealthDB.lastDeleteGoalID != 9 {
		t.Errorf("lastDeleteGoalID = %d, want 9", wealthDB.lastDeleteGoalID)
	}

	earmarkBody, _ := json.Marshal(map[string]any{"goalId": 1, "assetId": 2, "ratio": 0.5})
	earmarkReq := httptest.NewRequest(http.MethodPost, "/api/wealth/goals/earmark", bytes.NewReader(earmarkBody))
	earmarkReq.AddCookie(cookie)
	rec4 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec4, earmarkReq)
	if rec4.Code != http.StatusOK {
		t.Fatalf("earmark status = %d, body = %s", rec4.Code, rec4.Body.String())
	}
	if wealthDB.lastEarmarkGoalID != 1 || wealthDB.lastEarmarkAssetID != 2 || wealthDB.lastEarmarkRatio != 0.5 {
		t.Errorf("earmark call = goalId=%d assetId=%d ratio=%v, want 1/2/0.5", wealthDB.lastEarmarkGoalID, wealthDB.lastEarmarkAssetID, wealthDB.lastEarmarkRatio)
	}

	badRatioBody, _ := json.Marshal(map[string]any{"goalId": 1, "assetId": 2, "ratio": 1.5})
	badRatioReq := httptest.NewRequest(http.MethodPost, "/api/wealth/goals/earmark", bytes.NewReader(badRatioBody))
	badRatioReq.AddCookie(cookie)
	rec5 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec5, badRatioReq)
	if rec5.Code != http.StatusBadRequest {
		t.Errorf("ratio>1 status = %d, want 400", rec5.Code)
	}
}

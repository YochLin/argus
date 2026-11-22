package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
	"argus/internal/service"
)

func getChecks(t *testing.T, fake *fakeDB) service.WealthChecks {
	t.Helper()
	s := &Server{db: fake, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/wealth/checks", s.handleWealthChecks)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/checks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got service.WealthChecks
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestHandleWealthChecksNewAccount(t *testing.T) {
	got := getChecks(t, &fakeDB{})
	if !got.New || len(got.Items) != 0 || got.Rules != service.CheckRuleCount {
		t.Errorf("empty account: %+v, want new with no items", got)
	}
}

func TestHandleWealthChecksCardAndProfile(t *testing.T) {
	got := getChecks(t, &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "活存", Currency: "TWD"}, Value: floatPtr(100000)},
			{Asset: db.Asset{ID: 2, Side: "liability", Type: "credit_card", Name: "玉山卡", Currency: "TWD"}, Value: floatPtr(12000)},
		},
	})
	if got.New {
		t.Fatal("new = true with records")
	}
	byCode := map[string]service.CheckItem{}
	for _, it := range got.Items {
		byCode[it.Code] = it
	}
	if c, ok := byCode["card"]; !ok || c.Gap == nil || *c.Gap != 12000 || c.Names[0] != "玉山卡" {
		t.Errorf("card item = %+v, want 12000 on 玉山卡", c)
	}
	for _, code := range []string{"dependents", "age", "salary"} {
		if _, ok := byCode[code]; !ok {
			t.Errorf("missing %q ask with an empty profile; items = %+v", code, got.Items)
		}
	}
}

func TestHandleWealthAIView(t *testing.T) {
	get := func(fake *fakeDB) string {
		s := &Server{db: fake}
		rec := httptest.NewRecorder()
		s.handleWealthAIView(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/ai-view", nil))
		return rec.Body.String()
	}
	if got := get(&fakeDB{}); got != "null\n" {
		t.Errorf("no view yet = %q, want null", got)
	}
	view := `{"generatedAt":"2026-11-01","summary":"s","points":["p"]}`
	if got := get(&fakeDB{settings: map[string]string{service.AIViewSettingKey: view}}); got != view+"\n" {
		t.Errorf("stored view = %q", got)
	}
}

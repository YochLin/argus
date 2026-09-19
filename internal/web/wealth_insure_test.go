package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
)

func newWealthInsureTestServer(password string, fake *fakeDB, wealthDB wealthWriter) *Server {
	s := &Server{db: fake, wealthDB: wealthDB, password: password, quotes: &fakeQuotes{}, fxDB: &fakeFXDB{}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/wealth/insure", s.handleWealthInsureGet)
	s.mux.HandleFunc("POST /api/wealth/insure", s.requireWritable(s.requireAuth(s.handleWealthInsureCreate)))
	return s
}

// baseInsureFakeDB is the common household fixture every GET test builds
// on: a liquid deposit, a loan liability, and one active monthly expense —
// the three platform-computed need inputs (§8.16.1).
func baseInsureFakeDB() *fakeDB {
	depositValue, loanValue := 1000000.0, 400000.0
	return &fakeDB{
		wealthAssets: []db.AssetWithValue{
			{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid"}, Value: &depositValue},
			{Asset: db.Asset{ID: 2, Side: "liability", Type: "loan", Name: "信貸", AssetGroup: "hard"}, Value: &loanValue},
		},
		recurringCashflows: []db.RecurringCashflow{
			{ID: 1, Direction: "out", Name: "生活費", Amount: 50000, Currency: "TWD", Active: true},
		},
		settings: map[string]string{},
	}
}

// TestHandleWealthInsureGetNoProfile pins the degrade path: without the
// three "個人參數" set, life/accident/disability need stay nil (HasProfile
// gates them), but the constant-only kinds (ci/cancer/hospital) are always
// populated since they don't depend on the profile.
func TestHandleWealthInsureGetNoProfile(t *testing.T) {
	fake := baseInsureFakeDB()
	s := newWealthInsureTestServer("", fake, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/insure", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got wealthInsureResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HasProfile {
		t.Error("HasProfile = true, want false")
	}
	if len(got.Rows) != 6 {
		t.Fatalf("len(Rows) = %d, want 6", len(got.Rows))
	}
	if got.Rows[0].Kind != "life" || got.Rows[0].Need != nil {
		t.Errorf("Rows[0] (life) = %+v, want Need == nil", got.Rows[0])
	}
	if got.Rows[2].Kind != "ci" || got.Rows[2].Need == nil || *got.Rows[2].Need != 2000000 {
		t.Errorf("Rows[2] (ci) = %+v, want Need == 2000000", got.Rows[2])
	}
	if got.Rows[5].Kind != "hospital" || got.Rows[5].PerPeriod != "day" || got.Rows[5].Need == nil || *got.Rows[5].Need != 3000 {
		t.Errorf("Rows[5] (hospital) = %+v, want PerPeriod=day Need=3000", got.Rows[5])
	}
}

// TestHandleWealthInsureGetWithProfileAndCoverage exercises the full
// survivor-needs formula plus the have-side coverage aggregate and gap math.
func TestHandleWealthInsureGetWithProfileAndCoverage(t *testing.T) {
	fake := baseInsureFakeDB()
	fake.settings["profile.dependents"] = "1"
	fake.settings["profile.youngest_child_age"] = "10"
	fake.settings["profile.spouse_income"] = "0"
	fake.insuranceCoverages = []db.InsuranceCoverage{
		{ID: 1, AssetID: 3, Kind: "life", Amount: 5000000},
	}
	premium := 68000.0
	fake.wealthAssets = append(fake.wealthAssets, db.AssetWithValue{
		Asset: db.Asset{ID: 3, Side: "asset", Type: "insurance", Name: "終身壽險", Venue: "國泰人壽", Source: "manual"},
	})
	fake.insuranceDetails = map[int64]*db.InsuranceDetails{
		3: {Insurer: "國泰人壽", Insured: "本人", AnnualPremium: &premium},
	}

	s := newWealthInsureTestServer("", fake, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/insure", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got wealthInsureResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.HasProfile {
		t.Fatal("HasProfile = false, want true")
	}
	// life need = debt(400,000) + dependencyYears(22-10=12)*annualExpense(600,000) - liquid(1,000,000) = 6,600,000
	life := got.Rows[0]
	if life.Need == nil || *life.Need != 6600000 {
		t.Errorf("life.Need = %v, want 6600000", life.Need)
	}
	if life.Have != 5000000 {
		t.Errorf("life.Have = %v, want 5000000", life.Have)
	}
	if life.Gap == nil || *life.Gap != 1600000 {
		t.Errorf("life.Gap = %v, want 1600000", life.Gap)
	}
	// disability need = annualExpense/12 = 50,000 (monthly)
	disability := got.Rows[4]
	if disability.Need == nil || *disability.Need != 50000 {
		t.Errorf("disability.Need = %v, want 50000", disability.Need)
	}
	if got.Count != 1 {
		t.Errorf("Count = %d, want 1", got.Count)
	}
	if got.Premium != 68000 {
		t.Errorf("Premium = %v, want 68000", got.Premium)
	}
	if len(got.Policies) != 1 || got.Policies[0].Insurer != "國泰人壽" || got.Policies[0].Insured != "本人" {
		t.Errorf("Policies = %+v, want one row from 國泰人壽/本人", got.Policies)
	}
	// accident mirrors life's need (6,600,000) but has zero coverage (no
	// accident-kind policy in this fixture), so its gap (6,600,000) beats
	// life's partially-covered gap (1,600,000).
	if got.WorstKind != "accident" {
		t.Errorf("WorstKind = %q, want accident (uncovered, larger gap than life's partially-covered one)", got.WorstKind)
	}
}

// TestHandleWealthInsureGetExcludesArchivedPolicy makes sure a coverage
// belonging to an archived (soft-deleted) policy doesn't count toward have,
// premium, or the policy list.
// TestHandleWealthInsureGetZeroDependentsSkipsAgeRequirement makes sure a
// childless household isn't blocked on filling in a meaningless
// youngest-child-age field — dependents=0 alone (plus the spouse setting)
// is enough for HasProfile.
func TestHandleWealthInsureGetZeroDependentsSkipsAgeRequirement(t *testing.T) {
	fake := baseInsureFakeDB()
	fake.settings["profile.dependents"] = "0"
	fake.settings["profile.spouse_income"] = "0"
	// deliberately no profile.youngest_child_age

	s := newWealthInsureTestServer("", fake, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/insure", nil))
	var got wealthInsureResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.HasProfile {
		t.Fatal("HasProfile = false, want true (dependents=0 doesn't need an age)")
	}
	// life need = debt(400,000) + 0*expense - liquid(1,000,000), floored at 0
	if got.Rows[0].Need == nil || *got.Rows[0].Need != 0 {
		t.Errorf("life.Need = %v, want 0", got.Rows[0].Need)
	}
}

func TestHandleWealthInsureGetExcludesArchivedPolicy(t *testing.T) {
	fake := baseInsureFakeDB()
	fake.insuranceCoverages = []db.InsuranceCoverage{{ID: 1, AssetID: 3, Kind: "life", Amount: 5000000}}
	// ListAssetsWithValue(false) already excludes archived rows in the real
	// DB — the fake models that by simply not including asset 3 at all.
	s := newWealthInsureTestServer("", fake, &fakeWealthDB{})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/insure", nil))
	var got wealthInsureResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Count != 0 || len(got.Policies) != 0 || got.Rows[0].Have != 0 {
		t.Errorf("archived-policy coverage leaked into response: Count=%d Policies=%v life.Have=%v", got.Count, got.Policies, got.Rows[0].Have)
	}
}

func TestHandleWealthInsureCreate(t *testing.T) {
	fakeW := &fakeWealthDB{}
	s := newWealthInsureTestServer("secret", baseInsureFakeDB(), fakeW)
	cookie := loginAndGetCookie(t, s, "secret")

	body := `{"insurer":"南山人壽","name":"住院醫療","kind":"hospital","amount":3000,"insured":"本人"}`
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/insure", bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if fakeW.lastInsuranceAsset.Name != "住院醫療" || fakeW.lastInsuranceAsset.Venue != "南山人壽" {
		t.Errorf("lastInsuranceAsset = %+v, want name=住院醫療 venue=南山人壽", fakeW.lastInsuranceAsset)
	}
	if fakeW.lastInsuranceCoverage.Kind != "hospital" || fakeW.lastInsuranceCoverage.PerPeriod != "day" || fakeW.lastInsuranceCoverage.Amount != 3000 {
		t.Errorf("lastInsuranceCoverage = %+v, want kind=hospital perPeriod=day amount=3000 (server-derived, not client-supplied)", fakeW.lastInsuranceCoverage)
	}
}

func TestHandleWealthInsureCreateInvalidKind(t *testing.T) {
	fakeW := &fakeWealthDB{}
	s := newWealthInsureTestServer("secret", baseInsureFakeDB(), fakeW)
	cookie := loginAndGetCookie(t, s, "secret")

	body := `{"insurer":"南山人壽","name":"X","kind":"savings","amount":1000}`
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/insure", bytes.NewBufferString(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an invalid kind, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleWealthInsureCreateValidation(t *testing.T) {
	fakeW := &fakeWealthDB{}
	s := newWealthInsureTestServer("secret", baseInsureFakeDB(), fakeW)
	cookie := loginAndGetCookie(t, s, "secret")

	for _, body := range []string{
		`{"insurer":"","name":"X","kind":"life","amount":1000}`,
		`{"insurer":"X","name":"","kind":"life","amount":1000}`,
		`{"insurer":"X","name":"X","kind":"life","amount":0}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/wealth/insure", bytes.NewBufferString(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}

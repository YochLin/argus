package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
)

// fakeWealthDB implements wealthWriter, recording the last call's arguments
// so tests can assert what the handler passed through.
type fakeWealthDB struct {
	nextID int64

	lastNewAsset                          db.NewAsset
	lastDeposit                           db.DepositDetails
	lastLoan                              db.LoanDetails
	lastSnapshot                          db.AssetSnapshot
	lastArchiveID                         int64
	lastSettingKey, lastSettingValue      string
	lastNewCashflow                       db.NewRecurringCashflow
	lastDeactivateID                      int64
	lastNewGoal                           db.NewGoal
	lastDeleteGoalID                      int64
	lastEarmarkGoalID, lastEarmarkAssetID int64
	lastEarmarkRatio                      float64

	createErr     error
	snapshotErr   error
	archiveErr    error
	settingErr    error
	cashflowErr   error
	deactivateErr error
	goalErr       error
	deleteGoalErr error
	earmarkErr    error

	nextCashflowID int64
	nextGoalID     int64
}

func (f *fakeWealthDB) CreateAsset(a db.NewAsset) (int64, error) {
	f.lastNewAsset = a
	f.nextID++
	return f.nextID, f.createErr
}
func (f *fakeWealthDB) CreateDepositAsset(a db.NewAsset, det db.DepositDetails) (int64, error) {
	f.lastNewAsset, f.lastDeposit = a, det
	f.nextID++
	return f.nextID, f.createErr
}
func (f *fakeWealthDB) CreateLoanAsset(a db.NewAsset, det db.LoanDetails) (int64, error) {
	f.lastNewAsset, f.lastLoan = a, det
	f.nextID++
	return f.nextID, f.createErr
}
func (f *fakeWealthDB) UpsertAssetSnapshot(s db.AssetSnapshot) error {
	f.lastSnapshot = s
	return f.snapshotErr
}
func (f *fakeWealthDB) ArchiveAsset(id int64) error {
	f.lastArchiveID = id
	return f.archiveErr
}
func (f *fakeWealthDB) SetSetting(key, value string) error {
	f.lastSettingKey, f.lastSettingValue = key, value
	return f.settingErr
}
func (f *fakeWealthDB) CreateRecurringCashflow(c db.NewRecurringCashflow) (int64, error) {
	f.lastNewCashflow = c
	f.nextCashflowID++
	return f.nextCashflowID, f.cashflowErr
}
func (f *fakeWealthDB) DeactivateRecurringCashflow(id int64) error {
	f.lastDeactivateID = id
	return f.deactivateErr
}
func (f *fakeWealthDB) CreateGoal(g db.NewGoal) (int64, error) {
	f.lastNewGoal = g
	f.nextGoalID++
	return f.nextGoalID, f.goalErr
}
func (f *fakeWealthDB) DeleteGoal(id int64) error {
	f.lastDeleteGoalID = id
	return f.deleteGoalErr
}
func (f *fakeWealthDB) SetGoalAsset(goalID, assetID int64, ratio float64) error {
	f.lastEarmarkGoalID, f.lastEarmarkAssetID, f.lastEarmarkRatio = goalID, assetID, ratio
	return f.earmarkErr
}

func newWealthTestServer(password string, wealthDB wealthWriter, dbr dbReader) *Server {
	s := &Server{db: dbr, wealthDB: wealthDB, password: password}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/wealth/assets", s.handleWealthAssetsList)
	s.mux.HandleFunc("POST /api/wealth/assets", s.requireWritable(s.requireAuth(s.handleWealthAssetCreate)))
	s.mux.HandleFunc("POST /api/wealth/assets/snapshot", s.requireWritable(s.requireAuth(s.handleWealthAssetSnapshot)))
	s.mux.HandleFunc("POST /api/wealth/assets/archive", s.requireWritable(s.requireAuth(s.handleWealthAssetArchive)))
	s.mux.HandleFunc("GET /api/wealth/cash", s.handleWealthCashList)
	s.mux.HandleFunc("POST /api/wealth/cash", s.requireWritable(s.requireAuth(s.handleWealthCashCreate)))
	s.mux.HandleFunc("POST /api/wealth/cash/deactivate", s.requireWritable(s.requireAuth(s.handleWealthCashDeactivate)))
	s.mux.HandleFunc("GET /api/wealth/goals", s.handleWealthGoalsList)
	s.mux.HandleFunc("POST /api/wealth/goals", s.requireWritable(s.requireAuth(s.handleWealthGoalCreate)))
	s.mux.HandleFunc("POST /api/wealth/goals/delete", s.requireWritable(s.requireAuth(s.handleWealthGoalDelete)))
	s.mux.HandleFunc("POST /api/wealth/goals/earmark", s.requireWritable(s.requireAuth(s.handleWealthGoalEarmark)))
	return s
}

func TestHandleWealthAssetsList(t *testing.T) {
	value := 100000.0
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "玉山活存", AssetGroup: "liquid"}, Value: &value, AsOf: "2026-09-15"},
		{Asset: db.Asset{ID: 2, Side: "asset", Type: "other", Name: "未估價資產", AssetGroup: "hard"}},
		{Asset: db.Asset{ID: 3, Side: "liability", Type: "loan", Name: "房貸", AssetGroup: "hard", ArchivedAt: "2026-09-01"}},
	}}
	s := newWealthTestServer("secret", &fakeWealthDB{}, fake)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/assets", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Assets []assetResponse `json:"assets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Assets) != 2 {
		t.Fatalf("Assets = %d, want 2 (archived filtered by default)", len(got.Assets))
	}
	if got.Assets[0].Value == nil || *got.Assets[0].Value != 100000 {
		t.Errorf("Assets[0].Value = %v, want 100000", got.Assets[0].Value)
	}
	if got.Assets[1].Value != nil {
		t.Errorf("Assets[1].Value = %v, want nil (no snapshot yet, must not fabricate 0)", got.Assets[1].Value)
	}

	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/assets?archived=1", nil))
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Assets) != 3 {
		t.Errorf("Assets with archived=1 = %d, want 3", len(got.Assets))
	}
}

func TestHandleWealthAssetCreateDeposit(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	body, _ := json.Marshal(wealthAssetCreateRequest{
		Side: "asset", Type: "deposit", Name: "玉山活存", AssetGroup: "liquid",
		InitialValue: 50000, Deposit: &wealthAssetDepositRequest{Bank: "玉山銀行"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastNewAsset.Type != "deposit" || wealthDB.lastDeposit.Bank != "玉山銀行" {
		t.Errorf("CreateDepositAsset() got NewAsset=%+v DepositDetails=%+v", wealthDB.lastNewAsset, wealthDB.lastDeposit)
	}
	if wealthDB.lastSnapshot.Value != 50000 || wealthDB.lastSnapshot.Source != "manual" {
		t.Errorf("UpsertAssetSnapshot() got %+v, want value 50000 source manual", wealthDB.lastSnapshot)
	}
	if wealthDB.lastSnapshot.Date == "" {
		t.Error("UpsertAssetSnapshot() date is empty, want today's date defaulted")
	}
}

func TestHandleWealthAssetCreateLoan(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	rate := 2.1
	body, _ := json.Marshal(wealthAssetCreateRequest{
		Side: "liability", Type: "loan", Name: "房貸", AssetGroup: "hard",
		InitialValue: 8000000, Loan: &wealthAssetLoanRequest{Lender: "台北富邦", RatePct: &rate},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastLoan.Lender != "台北富邦" || wealthDB.lastLoan.RatePct == nil || *wealthDB.lastLoan.RatePct != 2.1 {
		t.Errorf("CreateLoanAsset() got LoanDetails=%+v", wealthDB.lastLoan)
	}
}

func TestHandleWealthAssetCreateValidation(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	cases := []wealthAssetCreateRequest{
		{Type: "deposit", Name: "x", AssetGroup: "liquid"},              // missing side
		{Side: "asset", Name: "x", AssetGroup: "liquid"},                // missing type
		{Side: "asset", Type: "deposit", AssetGroup: "liquid"},          // missing name
		{Side: "asset", Type: "deposit", Name: "x", AssetGroup: "nope"}, // bad group
	}
	for i, c := range cases {
		body, _ := json.Marshal(c)
		req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: status = %d, want 400, body = %s", i, rec.Code, rec.Body.String())
		}
	}
}

func TestHandleWealthAssetCreateRequiresAuth(t *testing.T) {
	s := newWealthTestServer("secret", &fakeWealthDB{}, &fakeDB{})
	body, _ := json.Marshal(wealthAssetCreateRequest{Side: "asset", Type: "deposit", Name: "x", AssetGroup: "liquid"})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (no auth cookie)", rec.Code)
	}
}

func TestHandleWealthAssetSnapshot(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	body, _ := json.Marshal(wealthSnapshotRequest{AssetID: 7, Value: 123456})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets/snapshot", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastSnapshot.AssetID != 7 || wealthDB.lastSnapshot.Value != 123456 || wealthDB.lastSnapshot.Source != "manual" {
		t.Errorf("UpsertAssetSnapshot() got %+v", wealthDB.lastSnapshot)
	}
}

func TestHandleWealthAssetArchive(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	body, _ := json.Marshal(wealthArchiveRequest{AssetID: 42})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets/archive", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if wealthDB.lastArchiveID != 42 {
		t.Errorf("ArchiveAsset() id = %d, want 42", wealthDB.lastArchiveID)
	}
}

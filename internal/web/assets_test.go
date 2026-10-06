package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	lastLog                               loggedValue
	lastSnapshotDelete                    loggedValue
	lastArchiveID, lastUnarchiveID        int64
	lastUpdateAssetID                     int64
	lastAssetEdit                         db.AssetEdit
	updateAssetErr                        error
	lastSettingKey, lastSettingValue      string
	lastNewCashflow                       db.NewRecurringCashflow
	lastUpdateCashflowID                  int64
	lastCashflowEdit                      db.RecurringCashflowEdit
	updateCashflowErr                     error
	lastDeactivateID                      int64
	lastDeactivateDay                     string
	lastResumeID, lastDeleteCashflowID    int64
	lastNewGoal                           db.NewGoal
	lastDeleteGoalID                      int64
	lastUpdateGoalID                      int64
	updateGoalErr                         error
	lastEarmarkGoalID, lastEarmarkAssetID int64
	lastEarmarkRatio                      float64
	lastGoalAssetsGoalID                  int64
	lastGoalAssets                        []db.GoalAsset
	lastRetirementGoalName                string
	lastRetirementGoalTargetAmount        float64
	lastRetirementGoalTargetDate          string
	lastInsuranceAsset                    db.NewAsset
	lastInsuranceDetails                  db.InsuranceDetails
	lastInsuranceCoverage                 db.InsuranceCoverage
	lastFundAsset                         db.NewAsset
	lastFundDetails                       db.FundDetails

	createErr         error
	snapshotErr       error
	logErr            error
	snapshotDeleteErr error
	archiveErr        error
	settingErr        error
	cashflowErr       error
	deactivateErr     error
	resumeErr         error
	deleteCashflowErr error
	goalErr           error
	deleteGoalErr     error
	earmarkErr        error
	goalAssetsErr     error
	retirementGoalErr error
	insuranceErr      error
	fundErr           error

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

// loggedValue is what LogAssetValue/DeleteAssetSnapshot were called with.
type loggedValue struct {
	AssetID int64
	Date    string
	Value   float64
	Today   string
}

func (f *fakeWealthDB) LogAssetValue(assetID int64, date string, value float64, today string) error {
	f.lastLog = loggedValue{assetID, date, value, today}
	return f.logErr
}
func (f *fakeWealthDB) DeleteAssetSnapshot(assetID int64, date, today string) error {
	f.lastSnapshotDelete = loggedValue{AssetID: assetID, Date: date, Today: today}
	return f.snapshotDeleteErr
}
func (f *fakeWealthDB) ArchiveAsset(id int64) error {
	f.lastArchiveID = id
	return f.archiveErr
}
func (f *fakeWealthDB) UnarchiveAsset(id int64) error {
	f.lastUnarchiveID = id
	return f.archiveErr
}
func (f *fakeWealthDB) UpdateAsset(id int64, e db.AssetEdit) error {
	f.lastUpdateAssetID, f.lastAssetEdit = id, e
	return f.updateAssetErr
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
func (f *fakeWealthDB) DeactivateRecurringCashflow(id int64, today string) error {
	f.lastDeactivateID, f.lastDeactivateDay = id, today
	return f.deactivateErr
}
func (f *fakeWealthDB) ResumeRecurringCashflow(id int64) error {
	f.lastResumeID = id
	return f.resumeErr
}
func (f *fakeWealthDB) DeleteRecurringCashflow(id int64) error {
	f.lastDeleteCashflowID = id
	return f.deleteCashflowErr
}
func (f *fakeWealthDB) UpdateRecurringCashflow(id int64, e db.RecurringCashflowEdit) error {
	f.lastUpdateCashflowID, f.lastCashflowEdit = id, e
	return f.updateCashflowErr
}
func (f *fakeWealthDB) CreateGoal(g db.NewGoal) (int64, error) {
	f.lastNewGoal = g
	f.nextGoalID++
	return f.nextGoalID, f.goalErr
}
func (f *fakeWealthDB) UpdateGoal(id int64, g db.NewGoal) error {
	f.lastUpdateGoalID, f.lastNewGoal = id, g
	return f.updateGoalErr
}
func (f *fakeWealthDB) DeleteGoal(id int64) error {
	f.lastDeleteGoalID = id
	return f.deleteGoalErr
}
func (f *fakeWealthDB) SetGoalAsset(goalID, assetID int64, ratio float64) error {
	f.lastEarmarkGoalID, f.lastEarmarkAssetID, f.lastEarmarkRatio = goalID, assetID, ratio
	return f.earmarkErr
}
func (f *fakeWealthDB) SetGoalAssets(goalID int64, set []db.GoalAsset) error {
	f.lastGoalAssetsGoalID, f.lastGoalAssets = goalID, set
	return f.goalAssetsErr
}
func (f *fakeWealthDB) UpsertRetirementGoal(name string, targetAmount float64, targetDate string) (int64, error) {
	f.lastRetirementGoalName, f.lastRetirementGoalTargetAmount, f.lastRetirementGoalTargetDate = name, targetAmount, targetDate
	f.nextGoalID++
	return f.nextGoalID, f.retirementGoalErr
}
func (f *fakeWealthDB) CreateInsuranceAsset(a db.NewAsset, det db.InsuranceDetails, cov db.InsuranceCoverage) (int64, error) {
	f.lastInsuranceAsset, f.lastInsuranceDetails, f.lastInsuranceCoverage = a, det, cov
	f.nextID++
	return f.nextID, f.insuranceErr
}
func (f *fakeWealthDB) CreateFundAsset(a db.NewAsset, det db.FundDetails) (int64, error) {
	f.lastFundAsset, f.lastFundDetails = a, det
	f.nextID++
	return f.nextID, f.fundErr
}

func newWealthTestServer(password string, wealthDB wealthWriter, dbr dbReader) *Server {
	s := &Server{db: dbr, wealthDB: wealthDB, password: password}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/wealth/assets", s.handleWealthAssetsList)
	s.mux.HandleFunc("POST /api/wealth/assets", s.requireWritable(s.requireAuth(s.handleWealthAssetCreate)))
	s.mux.HandleFunc("POST /api/wealth/assets/snapshot", s.requireWritable(s.requireAuth(s.handleWealthAssetSnapshot)))
	s.mux.HandleFunc("POST /api/wealth/assets/snapshot/delete", s.requireWritable(s.requireAuth(s.handleWealthAssetSnapshotDelete)))
	s.mux.HandleFunc("GET /api/wealth/assets/history", s.handleWealthAssetHistory)
	s.mux.HandleFunc("POST /api/wealth/assets/archive", s.requireWritable(s.requireAuth(s.handleWealthAssetArchive)))
	s.mux.HandleFunc("POST /api/wealth/assets/unarchive", s.requireWritable(s.requireAuth(s.handleWealthAssetUnarchive)))
	s.mux.HandleFunc("POST /api/wealth/assets/update", s.requireWritable(s.requireAuth(s.handleWealthAssetUpdate)))
	s.mux.HandleFunc("GET /api/wealth/cash", s.handleWealthCashList)
	s.mux.HandleFunc("POST /api/wealth/cash", s.requireWritable(s.requireAuth(s.handleWealthCashCreate)))
	s.mux.HandleFunc("POST /api/wealth/cash/deactivate", s.requireWritable(s.requireAuth(s.handleWealthCashDeactivate)))
	s.mux.HandleFunc("POST /api/wealth/cash/resume", s.requireWritable(s.requireAuth(s.handleWealthCashResume)))
	s.mux.HandleFunc("POST /api/wealth/cash/delete", s.requireWritable(s.requireAuth(s.handleWealthCashDelete)))
	s.mux.HandleFunc("POST /api/wealth/cash/update", s.requireWritable(s.requireAuth(s.handleWealthCashUpdate)))
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

// TestHandleWealthAssetsListPricesInTWD pins the TWD value the wealth pages
// show for a foreign-currency asset: today's rate, archived entries included,
// and no value at all (not a guess) for a currency with no rate or an asset with
// no snapshot yet.
func TestHandleWealthAssetsListPricesInTWD(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	usd, twd, eur := 1000.0, 50000.0, 20.0
	fake := &fakeDB{wealthAssets: []db.AssetWithValue{
		{Asset: db.Asset{ID: 1, Side: "asset", Type: "deposit", Name: "美元活存", Currency: "USD"}, Value: &usd},
		{Asset: db.Asset{ID: 2, Side: "asset", Type: "deposit", Name: "台幣活存", Currency: "TWD"}, Value: &twd},
		{Asset: db.Asset{ID: 3, Side: "asset", Type: "deposit", Name: "歐元活存", Currency: "EUR"}, Value: &eur},
		{Asset: db.Asset{ID: 4, Side: "asset", Type: "deposit", Name: "未估價美元", Currency: "USD"}},
		{Asset: db.Asset{ID: 5, Side: "asset", Type: "deposit", Name: "已封存美元", Currency: "USD", ArchivedAt: "2026-09-01"}, Value: &usd},
	}}
	s := newWealthTestServer("secret", &fakeWealthDB{}, fake)
	s.fxDB = &fakeFXDB{rates: map[string]float64{fxKey(today, "USDTWD"): 30}}
	s.quotes = &fakeQuotes{err: map[string]error{"EURTWD=X": errors.New("no quote")}}

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/assets?archived=1", nil))
	var got struct {
		Assets []assetResponse `json:"assets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status = %d, err = %v, body = %s", rec.Code, err, rec.Body.String())
	}
	want := map[int64]*float64{1: new(float64), 2: new(float64), 3: nil, 4: nil, 5: new(float64)}
	*want[1], *want[2], *want[5] = 30000, 50000, 30000
	for _, a := range got.Assets {
		w := want[a.ID]
		switch {
		case w == nil && a.ValueTwd != nil:
			t.Errorf("asset %d (%s) ValueTwd = %v, want nil", a.ID, a.Name, *a.ValueTwd)
		case w != nil && (a.ValueTwd == nil || *a.ValueTwd != *w):
			t.Errorf("asset %d (%s) ValueTwd = %v, want %v", a.ID, a.Name, a.ValueTwd, *w)
		}
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

func postWealthJSON(t *testing.T, s *Server, cookie *http.Cookie, path string, v any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(v)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestHandleWealthAssetSnapshotLogsToday(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	rec := postWealthJSON(t, s, cookie, "/api/wealth/assets/snapshot", wealthSnapshotRequest{AssetID: 7, Value: 123456})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	got := wealthDB.lastLog
	today := time.Now().Format("2006-01-02")
	if got.AssetID != 7 || got.Value != 123456 || got.Date != today || got.Today != today {
		t.Errorf("LogAssetValue() got %+v, want asset 7, 123456 on %s", got, today)
	}
}

func TestHandleWealthAssetSnapshotBackfillsAPastDate(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	rec := postWealthJSON(t, s, loginAndGetCookie(t, s, "secret"), "/api/wealth/assets/snapshot",
		wealthSnapshotRequest{AssetID: 7, Value: 5, Date: "2020-01-02"})
	if rec.Code != http.StatusOK || wealthDB.lastLog.Date != "2020-01-02" {
		t.Errorf("status = %d, logged %+v, want 200 on 2020-01-02", rec.Code, wealthDB.lastLog)
	}
}

func TestHandleWealthAssetSnapshotRejectsBadInput(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	for name, req := range map[string]wealthSnapshotRequest{
		"no asset id": {Value: 1},
		"bad date":    {AssetID: 7, Value: 1, Date: "07/15"},
		"future date": {AssetID: 7, Value: 1, Date: tomorrow},
	} {
		t.Run(name, func(t *testing.T) {
			wealthDB := &fakeWealthDB{}
			s := newWealthTestServer("secret", wealthDB, &fakeDB{})
			rec := postWealthJSON(t, s, loginAndGetCookie(t, s, "secret"), "/api/wealth/assets/snapshot", req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
			if wealthDB.lastLog != (loggedValue{}) {
				t.Errorf("LogAssetValue was called (%+v) for a rejected request", wealthDB.lastLog)
			}
		})
	}
}

func TestHandleWealthAssetSnapshotMapsDBErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"unknown asset":  {db.ErrAssetNotFound, http.StatusNotFound},
		"archived asset": {db.ErrAssetArchived, http.StatusConflict},
		"closed day":     {db.ErrSnapshotLocked, http.StatusConflict},
		"anything else":  {errors.New("disk full"), http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			s := newWealthTestServer("secret", &fakeWealthDB{logErr: tc.err}, &fakeDB{})
			rec := postWealthJSON(t, s, loginAndGetCookie(t, s, "secret"), "/api/wealth/assets/snapshot", wealthSnapshotRequest{AssetID: 7, Value: 1})
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestHandleWealthAssetSnapshotDelete(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")
	today := time.Now().Format("2006-01-02")

	rec := postWealthJSON(t, s, cookie, "/api/wealth/assets/snapshot/delete", wealthSnapshotDeleteRequest{AssetID: 7, Date: today})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if got := wealthDB.lastSnapshotDelete; got.AssetID != 7 || got.Date != today || got.Today != today {
		t.Errorf("DeleteAssetSnapshot() got %+v, want asset 7 on %s", got, today)
	}

	for name, tc := range map[string]struct {
		req  wealthSnapshotDeleteRequest
		err  error
		want int
	}{
		"no asset id":   {wealthSnapshotDeleteRequest{Date: today}, nil, http.StatusBadRequest},
		"bad date":      {wealthSnapshotDeleteRequest{AssetID: 7, Date: "yesterday"}, nil, http.StatusBadRequest},
		"closed day":    {wealthSnapshotDeleteRequest{AssetID: 7, Date: "2020-01-02"}, db.ErrSnapshotLocked, http.StatusConflict},
		"no such row":   {wealthSnapshotDeleteRequest{AssetID: 7, Date: today}, db.ErrSnapshotNotFound, http.StatusNotFound},
		"archived":      {wealthSnapshotDeleteRequest{AssetID: 7, Date: today}, db.ErrAssetArchived, http.StatusConflict},
		"anything else": {wealthSnapshotDeleteRequest{AssetID: 7, Date: today}, errors.New("disk full"), http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			s := newWealthTestServer("secret", &fakeWealthDB{snapshotDeleteErr: tc.err}, &fakeDB{})
			rec := postWealthJSON(t, s, loginAndGetCookie(t, s, "secret"), "/api/wealth/assets/snapshot/delete", tc.req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestHandleWealthAssetSnapshotRoutesRequireAuth(t *testing.T) {
	s := newWealthTestServer("secret", &fakeWealthDB{}, &fakeDB{})
	for _, path := range []string{"/api/wealth/assets/snapshot", "/api/wealth/assets/snapshot/delete"} {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{"assetId":7}`)))
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s status = %d, want 401 (no auth cookie)", path, rec.Code)
		}
	}
}

func TestHandleWealthAssetHistory(t *testing.T) {
	dbr := &fakeDB{assetSnapshots: map[int64][]db.AssetSnapshot{
		7: {{AssetID: 7, Date: "2026-07-15", Value: 120, Source: "manual"}, {AssetID: 7, Date: "2026-06-30", Value: 100, Source: "import"}},
	}}
	s := newWealthTestServer("secret", &fakeWealthDB{}, dbr)

	get := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/assets/history"+q, nil))
		return rec
	}

	rec := get("?assetId=7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Today     string              `json:"today"`
		Snapshots []wealthSnapshotRow `json:"snapshots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Today != time.Now().Format("2006-01-02") {
		t.Errorf("today = %q, want the server's date", resp.Today)
	}
	if len(resp.Snapshots) != 2 || resp.Snapshots[0] != (wealthSnapshotRow{Date: "2026-07-15", Value: 120, Source: "manual"}) || resp.Snapshots[1].Source != "import" {
		t.Errorf("snapshots = %+v, want the two rows newest first", resp.Snapshots)
	}

	// An asset with no records is an empty list, not null — the drawer maps it.
	if rec := get("?assetId=8"); !bytes.Contains(rec.Body.Bytes(), []byte(`"snapshots":[]`)) {
		t.Errorf("unknown asset body = %s, want an empty snapshots array", rec.Body.String())
	}
	for _, q := range []string{"", "?assetId=abc", "?assetId=0"} {
		if rec := get(q); rec.Code != http.StatusBadRequest {
			t.Errorf("history%s status = %d, want 400", q, rec.Code)
		}
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

func TestHandleWealthAssetUnarchive(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	body, _ := json.Marshal(wealthArchiveRequest{AssetID: 42})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets/unarchive", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || wealthDB.lastUnarchiveID != 42 {
		t.Fatalf("status = %d, UnarchiveAsset id = %d; want 200 and 42, body = %s", rec.Code, wealthDB.lastUnarchiveID, rec.Body.String())
	}

	// Without the auth cookie the write gate must reject it.
	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/wealth/assets/unarchive", bytes.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status without cookie = %d, want 401", rec.Code)
	}
}

func postWealthUpdate(t *testing.T, s *Server, cookie *http.Cookie, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/assets/update", bytes.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestHandleWealthAssetUpdate(t *testing.T) {
	wealthDB := &fakeWealthDB{}
	s := newWealthTestServer("secret", wealthDB, &fakeDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	rate := 2.4
	rec := postWealthUpdate(t, s, cookie, wealthAssetUpdateRequest{
		AssetID: 5, Name: "  房貸  ", AssetGroup: "hard", Venue: " 國泰 ",
		Loan: &wealthAssetLoanRequest{Lender: "國泰", RatePct: &rate},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	got := wealthDB.lastAssetEdit
	if wealthDB.lastUpdateAssetID != 5 || got.Name != "房貸" || got.Venue != "國泰" || got.AssetGroup != "hard" {
		t.Errorf("UpdateAsset(%d, %+v), want id 5 with trimmed name/venue", wealthDB.lastUpdateAssetID, got)
	}
	if got.Loan == nil || got.Loan.Lender != "國泰" || got.Loan.RatePct == nil || *got.Loan.RatePct != 2.4 || got.Deposit != nil {
		t.Errorf("edit details = loan %+v deposit %+v, want only the loan block", got.Loan, got.Deposit)
	}

	rec = postWealthUpdate(t, s, cookie, wealthAssetUpdateRequest{AssetID: 6, Name: "活存", AssetGroup: "liquid"})
	if rec.Code != http.StatusOK || wealthDB.lastAssetEdit.Deposit != nil || wealthDB.lastAssetEdit.Loan != nil {
		t.Errorf("a details-less edit: status = %d, edit = %+v, want 200 with no detail blocks", rec.Code, wealthDB.lastAssetEdit)
	}
}

func TestHandleWealthAssetUpdateRejectsBadInput(t *testing.T) {
	self := int64(5)
	for name, tc := range map[string]struct {
		req  wealthAssetUpdateRequest
		want int
	}{
		"no asset id":            {wealthAssetUpdateRequest{Name: "x", AssetGroup: "liquid"}, http.StatusBadRequest},
		"blank name":             {wealthAssetUpdateRequest{AssetID: 5, Name: "  ", AssetGroup: "liquid"}, http.StatusBadRequest},
		"unknown group":          {wealthAssetUpdateRequest{AssetID: 5, Name: "x", AssetGroup: "cash"}, http.StatusBadRequest},
		"deposit and loan":       {wealthAssetUpdateRequest{AssetID: 5, Name: "x", AssetGroup: "liquid", Deposit: &wealthAssetDepositRequest{}, Loan: &wealthAssetLoanRequest{}}, http.StatusBadRequest},
		"loan secured by itself": {wealthAssetUpdateRequest{AssetID: 5, Name: "x", AssetGroup: "hard", Loan: &wealthAssetLoanRequest{SecuredAssetID: &self}}, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			wealthDB := &fakeWealthDB{}
			s := newWealthTestServer("secret", wealthDB, &fakeDB{})
			rec := postWealthUpdate(t, s, loginAndGetCookie(t, s, "secret"), tc.req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
			if wealthDB.lastUpdateAssetID != 0 {
				t.Errorf("UpdateAsset was called (id %d) for a rejected request", wealthDB.lastUpdateAssetID)
			}
		})
	}
}

func TestHandleWealthAssetUpdateMapsDBErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"unknown asset":      {db.ErrAssetNotFound, http.StatusNotFound},
		"wrong detail block": {db.ErrAssetNoDetails, http.StatusBadRequest},
		"anything else":      {errors.New("disk full"), http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			s := newWealthTestServer("secret", &fakeWealthDB{updateAssetErr: tc.err}, &fakeDB{})
			rec := postWealthUpdate(t, s, loginAndGetCookie(t, s, "secret"), wealthAssetUpdateRequest{AssetID: 5, Name: "x", AssetGroup: "liquid"})
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

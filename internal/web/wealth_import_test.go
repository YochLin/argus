package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
)

func TestParseWealthImportCSV(t *testing.T) {
	t.Run("deposit and loan rows parse their type-specific columns", func(t *testing.T) {
		csv := "side,type,name,group,venue,currency,value,date,bank,accountNote,lender,ratePct,originalPrincipal,remainingMonths\n" +
			"asset,deposit,玉山活存,liquid,玉山銀行,TWD,100000,2026-09-01,玉山銀行,薪轉戶\n" +
			"liability,loan,房貸,hard,,TWD,3000000,2026-09-01,,,國泰世華,2.1,4000000,240\n"
		rows, err := parseWealthImportCSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		d := rows[0]
		if d.Status != "ok" || d.Type != "deposit" || d.Bank != "玉山銀行" || d.AccountNote != "薪轉戶" {
			t.Errorf("deposit row = %+v, want ok deposit with bank/accountNote set", d)
		}
		l := rows[1]
		if l.Status != "ok" || l.Type != "loan" || l.Lender != "國泰世華" {
			t.Errorf("loan row = %+v, want ok loan with lender set", l)
		}
		if l.RatePct == nil || *l.RatePct != 2.1 {
			t.Errorf("loan RatePct = %v, want 2.1", l.RatePct)
		}
		if l.OriginalPrincipal == nil || *l.OriginalPrincipal != 4000000 {
			t.Errorf("loan OriginalPrincipal = %v, want 4000000", l.OriginalPrincipal)
		}
		if l.RemainingMonths == nil || *l.RemainingMonths != 240 {
			t.Errorf("loan RemainingMonths = %v, want 240", l.RemainingMonths)
		}
	})

	t.Run("fund row parses its type-specific columns", func(t *testing.T) {
		csv := "side,type,name,group,venue,currency,value,date,bank,accountNote,lender,ratePct,originalPrincipal,remainingMonths,fundCode,fundPlatform,fundMonthlyAmount,fundNextContributionDate\n" +
			"asset,fund,元大台灣50,growth,券商,TWD,640000,2026-09-01,,,,,,,0050,券商,12000,2026-10-06\n"
		rows, err := parseWealthImportCSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		f := rows[0]
		if f.Status != "ok" || f.Type != "fund" || f.FundCode != "0050" || f.FundPlatform != "券商" {
			t.Errorf("fund row = %+v, want ok fund with code/platform set", f)
		}
		if f.FundMonthlyAmount == nil || *f.FundMonthlyAmount != 12000 {
			t.Errorf("fund FundMonthlyAmount = %v, want 12000", f.FundMonthlyAmount)
		}
		if f.FundNextContributionDate != "2026-10-06" {
			t.Errorf("fund FundNextContributionDate = %q, want 2026-10-06", f.FundNextContributionDate)
		}
	})

	t.Run("fund row with no monthly amount (lump-sum/stopped)", func(t *testing.T) {
		csv := "side,type,name,group,venue,currency,value,date,bank,accountNote,lender,ratePct,originalPrincipal,remainingMonths,fundCode,fundPlatform,fundMonthlyAmount,fundNextContributionDate\n" +
			"asset,fund,富邦科技（單筆）,growth,券商,TWD,411000,2026-09-01,,,,,,,0052,券商,,\n"
		rows, err := parseWealthImportCSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 || rows[0].Status != "ok" || rows[0].FundMonthlyAmount != nil {
			t.Errorf("rows = %+v, want one ok row with nil FundMonthlyAmount", rows)
		}
	})

	t.Run("other-type row needs no detail columns", func(t *testing.T) {
		rows, err := parseWealthImportCSV("side,type,name,group,venue,currency,value,date\n" +
			"asset,other,黃金,hard,,TWD,50000,2026-09-01\n")
		if err != nil || len(rows) != 1 || rows[0].Status != "ok" {
			t.Fatalf("rows = %+v, err = %v, want one ok row", rows, err)
		}
	})

	t.Run("no data rows", func(t *testing.T) {
		rows, err := parseWealthImportCSV("side,type,name,group,venue,currency,value,date\n")
		if err != nil || rows != nil {
			t.Errorf("got rows=%v err=%v, want nil,nil", rows, err)
		}
	})

	t.Run("invalid rows are marked error, not dropped", func(t *testing.T) {
		csv := "side,type,name,group,venue,currency,value,date\n" +
			"nope,deposit,X,liquid,,TWD,100,2026-09-01\n" + // bad side
			"asset,,X,liquid,,TWD,100,2026-09-01\n" + // missing type
			"asset,deposit,X,nogroup,,TWD,100,2026-09-01\n" + // bad group
			"asset,deposit,X,liquid,,TWD,notanumber,2026-09-01\n" + // bad value
			"asset,deposit,X,liquid,,TWD,100,not-a-date\n" + // bad date
			"asset,deposit,X\n" // too few columns
		rows, err := parseWealthImportCSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 6 {
			t.Fatalf("len(rows) = %d, want 6", len(rows))
		}
		for i, r := range rows {
			if r.Status != "error" {
				t.Errorf("row[%d].Status = %q, want error (message: %s)", i, r.Status, r.Message)
			}
		}
	})
}

func TestAnnotateWealthImportRows(t *testing.T) {
	existing := []db.AssetWithValue{
		{Asset: db.Asset{Name: "玉山活存", Type: "deposit"}},
	}
	rows := []wealthImportRow{
		{Status: "ok", Name: "玉山活存", Type: "deposit"},   // matches existing -> duplicate
		{Status: "ok", Name: "台幣定存", Type: "deposit"},   // new, unique in batch -> stays ok
		{Status: "ok", Name: "台幣定存", Type: "deposit"},   // duplicate within the same batch
		{Status: "error", Name: "壞資料", Type: "deposit"}, // untouched
	}
	annotateWealthImportRows(rows, existing)

	if rows[0].Status != "duplicate" {
		t.Errorf("rows[0].Status = %q, want duplicate (matches existing)", rows[0].Status)
	}
	if rows[1].Status != "ok" {
		t.Errorf("rows[1].Status = %q, want ok", rows[1].Status)
	}
	if rows[2].Status != "duplicate" {
		t.Errorf("rows[2].Status = %q, want duplicate (matches earlier row in batch)", rows[2].Status)
	}
	if rows[3].Status != "error" {
		t.Errorf("rows[3].Status = %q, want error left untouched", rows[3].Status)
	}
}

// fakeWealthImportDB records every create/snapshot call in order (unlike
// fakeWealthDB's single "last call" fields) — applyWealthImportRows writes
// several rows in one call, and the test needs to see all of them.
type fakeWealthImportDB struct {
	nextID       int64
	createdAsset []db.NewAsset
	createdLoan  []db.LoanDetails
	createdFund  []db.FundDetails
	snapshots    []db.AssetSnapshot
}

func (f *fakeWealthImportDB) CreateAsset(a db.NewAsset) (int64, error) {
	f.nextID++
	f.createdAsset = append(f.createdAsset, a)
	return f.nextID, nil
}
func (f *fakeWealthImportDB) CreateDepositAsset(a db.NewAsset, det db.DepositDetails) (int64, error) {
	f.nextID++
	f.createdAsset = append(f.createdAsset, a)
	return f.nextID, nil
}
func (f *fakeWealthImportDB) CreateLoanAsset(a db.NewAsset, det db.LoanDetails) (int64, error) {
	f.nextID++
	f.createdAsset = append(f.createdAsset, a)
	f.createdLoan = append(f.createdLoan, det)
	return f.nextID, nil
}
func (f *fakeWealthImportDB) UpsertAssetSnapshot(s db.AssetSnapshot) error {
	f.snapshots = append(f.snapshots, s)
	return nil
}
func (f *fakeWealthImportDB) ArchiveAsset(id int64) error        { return nil }
func (f *fakeWealthImportDB) SetSetting(key, value string) error { return nil }
func (f *fakeWealthImportDB) CreateRecurringCashflow(c db.NewRecurringCashflow) (int64, error) {
	return 0, nil
}
func (f *fakeWealthImportDB) DeactivateRecurringCashflow(id int64) error { return nil }
func (f *fakeWealthImportDB) CreateGoal(g db.NewGoal) (int64, error)     { return 0, nil }
func (f *fakeWealthImportDB) UpdateGoal(id int64, g db.NewGoal) error    { return nil }
func (f *fakeWealthImportDB) DeleteGoal(id int64) error                  { return nil }
func (f *fakeWealthImportDB) SetGoalAsset(goalID, assetID int64, ratio float64) error {
	return nil
}
func (f *fakeWealthImportDB) UpsertRetirementGoal(name string, targetAmount float64, targetDate string) (int64, error) {
	return 0, nil
}
func (f *fakeWealthImportDB) CreateInsuranceAsset(a db.NewAsset, det db.InsuranceDetails, cov db.InsuranceCoverage) (int64, error) {
	return 0, nil
}
func (f *fakeWealthImportDB) CreateFundAsset(a db.NewAsset, det db.FundDetails) (int64, error) {
	f.nextID++
	f.createdAsset = append(f.createdAsset, a)
	f.createdFund = append(f.createdFund, det)
	return f.nextID, nil
}

func TestApplyWealthImportRows(t *testing.T) {
	fake := &fakeWealthImportDB{}
	monthly := 12000.0
	rows := []wealthImportRow{
		{Status: "ok", Side: "asset", Type: "deposit", Name: "玉山活存", Group: "liquid", Currency: "TWD", Value: 100000, Date: "2026-09-01"},
		{Status: "ok", Side: "liability", Type: "loan", Name: "房貸", Group: "hard", Currency: "TWD", Value: 3000000, Date: "2026-09-01", Lender: "國泰世華"},
		{Status: "ok", Side: "asset", Type: "fund", Name: "元大台灣50", Group: "growth", Currency: "TWD", Value: 640000, Date: "2026-09-01",
			FundCode: "0050", FundPlatform: "券商", FundMonthlyAmount: &monthly, FundNextContributionDate: "2026-10-06"},
		{Status: "duplicate", Side: "asset", Type: "deposit", Name: "重複", Group: "liquid", Value: 1},
	}

	applied := applyWealthImportRows(fake, rows)
	if applied != 3 {
		t.Fatalf("applied = %d, want 3 (duplicate row skipped)", applied)
	}
	if len(fake.createdAsset) != 3 || len(fake.snapshots) != 3 {
		t.Fatalf("createdAsset=%d snapshots=%d, want 3 and 3", len(fake.createdAsset), len(fake.snapshots))
	}
	if len(fake.createdFund) != 1 || fake.createdFund[0].Code != "0050" || fake.createdFund[0].MonthlyAmount == nil || *fake.createdFund[0].MonthlyAmount != 12000 {
		t.Errorf("createdFund = %+v, want one entry with Code=0050 MonthlyAmount=12000", fake.createdFund)
	}
	for _, a := range fake.createdAsset {
		if a.Source != "import" {
			t.Errorf("created asset Source = %q, want \"import\"", a.Source)
		}
	}
	for _, s := range fake.snapshots {
		if s.Source != "import" {
			t.Errorf("snapshot Source = %q, want \"import\"", s.Source)
		}
	}
	if rows[0].Status != "applied" || rows[1].Status != "applied" || rows[2].Status != "applied" {
		t.Errorf("rows[0/1/2].Status = %q/%q/%q, want applied/applied/applied", rows[0].Status, rows[1].Status, rows[2].Status)
	}
	if rows[3].Status != "duplicate" {
		t.Errorf("rows[3].Status = %q, want unchanged duplicate", rows[3].Status)
	}
	if len(fake.createdLoan) != 1 || fake.createdLoan[0].Lender != "國泰世華" {
		t.Errorf("createdLoan = %+v, want one entry with Lender 國泰世華", fake.createdLoan)
	}
}

func TestHandleWealthImportEndToEnd(t *testing.T) {
	fakeDBRows := &fakeDB{}
	fake := &fakeWealthImportDB{}
	s := &Server{db: fakeDBRows, wealthDB: fake, password: "pw"}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("POST /api/wealth/import", s.requireWritable(s.requireAuth(s.handleWealthImport)))

	loginBody, _ := json.Marshal(map[string]string{"password": "pw"})
	loginRec := httptest.NewRecorder()
	s.mux.ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(loginBody)))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d", loginRec.Code)
	}
	cookies := loginRec.Result().Cookies()

	csvText := "side,type,name,group,venue,currency,value,date\n" +
		"asset,deposit,活存,liquid,,TWD,50000,2026-09-01\n"
	body, _ := json.Marshal(wealthImportRequest{CSV: csvText, DryRun: false})
	req := httptest.NewRequest(http.MethodPost, "/api/wealth/import", bytes.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp wealthImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Applied != 1 || len(resp.Rows) != 1 || resp.Rows[0].Status != "applied" {
		t.Errorf("resp = %+v, want 1 applied row", resp)
	}
	if len(fake.snapshots) != 1 || fake.snapshots[0].Value != 50000 {
		t.Errorf("snapshots = %+v, want one snapshot valued 50000", fake.snapshots)
	}
}

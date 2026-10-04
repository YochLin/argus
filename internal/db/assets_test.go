package db

import (
	"errors"
	"testing"
)

func TestCreateDepositAssetAndListWithValue(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateDepositAsset(
		NewAsset{Side: "asset", Type: "deposit", Name: "玉山活存", AssetGroup: "liquid"},
		DepositDetails{Bank: "玉山銀行"},
	)
	if err != nil {
		t.Fatalf("CreateDepositAsset() error = %v", err)
	}

	det, err := d.GetDepositDetails(id)
	if err != nil || det == nil || det.Bank != "玉山銀行" {
		t.Fatalf("GetDepositDetails() = %+v, %v; want bank 玉山銀行", det, err)
	}

	// A freshly created asset has no snapshot yet — must render as "no
	// value", not a fabricated 0 (see AssetWithValue's doc comment).
	assets, err := d.ListAssetsWithValue(false)
	if err != nil {
		t.Fatalf("ListAssetsWithValue() error = %v", err)
	}
	if len(assets) != 1 || assets[0].Value != nil {
		t.Fatalf("ListAssetsWithValue() = %+v, want 1 asset with nil Value", assets)
	}

	if err := d.UpsertAssetSnapshot(AssetSnapshot{AssetID: id, Date: "2026-09-15", Value: 100000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() error = %v", err)
	}

	assets, err = d.ListAssetsWithValue(false)
	if err != nil || len(assets) != 1 || assets[0].Value == nil || *assets[0].Value != 100000 {
		t.Fatalf("ListAssetsWithValue() after snapshot = %+v, %v; want value 100000", assets, err)
	}

	// A same-day correction overwrites, it doesn't append a second row.
	if err := d.UpsertAssetSnapshot(AssetSnapshot{AssetID: id, Date: "2026-09-15", Value: 105000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() (same-day correction) error = %v", err)
	}
	latest, err := d.GetLatestAssetSnapshot(id)
	if err != nil || latest == nil || latest.Value != 105000 {
		t.Fatalf("GetLatestAssetSnapshot() = %+v, %v; want value 105000", latest, err)
	}
}

func TestCreateLoanAssetNullableFields(t *testing.T) {
	d := newTestDB(t)

	rate := 2.1
	id, err := d.CreateLoanAsset(
		NewAsset{Side: "liability", Type: "loan", Name: "房貸", AssetGroup: "hard"},
		LoanDetails{Lender: "台北富邦", RatePct: &rate},
	)
	if err != nil {
		t.Fatalf("CreateLoanAsset() error = %v", err)
	}

	det, err := d.GetLoanDetails(id)
	if err != nil || det == nil {
		t.Fatalf("GetLoanDetails() = %+v, %v", det, err)
	}
	if det.RatePct == nil || *det.RatePct != 2.1 {
		t.Errorf("GetLoanDetails().RatePct = %v, want 2.1", det.RatePct)
	}
	if det.OriginalPrincipal != nil {
		t.Errorf("GetLoanDetails().OriginalPrincipal = %v, want nil (never set)", det.OriginalPrincipal)
	}
}

// TestArchiveAssetSoftDeletesAndIsIdempotent pins §9.1's "no hard delete"
// rule: archiving hides an asset from the default list but never removes
// its row or snapshot history, and archiving twice is a no-op rather than
// an error.
func TestArchiveAssetSoftDeletesAndIsIdempotent(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateAsset(NewAsset{Side: "asset", Type: "other", Name: "房子", AssetGroup: "hard"})
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	if err := d.ArchiveAsset(id); err != nil {
		t.Fatalf("ArchiveAsset() error = %v", err)
	}
	if err := d.ArchiveAsset(id); err != nil {
		t.Fatalf("ArchiveAsset() (second call) error = %v", err)
	}

	active, err := d.ListAssets(false)
	if err != nil || len(active) != 0 {
		t.Fatalf("ListAssets(false) after archive = %+v, %v; want empty", active, err)
	}

	all, err := d.ListAssets(true)
	if err != nil || len(all) != 1 || all[0].ArchivedAt == "" {
		t.Fatalf("ListAssets(true) after archive = %+v, %v; want 1 asset with ArchivedAt set", all, err)
	}
}

func TestUnarchiveAssetRestoresAndIsIdempotent(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateAsset(NewAsset{Side: "asset", Type: "estate", Name: "房子", AssetGroup: "hard"})
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	if err := d.ArchiveAsset(id); err != nil {
		t.Fatalf("ArchiveAsset() error = %v", err)
	}
	for i := 0; i < 2; i++ { // second call is a no-op, not an error
		if err := d.UnarchiveAsset(id); err != nil {
			t.Fatalf("UnarchiveAsset() call %d error = %v", i+1, err)
		}
	}
	if err := d.UnarchiveAsset(999); err != nil {
		t.Errorf("UnarchiveAsset(nonexistent) error = %v, want nil", err)
	}
	active, err := d.ListAssets(false)
	if err != nil || len(active) != 1 || active[0].ArchivedAt != "" {
		t.Fatalf("ListAssets(false) after unarchive = %+v, %v; want the asset back", active, err)
	}
}

// TestUpdateAssetEditsDescriptiveFieldsAndDetails pins what an edit may and
// may not touch: name/group/venue and the matching detail row change, while
// the value history, type and currency stay put.
func TestUpdateAssetEditsDescriptiveFieldsAndDetails(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateDepositAsset(
		NewAsset{Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid", Venue: "舊銀行", Currency: "USD"},
		DepositDetails{Bank: "舊銀行", AccountNote: "舊備註"},
	)
	if err != nil {
		t.Fatalf("CreateDepositAsset() error = %v", err)
	}
	if err := d.UpsertAssetSnapshot(AssetSnapshot{AssetID: id, Date: "2026-09-01", Value: 1000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() error = %v", err)
	}

	if err := d.UpdateAsset(id, AssetEdit{
		Name: "外幣活存", AssetGroup: "growth", Venue: "",
		Deposit: &DepositDetails{Bank: "新銀行"},
	}); err != nil {
		t.Fatalf("UpdateAsset() error = %v", err)
	}

	a, err := d.GetAsset(id)
	if err != nil || a == nil {
		t.Fatalf("GetAsset() = %+v, %v", a, err)
	}
	if a.Name != "外幣活存" || a.AssetGroup != "growth" || a.Venue != "" {
		t.Errorf("asset = %+v, want name 外幣活存 / group growth / venue cleared", a)
	}
	if a.Type != "deposit" || a.Currency != "USD" || a.Side != "asset" {
		t.Errorf("asset = %+v, type/currency/side must not change", a)
	}
	det, _ := d.GetDepositDetails(id)
	if det == nil || det.Bank != "新銀行" || det.AccountNote != "" {
		t.Errorf("deposit details = %+v, want bank 新銀行 and note cleared (wholesale replace)", det)
	}
	list, _ := d.ListAssetsWithValue(false)
	if len(list) != 1 || list[0].Value == nil || *list[0].Value != 1000 {
		t.Errorf("value after edit = %+v, want 1000 untouched", list)
	}

	// Details omitted → the detail row is left alone.
	if err := d.UpdateAsset(id, AssetEdit{Name: "再改名", AssetGroup: "growth"}); err != nil {
		t.Fatalf("UpdateAsset(no details) error = %v", err)
	}
	if det, _ := d.GetDepositDetails(id); det == nil || det.Bank != "新銀行" {
		t.Errorf("deposit details = %+v after a details-less edit, want bank 新銀行 kept", det)
	}
}

func TestUpdateAssetLoanDetails(t *testing.T) {
	d := newTestDB(t)

	rate := 2.1
	id, err := d.CreateLoanAsset(
		NewAsset{Side: "liability", Type: "loan", Name: "房貸", AssetGroup: "hard"},
		LoanDetails{Lender: "富邦", RatePct: &rate},
	)
	if err != nil {
		t.Fatalf("CreateLoanAsset() error = %v", err)
	}

	newRate, months := 2.4, int64(240)
	if err := d.UpdateAsset(id, AssetEdit{
		Name: "房貸", AssetGroup: "hard",
		Loan: &LoanDetails{Lender: "國泰", RatePct: &newRate, RemainingMonths: &months},
	}); err != nil {
		t.Fatalf("UpdateAsset() error = %v", err)
	}
	det, _ := d.GetLoanDetails(id)
	if det == nil || det.Lender != "國泰" || det.RatePct == nil || *det.RatePct != 2.4 ||
		det.RemainingMonths == nil || *det.RemainingMonths != 240 {
		t.Errorf("loan details = %+v, want lender 國泰 / 2.4%% / 240 months", det)
	}
}

func TestUpdateAssetErrors(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpdateAsset(999, AssetEdit{Name: "x", AssetGroup: "liquid"}); !errors.Is(err, ErrAssetNotFound) {
		t.Errorf("UpdateAsset(unknown id) = %v, want ErrAssetNotFound", err)
	}

	// A deposit has no loan_details row: the edit must fail as a whole, not
	// half-apply the name change.
	id, err := d.CreateDepositAsset(NewAsset{Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid"}, DepositDetails{})
	if err != nil {
		t.Fatalf("CreateDepositAsset() error = %v", err)
	}
	err = d.UpdateAsset(id, AssetEdit{Name: "改了", AssetGroup: "liquid", Loan: &LoanDetails{Lender: "x"}})
	if !errors.Is(err, ErrAssetNoDetails) {
		t.Errorf("UpdateAsset(loan block on a deposit) = %v, want ErrAssetNoDetails", err)
	}
	if a, _ := d.GetAsset(id); a == nil || a.Name != "活存" {
		t.Errorf("asset = %+v after a failed edit, want name 活存 (rolled back)", a)
	}
}

// TestListAssetsValueAsOfUsesLatestSnapshotOnOrBeforeDate pins the
// historical-lookup semantics wealth_home.go's YTD/MoM depend on: a date
// before the asset's first snapshot gets no value (it didn't exist yet, not
// a data gap), and a date between two snapshots gets the earlier one, not
// the globally-latest one ListAssetsWithValue would return.
func TestListAssetsValueAsOfUsesLatestSnapshotOnOrBeforeDate(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateAsset(NewAsset{Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid"})
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	if err := d.UpsertAssetSnapshot(AssetSnapshot{AssetID: id, Date: "2026-06-01", Value: 100000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() error = %v", err)
	}
	if err := d.UpsertAssetSnapshot(AssetSnapshot{AssetID: id, Date: "2026-09-01", Value: 150000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() error = %v", err)
	}

	before, err := d.ListAssetsValueAsOf("2026-01-01", false)
	if err != nil || len(before) != 1 || before[0].Value != nil {
		t.Fatalf("ListAssetsValueAsOf(before first snapshot) = %+v, %v; want Value nil", before, err)
	}

	mid, err := d.ListAssetsValueAsOf("2026-07-15", false)
	if err != nil || len(mid) != 1 || mid[0].Value == nil || *mid[0].Value != 100000 {
		t.Fatalf("ListAssetsValueAsOf(between snapshots) = %+v, %v; want value 100000", mid, err)
	}

	after, err := d.ListAssetsValueAsOf("2026-12-31", false)
	if err != nil || len(after) != 1 || after[0].Value == nil || *after[0].Value != 150000 {
		t.Fatalf("ListAssetsValueAsOf(after latest snapshot) = %+v, %v; want value 150000", after, err)
	}
}

// TestListAssetsValueAsOfKeepsAssetUntilItsArchiveDate pins that archiving an
// asset (a sold house, a closed account) doesn't rewrite the past: it still
// counts on every date before the day it was archived, and drops out from
// that day on. includeArchived=true always returns it.
func TestListAssetsValueAsOfKeepsAssetUntilItsArchiveDate(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateAsset(NewAsset{Side: "asset", Type: "estate", Name: "房子", AssetGroup: "hard"})
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	if err := d.UpsertAssetSnapshot(AssetSnapshot{AssetID: id, Date: "2026-06-01", Value: 8000000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() error = %v", err)
	}
	// Pin archived_at to a known day; ArchiveAsset stamps the real clock.
	if _, err := d.conn.Exec(`UPDATE assets SET archived_at = '2026-09-10 08:00:00' WHERE id = ?`, id); err != nil {
		t.Fatalf("set archived_at: %v", err)
	}

	for _, tc := range []struct {
		asOf        string
		wantPresent bool
	}{
		{"2026-07-01", true},  // held then, archived later
		{"2026-09-09", true},  // the day before archive
		{"2026-09-10", false}, // the archive day itself
		{"2026-12-31", false},
	} {
		got, err := d.ListAssetsValueAsOf(tc.asOf, false)
		if err != nil {
			t.Fatalf("ListAssetsValueAsOf(%s) error = %v", tc.asOf, err)
		}
		if present := len(got) == 1; present != tc.wantPresent {
			t.Errorf("ListAssetsValueAsOf(%s, false) = %+v, want present=%v", tc.asOf, got, tc.wantPresent)
		}
		if tc.wantPresent && (got[0].Value == nil || *got[0].Value != 8000000) {
			t.Errorf("ListAssetsValueAsOf(%s, false) value = %+v, want 8000000", tc.asOf, got[0].Value)
		}
	}

	all, err := d.ListAssetsValueAsOf("2026-12-31", true)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAssetsValueAsOf(includeArchived) = %+v, %v; want 1 asset", all, err)
	}
}

func TestGetAssetMissingReturnsNil(t *testing.T) {
	d := newTestDB(t)

	a, err := d.GetAsset(999)
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if a != nil {
		t.Errorf("GetAsset(999) = %+v, want nil", a)
	}
}

package db

import "testing"

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

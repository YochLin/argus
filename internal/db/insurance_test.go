package db

import "testing"

func TestCreateInsuranceAssetAndRead(t *testing.T) {
	d := newTestDB(t)

	premium := 68000.0
	years := int64(20)
	id, err := d.CreateInsuranceAsset(
		NewAsset{Name: "終身壽險", Venue: "國泰人壽"},
		InsuranceDetails{Insurer: "國泰人壽", Insured: "本人", AnnualPremium: &premium, PremiumYears: &years},
		InsuranceCoverage{Kind: "life", Amount: 5000000},
	)
	if err != nil {
		t.Fatalf("CreateInsuranceAsset: %v", err)
	}

	a, err := d.GetAsset(id)
	if err != nil || a == nil {
		t.Fatalf("GetAsset: %v, %v", a, err)
	}
	if a.Side != "asset" || a.Type != "insurance" || a.Venue != "國泰人壽" {
		t.Errorf("asset = %+v, want side=asset type=insurance venue=國泰人壽", a)
	}

	det, err := d.GetInsuranceDetails(id)
	if err != nil || det == nil {
		t.Fatalf("GetInsuranceDetails: %v, %v", det, err)
	}
	if det.Insurer != "國泰人壽" || det.Insured != "本人" || det.AnnualPremium == nil || *det.AnnualPremium != premium {
		t.Errorf("details = %+v, want insurer/insured/premium set", det)
	}
	if det.PremiumYears == nil || *det.PremiumYears != years {
		t.Errorf("details.PremiumYears = %v, want %d", det.PremiumYears, years)
	}

	coverages, err := d.ListInsuranceCoverages()
	if err != nil {
		t.Fatalf("ListInsuranceCoverages: %v", err)
	}
	if len(coverages) != 1 {
		t.Fatalf("len(coverages) = %d, want 1", len(coverages))
	}
	if coverages[0].AssetID != id || coverages[0].Kind != "life" || coverages[0].Amount != 5000000 || coverages[0].PerPeriod != "" {
		t.Errorf("coverage = %+v, want assetID=%d kind=life amount=5000000 perPeriod=\"\"", coverages[0], id)
	}
}

func TestCreateInsuranceAssetPerPeriod(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateInsuranceAsset(
		NewAsset{Name: "住院醫療"},
		InsuranceDetails{Insurer: "南山人壽"},
		InsuranceCoverage{Kind: "hospital", Amount: 3000, PerPeriod: "day"},
	)
	if err != nil {
		t.Fatalf("CreateInsuranceAsset: %v", err)
	}

	coverages, err := d.ListInsuranceCoverages()
	if err != nil {
		t.Fatalf("ListInsuranceCoverages: %v", err)
	}
	var got *InsuranceCoverage
	for i := range coverages {
		if coverages[i].AssetID == id {
			got = &coverages[i]
		}
	}
	if got == nil {
		t.Fatal("coverage for created asset not found")
	}
	if got.PerPeriod != "day" {
		t.Errorf("PerPeriod = %q, want day", got.PerPeriod)
	}
}

func TestGetInsuranceDetailsMissing(t *testing.T) {
	d := newTestDB(t)
	det, err := d.GetInsuranceDetails(999)
	if err != nil {
		t.Fatalf("GetInsuranceDetails: %v", err)
	}
	if det != nil {
		t.Errorf("GetInsuranceDetails(missing) = %+v, want nil", det)
	}
}

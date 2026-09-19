package db

import "testing"

func TestCreateFundAssetAndRead(t *testing.T) {
	d := newTestDB(t)

	monthly := 12000.0
	id, err := d.CreateFundAsset(
		NewAsset{Name: "元大台灣50", Venue: "券商"},
		FundDetails{Code: "0050", Platform: "券商", MonthlyAmount: &monthly, NextContributionDate: "2026-10-06"},
	)
	if err != nil {
		t.Fatalf("CreateFundAsset: %v", err)
	}

	a, err := d.GetAsset(id)
	if err != nil || a == nil {
		t.Fatalf("GetAsset: %v, %v", a, err)
	}
	if a.Side != "asset" || a.Type != "fund" {
		t.Errorf("asset = %+v, want side=asset type=fund", a)
	}

	det, err := d.GetFundDetails(id)
	if err != nil || det == nil {
		t.Fatalf("GetFundDetails: %v, %v", det, err)
	}
	if det.Code != "0050" || det.Platform != "券商" || det.NextContributionDate != "2026-10-06" {
		t.Errorf("details = %+v, want code=0050 platform=券商 next=2026-10-06", det)
	}
	if det.MonthlyAmount == nil || *det.MonthlyAmount != monthly {
		t.Errorf("details.MonthlyAmount = %v, want %v", det.MonthlyAmount, monthly)
	}
}

func TestCreateFundAssetStopped(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreateFundAsset(
		NewAsset{Name: "富邦科技（單筆）"},
		FundDetails{Code: "0052", Platform: "券商"},
	)
	if err != nil {
		t.Fatalf("CreateFundAsset: %v", err)
	}

	det, err := d.GetFundDetails(id)
	if err != nil || det == nil {
		t.Fatalf("GetFundDetails: %v, %v", det, err)
	}
	if det.MonthlyAmount != nil {
		t.Errorf("MonthlyAmount = %v, want nil (lump-sum/stopped)", *det.MonthlyAmount)
	}
}

func TestGetFundDetailsMissing(t *testing.T) {
	d := newTestDB(t)
	det, err := d.GetFundDetails(999)
	if err != nil {
		t.Fatalf("GetFundDetails: %v", err)
	}
	if det != nil {
		t.Errorf("GetFundDetails(missing) = %+v, want nil", det)
	}
}

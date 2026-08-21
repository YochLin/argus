package db

import "testing"

func TestFundamentalSnapshotRoundTrip(t *testing.T) {
	d := newTestDB(t)

	if got, err := d.GetFundamentalSnapshot("AAPL"); err != nil || got != nil {
		t.Fatalf("GetFundamentalSnapshot() before save = %v, %v, want nil, nil", got, err)
	}

	pct := 87.5
	quality := 1.2
	if err := d.SaveFundamentalSnapshot(FundamentalSnapshot{
		Ticker:            "AAPL",
		EPSAnnual:         6.13,
		PERatio:           28.4,
		PEPercentile:      &pct,
		OCF:               110000,
		NetIncome:         93000,
		CashFlowQuality:   &quality,
		AsOfFiscalYearEnd: "2025-09-27",
	}); err != nil {
		t.Fatalf("SaveFundamentalSnapshot() error = %v", err)
	}

	got, err := d.GetFundamentalSnapshot("AAPL")
	if err != nil {
		t.Fatalf("GetFundamentalSnapshot() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetFundamentalSnapshot() = nil, want a row")
	}
	if got.EPSAnnual != 6.13 || got.PERatio != 28.4 || got.AsOfFiscalYearEnd != "2025-09-27" {
		t.Errorf("GetFundamentalSnapshot() = %+v, want EPSAnnual=6.13 PERatio=28.4 AsOfFiscalYearEnd=2025-09-27", got)
	}
	if got.PEPercentile == nil || *got.PEPercentile != 87.5 {
		t.Errorf("PEPercentile = %v, want 87.5", got.PEPercentile)
	}
	if got.CashFlowQuality == nil || *got.CashFlowQuality != 1.2 {
		t.Errorf("CashFlowQuality = %v, want 1.2", got.CashFlowQuality)
	}
	if got.FetchedAt == "" {
		t.Error("FetchedAt = \"\", want a timestamp")
	}

	// Upsert: a second save with nil percentile/quality must clear them, not
	// leave the old values behind.
	if err := d.SaveFundamentalSnapshot(FundamentalSnapshot{Ticker: "AAPL", EPSAnnual: 7.0}); err != nil {
		t.Fatalf("SaveFundamentalSnapshot() (upsert) error = %v", err)
	}
	got, err = d.GetFundamentalSnapshot("AAPL")
	if err != nil {
		t.Fatalf("GetFundamentalSnapshot() after upsert error = %v", err)
	}
	if got.EPSAnnual != 7.0 {
		t.Errorf("EPSAnnual after upsert = %v, want 7.0", got.EPSAnnual)
	}
	if got.PEPercentile != nil {
		t.Errorf("PEPercentile after upsert = %v, want nil (cleared)", *got.PEPercentile)
	}
	if got.CashFlowQuality != nil {
		t.Errorf("CashFlowQuality after upsert = %v, want nil (cleared)", *got.CashFlowQuality)
	}
}

package service

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

func TestComputeMAEMFE(t *testing.T) {
	legs := []db.Transaction{
		makeTx("AAPL", "BUY", 10, 100, 0, 0, "2026-06-01"),
		makeTx("AAPL", "SELL", 10, 120, 0, 200, "2026-06-10"),
	}

	d1, _ := time.Parse("2006-01-02", "2026-06-01")
	d2, _ := time.Parse("2006-01-02", "2026-06-05")
	d3, _ := time.Parse("2006-01-02", "2026-06-10")

	candles := []data.Candle{
		{Date: d1, High: 105, Low: 95},  // low -5%, high +5%
		{Date: d2, High: 130, Low: 90},  // low -10%, high +30%
		{Date: d3, High: 125, Low: 115}, // low +15%, high +25%
	}

	res := ComputeMAEMFE(candles, legs, d1, d3)
	if !res.OK {
		t.Fatalf("ComputeMAEMFE res.OK = false, want true")
	}
	if res.MAEPct != -10 {
		t.Errorf("res.MAEPct = %v, want -10", res.MAEPct)
	}
	if res.MFEPct != 30 {
		t.Errorf("res.MFEPct = %v, want 30", res.MFEPct)
	}

	ret, ok := RoundReturnPct(legs)
	if !ok || ret != 20.0 {
		t.Errorf("RoundReturnPct = %v (ok=%v), want 20.0", ret, ok)
	}
}

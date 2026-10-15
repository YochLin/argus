package db

import (
	"testing"

	"argus/internal/market"
)

func TestSaveNetWorthSnapshotUpsert(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-07-05", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	// Same date should replace, not conflict.
	if err := d.SaveNetWorthSnapshot("2026-07-05", market.US, 1200); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() (upsert) error = %v", err)
	}
}

func TestGetLatestNetWorthEmpty(t *testing.T) {
	d := newTestDB(t)

	_, _, ok, err := d.GetLatestNetWorth(market.US)
	if err != nil {
		t.Fatalf("GetLatestNetWorth() error = %v", err)
	}
	if ok {
		t.Error("GetLatestNetWorth() on an empty table should return ok=false")
	}
}

func TestGetLatestNetWorth(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-07-01", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-08", market.US, 1100); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}

	date, total, ok, err := d.GetLatestNetWorth(market.US)
	if err != nil || !ok || date != "2026-07-08" || total != 1100 {
		t.Errorf("GetLatestNetWorth() = %q, %v, %v, %v; want \"2026-07-08\", 1100, true, nil", date, total, ok, err)
	}
}

func TestGetNetWorthOnOrBefore(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-07-01", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-03", market.US, 1050); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}

	t.Run("exact date match", func(t *testing.T) {
		total, ok, err := d.GetNetWorthOnOrBefore("2026-07-03", market.US)
		if err != nil || !ok || total != 1050 {
			t.Errorf("GetNetWorthOnOrBefore(2026-07-03) = %v, %v, %v; want 1050, true, nil", total, ok, err)
		}
	})

	t.Run("falls back to most recent prior date", func(t *testing.T) {
		// 2026-07-02 has no snapshot (e.g. a weekend) — should fall back to 07-01.
		total, ok, err := d.GetNetWorthOnOrBefore("2026-07-02", market.US)
		if err != nil || !ok || total != 1000 {
			t.Errorf("GetNetWorthOnOrBefore(2026-07-02) = %v, %v, %v; want 1000, true, nil", total, ok, err)
		}
	})

	t.Run("no snapshot on or before date", func(t *testing.T) {
		_, ok, err := d.GetNetWorthOnOrBefore("2026-06-01", market.US)
		if err != nil {
			t.Fatalf("GetNetWorthOnOrBefore() error = %v", err)
		}
		if ok {
			t.Error("GetNetWorthOnOrBefore() before any snapshot should return ok=false")
		}
	})
}

func TestGetNetWorthRange(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveNetWorthSnapshot("2026-06-30", market.US, 900); err != nil { // outside range
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-01", market.US, 1000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-07-15", market.US, 1100); err != nil {
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}
	if err := d.SaveNetWorthSnapshot("2026-08-01", market.US, 1200); err != nil { // outside range
		t.Fatalf("SaveNetWorthSnapshot() error = %v", err)
	}

	got, err := d.GetNetWorthRange("2026-07-01", "2026-07-31", market.US)
	if err != nil {
		t.Fatalf("GetNetWorthRange() error = %v", err)
	}
	want := []NetWorthPoint{{Date: "2026-07-01", Total: 1000}, {Date: "2026-07-15", Total: 1100}}
	if len(got) != len(want) {
		t.Fatalf("GetNetWorthRange() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetNetWorthRange()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGetNetWorthRangeEmpty(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetNetWorthRange("2026-07-01", "2026-07-31", market.US)
	if err != nil {
		t.Fatalf("GetNetWorthRange() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetNetWorthRange() on empty table = %+v, want empty", got)
	}
}

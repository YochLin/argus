package web

import (
	"testing"

	"argus/internal/db"
	"argus/internal/market"
)

// TestBuildMonthly_MonthBoundaryAndNullMonths pins the two documented
// behaviors (docs/phase-8-trader-analytics.md §5.1): a cross-year fixture
// splits correctly into separate YearlyPnL rows, and a month with no data
// day at all comes back nil, not 0 — distinguishing "no data yet" from a
// real breakeven month.
func TestBuildMonthly_MonthBoundaryAndNullMonths(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2025-12-15"),
		},
		snapshots: []db.DailySnapshot{
			snap("AAPL", "2025-12-15", 100),
			snap("AAPL", "2025-12-31", 110), // +10/share * 10 = +100 in December
			snap("AAPL", "2026-01-02", 120), // +10/share * 10 = +100 in January (no data in between)
		},
	}

	got, err := buildMonthly(fdb, market.US)
	if err != nil {
		t.Fatalf("buildMonthly() error = %v", err)
	}
	if len(got.Years) != 2 {
		t.Fatalf("Years = %+v, want 2 rows (2025, 2026)", got.Years)
	}

	y2025 := got.Years[0]
	if y2025.Year != 2025 {
		t.Fatalf("Years[0].Year = %d, want 2025", y2025.Year)
	}
	dec := y2025.Months[11]
	if dec == nil || *dec != 100 {
		t.Errorf("Dec 2025 = %v, want 100", dec)
	}
	for i, m := range y2025.Months {
		if i == 11 {
			continue
		}
		if m != nil {
			t.Errorf("2025 month %d = %v, want nil (no data)", i+1, *m)
		}
	}
	if y2025.Total != 100 {
		t.Errorf("2025 Total = %v, want 100", y2025.Total)
	}

	y2026 := got.Years[1]
	if y2026.Year != 2026 {
		t.Fatalf("Years[1].Year = %d, want 2026", y2026.Year)
	}
	jan := y2026.Months[0]
	if jan == nil || *jan != 100 {
		t.Errorf("Jan 2026 = %v, want 100", jan)
	}
	if y2026.Months[1] != nil {
		t.Errorf("Feb 2026 = %v, want nil", *y2026.Months[1])
	}
}

func TestBuildMonthly_Empty(t *testing.T) {
	got, err := buildMonthly(&fakeDB{}, market.US)
	if err != nil {
		t.Fatalf("buildMonthly() error = %v", err)
	}
	if len(got.Years) != 0 {
		t.Errorf("Years = %+v, want empty", got.Years)
	}
}

func TestBuildMonthly_MarketFilter(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-01-01"),
			tx("2330", "BUY", 1000, 900, "2026-01-01"),
		},
		snapshots: []db.DailySnapshot{
			snap("AAPL", "2026-01-01", 100),
			snap("AAPL", "2026-01-02", 105),
			snap("2330", "2026-01-01", 900),
			snap("2330", "2026-01-02", 910),
		},
	}

	us, err := buildMonthly(fdb, market.US)
	if err != nil {
		t.Fatalf("buildMonthly(US) error = %v", err)
	}
	if len(us.Years) != 1 || *us.Years[0].Months[0] != 50 { // (105-100)*10
		t.Errorf("buildMonthly(US) = %+v, want Jan 2026 = 50", us.Years)
	}

	tw, err := buildMonthly(fdb, market.TW)
	if err != nil {
		t.Fatalf("buildMonthly(TW) error = %v", err)
	}
	if len(tw.Years) != 1 || *tw.Years[0].Months[0] != 10000 { // (910-900)*1000
		t.Errorf("buildMonthly(TW) = %+v, want Jan 2026 = 10000", tw.Years)
	}
}

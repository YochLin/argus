package web

import (
	"errors"
	"testing"
	"time"

	"argus/internal/db"
)

func TestMonthBounds(t *testing.T) {
	tests := []struct {
		month     string
		wantStart string
		wantEnd   string
		wantOK    bool
	}{
		{"2026-07", "2026-07-01", "2026-07-31", true},
		{"2026-02", "2026-02-01", "2026-02-28", true}, // 2026 not a leap year
		{"2028-02", "2028-02-01", "2028-02-29", true}, // 2028 is a leap year
		{"2026-12", "2026-12-01", "2026-12-31", true}, // year-end doesn't wrap
		{"not-a-month", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		start, end, ok := monthBounds(tt.month)
		if ok != tt.wantOK || start != tt.wantStart || end != tt.wantEnd {
			t.Errorf("monthBounds(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.month, start, end, ok, tt.wantStart, tt.wantEnd, tt.wantOK)
		}
	}
}

func TestBuildCalendar_Empty(t *testing.T) {
	got, err := buildCalendar(&fakeDB{}, "2026-07")
	if err != nil {
		t.Fatalf("buildCalendar() error = %v", err)
	}
	if got.Month != "2026-07" {
		t.Errorf("Month = %q, want 2026-07", got.Month)
	}
	if got.Days == nil || len(got.Days) != 0 {
		t.Errorf("Days = %v, want empty non-nil slice", got.Days)
	}
	if got.Transactions == nil || len(got.Transactions) != 0 {
		t.Errorf("Transactions = %v, want empty non-nil slice", got.Transactions)
	}
}

func TestBuildCalendar_InvalidMonthFallsBackToCurrent(t *testing.T) {
	got, err := buildCalendar(&fakeDB{}, "garbage")
	if err != nil {
		t.Fatalf("buildCalendar() error = %v", err)
	}
	wantMonth := time.Now().Format("2006-01")
	if got.Month != wantMonth {
		t.Errorf("Month = %q, want current month %q", got.Month, wantMonth)
	}
}

func TestBuildCalendar_DaysAndTransactionsScopedToMonth(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-29"),
			tx("AAPL", "BUY", 5, 110, "2026-07-01"),
			tx("AAPL", "SELL", 5, 130, "2026-07-15"),
			tx("AAPL", "BUY", 3, 140, "2026-08-01"), // outside the requested month
		},
		snapshots: []db.DailySnapshot{
			snap("AAPL", "2026-06-29", 100),
			snap("AAPL", "2026-06-30", 105),
			snap("AAPL", "2026-07-01", 110),
			snap("AAPL", "2026-07-15", 130),
		},
	}

	got, err := buildCalendar(fdb, "2026-07")
	if err != nil {
		t.Fatalf("buildCalendar() error = %v", err)
	}

	for _, d := range got.Days {
		if d.Date < "2026-07-01" || d.Date > "2026-07-31" {
			t.Errorf("Days contains out-of-month date %s", d.Date)
		}
	}
	if len(got.Days) == 0 {
		t.Error("Days is empty, want at least the 07-01/07-15 mark-to-market entries")
	}

	if len(got.Transactions) != 2 {
		t.Fatalf("Transactions = %+v, want exactly the 2 July rows (June/August excluded)", got.Transactions)
	}
	for _, txr := range got.Transactions {
		if txr.Date < "2026-07-01" || txr.Date > "2026-07-31" {
			t.Errorf("Transactions contains out-of-month date %s", txr.Date)
		}
	}
}

func TestBuildCalendar_MonthBeforeFirstTradeHasNoDays(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-07-01"),
		},
		snapshots: []db.DailySnapshot{
			snap("AAPL", "2026-07-01", 100),
		},
	}

	got, err := buildCalendar(fdb, "2026-05")
	if err != nil {
		t.Fatalf("buildCalendar() error = %v", err)
	}
	if len(got.Days) != 0 {
		t.Errorf("Days = %v, want empty (month predates the first trade)", got.Days)
	}
	if len(got.Transactions) != 0 {
		t.Errorf("Transactions = %v, want empty", got.Transactions)
	}
}

func TestBuildCalendar_SnapshotErrorPropagates(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-07-01"),
		},
		snapshotsErr: errors.New("provider down"),
	}
	if _, err := buildCalendar(fdb, "2026-07"); err == nil {
		t.Error("buildCalendar() error = nil, want the snapshot fetch error to propagate")
	}
}

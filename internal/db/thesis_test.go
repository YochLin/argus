package db

import "testing"

func TestGetThesisUnsetTickerIsNotFound(t *testing.T) {
	d := newTestDB(t)

	_, ok, err := d.GetThesis("AAPL")
	if err != nil {
		t.Fatalf("GetThesis() error = %v", err)
	}
	if ok {
		t.Error("GetThesis() on a ticker with no thesis should return ok=false")
	}
}

func TestThesisRoundTripAndSameDayUpsert(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetThesis("AAPL", "long-term compounder, services growth"); err != nil {
		t.Fatalf("SetThesis() error = %v", err)
	}
	got, ok, err := d.GetThesis("AAPL")
	if err != nil || !ok || got != "long-term compounder, services growth" {
		t.Errorf("GetThesis() = %q, %v, %v; want the set thesis, true, nil", got, ok, err)
	}

	// A second SetThesis on the same calendar day upserts in place rather
	// than appending a new entry (migration 18's "one edit per ticker per
	// day" rule).
	if err := d.SetThesis("AAPL", "changed my mind: AI capex risk"); err != nil {
		t.Fatalf("SetThesis() (same-day upsert) error = %v", err)
	}
	got, ok, err = d.GetThesis("AAPL")
	if err != nil || !ok || got != "changed my mind: AI capex risk" {
		t.Errorf("GetThesis() after same-day upsert = %q, %v, %v; want the new thesis, true, nil", got, ok, err)
	}

	entries, err := d.GetThesisEntriesInRange("AAPL", "2000-01-01", "")
	if err != nil {
		t.Fatalf("GetThesisEntriesInRange() error = %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("GetThesisEntriesInRange() = %d entries, want 1 (same-day edits collapse into one row)", len(entries))
	}
}

// TestThesisEntriesAppendAcrossDays pins the "cross-day append" half of
// migration 18's rule: a thesis written on a different calendar day is a
// new row, not an overwrite of the previous day's. Backdates the first
// entry's created_at directly (SetThesis itself always writes "now") to
// simulate the passage of a day without sleeping in the test.
func TestThesisEntriesAppendAcrossDays(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetThesis("AAPL", "day one: entering on the breakout"); err != nil {
		t.Fatalf("SetThesis() error = %v", err)
	}
	if _, err := d.conn.Exec(
		`UPDATE thesis_entries SET created_at = '2026-01-01 09:00:00' WHERE ticker = 'AAPL'`,
	); err != nil {
		t.Fatalf("backdate entry: %v", err)
	}
	if err := d.SetThesis("AAPL", "day two: trimming into strength"); err != nil {
		t.Fatalf("SetThesis() error = %v", err)
	}

	entries, err := d.GetThesisEntriesInRange("AAPL", "2000-01-01", "")
	if err != nil {
		t.Fatalf("GetThesisEntriesInRange() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("GetThesisEntriesInRange() = %d entries, want 2 (different days)", len(entries))
	}
	if entries[0].CreatedAt != "2026-01-01" || entries[0].Text != "day one: entering on the breakout" {
		t.Errorf("entries[0] = %+v, want the backdated day-one entry", entries[0])
	}
	if entries[1].Text != "day two: trimming into strength" {
		t.Errorf("entries[1] = %+v, want the day-two entry", entries[1])
	}

	got, ok, err := d.GetThesis("AAPL")
	if err != nil || !ok || got != "day two: trimming into strength" {
		t.Errorf("GetThesis() = %q, %v, %v; want the latest entry", got, ok, err)
	}
}

func TestGetThesisEntriesInRangeFiltersByDate(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetThesis("AAPL", "in range"); err != nil {
		t.Fatalf("SetThesis() error = %v", err)
	}
	if _, err := d.conn.Exec(
		`UPDATE thesis_entries SET created_at = '2026-03-15 09:00:00' WHERE ticker = 'AAPL'`,
	); err != nil {
		t.Fatalf("backdate entry: %v", err)
	}

	entries, err := d.GetThesisEntriesInRange("AAPL", "2026-03-01", "2026-03-31")
	if err != nil || len(entries) != 1 {
		t.Fatalf("GetThesisEntriesInRange(in range) = %v, %v; want 1 entry", entries, err)
	}

	entries, err = d.GetThesisEntriesInRange("AAPL", "2026-04-01", "")
	if err != nil || len(entries) != 0 {
		t.Fatalf("GetThesisEntriesInRange(after) = %v, %v; want 0 entries", entries, err)
	}

	entries, err = d.GetThesisEntriesInRange("AAPL", "2000-01-01", "2026-03-10")
	if err != nil || len(entries) != 0 {
		t.Fatalf("GetThesisEntriesInRange(before) = %v, %v; want 0 entries", entries, err)
	}
}

package db

import "testing"

// seedJournal inserts a Phase 21 journal row directly — nothing writes
// thesis_entries any more (Phase 27 P1b), but old rows still exist in real
// databases and service.RoundThesis falls back to them.
func seedJournal(t *testing.T, d *DB, ticker, text, date string) {
	t.Helper()
	if _, err := d.conn.Exec(
		`INSERT INTO thesis_entries (ticker, text, created_at) VALUES (?, ?, ? || ' 09:00:00')`,
		ticker, text, date,
	); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
}

func TestGetRoundThesisUnsetIsNotFound(t *testing.T) {
	d := newTestDB(t)

	_, ok, err := d.GetRoundThesis("AAPL", "2026-03-02")
	if err != nil || ok {
		t.Errorf("GetRoundThesis() = _, %v, %v; want ok=false, err=nil", ok, err)
	}
}

func TestRoundThesisOverwriteAndPerRound(t *testing.T) {
	d := newTestDB(t)

	for _, s := range []struct{ start, text string }{
		{"2026-01-05", "round one"},
		{"2026-03-02", "round two"},
		{"2026-03-02", "round two, revised"},
	} {
		if err := d.SetRoundThesis("AAPL", s.start, s.text); err != nil {
			t.Fatalf("SetRoundThesis() error = %v", err)
		}
	}

	for start, want := range map[string]string{"2026-01-05": "round one", "2026-03-02": "round two, revised"} {
		if got, ok, err := d.GetRoundThesis("AAPL", start); err != nil || !ok || got != want {
			t.Errorf("GetRoundThesis(%s) = %q, %v, %v; want %q", start, got, ok, err, want)
		}
	}
}

func TestRecordBuyPromotesPreBuyDraft(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetRoundThesis("AAPL", "", "draft written while watching"); err != nil {
		t.Fatalf("SetRoundThesis() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-03-02"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if got, ok, _ := d.GetRoundThesis("AAPL", "2026-03-02"); !ok || got != "draft written while watching" {
		t.Errorf("round thesis after first buy = %q, %v; want the draft re-keyed to the round's start", got, ok)
	}
	if _, ok, _ := d.GetRoundThesis("AAPL", ""); ok {
		t.Error("the pre-buy draft slot should be empty once a round has opened")
	}

	// Adding to an open position isn't a new round: a fresh draft stays put.
	if err := d.SetRoundThesis("AAPL", "", "stray draft"); err != nil {
		t.Fatalf("SetRoundThesis() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 5, 110, 0, "2026-03-09"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, ok, _ := d.GetRoundThesis("AAPL", ""); !ok {
		t.Error("a buy into an already-open position must not consume the draft")
	}

	// Sell out, then buy again: that is a new round and takes the draft.
	if _, _, err := d.RecordSell("AAPL", 15, 120, 0, "2026-03-20"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-04-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if got, ok, _ := d.GetRoundThesis("AAPL", "2026-04-01"); !ok || got != "stray draft" {
		t.Errorf("second round thesis = %q, %v; want the draft", got, ok)
	}
	if got, _, _ := d.GetRoundThesis("AAPL", "2026-03-02"); got != "draft written while watching" {
		t.Errorf("first round thesis = %q; must survive the second round opening", got)
	}
}

func TestGetThesisEntriesInRangeFiltersByDate(t *testing.T) {
	d := newTestDB(t)
	seedJournal(t, d, "AAPL", "in range", "2026-03-15")

	entries, err := d.GetThesisEntriesInRange("AAPL", "2026-03-01", "2026-03-31")
	if err != nil || len(entries) != 1 || entries[0].CreatedAt != "2026-03-15" {
		t.Fatalf("GetThesisEntriesInRange(in range) = %v, %v; want the 2026-03-15 entry", entries, err)
	}

	entries, err = d.GetThesisEntriesInRange("AAPL", "2026-04-01", "")
	if err != nil || len(entries) != 0 {
		t.Fatalf("GetThesisEntriesInRange(after) = %v, %v; want 0 entries", entries, err)
	}

	entries, err = d.GetThesisEntriesInRange("AAPL", "", "2026-03-10")
	if err != nil || len(entries) != 0 {
		t.Fatalf("GetThesisEntriesInRange(before) = %v, %v; want 0 entries", entries, err)
	}
}

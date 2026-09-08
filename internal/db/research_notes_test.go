package db

import "testing"

func TestResearchNotesEmptyTickerReturnsNoRows(t *testing.T) {
	d := newTestDB(t)

	notes, err := d.GetResearchNotesByTicker("AAPL")
	if err != nil {
		t.Fatalf("GetResearchNotesByTicker() error = %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("GetResearchNotesByTicker() = %d notes, want 0", len(notes))
	}
}

// TestResearchNoteSameDayUpsert pins migration 28's "one note per ticker per
// day" rule, same shape as thesis_test.go's same-day upsert case: a second
// UpsertResearchNote call on the same calendar day overwrites the first
// row's tag/text rather than appending a new one.
func TestResearchNoteSameDayUpsert(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertResearchNote("AAPL", "TECHNICAL", "broke above the 20d"); err != nil {
		t.Fatalf("UpsertResearchNote() error = %v", err)
	}
	if err := d.UpsertResearchNote("AAPL", "NEWS", "changed my mind: earnings beat"); err != nil {
		t.Fatalf("UpsertResearchNote() (same-day upsert) error = %v", err)
	}

	notes, err := d.GetResearchNotesByTicker("AAPL")
	if err != nil {
		t.Fatalf("GetResearchNotesByTicker() error = %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("GetResearchNotesByTicker() = %d notes, want 1 (same-day edits collapse into one row)", len(notes))
	}
	if notes[0].Tag != "NEWS" || notes[0].Text != "changed my mind: earnings beat" {
		t.Errorf("GetResearchNotesByTicker()[0] = %+v, want the overwritten tag/text", notes[0])
	}
}

func TestResearchNotePinAndDelete(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertResearchNote("AAPL", "TECHNICAL", "watching support"); err != nil {
		t.Fatalf("UpsertResearchNote() error = %v", err)
	}
	notes, err := d.GetResearchNotesByTicker("AAPL")
	if err != nil || len(notes) != 1 {
		t.Fatalf("GetResearchNotesByTicker() = %v, %v; want 1 note, nil", notes, err)
	}
	id := notes[0].ID
	if notes[0].Pinned {
		t.Error("a freshly inserted note should not be pinned")
	}

	if err := d.SetResearchNotePinned(id, true); err != nil {
		t.Fatalf("SetResearchNotePinned() error = %v", err)
	}
	notes, err = d.GetResearchNotesByTicker("AAPL")
	if err != nil || len(notes) != 1 || !notes[0].Pinned {
		t.Fatalf("GetResearchNotesByTicker() after pin = %v, %v; want 1 pinned note, nil", notes, err)
	}

	if err := d.DeleteResearchNote(id); err != nil {
		t.Fatalf("DeleteResearchNote() error = %v", err)
	}
	notes, err = d.GetResearchNotesByTicker("AAPL")
	if err != nil {
		t.Fatalf("GetResearchNotesByTicker() error = %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("GetResearchNotesByTicker() after delete = %d notes, want 0", len(notes))
	}
}

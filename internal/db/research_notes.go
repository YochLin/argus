package db

import "time"

// ResearchNote is one row of the chart page's per-ticker research-notes
// journal — same "one entry per calendar day, upserted" shape as
// ThesisEntry (see thesis.go), plus Tag (a fixed, frontend-chosen category)
// and Pinned (surfaced above the chronological list on the notebook card).
type ResearchNote struct {
	ID        int64
	Ticker    string
	Tag       string
	Text      string
	Pinned    bool
	CreatedAt string // "YYYY-MM-DD", via date(created_at) projection
}

// cst is Taiwan's fixed UTC+8 zone (no DST), the calendar the one-note-a-day
// rule runs on. SQLite's CURRENT_TIMESTAMP is UTC, so a note written between
// 00:00 and 08:00 CST would land on the previous day's key.
var cst = time.FixedZone("CST", 8*3600)

// UpsertResearchNote writes ticker's research note for today (Taiwan time),
// overwriting whatever was already recorded today (via migration 28's unique
// index on (ticker, date(created_at))) — same one-entry-per-day convention as
// SetThesis. Pinned is left untouched by an overwrite: editing today's text
// shouldn't silently un-pin it.
//
// created_at is stored as a CST wall-clock timestamp, so date(created_at) —
// the index key and the label GetResearchNotesByTicker returns — is the
// Taiwan day. Rows written before this carry UTC timestamps; their labels
// stay as they were.
func (d *DB) UpsertResearchNote(ticker, tag, text string) error {
	return d.upsertResearchNoteAt(ticker, tag, text, time.Now())
}

func (d *DB) upsertResearchNoteAt(ticker, tag, text string, at time.Time) error {
	_, err := d.conn.Exec(`
		INSERT INTO research_notes (ticker, tag, text, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(ticker, date(created_at)) DO UPDATE SET
			tag = excluded.tag,
			text = excluded.text,
			created_at = excluded.created_at`,
		ticker, tag, text, at.In(cst).Format("2006-01-02 15:04:05"),
	)
	return err
}

// GetResearchNotesByTicker returns every research note recorded for ticker,
// most recent first. Search/tag-filter/month-grouping/pagination are all
// left to the frontend against this one list — a single ticker's daily
// notes stay small (at most one row per trading day it was followed), so
// there's no server-side query worth adding.
func (d *DB) GetResearchNotesByTicker(ticker string) ([]ResearchNote, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, tag, text, pinned, date(created_at) FROM research_notes
		 WHERE ticker = ? ORDER BY created_at DESC, id DESC`,
		ticker,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ResearchNote
	for rows.Next() {
		var n ResearchNote
		if err := rows.Scan(&n.ID, &n.Ticker, &n.Tag, &n.Text, &n.Pinned, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SetResearchNotePinned toggles one note's pinned flag by id.
func (d *DB) SetResearchNotePinned(id int64, pinned bool) error {
	val := 0
	if pinned {
		val = 1
	}
	_, err := d.conn.Exec(`UPDATE research_notes SET pinned = ? WHERE id = ?`, val, id)
	return err
}

// DeleteResearchNote removes one note by id.
func (d *DB) DeleteResearchNote(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM research_notes WHERE id = ?`, id)
	return err
}

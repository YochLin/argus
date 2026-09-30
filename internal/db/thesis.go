package db

import "database/sql"

// ThesisEntry is one row of Phase 21's append-only holding-rationale journal
// (migration 18). Since Phase 27 P1b nothing writes it any more — it is only
// read, as service.RoundThesis's fallback for rounds with no saved
// round_theses row. CreatedAt is a plain
// "YYYY-MM-DD" date (via GetThesisEntriesInRange's date(created_at)
// projection), not a full timestamp — entries are written at most once per
// calendar day, so the day is all a caller ever needs to display.
type ThesisEntry struct {
	ID        int64
	Ticker    string
	Text      string
	CreatedAt string
}

// GetRoundThesis returns the saved thesis of ticker's round starting at
// roundStart ("" = the pre-buy draft), or ok=false if none is saved. Phase 27
// P1b's per-round replacement for the journal's per-ticker "latest entry"
// read; callers wanting the journal fallback go through service.RoundThesis.
func (d *DB) GetRoundThesis(ticker, roundStart string) (string, bool, error) {
	var text string
	err := d.conn.QueryRow(
		`SELECT text FROM round_theses WHERE ticker = ? AND round_start = ?`,
		ticker, roundStart,
	).Scan(&text)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

// SetRoundThesis overwrites ticker's thesis for the round starting at
// roundStart ("" = the pre-buy draft) — the single write path /thesis, the
// web chart page and the buy form's thesis field all end up at.
func (d *DB) SetRoundThesis(ticker, roundStart, text string) error {
	_, err := d.conn.Exec(`
		INSERT INTO round_theses (ticker, round_start, text, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker, round_start) DO UPDATE SET
			text = excluded.text,
			updated_at = excluded.updated_at`,
		ticker, roundStart, text,
	)
	return err
}

// GetThesisEntriesInRange returns every thesis entry recorded for ticker
// between from and to (both "YYYY-MM-DD", inclusive; to == "" means through
// now), oldest first — the web dashboard's round-detail page's "every
// thesis written while this round was open" read.
func (d *DB) GetThesisEntriesInRange(ticker, from, to string) ([]ThesisEntry, error) {
	query := `SELECT id, ticker, text, date(created_at) FROM thesis_entries WHERE ticker = ? AND date(created_at) >= ?`
	args := []any{ticker, from}
	if to != "" {
		query += ` AND date(created_at) <= ?`
		args = append(args, to)
	}
	query += ` ORDER BY created_at, id`
	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ThesisEntry
	for rows.Next() {
		var e ThesisEntry
		if err := rows.Scan(&e.ID, &e.Ticker, &e.Text, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

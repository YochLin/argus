package db

import "database/sql"

// ThesisEntry is one row of Phase 21's append-only holding-rationale journal
// (see migration 18's doc comment in db.go). CreatedAt is a plain
// "YYYY-MM-DD" date (via GetThesisEntriesInRange's date(created_at)
// projection), not a full timestamp — entries are written at most once per
// calendar day, so the day is all a caller ever needs to display.
type ThesisEntry struct {
	ID        int64
	Ticker    string
	Text      string
	CreatedAt string
}

// GetThesis returns ticker's most recently recorded thesis, or ok=false if
// none exists — the single read path /thesis, the buy nudge, /review, and
// the web dashboard all call, now backed by thesis_entries instead of the
// old one-row-per-ticker thesis table.
func (d *DB) GetThesis(ticker string) (string, bool, error) {
	var text string
	err := d.conn.QueryRow(
		`SELECT text FROM thesis_entries WHERE ticker = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		ticker,
	).Scan(&text)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

// SetThesis appends ticker's thesis for today, upserting same-day entries
// (via the migration-18 unique index on (ticker, date(created_at))) so a
// same-day typo fix stays one row while a change of mind on a later day
// starts a new one — the single write path /thesis and the web buy form's
// optional thesis field both call.
//
// ponytail: two genuinely different thoughts recorded on the same calendar
// day still collapse into one row. Upgrade to "only the latest row is
// editable in place" if that ever actually happens.
func (d *DB) SetThesis(ticker, text string) error {
	_, err := d.conn.Exec(`
		INSERT INTO thesis_entries (ticker, text, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker, date(created_at)) DO UPDATE SET
			text = excluded.text,
			created_at = excluded.created_at`,
		ticker, text,
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

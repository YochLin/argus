package db

// BlockedNewsSource is one row from Phase 19 PR2's news-source blacklist
// (migration 23) — see docs/phase-19-llm-transparency.md §5.
type BlockedNewsSource struct {
	Source    string
	CreatedAt string
}

// BlockNewsSource adds source to the blacklist (idempotent — re-blocking an
// already-blocked source is a no-op, not an error).
func (d *DB) BlockNewsSource(source string) error {
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO blocked_news_sources (source) VALUES (?)`, source)
	return err
}

// UnblockNewsSource removes source from the blacklist. No error if it wasn't
// blocked in the first place.
func (d *DB) UnblockNewsSource(source string) error {
	_, err := d.conn.Exec(`DELETE FROM blocked_news_sources WHERE source = ?`, source)
	return err
}

// IsNewsSourceBlocked reports whether source is blocked, case-insensitively
// and trimmed — this is the point lookup behind the closure main.go passes
// into data.NewNewsFilter/data.NewMarketNewsFilter, called once per news item
// per fetch, so it stays a single indexed-by-primary-key query rather than
// pulling the whole list and scanning it in Go.
func (d *DB) IsNewsSourceBlocked(source string) (bool, error) {
	var exists bool
	err := d.conn.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM blocked_news_sources WHERE LOWER(TRIM(source)) = LOWER(TRIM(?)))`,
		source,
	).Scan(&exists)
	return exists, err
}

// ListBlockedNewsSources returns every blocked source, newest first — the
// /llm page's blocklist panel (§5.3).
func (d *DB) ListBlockedNewsSources() ([]BlockedNewsSource, error) {
	rows, err := d.conn.Query(`SELECT source, created_at FROM blocked_news_sources ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BlockedNewsSource
	for rows.Next() {
		var s BlockedNewsSource
		if err := rows.Scan(&s.Source, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

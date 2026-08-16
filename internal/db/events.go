package db

// PriceEvent is one row from Phase 20's gap/big-move event log (migration
// 18) — distinct from signals.PriceEvent (same name, different package,
// different fields), same convention as db.DailySnapshot vs the signals
// package staying independent of each other.
type PriceEvent struct {
	ID        int64
	Ticker    string
	Market    string
	Date      string
	GapPct    float64
	ChangePct float64
	Summary   string
	CreatedAt string
}

// HasPriceEvent reports whether ticker already has a price_events row for
// date — RunClosingSnapshot's dedup check before writing, guarding against a
// manual rerun or job retry re-triggering the same day's event.
func (d *DB) HasPriceEvent(ticker, date string) (bool, error) {
	var exists bool
	err := d.conn.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM price_events WHERE ticker = ? AND date = ?)`,
		ticker, date,
	).Scan(&exists)
	return exists, err
}

// SavePriceEvent inserts one price_events row. summary == "" is a valid,
// expected value — see the migration's doc comment — meaning "recorded, but
// this run's top-3 LLM writeup cap was already spent." Returns an error if
// the (ticker, date) unique index is violated; callers should log and skip
// rather than treat that as fatal.
func (d *DB) SavePriceEvent(ev PriceEvent) error {
	_, err := d.conn.Exec(
		`INSERT INTO price_events (ticker, market, date, gap_pct, change_pct, summary) VALUES (?, ?, ?, ?, ?, ?)`,
		ev.Ticker, ev.Market, ev.Date, ev.GapPct, ev.ChangePct, ev.Summary,
	)
	return err
}

// GetRecentPriceEvents returns the most recent limit price_events rows
// across every ticker, newest first — /events' no-argument form.
func (d *DB) GetRecentPriceEvents(limit int) ([]PriceEvent, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, market, date, gap_pct, change_pct, summary, created_at
		 FROM price_events ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PriceEvent
	for rows.Next() {
		var ev PriceEvent
		if err := rows.Scan(&ev.ID, &ev.Ticker, &ev.Market, &ev.Date, &ev.GapPct, &ev.ChangePct, &ev.Summary, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// GetPriceEventsForTicker returns the most recent limit price_events rows
// for ticker, newest first — /events TICKER's form.
func (d *DB) GetPriceEventsForTicker(ticker string, limit int) ([]PriceEvent, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, market, date, gap_pct, change_pct, summary, created_at
		 FROM price_events WHERE ticker = ? ORDER BY id DESC LIMIT ?`,
		ticker, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PriceEvent
	for rows.Next() {
		var ev PriceEvent
		if err := rows.Scan(&ev.ID, &ev.Ticker, &ev.Market, &ev.Date, &ev.GapPct, &ev.ChangePct, &ev.Summary, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

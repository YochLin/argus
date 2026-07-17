package db

import "strings"

// Lesson is one row from Phase 3.9's trade-review feedback loop (see
// docs/research-tradingagents.md's "反思回饋迴路" section and migration 10):
// a short, distilled takeaway parsed out of a past ReviewTrade reply.
type Lesson struct {
	ID        int64
	Ticker    string
	Date      string
	Lesson    string
	CreatedAt string
}

// SaveLesson inserts one lesson row, called by reviewClosedTrade/handleReview
// after parsing a non-empty lesson out of an LLM.ReviewTrade reply.
// Deliberately no dedup/uniqueness constraint — re-running /review on the
// same closed round is expected to add a fresh row rather than silently
// no-op, same as this project's other append-only logs (scan_hits,
// recommendations).
func (d *DB) SaveLesson(ticker, date, lesson string) error {
	_, err := d.conn.Exec(
		`INSERT INTO trade_lessons (ticker, date, lesson) VALUES (?, ?, ?)`,
		ticker, date, lesson,
	)
	return err
}

// GetLessonsForTickers returns every lesson on record for any ticker in
// tickers, keyed by ticker and ordered oldest-first within each ticker —
// bot.loadPastLessons's batched read (one query for the whole watchlist ∪
// candidate set, same one-call-not-N-calls principle as
// GetLatestRecommendations) for the "same ticker: bring all of them" half
// of the feedback loop. Unlike GetRecentLessons (capped, cross-ticker),
// this deliberately isn't limited — a low-frequency single-user bot will
// only ever accumulate a handful of closed rounds per ticker, so there's no
// unbounded-growth risk to guard against.
func (d *DB) GetLessonsForTickers(tickers []string) (map[string][]Lesson, error) {
	if len(tickers) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(tickers))
	args := make([]any, len(tickers))
	for i, t := range tickers {
		placeholders[i] = "?"
		args[i] = t
	}
	query := `SELECT id, ticker, date, lesson, created_at FROM trade_lessons WHERE ticker IN (` +
		strings.Join(placeholders, ",") + `) ORDER BY ticker, date, id`
	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]Lesson)
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.Ticker, &l.Date, &l.Lesson, &l.CreatedAt); err != nil {
			return nil, err
		}
		out[l.Ticker] = append(out[l.Ticker], l)
	}
	return out, rows.Err()
}

// GetRecentLessons returns the most recent limit lessons across every
// ticker, newest first — bot.loadRecentLessons's read for the "cross
// ticker: recent N, general" half of the feedback loop (see
// docs/research-tradingagents.md). Deliberately not deduplicated against
// GetLessonsForTickers' per-ticker results: a ticker's own lesson
// occasionally also showing up in this general pool is harmless overlap
// for in-context learning, and excluding it would need a per-ticker query
// instead of one global call reused across the whole recommendation batch.
func (d *DB) GetRecentLessons(limit int) ([]Lesson, error) {
	rows, err := d.conn.Query(
		`SELECT id, ticker, date, lesson, created_at FROM trade_lessons ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Lesson
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.Ticker, &l.Date, &l.Lesson, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

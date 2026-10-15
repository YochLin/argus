package db

import (
	"fmt"
	"strings"
	"time"

	"argus/internal/market"
)

type Recommendation struct {
	ID        int64
	Date      string
	Ticker    string
	Action    string // BUY / SELL / HOLD ("" for rows saved before the column existed)
	Reason    string
	Price     float64 // price at recommendation time (0 for rows saved before the column existed)
	Source    string  // "watchlist" / "movers" / "scan" ("" for rows saved before the column existed — display as "watchlist")
	Market    string  // "us" / "tw" — derived from Ticker via market.Of at write time, never caller-supplied (Phase 6, see migration 12)
	CreatedAt time.Time
}

func (d *DB) SaveRecommendations(date string, recs []Recommendation) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range recs {
		_, err := tx.Exec(`INSERT INTO recommendations (date, ticker, action, reason, price, source, market) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			date, r.Ticker, r.Action, r.Reason, r.Price, r.Source, string(market.Of(r.Ticker)))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetRecommendationsSince returns all recommendations dated fromDate or later
// (dates are lexicographically comparable YYYY-MM-DD strings), oldest first.
func (d *DB) GetRecommendationsSince(fromDate string) ([]Recommendation, error) {
	rows, err := d.conn.Query(
		`SELECT id, date, ticker, action, reason, price, source, market FROM recommendations
		 WHERE date >= ? ORDER BY date, id`,
		fromDate,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price, &r.Source, &r.Market); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// GetLatestRecommendations returns each ticker's most recent recommendation
// row (by insertion order, i.e. highest id) in one batched query, keyed by
// ticker — tickers with no recommendation history are simply absent from the
// map. Backs Phase 3.8's recommendation continuity: the caller feeds "what
// did we say last time" into today's prompt so a reversal comes with an
// explanation instead of silently contradicting an earlier call.
func (d *DB) GetLatestRecommendations(tickers []string) (map[string]Recommendation, error) {
	if len(tickers) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(tickers))
	args := make([]any, len(tickers))
	for i, t := range tickers {
		placeholders[i] = "?"
		args[i] = t
	}
	query := fmt.Sprintf(`
		SELECT id, date, ticker, action, reason, price, market FROM recommendations
		WHERE id IN (
			SELECT MAX(id) FROM recommendations WHERE ticker IN (%s) GROUP BY ticker
		)`, strings.Join(placeholders, ","))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]Recommendation)
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price, &r.Market); err != nil {
			return nil, err
		}
		out[r.Ticker] = r
	}
	return out, rows.Err()
}

// GetRecommendationsForTicker returns ticker's recommendations dated within
// [from, to] inclusive, oldest first — a single-ticker, bounded-window
// counterpart to GetRecommendationsSince (which is whole-table and only has
// a lower bound, right for /track's rolling-window scan but not for a closed
// trade's fixed holding period).
func (d *DB) GetRecommendationsForTicker(ticker, from, to string) ([]Recommendation, error) {
	rows, err := d.conn.Query(
		`SELECT id, date, ticker, action, reason, price, source, market FROM recommendations
		 WHERE ticker = ? AND date BETWEEN ? AND ? ORDER BY date, id`,
		ticker, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price, &r.Source, &r.Market); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

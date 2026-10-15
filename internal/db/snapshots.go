package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type DailySnapshot struct {
	ID            int64
	Ticker        string
	Date          string
	Open          float64
	Close         float64
	High          float64
	Low           float64
	Volume        int64
	ChangePercent float64
	CreatedAt     time.Time
}

func (d *DB) SaveSnapshot(s DailySnapshot) error {
	_, err := d.conn.Exec(`
		INSERT OR REPLACE INTO daily_snapshots
		(ticker, date, open, close, high, low, volume, change_percent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Ticker, s.Date, s.Open, s.Close, s.High, s.Low, s.Volume, s.ChangePercent,
	)
	return err
}

// GetSnapshotClose returns the stored closing price for ticker on date, or
// ok=false if no snapshot exists for that day.
func (d *DB) GetSnapshotClose(ticker, date string) (close float64, ok bool, err error) {
	err = d.conn.QueryRow(
		`SELECT close FROM daily_snapshots WHERE ticker = ? AND date = ?`,
		ticker, date,
	).Scan(&close)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return close, true, nil
}

// GetLatestSnapshot returns the most recent daily_snapshots row for ticker
// regardless of date, or ok=false if none exists yet. Used by chat context
// injection, which wants "whatever we last saw" rather than a specific
// day's close.
func (d *DB) GetLatestSnapshot(ticker string) (DailySnapshot, bool, error) {
	s := DailySnapshot{Ticker: ticker}
	err := d.conn.QueryRow(`
		SELECT date, open, close, high, low, volume, change_percent
		FROM daily_snapshots WHERE ticker = ? ORDER BY date DESC LIMIT 1`,
		ticker,
	).Scan(&s.Date, &s.Open, &s.Close, &s.High, &s.Low, &s.Volume, &s.ChangePercent)
	if err == sql.ErrNoRows {
		return DailySnapshot{}, false, nil
	}
	if err != nil {
		return DailySnapshot{}, false, err
	}
	return s, true, nil
}

// GetPeakClose returns the highest daily_snapshots close recorded for ticker
// on or after sinceDate, or ok=false if there's no snapshot in that range
// yet. Backs the trailing-stop check's running-high, computed on demand from
// existing snapshot history rather than a separately maintained column.
func (d *DB) GetPeakClose(ticker, sinceDate string) (float64, bool, error) {
	var peak sql.NullFloat64
	err := d.conn.QueryRow(
		`SELECT MAX(close) FROM daily_snapshots WHERE ticker = ? AND date >= ?`,
		ticker, sinceDate,
	).Scan(&peak)
	if err != nil {
		return 0, false, err
	}
	if !peak.Valid {
		return 0, false, nil
	}
	return peak.Float64, true, nil
}

// GetCloseExtremes returns the highest and lowest daily_snapshots close for
// ticker within [from, to] inclusive, or ok=false if there's no snapshot in
// that range. Unlike GetPeakClose (which only has a lower bound — right for
// a still-open position's running high), a closed trade's review needs both
// ends of a bounded window so a ticker that's kept trading (and setting new
// highs) after the position was closed doesn't leak into "how far the exit
// was from the period's high."
func (d *DB) GetCloseExtremes(ticker, from, to string) (high, low float64, ok bool, err error) {
	var h, l sql.NullFloat64
	err = d.conn.QueryRow(
		`SELECT MAX(close), MIN(close) FROM daily_snapshots WHERE ticker = ? AND date BETWEEN ? AND ?`,
		ticker, from, to,
	).Scan(&h, &l)
	if err != nil {
		return 0, 0, false, err
	}
	if !h.Valid {
		return 0, 0, false, nil
	}
	return h.Float64, l.Float64, true, nil
}

// GetSnapshotCloseRange returns ticker's first and last daily_snapshots
// close with date in [from, to] inclusive, or ok=false if fewer than two
// rows exist in that range (a single row can't express a period change).
// Unlike GetSnapshotClose (one exact date), this is the monthly report's
// "SPY start vs. end of month" lookup — first/last by date, not by row
// insertion order.
func (d *DB) GetSnapshotCloseRange(ticker, from, to string) (first, last float64, ok bool, err error) {
	rows, err := d.conn.Query(
		`SELECT close FROM daily_snapshots WHERE ticker = ? AND date BETWEEN ? AND ? ORDER BY date`,
		ticker, from, to,
	)
	if err != nil {
		return 0, 0, false, err
	}
	defer rows.Close()

	var closes []float64
	for rows.Next() {
		var c float64
		if err := rows.Scan(&c); err != nil {
			return 0, 0, false, err
		}
		closes = append(closes, c)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, false, err
	}
	if len(closes) < 2 {
		return 0, 0, false, nil
	}
	return closes[0], closes[len(closes)-1], true, nil
}

// GetRecentCloses returns ticker's n most recent daily_snapshots closes
// recorded so far, oldest first — RunClosingSnapshot's cumulative-decline
// check calls this before writing today's own row, so the result is exactly
// "the last n sessions before today," no date filter needed. Returns fewer
// than n (possibly zero) rows if daily_snapshots doesn't have enough history
// yet for ticker, which the caller treats as "not enough data, skip."
func (d *DB) GetRecentCloses(ticker string, n int) ([]float64, error) {
	rows, err := d.conn.Query(
		`SELECT close FROM (SELECT close, date FROM daily_snapshots WHERE ticker = ? ORDER BY date DESC LIMIT ?) sub ORDER BY date ASC`,
		ticker, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var closes []float64
	for rows.Next() {
		var c float64
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		closes = append(closes, c)
	}
	return closes, rows.Err()
}

// GetDailySnapshotsForTickers returns every daily_snapshots row for any of
// tickers dated within [from, to] inclusive, ordered by ticker then date —
// a batched IN query (same one-call-not-N-calls principle as
// GetLatestRecommendations/GetLessonsForTickers) rather than a per-ticker
// loop, since internal/web's daily P&L replay engine needs the full
// multi-ticker OHLCV series for a date range in one shot (every other
// daily_snapshots reader in this file returns a single scalar/pair, never a
// multi-row series across tickers). Returns nil, nil for an empty tickers.
func (d *DB) GetDailySnapshotsForTickers(tickers []string, from, to string) ([]DailySnapshot, error) {
	if len(tickers) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(tickers))
	args := make([]any, 0, len(tickers)+2)
	for i, t := range tickers {
		placeholders[i] = "?"
		args = append(args, t)
	}
	args = append(args, from, to)

	query := fmt.Sprintf(`
		SELECT id, ticker, date, open, close, high, low, volume, change_percent, created_at
		FROM daily_snapshots
		WHERE ticker IN (%s) AND date BETWEEN ? AND ?
		ORDER BY ticker, date`, strings.Join(placeholders, ","))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailySnapshot
	for rows.Next() {
		var s DailySnapshot
		if err := rows.Scan(&s.ID, &s.Ticker, &s.Date, &s.Open, &s.Close, &s.High, &s.Low, &s.Volume, &s.ChangePercent, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

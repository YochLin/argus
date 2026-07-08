package db

import (
	_ "embed"
	"strings"
	"time"
)

// sp500Tickers is the embedded seed list for the universe table's 'sp500'
// source tier — one ticker symbol per line, extracted from the Symbol column
// of https://github.com/datasets/s-and-p-500-companies (which itself syncs
// Wikipedia's S&P 500 constituents list). Only the ticker is ever needed, so
// this is a plain symbol list rather than the full CSV (sector/HQ/etc.,
// which also has quoting complications like `"Saint Paul, Minnesota"`).
//
//go:embed sp500_tickers.txt
var sp500Tickers string

// UniverseEntry is one ticker in the Phase 2.6 candidate scan pool.
type UniverseEntry struct {
	Ticker  string
	Source  string // "sp500" (seeded) or "manual" (/universe add)
	AddedAt time.Time
}

// seedSP500 bulk-inserts the embedded S&P 500 ticker list with source='sp500'
// the first time it's ever called against a given database (checked via a
// count query, not a migration step, since it's data not schema). It's
// deliberately *not* re-synced on every startup — a user's manual
// `/universe remove` of a seeded ticker should stick, and refreshing the
// S&P 500 list against Wikipedia is a separate, not-yet-built feature (see
// PLAN.md).
func (d *DB) seedSP500() error {
	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM universe WHERE source = 'sp500'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, line := range strings.Split(strings.TrimSpace(sp500Tickers), "\n") {
		ticker := strings.TrimSpace(line)
		if ticker == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO universe (ticker, source) VALUES (?, 'sp500')`, ticker); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddUniverseTicker adds ticker to the scan pool under source (e.g. "manual"
// for /universe add). A ticker already present keeps its existing source.
func (d *DB) AddUniverseTicker(ticker, source string) error {
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO universe (ticker, source) VALUES (?, ?)`, ticker, source)
	return err
}

// RemoveUniverseTicker removes ticker from the scan pool regardless of source.
func (d *DB) RemoveUniverseTicker(ticker string) error {
	_, err := d.conn.Exec(`DELETE FROM universe WHERE ticker = ?`, ticker)
	return err
}

// GetUniverse returns every ticker in the scan pool, ordered by ticker.
func (d *DB) GetUniverse() ([]UniverseEntry, error) {
	rows, err := d.conn.Query(`SELECT ticker, source, added_at FROM universe ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []UniverseEntry
	for rows.Next() {
		var e UniverseEntry
		if err := rows.Scan(&e.Ticker, &e.Source, &e.AddedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// SaveScanHit logs that ticker's daily universe scan found reason (a signal
// message) on date. Multiple hits for the same ticker/date are allowed — read
// back grouped by GetScanHits.
func (d *DB) SaveScanHit(ticker, date, reason string) error {
	_, err := d.conn.Exec(`INSERT INTO scan_hits (ticker, date, reason) VALUES (?, ?, ?)`, ticker, date, reason)
	return err
}

// GetScanHits returns every scan hit logged for date, keyed by ticker with
// multiple reasons for the same ticker joined by "; ".
func (d *DB) GetScanHits(date string) (map[string]string, error) {
	rows, err := d.conn.Query(`SELECT ticker, reason FROM scan_hits WHERE date = ? ORDER BY ticker, created_at`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var ticker, reason string
		if err := rows.Scan(&ticker, &reason); err != nil {
			return nil, err
		}
		if existing, ok := out[ticker]; ok {
			out[ticker] = existing + "; " + reason
		} else {
			out[ticker] = reason
		}
	}
	return out, rows.Err()
}

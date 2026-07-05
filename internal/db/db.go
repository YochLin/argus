package db

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type Watchlist struct {
	Ticker    string
	AddedAt   time.Time
}

type DailySnapshot struct {
	ID          int64
	Ticker      string
	Date        string
	Open        float64
	Close       float64
	High        float64
	Low         float64
	Volume      int64
	ChangePercent float64
	CreatedAt   time.Time
}

type Recommendation struct {
	ID        int64
	Date      string
	Ticker    string
	Reason    string
	CreatedAt time.Time
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS watchlist (
			ticker TEXT PRIMARY KEY,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS daily_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ticker TEXT NOT NULL,
			date TEXT NOT NULL,
			open REAL,
			close REAL,
			high REAL,
			low REAL,
			volume INTEGER,
			change_percent REAL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(ticker, date)
		);

		CREATE TABLE IF NOT EXISTS recommendations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date TEXT NOT NULL,
			ticker TEXT NOT NULL,
			reason TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

func (d *DB) AddTicker(ticker string) error {
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO watchlist (ticker) VALUES (?)`, ticker)
	return err
}

func (d *DB) RemoveTicker(ticker string) error {
	_, err := d.conn.Exec(`DELETE FROM watchlist WHERE ticker = ?`, ticker)
	return err
}

func (d *DB) GetWatchlist() ([]string, error) {
	rows, err := d.conn.Query(`SELECT ticker FROM watchlist ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tickers = append(tickers, t)
	}
	return tickers, rows.Err()
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

func (d *DB) SaveRecommendations(date string, recs []Recommendation) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range recs {
		_, err := tx.Exec(`INSERT INTO recommendations (date, ticker, reason) VALUES (?, ?, ?)`,
			date, r.Ticker, r.Reason)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) Close() error {
	return d.conn.Close()
}

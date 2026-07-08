package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNoPosition is returned by RecordSell when there is no open position for
// the ticker to sell from.
var ErrNoPosition = errors.New("no position for ticker")

// ErrInsufficientShares is returned by RecordSell when the sell size exceeds
// the currently held shares — this project only tracks long positions, so
// selling more than is held would go negative, which isn't representable.
var ErrInsufficientShares = errors.New("insufficient shares for sell")

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
	Action    string // BUY / SELL / HOLD ("" for rows saved before the column existed)
	Reason    string
	Price     float64 // price at recommendation time (0 for rows saved before the column existed)
	CreatedAt time.Time
}

// Position is the current open holding for a ticker: total shares and the
// cost-basis-weighted average price paid across all buys, net of sells.
type Position struct {
	Ticker    string
	Shares    float64
	AvgCost   float64
	UpdatedAt time.Time
}

// Transaction is one recorded buy or sell. RealizedPnL is only meaningful
// for SELL rows (0 for BUY) — proceeds minus the shares' cost basis at
// AvgCost, minus fee.
type Transaction struct {
	ID          int64
	Ticker      string
	Side        string // BUY / SELL
	Shares      float64
	Price       float64
	Fee         float64
	Date        string
	RealizedPnL float64
	CreatedAt   time.Time
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

// migrations is the ordered list of incremental schema steps. The DB's
// PRAGMA user_version records how many have been applied, so migrate() only
// runs the ones past it — this is how columns get added to existing
// databases, which bare CREATE TABLE IF NOT EXISTS can't do. Append new
// steps at the end; never edit or reorder ones that have shipped, since
// deployed databases have already recorded them as applied.
var migrations = []string{
	// 1: base schema. Kept idempotent (IF NOT EXISTS) because databases
	// created before versioning existed have these tables at user_version 0.
	`
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
	`,
	// 2: signal_states remembers the last state we notified per
	// (ticker, signal family) so daily signal checks can diff against it —
	// MACD golden/death-cross detection and RSI dedup both live here.
	// recommendations gains the explicit action (BUY/SELL/HOLD) and the
	// price at recommendation time, which /track compares against later.
	`
	CREATE TABLE IF NOT EXISTS signal_states (
		ticker TEXT NOT NULL,
		signal TEXT NOT NULL,
		state TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (ticker, signal)
	);

	ALTER TABLE recommendations ADD COLUMN action TEXT NOT NULL DEFAULT '';
	ALTER TABLE recommendations ADD COLUMN price REAL NOT NULL DEFAULT 0;
	`,
	// 3: positions/transactions back Phase 2's asset tracking. positions
	// holds one row per ticker with the cost-basis-weighted average price
	// (RecordBuy/RecordSell keep it in sync); transactions is the full
	// buy/sell log, including realized_pnl for sells. net_worth_snapshots
	// records total position value once a day (RunClosingSnapshot) so a net
	// worth curve can be drawn later.
	`
	CREATE TABLE IF NOT EXISTS positions (
		ticker TEXT PRIMARY KEY,
		shares REAL NOT NULL,
		avg_cost REAL NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		side TEXT NOT NULL,
		shares REAL NOT NULL,
		price REAL NOT NULL,
		fee REAL NOT NULL DEFAULT 0,
		date TEXT NOT NULL,
		realized_pnl REAL NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS net_worth_snapshots (
		date TEXT PRIMARY KEY,
		total_value REAL NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
}

func (d *DB) migrate() error {
	var version int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		if _, err := d.conn.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		// PRAGMA doesn't support parameter binding.
		if _, err := d.conn.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			return fmt.Errorf("set user_version %d: %w", i+1, err)
		}
	}
	return nil
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

func (d *DB) SaveRecommendations(date string, recs []Recommendation) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range recs {
		_, err := tx.Exec(`INSERT INTO recommendations (date, ticker, action, reason, price) VALUES (?, ?, ?, ?, ?)`,
			date, r.Ticker, r.Action, r.Reason, r.Price)
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
		`SELECT id, date, ticker, action, reason, price FROM recommendations
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
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// GetSignalState returns the last recorded state for (ticker, signal), or ""
// if none has been recorded yet.
func (d *DB) GetSignalState(ticker, signal string) (string, error) {
	var state string
	err := d.conn.QueryRow(
		`SELECT state FROM signal_states WHERE ticker = ? AND signal = ?`,
		ticker, signal,
	).Scan(&state)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return state, nil
}

// SetSignalState upserts the current state for (ticker, signal).
func (d *DB) SetSignalState(ticker, signal, state string) error {
	_, err := d.conn.Exec(`
		INSERT INTO signal_states (ticker, signal, state, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker, signal) DO UPDATE SET
			state = excluded.state,
			updated_at = excluded.updated_at`,
		ticker, signal, state,
	)
	return err
}

// RecordBuy records a BUY transaction and folds it into the ticker's
// position, recomputing the weighted-average cost (existing cost basis plus
// this purchase's shares*price+fee, divided by the new total shares). It
// returns the position as it stands after the buy.
func (d *DB) RecordBuy(ticker string, shares, price, fee float64, date string) (Position, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return Position{}, err
	}
	defer tx.Rollback()

	var existingShares, existingCost float64
	err = tx.QueryRow(`SELECT shares, avg_cost FROM positions WHERE ticker = ?`, ticker).Scan(&existingShares, &existingCost)
	if err != nil && err != sql.ErrNoRows {
		return Position{}, err
	}

	totalShares := existingShares + shares
	avgCost := (existingShares*existingCost + shares*price + fee) / totalShares

	if _, err := tx.Exec(`
		INSERT INTO positions (ticker, shares, avg_cost, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker) DO UPDATE SET
			shares = excluded.shares,
			avg_cost = excluded.avg_cost,
			updated_at = excluded.updated_at`,
		ticker, totalShares, avgCost,
	); err != nil {
		return Position{}, err
	}

	if _, err := tx.Exec(`
		INSERT INTO transactions (ticker, side, shares, price, fee, date)
		VALUES (?, 'BUY', ?, ?, ?, ?)`,
		ticker, shares, price, fee, date,
	); err != nil {
		return Position{}, err
	}

	if err := tx.Commit(); err != nil {
		return Position{}, err
	}
	return Position{Ticker: ticker, Shares: totalShares, AvgCost: avgCost}, nil
}

// RecordSell records a SELL transaction against an existing position,
// returning the realized P&L for this sale ((price - avgCost)*shares - fee)
// and the position as it stands afterward. It returns ErrNoPosition if there
// is nothing open for the ticker, or ErrInsufficientShares if shares exceeds
// what's held — this project only tracks long positions, so short-selling
// isn't representable. Selling the full position deletes the positions row
// rather than leaving a zero-share one behind.
func (d *DB) RecordSell(ticker string, shares, price, fee float64, date string) (Position, float64, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return Position{}, 0, err
	}
	defer tx.Rollback()

	var existingShares, existingCost float64
	err = tx.QueryRow(`SELECT shares, avg_cost FROM positions WHERE ticker = ?`, ticker).Scan(&existingShares, &existingCost)
	if err == sql.ErrNoRows {
		return Position{}, 0, ErrNoPosition
	}
	if err != nil {
		return Position{}, 0, err
	}
	if shares > existingShares {
		return Position{}, 0, ErrInsufficientShares
	}

	realizedPnL := (price-existingCost)*shares - fee
	remainingShares := existingShares - shares

	if math.Abs(remainingShares) < 1e-9 {
		remainingShares = 0
		if _, err := tx.Exec(`DELETE FROM positions WHERE ticker = ?`, ticker); err != nil {
			return Position{}, 0, err
		}
	} else {
		if _, err := tx.Exec(`
			UPDATE positions SET shares = ?, updated_at = CURRENT_TIMESTAMP WHERE ticker = ?`,
			remainingShares, ticker,
		); err != nil {
			return Position{}, 0, err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO transactions (ticker, side, shares, price, fee, date, realized_pnl)
		VALUES (?, 'SELL', ?, ?, ?, ?, ?)`,
		ticker, shares, price, fee, date, realizedPnL,
	); err != nil {
		return Position{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return Position{}, 0, err
	}
	return Position{Ticker: ticker, Shares: remainingShares, AvgCost: existingCost}, realizedPnL, nil
}

// GetPosition returns the current position for ticker, or ok=false if
// there's no open position.
func (d *DB) GetPosition(ticker string) (Position, bool, error) {
	p := Position{Ticker: ticker}
	err := d.conn.QueryRow(
		`SELECT shares, avg_cost, updated_at FROM positions WHERE ticker = ?`, ticker,
	).Scan(&p.Shares, &p.AvgCost, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return Position{}, false, nil
	}
	if err != nil {
		return Position{}, false, err
	}
	return p, true, nil
}

// GetPositions returns every open position, ordered by ticker.
func (d *DB) GetPositions() ([]Position, error) {
	rows, err := d.conn.Query(`SELECT ticker, shares, avg_cost, updated_at FROM positions ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var positions []Position
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.Ticker, &p.Shares, &p.AvgCost, &p.UpdatedAt); err != nil {
			return nil, err
		}
		positions = append(positions, p)
	}
	return positions, rows.Err()
}

// GetRealizedPnL sums realized_pnl across every SELL transaction ever
// recorded, for /portfolio's cumulative realized P&L line.
func (d *DB) GetRealizedPnL() (float64, error) {
	var total sql.NullFloat64
	if err := d.conn.QueryRow(`SELECT SUM(realized_pnl) FROM transactions WHERE side = 'SELL'`).Scan(&total); err != nil {
		return 0, err
	}
	return total.Float64, nil
}

// SaveNetWorthSnapshot upserts the total position value for date.
func (d *DB) SaveNetWorthSnapshot(date string, total float64) error {
	_, err := d.conn.Exec(`
		INSERT INTO net_worth_snapshots (date, total_value)
		VALUES (?, ?)
		ON CONFLICT(date) DO UPDATE SET total_value = excluded.total_value`,
		date, total,
	)
	return err
}

func (d *DB) Close() error {
	return d.conn.Close()
}

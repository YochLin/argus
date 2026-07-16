package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
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
	Ticker  string
	AddedAt time.Time
}

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

type Recommendation struct {
	ID        int64
	Date      string
	Ticker    string
	Action    string // BUY / SELL / HOLD ("" for rows saved before the column existed)
	Reason    string
	Price     float64 // price at recommendation time (0 for rows saved before the column existed)
	Source    string  // "watchlist" / "movers" / "scan" ("" for rows saved before the column existed — display as "watchlist")
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

// New opens the main read-write connection used by the bot process itself.
// busy_timeout is set (see OpenReadOnly/OpenForWrites below) so that a
// second connection to the same file briefly holding a write lock — the MCP
// subprocess's own writable connection, see OpenForWrites — makes this
// connection wait rather than fail outright with "database is locked";
// both sides are low-frequency single-user writers, so contention should be
// rare, but there was no reason to leave the default (no wait at all) now
// that a second writer process exists at all.
func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		return nil, err
	}
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	if err := d.seedSP500(); err != nil {
		return nil, err
	}
	return d, nil
}

// OpenReadOnly opens a second connection to the SQLite database at path for
// a separate process that must never write to it — the MCP server
// subprocess's read-only query tools (see internal/mcptools and PLAN.md's
// Phase 3.5 "DB 唯讀查詢工具" item), which relaxes that phase's original
// "don't touch the DB at all" decision now that it's clear a
// same-process-family read-only connection carries none of the
// cross-process write-conflict risk that decision was guarding against.
//
// Unlike New, this skips migrate()/seedSP500() (both write) — it assumes
// the schema already exists, which holds in practice since this connection
// is only ever opened after the main bot process has already opened (and
// migrated) the database.
//
// modernc.org/sqlite's "mode=ro" DSN query parameter is silently ignored —
// confirmed against the driver's own test suite (all_test.go's
// TestInMemory: "// This parameter should be ignored"); this pure-Go
// driver always opens with SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE
// regardless of what the DSN says, so "mode=ro" would be a lie here. Real
// enforcement instead comes from `PRAGMA query_only = ON`, applied via the
// driver's `_pragma` DSN parameter: SQLite rejects any write statement on
// a query_only connection with "attempt to write a readonly database",
// independent of how the file was opened. The `file:` prefix is required
// for the driver to honor the query string at all — without it, the
// driver's DSN parser strips everything after `?` before opening.
func OpenReadOnly(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=query_only(1)", path))
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

// OpenForWrites opens a second, genuinely writable connection to the SQLite
// database at path for a separate process — the MCP server subprocess's
// watchlist write-pilot tools (add_to_watchlist/remove_from_watchlist, see
// internal/mcptools and PLAN.md's Phase 3.5 "watchlist 寫入工具" item).
// Deliberately a distinct connection from OpenReadOnly (not just "the same
// thing minus query_only") so the four pre-existing read-only tools keep
// their hard DB-level guarantee that they can never write, even if a future
// bug tried to route a write through them — only the tools that are
// explicitly meant to mutate data get a handle capable of it.
//
// Like OpenReadOnly, this skips migrate()/seedSP500() (schema ownership
// stays with the main bot process) and sets busy_timeout so a write here
// waits rather than immediately erroring if it briefly races the main
// process's own connection (see New's doc comment).
func OpenForWrites(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
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
	// 4: universe is Phase 2.6's candidate scan pool — much bigger than
	// watchlist, seeded once from an embedded S&P 500 list (see universe.go's
	// seedSP500) plus whatever the user adds manually via /universe add.
	// scan_hits logs which universe tickers the daily scan job found a fresh
	// RSI/MACD signal on (no uniqueness constraint: a ticker can log more than
	// one hit the same day) so the same evening's daily report can pull
	// today's rows and upgrade those tickers into LLM candidates.
	`
	CREATE TABLE IF NOT EXISTS universe (
		ticker TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS scan_hits (
		ticker TEXT NOT NULL,
		date TEXT NOT NULL,
		reason TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 5: recommendations gains a source column ("watchlist"/"movers"/"scan")
	// so /track can break its hit rate down by which candidate-sourcing path
	// actually produced a given call (Phase 3.8's deferred-from-2.6 "成效對照").
	// Existing rows get "" rather than being backfilled to "watchlist" — the
	// read path treats an empty source as "watchlist" for display, keeping
	// this migration a single cheap ALTER TABLE.
	`ALTER TABLE recommendations ADD COLUMN source TEXT NOT NULL DEFAULT '';`,
	// 6: settings is a generic single-value key/value table, first used by
	// Phase 3.6's manually-declared cash balance (key "cash_balance") — see
	// GetSetting/SetSetting. Generic rather than a dedicated cash_balance
	// table since "a table that stores a single value" is exactly what
	// PLAN.md's Phase 3.6 item asked for, and any future single-value
	// setting (there will likely be more as this grows into a broader
	// personal assistant, per CLAUDE.md's project description) reuses this
	// table instead of its own one-off migration.
	`
	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 7: thesis holds one free-text holding rationale per ticker (Phase 3.6
	// expansion's "論點日誌"), set/overwritten wholesale by /thesis — a
	// dedicated table rather than another settings key, since settings is for
	// single global values and this is one row per ticker. Deliberately no
	// history (no timestamped multi-entry log): a single-user low-frequency
	// bot doesn't need a thesis audit trail, just "what do I currently
	// believe about this position."
	`
	CREATE TABLE IF NOT EXISTS thesis (
		ticker TEXT PRIMARY KEY,
		thesis TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 8: pending_actions backs Phase 4's write-gating infrastructure (see
	// pending_actions.go) — a write tool running in the MCP subprocess (e.g.
	// record_buy/record_sell) has no Telegram bot of its own to ask for
	// confirmation, so it can only leave a proposal here; status moves
	// pending -> sent -> confirmed/rejected, driven by the main bot process.
	// No foreign key to any other table: action_type plus a free-form JSON
	// payload is enough for the bot to know what to execute once confirmed,
	// which keeps this table reusable for any future write-gated action
	// type, not just trades.
	`
	CREATE TABLE IF NOT EXISTS pending_actions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		action_type TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
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

func (d *DB) SaveRecommendations(date string, recs []Recommendation) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range recs {
		_, err := tx.Exec(`INSERT INTO recommendations (date, ticker, action, reason, price, source) VALUES (?, ?, ?, ?, ?, ?)`,
			date, r.Ticker, r.Action, r.Reason, r.Price, r.Source)
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
		`SELECT id, date, ticker, action, reason, price, source FROM recommendations
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
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price, &r.Source); err != nil {
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
		SELECT id, date, ticker, action, reason, price FROM recommendations
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
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price); err != nil {
			return nil, err
		}
		out[r.Ticker] = r
	}
	return out, rows.Err()
}

// GetEarliestBuyDate returns the date of ticker's first recorded BUY
// transaction, or ok=false if there is none on record. Used to anchor the
// trailing-stop peak-close lookup to "since this position was first opened"
// rather than an arbitrary window.
func (d *DB) GetEarliestBuyDate(ticker string) (string, bool, error) {
	var date sql.NullString
	err := d.conn.QueryRow(
		`SELECT MIN(date) FROM transactions WHERE ticker = ? AND side = 'BUY'`,
		ticker,
	).Scan(&date)
	if err != nil {
		return "", false, err
	}
	if !date.Valid {
		return "", false, nil
	}
	return date.String, true, nil
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

// GetLatestNetWorth returns the most recent net_worth_snapshots row
// regardless of date, or ok=false if none exists yet — same "most recent
// regardless of date" shape as GetLatestSnapshot, for the weekly review's
// net-worth line (Phase 3.6 PR2), net_worth_snapshots' first reader.
func (d *DB) GetLatestNetWorth() (date string, total float64, ok bool, err error) {
	err = d.conn.QueryRow(
		`SELECT date, total_value FROM net_worth_snapshots ORDER BY date DESC LIMIT 1`,
	).Scan(&date, &total)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return date, total, true, nil
}

// GetNetWorthOnOrBefore returns the most recent net_worth_snapshots row with
// date <= the given date, or ok=false if none exists — used to find a
// baseline "about a week ago" even when that exact date wasn't a trading
// day (weekend/holiday), same reasoning as GetPeakClose's date-range query.
func (d *DB) GetNetWorthOnOrBefore(date string) (float64, bool, error) {
	var total float64
	err := d.conn.QueryRow(
		`SELECT total_value FROM net_worth_snapshots WHERE date <= ? ORDER BY date DESC LIMIT 1`,
		date,
	).Scan(&total)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return total, true, nil
}

// GetSetting returns the stored value for key, or ok=false if it's never
// been set.
func (d *DB) GetSetting(key string) (string, bool, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSetting upserts value for key.
func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		key, value,
	)
	return err
}

// GetThesis returns the user's holding rationale for ticker, or ok=false if
// /thesis has never been run for it.
func (d *DB) GetThesis(ticker string) (string, bool, error) {
	var thesis string
	err := d.conn.QueryRow(`SELECT thesis FROM thesis WHERE ticker = ?`, ticker).Scan(&thesis)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return thesis, true, nil
}

// SetThesis upserts ticker's holding rationale, replacing whatever was there
// before — see the thesis table's migration-7 doc comment for why this is a
// single overwritable field rather than a journaled history.
func (d *DB) SetThesis(ticker, thesis string) error {
	_, err := d.conn.Exec(`
		INSERT INTO thesis (ticker, thesis, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker) DO UPDATE SET
			thesis = excluded.thesis,
			updated_at = excluded.updated_at`,
		ticker, thesis,
	)
	return err
}

// Backup writes a consistent point-in-time copy of the database to destPath
// via SQLite's VACUUM INTO, which is safe to run against a live database
// (unlike copying the file directly, which can catch it mid-write or miss
// WAL-journaled pages). destPath is bound as a parameter rather than
// interpolated into the SQL string, even though it's caller-controlled
// (config, not user input) — no reason to hand-format a file path into SQL
// when the driver can bind it safely.
func (d *DB) Backup(destPath string) error {
	_, err := d.conn.Exec(`VACUUM INTO ?`, destPath)
	return err
}

func (d *DB) Close() error {
	return d.conn.Close()
}

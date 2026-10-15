package db

import (
	"database/sql"

	"argus/internal/market"
)

// TransactionExtIDExists reports whether a transactions row with this
// ext_id has already been recorded (Phase 16's Shioaji sync dedup key —
// see migration 17). Always false for "" since that's not a real identity.
func (d *DB) TransactionExtIDExists(extID string) (bool, error) {
	if extID == "" {
		return false, nil
	}
	var exists bool
	err := d.conn.QueryRow(`SELECT EXISTS(SELECT 1 FROM transactions WHERE ext_id = ?)`, extID).Scan(&exists)
	return exists, err
}

// GetPosition returns the current position for ticker, or ok=false if
// there's no open position.
// openedDateSubquery is the oldest still-open BUY lot's date for a ticker
// (FIFO, same lots RecordSell consumes) — "held since", distinct from
// positions.updated_at which moves on every top-up/partial sell.
const openedDateSubquery = `COALESCE((SELECT MIN(date) FROM transactions WHERE ticker = positions.ticker AND side = 'BUY' AND remaining_shares > 1e-9), '')`

func (d *DB) GetPosition(ticker string) (Position, bool, error) {
	p := Position{Ticker: ticker}
	err := d.conn.QueryRow(
		`SELECT shares, avg_cost, stop_price, market, updated_at, `+openedDateSubquery+` FROM positions WHERE ticker = ?`, ticker,
	).Scan(&p.Shares, &p.AvgCost, &p.StopPrice, &p.Market, &p.UpdatedAt, &p.OpenedDate)
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
	rows, err := d.conn.Query(`SELECT ticker, shares, avg_cost, stop_price, market, updated_at, ` + openedDateSubquery + ` FROM positions ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var positions []Position
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.Ticker, &p.Shares, &p.AvgCost, &p.StopPrice, &p.Market, &p.UpdatedAt, &p.OpenedDate); err != nil {
			return nil, err
		}
		positions = append(positions, p)
	}
	return positions, rows.Err()
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

// GetTransactions returns every recorded buy/sell for ticker, oldest first —
// transactions has been write-only (RecordBuy/RecordSell) until Phase 3.8's
// sell-review feature needed to read the full history back to segment it
// into trade rounds (see bot.lastClosedRound).
func (d *DB) GetTransactions(ticker string) ([]Transaction, error) {
	rows, err := d.conn.Query(`
		SELECT id, ticker, side, shares, price, fee, date, realized_pnl, stop_price, market, created_at
		FROM transactions WHERE ticker = ? ORDER BY date, id`,
		ticker,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.Ticker, &t.Side, &t.Shares, &t.Price, &t.Fee, &t.Date, &t.RealizedPnL, &t.StopPrice, &t.Market, &t.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

// GetAllTransactions returns every recorded buy/sell across every ticker,
// oldest first — the whole-table counterpart to GetTransactions(ticker).
// internal/web's daily P&L replay engine needs this: reconstructing shares
// held on any given day requires walking every transaction from the very
// first one, not a bounded window (unlike GetTransactionStats' date-range
// aggregate, which only needs a window's totals, not per-row detail).
func (d *DB) GetAllTransactions() ([]Transaction, error) {
	rows, err := d.conn.Query(`
		SELECT id, ticker, side, shares, price, fee, date, realized_pnl, stop_price, market, created_at
		FROM transactions ORDER BY date, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.Ticker, &t.Side, &t.Shares, &t.Price, &t.Fee, &t.Date, &t.RealizedPnL, &t.StopPrice, &t.Market, &t.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

// GetRealizedPnL sums realized_pnl across every SELL transaction ever
// recorded in market, for /portfolio's per-market cumulative realized P&L
// line (Phase 6: money never sums across markets, see
// docs/phase-6-tw-market.md §3.2).
func (d *DB) GetRealizedPnL(m market.MarketID) (float64, error) {
	var total sql.NullFloat64
	if err := d.conn.QueryRow(`SELECT SUM(realized_pnl) FROM transactions WHERE side = 'SELL' AND market = ?`, string(m)).Scan(&total); err != nil {
		return 0, err
	}
	return total.Float64, nil
}

// GetTransactionStats summarizes every transaction with date in [from, to]
// inclusive, for the monthly report's trade-count and realized-P&L lines.
// sellCount is returned separately from realized (rather than the caller
// inferring "no sells" from realized == 0) because a month with SELLs that
// happen to net to exactly zero must still render its realized-P&L line —
// see docs/phase-3.6-monthly-report.md's distinction between "nothing to
// show" (skip) and "the number is zero" (show it).
func (d *DB) GetTransactionStats(from, to string) (count, sellCount int, realized float64, err error) {
	var sellCountNull sql.NullInt64
	var realizedNull sql.NullFloat64
	err = d.conn.QueryRow(`
		SELECT COUNT(*),
		       SUM(CASE WHEN side = 'SELL' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN side = 'SELL' THEN realized_pnl ELSE 0 END)
		FROM transactions WHERE date BETWEEN ? AND ?`,
		from, to,
	).Scan(&count, &sellCountNull, &realizedNull)
	if err != nil {
		return 0, 0, 0, err
	}
	return count, int(sellCountNull.Int64), realizedNull.Float64, nil
}

// GetTransactionStatsByMarket is GetTransactionStats scoped to one market
// (Phase 6 PR2, see docs/phase-6-tw-market.md §5.3) — RunMonthlyReport's
// per-market blocks must not sum a US and a TW transaction into the same
// count/realized-P&L figure, so this filters by the transactions.market
// column (populated at write time by RecordBuy/RecordSell, see that
// migration's doc comment) rather than reusing the whole-table query.
func (d *DB) GetTransactionStatsByMarket(from, to string, m market.MarketID) (count, sellCount int, realized float64, err error) {
	var sellCountNull sql.NullInt64
	var realizedNull sql.NullFloat64
	err = d.conn.QueryRow(`
		SELECT COUNT(*),
		       SUM(CASE WHEN side = 'SELL' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN side = 'SELL' THEN realized_pnl ELSE 0 END)
		FROM transactions WHERE date BETWEEN ? AND ? AND market = ?`,
		from, to, string(m),
	).Scan(&count, &sellCountNull, &realizedNull)
	if err != nil {
		return 0, 0, 0, err
	}
	return count, int(sellCountNull.Int64), realizedNull.Float64, nil
}

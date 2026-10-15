package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"argus/internal/market"
)

// ErrNoPosition is returned by RecordSell when there is no open position for
// the ticker to sell from.
var ErrNoPosition = errors.New("no position for ticker")

// ErrInsufficientShares is returned by RecordSell when the sell size exceeds
// the currently held shares — this project only tracks long positions, so
// selling more than is held would go negative, which isn't representable.
var ErrInsufficientShares = errors.New("insufficient shares for sell")

// ErrTransactionNotFound is returned by DeleteTransaction when id doesn't
// exist.
var ErrTransactionNotFound = errors.New("transaction not found")

// ErrNotLatestTransaction is returned by DeleteTransaction when id isn't the
// most recently recorded transaction for its ticker — see that function's
// doc comment for why only the latest one is safe to undo.
var ErrNotLatestTransaction = errors.New("only the most recent transaction for a ticker can be deleted")

// Position is the current open holding for a ticker: total shares and the
// cost-basis-weighted average price paid across all buys, net of sells.
// StopPrice is Phase 3.11's per-trade stop-loss level (0 = unset, meaning
// checkStopLossAlerts falls back to the global STOP_LOSS_PCT for this
// ticker) — see SetStopPrice. It belongs to the open position, not the
// ticker itself: RecordSell deletes the positions row on a full close, and
// the stop price correctly disappears with it, while RecordBuy topping up
// an existing position deliberately leaves it untouched (whether to adjust
// a stop after adding shares is the user's call, not automatic).
type Position struct {
	Ticker     string
	Shares     float64
	AvgCost    float64
	StopPrice  float64
	Market     string // "us" / "tw" — derived from Ticker via market.Of at write time, never caller-supplied (Phase 6, see migration 12)
	UpdatedAt  time.Time
	OpenedDate string // date of the oldest still-open BUY lot (FIFO), i.e. "held since" — not UpdatedAt, which moves on every top-up/partial sell
}

// Transaction is one recorded buy or sell. RealizedPnL is only meaningful
// for SELL rows (0 for BUY) — proceeds minus the shares' cost basis at
// AvgCost, minus fee. StopPrice (Phase 8 PR1, migration 13) is likewise
// SELL-only: the position's stop_price at the moment of sale, 0 for every
// BUY row and for any SELL predating this migration or made with no stop
// set — see RecordSell and docs/phase-8-trader-analytics.md §3.1.
type Transaction struct {
	ID          int64
	Ticker      string
	Side        string // BUY / SELL
	Shares      float64
	Price       float64
	Fee         float64
	Date        string
	RealizedPnL float64
	StopPrice   float64
	Market      string // "us" / "tw" — derived from Ticker via market.Of at write time, never caller-supplied (Phase 6, see migration 12)
	CreatedAt   time.Time
}

// lotAvgCost returns ticker's total remaining shares and FIFO-weighted
// average cost across its open BUY lots (transactions rows with
// remaining_shares > 0) — the single place RecordBuy/RecordSell derive
// positions.shares/avg_cost from, rather than incrementally blending, so
// avg_cost always reflects only the purchase lots that haven't been sold
// yet. A lot's own cost per share is price + fee/shares (that buy's fee
// spread only over its own shares, not the ticker's whole position).
func lotAvgCost(tx *sql.Tx, ticker string) (shares, avgCost float64, err error) {
	rows, err := tx.Query(`
		SELECT remaining_shares, price, fee, shares FROM transactions
		WHERE ticker = ? AND side = 'BUY' AND remaining_shares > 1e-9`, ticker)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var totalShares, totalCost float64
	for rows.Next() {
		var remaining, price, fee, origShares float64
		if err := rows.Scan(&remaining, &price, &fee, &origShares); err != nil {
			return 0, 0, err
		}
		totalShares += remaining
		totalCost += remaining * (price + fee/origShares)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if totalShares <= 1e-9 {
		return 0, 0, nil
	}
	return totalShares, totalCost / totalShares, nil
}

// RecordBuy records a BUY transaction as a new FIFO purchase lot
// (remaining_shares = shares) and recomputes the ticker's position from
// every open lot via lotAvgCost. It returns the position as it stands after
// the buy.
func (d *DB) RecordBuy(ticker string, shares, price, fee float64, date string) (Position, error) {
	return d.RecordBuyExt(ticker, shares, price, fee, date, "")
}

// RecordBuyExt is RecordBuy plus extID, an idempotency key from an external
// trade source (Phase 16's Shioaji sync) — "" (RecordBuy's behavior) for
// every other caller (web/paper/CSV import). A non-empty extID that's
// already on record fails with a unique-constraint error from the
// migration-17 partial index; callers that sync from an external source
// are expected to pre-filter against that, not rely on this as their only
// dedup check.
func (d *DB) RecordBuyExt(ticker string, shares, price, fee float64, date, extID string) (Position, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return Position{}, err
	}
	defer tx.Rollback()

	m := string(market.Of(ticker))

	// A buy from flat opens a new round; a thesis drafted before it (round_start
	// '') becomes that round's. Done here, not lazily on read, so the draft can't
	// be shown for one round and then overwritten by the next.
	var held float64
	err = tx.QueryRow(`SELECT shares FROM positions WHERE ticker = ?`, ticker).Scan(&held)
	if err != nil && err != sql.ErrNoRows {
		return Position{}, err
	}
	opensRound := err == sql.ErrNoRows || held < 1e-9

	if _, err := tx.Exec(`
		INSERT INTO transactions (ticker, side, shares, price, fee, date, remaining_shares, market, ext_id)
		VALUES (?, 'BUY', ?, ?, ?, ?, ?, ?, ?)`,
		ticker, shares, price, fee, date, shares, m, extID,
	); err != nil {
		return Position{}, err
	}
	if opensRound {
		if _, err := tx.Exec(
			`UPDATE OR IGNORE round_theses SET round_start = ? WHERE ticker = ? AND round_start = ''`,
			date, ticker,
		); err != nil {
			return Position{}, err
		}
	}

	totalShares, avgCost, err := lotAvgCost(tx, ticker)
	if err != nil {
		return Position{}, err
	}

	if _, err := tx.Exec(`
		INSERT INTO positions (ticker, shares, avg_cost, market, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker) DO UPDATE SET
			shares = excluded.shares,
			avg_cost = excluded.avg_cost,
			updated_at = excluded.updated_at`,
		ticker, totalShares, avgCost, m,
	); err != nil {
		return Position{}, err
	}

	if err := tx.Commit(); err != nil {
		return Position{}, err
	}
	return Position{Ticker: ticker, Shares: totalShares, AvgCost: avgCost, Market: m}, nil
}

// RecordSell records a SELL transaction against an existing position using
// FIFO lot matching — the oldest open BUY lot(s) are consumed first, same as
// both US and TW brokers — to compute this sale's realized P&L and the
// remaining position's cost basis (lotAvgCost over whatever lots are left).
// It returns ErrNoPosition if there is nothing open for the ticker, or
// ErrInsufficientShares if shares exceeds what's held — this project only
// tracks long positions, so short-selling isn't representable. Selling the
// full position deletes the positions row rather than leaving a zero-share
// one behind.
func (d *DB) RecordSell(ticker string, shares, price, fee float64, date string) (Position, float64, error) {
	return d.RecordSellExt(ticker, shares, price, fee, date, "")
}

// RecordSellExt is RecordSell plus extID — see RecordBuyExt's doc comment.
func (d *DB) RecordSellExt(ticker string, shares, price, fee float64, date, extID string) (Position, float64, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return Position{}, 0, err
	}
	defer tx.Rollback()

	var existingShares, stopPrice float64
	err = tx.QueryRow(`SELECT shares, stop_price FROM positions WHERE ticker = ?`, ticker).Scan(&existingShares, &stopPrice)
	if err == sql.ErrNoRows {
		return Position{}, 0, ErrNoPosition
	}
	if err != nil {
		return Position{}, 0, err
	}
	if shares > existingShares {
		return Position{}, 0, ErrInsufficientShares
	}

	lotRows, err := tx.Query(`
		SELECT id, remaining_shares, price, fee, shares FROM transactions
		WHERE ticker = ? AND side = 'BUY' AND remaining_shares > 1e-9
		ORDER BY date, id`, ticker)
	if err != nil {
		return Position{}, 0, err
	}
	type openLot struct {
		id                                int64
		remaining, price, fee, origShares float64
	}
	var lots []openLot
	for lotRows.Next() {
		var l openLot
		if err := lotRows.Scan(&l.id, &l.remaining, &l.price, &l.fee, &l.origShares); err != nil {
			lotRows.Close()
			return Position{}, 0, err
		}
		lots = append(lots, l)
	}
	if err := lotRows.Err(); err != nil {
		lotRows.Close()
		return Position{}, 0, err
	}
	lotRows.Close()

	needed := shares
	var costBasis float64
	for _, lot := range lots {
		if needed <= 1e-9 {
			break
		}
		consume := math.Min(needed, lot.remaining)
		costBasis += consume * (lot.price + lot.fee/lot.origShares)
		if _, err := tx.Exec(`UPDATE transactions SET remaining_shares = ? WHERE id = ?`, lot.remaining-consume, lot.id); err != nil {
			return Position{}, 0, err
		}
		needed -= consume
	}
	if needed > 1e-9 {
		// positions.shares (checked above) is supposed to always equal the
		// sum of open lots' remaining_shares — this means the two are out of
		// sync, a data-integrity bug rather than a normal error a caller
		// should retry.
		return Position{}, 0, fmt.Errorf("db: RecordSell: %s open lots %v shares short of the %v requested — position/lot data out of sync", ticker, shares-needed, shares)
	}

	realizedPnL := shares*price - costBasis - fee
	remainingShares := existingShares - shares
	var avgCost float64

	if math.Abs(remainingShares) < 1e-9 {
		remainingShares = 0
		if _, err := tx.Exec(`DELETE FROM positions WHERE ticker = ?`, ticker); err != nil {
			return Position{}, 0, err
		}
	} else {
		_, avgCost, err = lotAvgCost(tx, ticker)
		if err != nil {
			return Position{}, 0, err
		}
		if _, err := tx.Exec(`
			UPDATE positions SET shares = ?, avg_cost = ?, updated_at = CURRENT_TIMESTAMP WHERE ticker = ?`,
			remainingShares, avgCost, ticker,
		); err != nil {
			return Position{}, 0, err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO transactions (ticker, side, shares, price, fee, date, realized_pnl, stop_price, market, ext_id)
		VALUES (?, 'SELL', ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticker, shares, price, fee, date, realizedPnL, stopPrice, string(market.Of(ticker)), extID,
	); err != nil {
		return Position{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return Position{}, 0, err
	}
	return Position{Ticker: ticker, Shares: remainingShares, AvgCost: avgCost, Market: string(market.Of(ticker))}, realizedPnL, nil
}

// DeleteTransaction undoes a single mis-entered trade — for typo recovery
// via a bot command, not general trade-history editing. It only allows
// deleting the most recent transaction recorded for its ticker
// (ErrNotLatestTransaction otherwise): an older BUY may already have had
// some of its shares FIFO-consumed by a later SELL whose realized_pnl is
// already baked into that SELL's row (see RecordSellExt), and an older
// SELL's consumed lots may have been touched by even-later trades — neither
// is safely reversible without recomputing every transaction after it. The
// latest transaction has no such successor, so undoing it is exact: a BUY's
// lot is simply removed (nothing could have consumed it yet), and a SELL's
// shares are restored to the same lots it took them from, oldest-first —
// the mirror image of the FIFO consumption RecordSellExt did, safe to
// replay in reverse because nothing later has touched those lots since.
// Returns the deleted row so the caller (bot/web) can reverse its own
// derived state, e.g. cash balance.
func (d *DB) DeleteTransaction(id int64) (Transaction, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return Transaction{}, err
	}
	defer tx.Rollback()

	var t Transaction
	err = tx.QueryRow(`
		SELECT id, ticker, side, shares, price, fee, date, realized_pnl, stop_price, market, created_at
		FROM transactions WHERE id = ?`, id,
	).Scan(&t.ID, &t.Ticker, &t.Side, &t.Shares, &t.Price, &t.Fee, &t.Date, &t.RealizedPnL, &t.StopPrice, &t.Market, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return Transaction{}, ErrTransactionNotFound
	}
	if err != nil {
		return Transaction{}, err
	}

	var maxID int64
	if err := tx.QueryRow(`SELECT MAX(id) FROM transactions WHERE ticker = ?`, t.Ticker).Scan(&maxID); err != nil {
		return Transaction{}, err
	}
	if maxID != id {
		return Transaction{}, ErrNotLatestTransaction
	}

	if t.Side == "SELL" {
		lotRows, err := tx.Query(`
			SELECT id, remaining_shares, shares FROM transactions
			WHERE ticker = ? AND side = 'BUY' ORDER BY date, id`, t.Ticker)
		if err != nil {
			return Transaction{}, err
		}
		type lot struct {
			id                int64
			remaining, shares float64
		}
		var lots []lot
		for lotRows.Next() {
			var l lot
			if err := lotRows.Scan(&l.id, &l.remaining, &l.shares); err != nil {
				lotRows.Close()
				return Transaction{}, err
			}
			lots = append(lots, l)
		}
		if err := lotRows.Err(); err != nil {
			lotRows.Close()
			return Transaction{}, err
		}
		lotRows.Close()

		need := t.Shares
		for _, l := range lots {
			if need <= 1e-9 {
				break
			}
			restore := math.Min(need, l.shares-l.remaining)
			if restore <= 1e-9 {
				continue
			}
			if _, err := tx.Exec(`UPDATE transactions SET remaining_shares = ? WHERE id = ?`, l.remaining+restore, l.id); err != nil {
				return Transaction{}, err
			}
			need -= restore
		}
		if need > 1e-9 {
			return Transaction{}, fmt.Errorf("db: DeleteTransaction: %s open lots %v shares short of the %v to restore — position/lot data out of sync", t.Ticker, t.Shares-need, t.Shares)
		}
	}

	if _, err := tx.Exec(`DELETE FROM transactions WHERE id = ?`, id); err != nil {
		return Transaction{}, err
	}

	totalShares, avgCost, err := lotAvgCost(tx, t.Ticker)
	if err != nil {
		return Transaction{}, err
	}
	if totalShares <= 1e-9 {
		if _, err := tx.Exec(`DELETE FROM positions WHERE ticker = ?`, t.Ticker); err != nil {
			return Transaction{}, err
		}
	} else {
		if _, err := tx.Exec(`
			INSERT INTO positions (ticker, shares, avg_cost, stop_price, market, updated_at)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(ticker) DO UPDATE SET
				shares = excluded.shares,
				avg_cost = excluded.avg_cost,
				updated_at = excluded.updated_at`,
			t.Ticker, totalShares, avgCost, t.StopPrice, t.Market,
		); err != nil {
			return Transaction{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Transaction{}, err
	}
	return t, nil
}

// SetStopPrice sets ticker's per-trade stop-loss price (Phase 3.11 PR1, see
// the Position.StopPrice field comment). Returns ErrNoPosition — the same
// sentinel RecordSell uses, checked the same way via errors.Is — when there
// is no open position for ticker to set it on, detected via RowsAffected
// rather than a separate existence check.
func (d *DB) SetStopPrice(ticker string, price float64) error {
	res, err := d.conn.Exec(`UPDATE positions SET stop_price = ? WHERE ticker = ?`, price, ticker)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoPosition
	}
	return nil
}

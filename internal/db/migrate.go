package db

import (
	"database/sql"
	"fmt"
	"math"
)

func (d *DB) migrate() error {
	var version int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		if err := d.applyMigration(i + 1); err != nil {
			return err
		}
	}
	return nil
}

// migrationGoSteps are the migrations that need Go code after their SQL ran
// (keyed by 1-based migration number): FIFO replay needs per-ticker running
// state a single SQL statement can't carry.
var migrationGoSteps = map[int]func(*sql.Tx) error{
	16: backfillFIFOLots,
}

// applyMigration runs migration n (1-based), its Go step if it has one, and
// the user_version bump in a single transaction. SQLite DDL is transactional,
// so a step that fails half-way (migration 12 is four ALTERs, four UPDATEs and
// a table rebuild) leaves neither a half-applied schema nor an advanced
// user_version; run unwrapped, it left the former behind and the next boot
// died re-running the step on "duplicate column".
func (d *DB) applyMigration(n int) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("migration %d: begin: %w", n, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(migrations[n-1]); err != nil {
		return fmt.Errorf("migration %d: %w", n, err)
	}
	if step := migrationGoSteps[n]; step != nil {
		if err := step(tx); err != nil {
			return fmt.Errorf("migration %d backfill: %w", n, err)
		}
	}
	// PRAGMA doesn't support parameter binding.
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, n)); err != nil {
		return fmt.Errorf("set user_version %d: %w", n, err)
	}
	return tx.Commit()
}

// fifoLot is one BUY transaction's purchase lot state while backfillFIFOLots
// replays history — remaining starts at shares and is decremented by later
// SELLs in the same ticker, oldest lot first.
type fifoLot struct {
	id             int64
	remaining      float64
	price, fee     float64
	originalShares float64
}

// backfillFIFOLots is migration 16's one-time Go-code step (see that
// migration's doc comment): it replays every ticker's transaction history in
// date order to compute each BUY row's true remaining_shares under FIFO —
// the same lot-consumption logic RecordSell now uses going forward — then
// recomputes positions.avg_cost for every currently open position from the
// resulting open lots, which is what actually fixes the reported mismatch
// for positions that already existed before this migration ran.
// It runs inside applyMigration's transaction (tx), so it neither begins nor
// commits one of its own.
func backfillFIFOLots(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, ticker, side, shares, price, fee FROM transactions ORDER BY ticker, date, id`)
	if err != nil {
		return err
	}
	openLots := make(map[string][]*fifoLot)
	for rows.Next() {
		var id int64
		var ticker, side string
		var shares, price, fee float64
		if err := rows.Scan(&id, &ticker, &side, &shares, &price, &fee); err != nil {
			rows.Close()
			return err
		}
		switch side {
		case "BUY":
			openLots[ticker] = append(openLots[ticker], &fifoLot{id: id, remaining: shares, price: price, fee: fee, originalShares: shares})
		case "SELL":
			lots := openLots[ticker]
			needed := shares
			for len(lots) > 0 && needed > 1e-9 {
				lot := lots[0]
				consume := math.Min(needed, lot.remaining)
				lot.remaining -= consume
				needed -= consume
				if lot.remaining <= 1e-9 {
					lots = lots[1:]
				}
			}
			openLots[ticker] = lots
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, lots := range openLots {
		for _, lot := range lots {
			if lot.remaining <= 1e-9 {
				continue
			}
			if _, err := tx.Exec(`UPDATE transactions SET remaining_shares = ? WHERE id = ?`, lot.remaining, lot.id); err != nil {
				return err
			}
		}
	}

	posRows, err := tx.Query(`SELECT ticker FROM positions`)
	if err != nil {
		return err
	}
	var tickers []string
	for posRows.Next() {
		var t string
		if err := posRows.Scan(&t); err != nil {
			posRows.Close()
			return err
		}
		tickers = append(tickers, t)
	}
	if err := posRows.Err(); err != nil {
		posRows.Close()
		return err
	}
	posRows.Close()

	for _, ticker := range tickers {
		lots := openLots[ticker]
		var totalShares, totalCost float64
		for _, lot := range lots {
			if lot.remaining <= 1e-9 {
				continue
			}
			costPerShare := lot.price + lot.fee/lot.originalShares
			totalShares += lot.remaining
			totalCost += lot.remaining * costPerShare
		}
		if totalShares <= 1e-9 {
			continue
		}
		if _, err := tx.Exec(`UPDATE positions SET avg_cost = ? WHERE ticker = ?`, totalCost/totalShares, ticker); err != nil {
			return err
		}
	}

	return nil
}

package db

import (
	"database/sql"
	"errors"
	"math"
	"time"

	"argus/internal/option"
)

// ErrCrossesZero is returned by RecordOption when an order would flip a
// position from long to short (or vice versa) in one trade — e.g. selling 3
// contracts against a 2-long position. Rejected rather than split into
// "close 2, open short 1" automatically: ponytail: allowing it would double
// RecordOption's branch count for a mistake nobody makes by accident (see
// docs/phase-12-options.md §3.3).
var ErrCrossesZero = errors.New("order crosses zero — close the position before reversing it")

// ErrOptionSymbolTaken is returned by EditOptionPosition when the corrected
// contract is already held as a separate position.
var ErrOptionSymbolTaken = errors.New("that contract already has an open position")

// ErrOptionStockLinked is returned when a correction would take a close into
// or out of ASSIGNED/EXERCISED: those rows generated a stock trade through
// resolveOption and nothing links the two, so the stock side couldn't follow.
var ErrOptionStockLinked = errors.New("assigned or exercised close has a stock trade")

// OptionPosition is one open contract (migration 15, see migrations.go). Contracts
// is signed — positive for a long (buyer) position, negative for a short
// (seller) position — the single abstraction RecordOption's realized P&L
// formula depends on (docs/phase-12-options.md §3.2). AvgPremium is always
// positive (the per-share premium paid or received on average, folding in
// fees the same way Position.AvgCost folds in RecordBuy's fee).
type OptionPosition struct {
	ContractSymbol string
	Underlying     string
	Right          string // "C" or "P"
	Strike         float64
	Expiry         string // YYYY-MM-DD
	Multiplier     int64
	Contracts      float64
	AvgPremium     float64
	StopPremium    float64
	Edited         bool
	UpdatedAt      time.Time
}

// OptionTransaction is one recorded option order. Contracts is always
// positive (the order size); Action (BTO/STC/STO/BTC/EXPIRED/ASSIGNED/
// EXERCISED) records what it meant to the position at the time, derived by
// RecordOption rather than caller-supplied — see docs/phase-12-options.md
// §3.3.
type OptionTransaction struct {
	ID             int64
	ContractSymbol string
	Underlying     string
	Right          string
	Strike         float64
	Expiry         string
	Multiplier     int64
	Action         string
	Contracts      float64
	Premium        float64
	Fee            float64
	Date           string
	RealizedPnL    float64
	Edited         bool
	CreatedAt      time.Time
}

// Option order actions — see OptionTransaction.Action.
const (
	OptionActionBuyToOpen   = "BTO"
	OptionActionSellToClose = "STC"
	OptionActionSellToOpen  = "STO"
	OptionActionBuyToClose  = "BTC"
	OptionActionExpired     = "EXPIRED"
	OptionActionAssigned    = "ASSIGNED"
	OptionActionExercised   = "EXERCISED"
	// OptionActionClosed is what EditOptionClosed takes for "closed by a
	// trade": whether that row is STC or BTC depends on the position's side.
	OptionActionClosed = "CLOSED"
)

const defaultOptionMultiplier = 100

// RecordOption is the options ledger's single write path — every
// /obuy/osell order and every EXPIRED/ASSIGNED/EXERCISED resolution goes
// through it, so §3.2's realized-P&L formula only appears once. symbol must
// be a valid OCC contract symbol (option.Parse); side is "BUY" or "SELL";
// contracts is always a positive order size (RecordOption derives the
// signed position delta itself, same "caller states magnitude + direction
// separately" shape as RecordBuy/RecordSell's shares+side).
//
// A same-direction order (or no existing position) opens/adds, recomputing
// a weighted-average premium exactly like RecordBuy's avg_cost. An
// opposite-direction order closes/reduces, computing realized P&L from the
// position's existing signed contracts (not the order's) — see the formula
// comment inline below. An order whose size exceeds the existing position
// returns ErrCrossesZero rather than flipping long<->short in one trade.
func (d *DB) RecordOption(symbol, side string, contracts, premium, fee float64, date string) (OptionPosition, float64, error) {
	c, err := option.Parse(symbol)
	if err != nil {
		return OptionPosition{}, 0, err
	}
	symbol = option.Format(c.Underlying, c.Right, c.Expiry, c.Strike)

	tx, err := d.conn.Begin()
	if err != nil {
		return OptionPosition{}, 0, err
	}
	defer tx.Rollback()

	var existingContracts, existingAvgPremium, existingStop float64
	multiplier := int64(defaultOptionMultiplier)
	err = tx.QueryRow(
		`SELECT contracts, avg_premium, stop_premium, multiplier FROM option_positions WHERE contract_symbol = ?`,
		symbol,
	).Scan(&existingContracts, &existingAvgPremium, &existingStop, &multiplier)
	hasPosition := true
	if err == sql.ErrNoRows {
		hasPosition = false
		err = nil
	}
	if err != nil {
		return OptionPosition{}, 0, err
	}

	delta := contracts
	if side == "SELL" {
		delta = -contracts
	}
	newContracts := existingContracts + delta

	var action string
	var realizedPnL float64
	opening := !hasPosition || sameSign(existingContracts, delta)

	if opening {
		if delta > 0 {
			action = OptionActionBuyToOpen
		} else {
			action = OptionActionSellToOpen
		}
		totalAbs := math.Abs(existingContracts) + contracts
		totalDollars := math.Abs(existingContracts)*existingAvgPremium*float64(multiplier) + contracts*premium*float64(multiplier) + fee
		newAvgPremium := totalDollars / (totalAbs * float64(multiplier))

		if _, err := tx.Exec(`
			INSERT INTO option_positions (contract_symbol, underlying, right, strike, expiry, multiplier, contracts, avg_premium, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(contract_symbol) DO UPDATE SET
				contracts = excluded.contracts,
				avg_premium = excluded.avg_premium,
				updated_at = excluded.updated_at`,
			symbol, c.Underlying, string(c.Right), c.Strike, c.Expiry.Format("2006-01-02"), defaultOptionMultiplier, newContracts, newAvgPremium,
		); err != nil {
			return OptionPosition{}, 0, err
		}
		existingAvgPremium = newAvgPremium
	} else {
		if math.Abs(delta) > math.Abs(existingContracts) {
			return OptionPosition{}, 0, ErrCrossesZero
		}
		if delta > 0 {
			action = OptionActionBuyToClose
		} else {
			action = OptionActionSellToClose
		}

		// realized = (exit − entry) × contracts × multiplier − fees, where
		// "contracts" here is the *closed* amount signed the same as the
		// existing position being closed (docs/phase-12-options.md §3.2) —
		// not this order's own signed delta.
		closedAbs := math.Min(math.Abs(delta), math.Abs(existingContracts))
		closedSigned := closedAbs
		if existingContracts < 0 {
			closedSigned = -closedAbs
		}
		realizedPnL = (premium-existingAvgPremium)*closedSigned*float64(multiplier) - fee

		if math.Abs(newContracts) < 1e-9 {
			newContracts = 0
			if _, err := tx.Exec(`DELETE FROM option_positions WHERE contract_symbol = ?`, symbol); err != nil {
				return OptionPosition{}, 0, err
			}
		} else {
			if _, err := tx.Exec(
				`UPDATE option_positions SET contracts = ?, updated_at = CURRENT_TIMESTAMP WHERE contract_symbol = ?`,
				newContracts, symbol,
			); err != nil {
				return OptionPosition{}, 0, err
			}
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO option_transactions (contract_symbol, underlying, right, strike, expiry, multiplier, action, contracts, premium, fee, date, realized_pnl, entry_premium)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		symbol, c.Underlying, string(c.Right), c.Strike, c.Expiry.Format("2006-01-02"), multiplier, action, contracts, premium, fee, date, realizedPnL, existingAvgPremium,
	); err != nil {
		return OptionPosition{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return OptionPosition{}, 0, err
	}

	return OptionPosition{
		ContractSymbol: symbol,
		Underlying:     c.Underlying,
		Right:          string(c.Right),
		Strike:         c.Strike,
		Expiry:         c.Expiry.Format("2006-01-02"),
		Multiplier:     multiplier,
		Contracts:      newContracts,
		AvgPremium:     existingAvgPremium,
		StopPremium:    existingStop,
	}, realizedPnL, nil
}

func sameSign(a, b float64) bool {
	return (a > 0 && b > 0) || (a < 0 && b < 0)
}

// GetOptionPositions returns every open option position, ordered by
// underlying/expiry/strike.
func (d *DB) GetOptionPositions() ([]OptionPosition, error) {
	rows, err := d.conn.Query(`
		SELECT contract_symbol, underlying, right, strike, expiry, multiplier, contracts, avg_premium, stop_premium, edited, updated_at
		FROM option_positions ORDER BY underlying, expiry, strike`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OptionPosition
	for rows.Next() {
		var p OptionPosition
		if err := rows.Scan(&p.ContractSymbol, &p.Underlying, &p.Right, &p.Strike, &p.Expiry, &p.Multiplier, &p.Contracts, &p.AvgPremium, &p.StopPremium, &p.Edited, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetOptionPositionsByUnderlying returns underlying's open option positions
// — backs the covered-call/CSP collateral checks (§3.5) and /oassign.
func (d *DB) GetOptionPositionsByUnderlying(underlying string) ([]OptionPosition, error) {
	rows, err := d.conn.Query(`
		SELECT contract_symbol, underlying, right, strike, expiry, multiplier, contracts, avg_premium, stop_premium, edited, updated_at
		FROM option_positions WHERE underlying = ? ORDER BY expiry, strike`, underlying)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OptionPosition
	for rows.Next() {
		var p OptionPosition
		if err := rows.Scan(&p.ContractSymbol, &p.Underlying, &p.Right, &p.Strike, &p.Expiry, &p.Multiplier, &p.Contracts, &p.AvgPremium, &p.StopPremium, &p.Edited, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetOptionPosition returns a single open position by contract symbol, or
// ok=false if it isn't held.
func (d *DB) GetOptionPosition(symbol string) (OptionPosition, bool, error) {
	p := OptionPosition{ContractSymbol: symbol}
	err := d.conn.QueryRow(`
		SELECT underlying, right, strike, expiry, multiplier, contracts, avg_premium, stop_premium, edited, updated_at
		FROM option_positions WHERE contract_symbol = ?`, symbol,
	).Scan(&p.Underlying, &p.Right, &p.Strike, &p.Expiry, &p.Multiplier, &p.Contracts, &p.AvgPremium, &p.StopPremium, &p.Edited, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return OptionPosition{}, false, nil
	}
	if err != nil {
		return OptionPosition{}, false, err
	}
	return p, true, nil
}

// GetAllOptionTransactions returns every option order across every
// underlying, newest first — backs the web dashboard's options page closed-
// position list (Phase 12 PR4), which needs the full history rather than
// one underlying at a time.
func (d *DB) GetAllOptionTransactions() ([]OptionTransaction, error) {
	rows, err := d.conn.Query(`
		SELECT id, contract_symbol, underlying, right, strike, expiry, multiplier, action, contracts, premium, fee, date, realized_pnl, edited, created_at
		FROM option_transactions ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OptionTransaction
	for rows.Next() {
		var t OptionTransaction
		if err := rows.Scan(&t.ID, &t.ContractSymbol, &t.Underlying, &t.Right, &t.Strike, &t.Expiry, &t.Multiplier, &t.Action, &t.Contracts, &t.Premium, &t.Fee, &t.Date, &t.RealizedPnL, &t.Edited, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetOptionStop sets symbol's per-contract stop premium — mirrors
// SetStopPrice's ErrNoPosition-via-RowsAffected shape.
func (d *DB) SetOptionStop(symbol string, premium float64) error {
	res, err := d.conn.Exec(`UPDATE option_positions SET stop_premium = ? WHERE contract_symbol = ?`, premium, symbol)
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

// ResolveOption closes symbol's entire remaining position at premium 0
// under an explicit action (OptionActionExpired/Assigned/Exercised) rather
// than RecordOption's derived BTO/STC/STO/BTC — the caller already knows
// exactly what happened, and forcing a "trade" through RecordOption's
// direction-inference logic would be a worse fit than just writing what
// occurred. Realized P&L is the same §3.2 formula with exit=0: a short
// position's full premium is pocketed (positive), a long position's full
// premium is forfeited (negative). Returns ErrNoPosition if symbol isn't
// currently held.
func (d *DB) ResolveOption(symbol, action, date string) (OptionPosition, float64, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return OptionPosition{}, 0, err
	}
	defer tx.Rollback()

	var existingContracts, existingAvgPremium float64
	multiplier := int64(defaultOptionMultiplier)
	err = tx.QueryRow(
		`SELECT contracts, avg_premium, multiplier FROM option_positions WHERE contract_symbol = ?`,
		symbol,
	).Scan(&existingContracts, &existingAvgPremium, &multiplier)
	if err == sql.ErrNoRows {
		return OptionPosition{}, 0, ErrNoPosition
	}
	if err != nil {
		return OptionPosition{}, 0, err
	}

	c, err := option.Parse(symbol)
	if err != nil {
		return OptionPosition{}, 0, err
	}

	realizedPnL := -existingAvgPremium * existingContracts * float64(multiplier)

	if _, err := tx.Exec(`DELETE FROM option_positions WHERE contract_symbol = ?`, symbol); err != nil {
		return OptionPosition{}, 0, err
	}
	if _, err := tx.Exec(`
		INSERT INTO option_transactions (contract_symbol, underlying, right, strike, expiry, multiplier, action, contracts, premium, fee, date, realized_pnl, entry_premium)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)`,
		symbol, c.Underlying, string(c.Right), c.Strike, c.Expiry.Format("2006-01-02"), multiplier, action, math.Abs(existingContracts), date, realizedPnL, existingAvgPremium,
	); err != nil {
		return OptionPosition{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return OptionPosition{}, 0, err
	}

	return OptionPosition{
		ContractSymbol: symbol,
		Underlying:     c.Underlying,
		Right:          string(c.Right),
		Strike:         c.Strike,
		Expiry:         c.Expiry.Format("2006-01-02"),
		Multiplier:     multiplier,
		Contracts:      0,
		AvgPremium:     existingAvgPremium,
	}, realizedPnL, nil
}

// lastOptionCloseID is symbol's newest closing row, 0 if none: the BTO/STO
// rows after it are the ones that opened the current position.
func lastOptionCloseID(tx *sql.Tx, symbol string) (int64, error) {
	var id int64
	err := tx.QueryRow(`
		SELECT COALESCE(MAX(id), 0) FROM option_transactions
		WHERE contract_symbol = ? AND action NOT IN (?, ?)`,
		symbol, OptionActionBuyToOpen, OptionActionSellToOpen,
	).Scan(&id)
	return id, err
}

// EditOptionPosition corrects an open position: any of its contract, side,
// size or average premium, for an entry typed wrong. It writes the position
// directly rather than a new order, because the old values never happened.
// The ledger follows the contract and the side but keeps its original sizes
// and premiums; nothing computes from BTO/STO rows, the position row is the
// live state. side is "BUY" or "SELL", contracts a positive size.
func (d *DB) EditOptionPosition(symbol, newSymbol, side string, contracts, premium float64) (OptionPosition, error) {
	c, err := option.Parse(newSymbol)
	if err != nil {
		return OptionPosition{}, err
	}
	newSymbol = option.Format(c.Underlying, c.Right, c.Expiry, c.Strike)
	expiry := c.Expiry.Format("2006-01-02")
	signed, action := contracts, OptionActionBuyToOpen
	if side == "SELL" {
		signed, action = -contracts, OptionActionSellToOpen
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return OptionPosition{}, err
	}
	defer tx.Rollback()

	if newSymbol != symbol {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM option_positions WHERE contract_symbol = ?`, newSymbol).Scan(&n); err != nil {
			return OptionPosition{}, err
		}
		if n > 0 {
			return OptionPosition{}, ErrOptionSymbolTaken
		}
	}
	res, err := tx.Exec(`
		UPDATE option_positions SET contract_symbol = ?, underlying = ?, right = ?, strike = ?, expiry = ?,
			contracts = ?, avg_premium = ?, edited = 1, updated_at = CURRENT_TIMESTAMP
		WHERE contract_symbol = ?`,
		newSymbol, c.Underlying, string(c.Right), c.Strike, expiry, signed, premium, symbol)
	if err != nil {
		return OptionPosition{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return OptionPosition{}, err
	} else if n == 0 {
		return OptionPosition{}, ErrNoPosition
	}
	if _, err := tx.Exec(`
		UPDATE option_transactions SET contract_symbol = ?, underlying = ?, right = ?, strike = ?, expiry = ?
		WHERE contract_symbol = ?`,
		newSymbol, c.Underlying, string(c.Right), c.Strike, expiry, symbol); err != nil {
		return OptionPosition{}, err
	}
	lastClose, err := lastOptionCloseID(tx, newSymbol)
	if err != nil {
		return OptionPosition{}, err
	}
	if _, err := tx.Exec(`
		UPDATE option_transactions SET action = ?
		WHERE contract_symbol = ? AND id > ? AND action IN (?, ?)`,
		action, newSymbol, lastClose, OptionActionBuyToOpen, OptionActionSellToOpen); err != nil {
		return OptionPosition{}, err
	}
	if err := tx.Commit(); err != nil {
		return OptionPosition{}, err
	}
	return OptionPosition{ContractSymbol: newSymbol, Underlying: c.Underlying, Right: string(c.Right), Strike: c.Strike, Expiry: expiry, Contracts: signed, AvgPremium: premium, Multiplier: defaultOptionMultiplier, Edited: true}, nil
}

// DeleteOptionPosition removes a position entered by mistake, with the opens
// behind it. Earlier closes of the same contract stay: they already happened.
func (d *DB) DeleteOptionPosition(symbol string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM option_positions WHERE contract_symbol = ?`, symbol)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNoPosition
	}
	lastClose, err := lastOptionCloseID(tx, symbol)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM option_transactions WHERE contract_symbol = ? AND id > ? AND action IN (?, ?)`,
		symbol, lastClose, OptionActionBuyToOpen, OptionActionSellToOpen); err != nil {
		return err
	}
	return tx.Commit()
}

// optionClose is the part of a closing row its corrections need.
type optionClose struct {
	symbol, underlying, right, expiry, action string
	strike, contracts, entryPremium           float64
	multiplier                                int64
	short                                     bool // the position it closed
}

// getOptionClose loads closing row id, ErrTransactionNotFound for an id that
// isn't one. short comes from the action; EXPIRED is the one close that can
// be either side, so it asks the open before it.
func getOptionClose(tx *sql.Tx, id int64) (optionClose, error) {
	var c optionClose
	err := tx.QueryRow(`
		SELECT contract_symbol, underlying, right, strike, expiry, multiplier, action, contracts, entry_premium
		FROM option_transactions WHERE id = ? AND action NOT IN (?, ?)`,
		id, OptionActionBuyToOpen, OptionActionSellToOpen,
	).Scan(&c.symbol, &c.underlying, &c.right, &c.strike, &c.expiry, &c.multiplier, &c.action, &c.contracts, &c.entryPremium)
	if err == sql.ErrNoRows {
		return c, ErrTransactionNotFound
	}
	if err != nil {
		return c, err
	}
	switch c.action {
	case OptionActionBuyToClose, OptionActionAssigned:
		c.short = true
	case OptionActionExpired:
		var open string
		err := tx.QueryRow(`
			SELECT action FROM option_transactions
			WHERE contract_symbol = ? AND id < ? AND action IN (?, ?) ORDER BY id DESC LIMIT 1`,
			c.symbol, id, OptionActionBuyToOpen, OptionActionSellToOpen,
		).Scan(&open)
		if err != nil && err != sql.ErrNoRows {
			return c, err
		}
		c.short = open == OptionActionSellToOpen
	}
	return c, nil
}

func stockLinkedAction(action string) bool {
	return action == OptionActionAssigned || action == OptionActionExercised
}

// EditOptionClosed corrects a closed record. realizedPnL is taken as given,
// so it can hold what the broker actually booked. action is
// OptionActionClosed, OptionActionExpired, or the row's own ASSIGNED/EXERCISED
// (ErrOptionStockLinked for any move into or out of those).
func (d *DB) EditOptionClosed(id int64, action string, contracts float64, date string, realizedPnL float64) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	c, err := getOptionClose(tx, id)
	if err != nil {
		return err
	}
	if action == OptionActionClosed {
		action = OptionActionSellToClose
		if c.short {
			action = OptionActionBuyToClose
		}
	}
	if action != c.action && (stockLinkedAction(action) || stockLinkedAction(c.action)) {
		return ErrOptionStockLinked
	}
	if _, err := tx.Exec(`
		UPDATE option_transactions SET action = ?, contracts = ?, date = ?, realized_pnl = ?, edited = 1 WHERE id = ?`,
		action, contracts, date, realizedPnL, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteOptionClosed removes a closed record and with it its realized P&L;
// the position stays closed (UndoOptionClose is the one that reopens it).
// An ASSIGNED/EXERCISED row's stock trade is left alone.
func (d *DB) DeleteOptionClosed(id int64) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := getOptionClose(tx, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM option_transactions WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// UndoOptionClose puts a close's contracts back into the open position and
// removes the record. Weighted against whatever is open now, which for a
// partial close is the same premium again. A position since reopened on the
// other side returns ErrCrossesZero, and ASSIGNED/EXERCISED ErrOptionStockLinked.
// A stop set before a full close is not restored: its row was deleted.
func (d *DB) UndoOptionClose(id int64) (OptionPosition, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return OptionPosition{}, err
	}
	defer tx.Rollback()

	c, err := getOptionClose(tx, id)
	if err != nil {
		return OptionPosition{}, err
	}
	if stockLinkedAction(c.action) {
		return OptionPosition{}, ErrOptionStockLinked
	}
	signed := c.contracts
	if c.short {
		signed = -signed
	}

	var open, avg float64
	err = tx.QueryRow(`SELECT contracts, avg_premium FROM option_positions WHERE contract_symbol = ?`, c.symbol).Scan(&open, &avg)
	if err != nil && err != sql.ErrNoRows {
		return OptionPosition{}, err
	}
	if open != 0 && !sameSign(open, signed) {
		return OptionPosition{}, ErrCrossesZero
	}
	total := open + signed
	newAvg := (math.Abs(open)*avg + c.contracts*c.entryPremium) / math.Abs(total)

	if _, err := tx.Exec(`
		INSERT INTO option_positions (contract_symbol, underlying, right, strike, expiry, multiplier, contracts, avg_premium, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(contract_symbol) DO UPDATE SET
			contracts = excluded.contracts,
			avg_premium = excluded.avg_premium,
			updated_at = excluded.updated_at`,
		c.symbol, c.underlying, c.right, c.strike, c.expiry, c.multiplier, total, newAvg); err != nil {
		return OptionPosition{}, err
	}
	if _, err := tx.Exec(`DELETE FROM option_transactions WHERE id = ?`, id); err != nil {
		return OptionPosition{}, err
	}
	if err := tx.Commit(); err != nil {
		return OptionPosition{}, err
	}
	return OptionPosition{ContractSymbol: c.symbol, Underlying: c.underlying, Right: c.right, Strike: c.strike, Expiry: c.expiry, Multiplier: c.multiplier, Contracts: total, AvgPremium: newAvg}, nil
}

// SaveATMIV records one day's at-the-money implied volatility for
// underlying — see migration 15's doc comment for why this starts now
// despite having no consumer yet.
func (d *DB) SaveATMIV(underlying, date string, atmIV float64, dte int) error {
	_, err := d.conn.Exec(`
		INSERT OR REPLACE INTO iv_history (underlying, date, atm_iv, dte) VALUES (?, ?, ?, ?)`,
		underlying, date, atmIV, dte,
	)
	return err
}

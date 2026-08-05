package db

import (
	"database/sql"
	"math"
	"time"

	"argus/internal/option"
)

// OptionPosition is one open contract (migration 15, see db.go). Contracts
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
		INSERT INTO option_transactions (contract_symbol, underlying, right, strike, expiry, multiplier, action, contracts, premium, fee, date, realized_pnl)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		symbol, c.Underlying, string(c.Right), c.Strike, c.Expiry.Format("2006-01-02"), multiplier, action, contracts, premium, fee, date, realizedPnL,
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
		SELECT contract_symbol, underlying, right, strike, expiry, multiplier, contracts, avg_premium, stop_premium, updated_at
		FROM option_positions ORDER BY underlying, expiry, strike`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OptionPosition
	for rows.Next() {
		var p OptionPosition
		if err := rows.Scan(&p.ContractSymbol, &p.Underlying, &p.Right, &p.Strike, &p.Expiry, &p.Multiplier, &p.Contracts, &p.AvgPremium, &p.StopPremium, &p.UpdatedAt); err != nil {
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
		SELECT contract_symbol, underlying, right, strike, expiry, multiplier, contracts, avg_premium, stop_premium, updated_at
		FROM option_positions WHERE underlying = ? ORDER BY expiry, strike`, underlying)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OptionPosition
	for rows.Next() {
		var p OptionPosition
		if err := rows.Scan(&p.ContractSymbol, &p.Underlying, &p.Right, &p.Strike, &p.Expiry, &p.Multiplier, &p.Contracts, &p.AvgPremium, &p.StopPremium, &p.UpdatedAt); err != nil {
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
		SELECT underlying, right, strike, expiry, multiplier, contracts, avg_premium, stop_premium, updated_at
		FROM option_positions WHERE contract_symbol = ?`, symbol,
	).Scan(&p.Underlying, &p.Right, &p.Strike, &p.Expiry, &p.Multiplier, &p.Contracts, &p.AvgPremium, &p.StopPremium, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return OptionPosition{}, false, nil
	}
	if err != nil {
		return OptionPosition{}, false, err
	}
	return p, true, nil
}

// GetOptionTransactionsByUnderlying returns underlying's full option order
// history, newest first.
func (d *DB) GetOptionTransactionsByUnderlying(underlying string) ([]OptionTransaction, error) {
	rows, err := d.conn.Query(`
		SELECT id, contract_symbol, underlying, right, strike, expiry, multiplier, action, contracts, premium, fee, date, realized_pnl, created_at
		FROM option_transactions WHERE underlying = ? ORDER BY id DESC`, underlying)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OptionTransaction
	for rows.Next() {
		var t OptionTransaction
		if err := rows.Scan(&t.ID, &t.ContractSymbol, &t.Underlying, &t.Right, &t.Strike, &t.Expiry, &t.Multiplier, &t.Action, &t.Contracts, &t.Premium, &t.Fee, &t.Date, &t.RealizedPnL, &t.CreatedAt); err != nil {
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
		INSERT INTO option_transactions (contract_symbol, underlying, right, strike, expiry, multiplier, action, contracts, premium, fee, date, realized_pnl)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)`,
		symbol, c.Underlying, string(c.Right), c.Strike, c.Expiry.Format("2006-01-02"), multiplier, action, math.Abs(existingContracts), date, realizedPnL,
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

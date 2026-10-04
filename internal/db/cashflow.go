package db

import (
	"database/sql"
	"errors"
)

// RecurringCashflow is one recurring_cashflows row (migration 30, see
// docs/phase-9-asset-platform.md §8.4) — a hand-maintained monthly amount,
// not a transaction ledger. DayOfMonth/AssetID/Category are nil/"" when
// unset: a pure income/expense line (e.g. salary) has no linked asset, and a
// flow with no fixed day never appears in the 90-day event table, only in
// the monthly total.
type RecurringCashflow struct {
	ID         int64
	Direction  string // "in" | "out"
	Name       string
	Amount     float64
	Currency   string
	DayOfMonth *int64
	AssetID    *int64
	Category   string
	Active     bool
	// PausedAt is the day the flow was paused ("2006-01-02"); "" while active,
	// and for a flow paused before migration 38 (the day was never kept).
	PausedAt string
}

// NewRecurringCashflow is the caller-supplied subset of RecurringCashflow's
// fields for CreateRecurringCashflow — ID/Active don't exist yet at creation
// (Active always starts true).
type NewRecurringCashflow struct {
	Direction  string
	Name       string
	Amount     float64
	Currency   string
	DayOfMonth *int64
	AssetID    *int64
	Category   string
}

// CreateRecurringCashflow inserts a new recurring cashflow line, active by
// default.
func (d *DB) CreateRecurringCashflow(c NewRecurringCashflow) (int64, error) {
	if c.Currency == "" {
		c.Currency = "TWD"
	}
	res, err := d.conn.Exec(`
		INSERT INTO recurring_cashflows (direction, name, amount, currency, day_of_month, asset_id, category, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
		c.Direction, c.Name, c.Amount, c.Currency, nullableInt64(c.DayOfMonth), nullableInt64(c.AssetID), nullableString(c.Category),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecurringCashflowEdit is the editable part of a recurring cashflow. The
// direction (in/out), currency, linked asset and active flag are not here:
// those stay "pause and add a new line". Every field is overwritten, so a nil
// DayOfMonth clears the day and an empty Category clears the category.
type RecurringCashflowEdit struct {
	Name       string
	Amount     float64
	DayOfMonth *int64
	Category   string
}

// UpdateRecurringCashflow applies a RecurringCashflowEdit, paused lines
// included. Returns ErrCashflowNotFound for an unknown id.
func (d *DB) UpdateRecurringCashflow(id int64, e RecurringCashflowEdit) error {
	res, err := d.conn.Exec(`UPDATE recurring_cashflows SET name = ?, amount = ?, day_of_month = ?, category = ? WHERE id = ?`,
		e.Name, e.Amount, nullableInt64(e.DayOfMonth), nullableString(e.Category), id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrCashflowNotFound
	}
	return nil
}

// ListRecurringCashflows returns every recurring cashflow, newest first.
// activeOnly=true filters active = 1, the default for /w/cash's list and
// monthly-total math — a paused flow shouldn't silently keep counting.
func (d *DB) ListRecurringCashflows(activeOnly bool) ([]RecurringCashflow, error) {
	query := `SELECT id, direction, name, amount, currency, day_of_month, asset_id, category, active, paused_at FROM recurring_cashflows`
	if activeOnly {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY id DESC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecurringCashflow
	for rows.Next() {
		var c RecurringCashflow
		var dayOfMonth, assetID sql.NullInt64
		var category, pausedAt sql.NullString
		var active int
		if err := rows.Scan(&c.ID, &c.Direction, &c.Name, &c.Amount, &c.Currency, &dayOfMonth, &assetID, &category, &active, &pausedAt); err != nil {
			return nil, err
		}
		if dayOfMonth.Valid {
			c.DayOfMonth = &dayOfMonth.Int64
		}
		if assetID.Valid {
			c.AssetID = &assetID.Int64
		}
		c.Category = category.String
		c.PausedAt = pausedAt.String
		c.Active = active != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeactivateRecurringCashflow pauses a recurring cashflow (active = 0) and
// stamps the day it was paused. A paused flow stays in the table — a SIP put on
// hold is still meaningful, and ResumeRecurringCashflow brings it back — until
// DeleteRecurringCashflow removes it for good. Pausing an already-paused flow
// is a no-op that keeps its original date; an unknown id is ErrCashflowNotFound.
func (d *DB) DeactivateRecurringCashflow(id int64, today string) error {
	res, err := d.conn.Exec(`UPDATE recurring_cashflows SET active = 0, paused_at = ? WHERE id = ? AND active = 1`, today, id)
	if err != nil {
		return err
	}
	return d.cashflowTouched(res, id)
}

// ResumeRecurringCashflow puts a paused flow back into the monthly totals and
// clears its paused date. Resuming an active flow is a no-op; an unknown id is
// ErrCashflowNotFound.
func (d *DB) ResumeRecurringCashflow(id int64) error {
	res, err := d.conn.Exec(`UPDATE recurring_cashflows SET active = 1, paused_at = NULL WHERE id = ? AND active = 0`, id)
	if err != nil {
		return err
	}
	return d.cashflowTouched(res, id)
}

// DeleteRecurringCashflow removes a paused flow for good. An active one is
// ErrCashflowActive (it still counts toward the totals and there is no undo, so
// it has to be paused first); an unknown id is ErrCashflowNotFound.
func (d *DB) DeleteRecurringCashflow(id int64) error {
	res, err := d.conn.Exec(`DELETE FROM recurring_cashflows WHERE id = ? AND active = 0`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n > 0 {
		return nil
	}
	if err := d.cashflowTouched(res, id); err != nil {
		return err
	}
	return ErrCashflowActive
}

// cashflowTouched settles a conditional UPDATE/DELETE that matched no rows:
// either the id doesn't exist (ErrCashflowNotFound) or the row was already in
// the wanted state (nil).
func (d *DB) cashflowTouched(res sql.Result, id int64) error {
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n > 0 {
		return nil
	}
	var one int
	switch err := d.conn.QueryRow(`SELECT 1 FROM recurring_cashflows WHERE id = ?`, id).Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrCashflowNotFound
	default:
		return err
	}
}

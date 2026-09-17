package db

import "database/sql"

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

// ListRecurringCashflows returns every recurring cashflow, newest first.
// activeOnly=true filters active = 1, the default for /w/cash's list and
// monthly-total math — a paused flow shouldn't silently keep counting.
func (d *DB) ListRecurringCashflows(activeOnly bool) ([]RecurringCashflow, error) {
	query := `SELECT id, direction, name, amount, currency, day_of_month, asset_id, category, active FROM recurring_cashflows`
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
		var category sql.NullString
		var active int
		if err := rows.Scan(&c.ID, &c.Direction, &c.Name, &c.Amount, &c.Currency, &dayOfMonth, &assetID, &category, &active); err != nil {
			return nil, err
		}
		if dayOfMonth.Valid {
			c.DayOfMonth = &dayOfMonth.Int64
		}
		if assetID.Valid {
			c.AssetID = &assetID.Int64
		}
		c.Category = category.String
		c.Active = active != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeactivateRecurringCashflow pauses a recurring cashflow (active = 0)
// rather than deleting it — same soft-delete convention as ArchiveAsset, and
// for the same reason: a paused flow (e.g. a SIP put on hold) is still
// meaningful history, not garbage. A no-op on an already-inactive or
// nonexistent id.
func (d *DB) DeactivateRecurringCashflow(id int64) error {
	_, err := d.conn.Exec(`UPDATE recurring_cashflows SET active = 0 WHERE id = ? AND active = 1`, id)
	return err
}

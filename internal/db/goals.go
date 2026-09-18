package db

import "database/sql"

// Goal is one goals row (migration 31, see docs/phase-9-asset-platform.md
// §8.8) — a savings target with an optional date, e.g. "child's education
// fund" or "house down payment". The retirement row (Kind == "retirement")
// is the same table (§8.8's "retirement_plans 不另開"); PR7 owns computing
// its TargetAmount from the retirement calc, PR6 only needs the row to
// exist and render like any other goal.
type Goal struct {
	ID           int64
	Name         string
	Kind         string // "retirement" | "general"
	TargetAmount float64
	Currency     string
	TargetDate   string // "" = no target date
	Note         string
	CreatedAt    string
}

// NewGoal is the caller-supplied subset of Goal's fields for CreateGoal —
// ID/CreatedAt don't exist yet at creation time.
type NewGoal struct {
	Name         string
	Kind         string
	TargetAmount float64
	Currency     string
	TargetDate   string
	Note         string
}

// GoalAsset is one goal_assets earmark row — Ratio lets an asset count only
// partially toward a goal (e.g. a house 30% earmarked for retirement, the
// rest for nothing in particular).
type GoalAsset struct {
	GoalID  int64
	AssetID int64
	Ratio   float64
}

// CreateGoal inserts a new goal, defaulting Kind to "general" and Currency
// to "TWD" like the rest of the wealth tables.
func (d *DB) CreateGoal(g NewGoal) (int64, error) {
	if g.Kind == "" {
		g.Kind = "general"
	}
	if g.Currency == "" {
		g.Currency = "TWD"
	}
	res, err := d.conn.Exec(`
		INSERT INTO goals (name, kind, target_amount, currency, target_date, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		g.Name, g.Kind, g.TargetAmount, g.Currency, nullableString(g.TargetDate), nullableString(g.Note),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListGoals returns every goal, newest first.
func (d *DB) ListGoals() ([]Goal, error) {
	rows, err := d.conn.Query(`SELECT id, name, kind, target_amount, currency, target_date, note, created_at FROM goals ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Goal
	for rows.Next() {
		var g Goal
		var targetDate, note sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.Kind, &g.TargetAmount, &g.Currency, &targetDate, &note, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.TargetDate = targetDate.String
		g.Note = note.String
		out = append(out, g)
	}
	return out, rows.Err()
}

// DeleteGoal removes a goal and its earmarks. Unlike assets/cashflows, a
// goal has no soft-delete convention in the schema (§8.8 draws no such
// column) — it's a plan, not a financial ledger entry with history worth
// preserving, so a hard delete is the honest operation.
func (d *DB) DeleteGoal(id int64) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM goal_assets WHERE goal_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM goals WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ListAllGoalAssets returns every earmark row across every goal, so the
// handler can group them per goal in one query rather than N+1.
func (d *DB) ListAllGoalAssets() ([]GoalAsset, error) {
	rows, err := d.conn.Query(`SELECT goal_id, asset_id, ratio FROM goal_assets ORDER BY goal_id, asset_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GoalAsset
	for rows.Next() {
		var ga GoalAsset
		if err := rows.Scan(&ga.GoalID, &ga.AssetID, &ga.Ratio); err != nil {
			return nil, err
		}
		out = append(out, ga)
	}
	return out, rows.Err()
}

// UpsertRetirementGoal creates or updates the single kind="retirement" goal
// row — PR7's "存目標＋就地重算" (§8.7): the retirement page's quick-switch
// buttons call this instead of a form, since §8.8 folded retirement_plans
// into goals and there's only ever one retirement row.
func (d *DB) UpsertRetirementGoal(name string, targetAmount float64, targetDate string) (int64, error) {
	var id int64
	err := d.conn.QueryRow(`SELECT id FROM goals WHERE kind = 'retirement' LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return d.CreateGoal(NewGoal{Name: name, Kind: "retirement", TargetAmount: targetAmount, Currency: "TWD", TargetDate: targetDate})
	}
	if err != nil {
		return 0, err
	}
	_, err = d.conn.Exec(`UPDATE goals SET name = ?, target_amount = ?, target_date = ? WHERE id = ?`, name, targetAmount, targetDate, id)
	return id, err
}

// SetGoalAsset upserts an earmark's ratio, or removes it when ratio <= 0 —
// one call covers both linking an asset to a goal and unlinking it.
func (d *DB) SetGoalAsset(goalID, assetID int64, ratio float64) error {
	if ratio <= 0 {
		_, err := d.conn.Exec(`DELETE FROM goal_assets WHERE goal_id = ? AND asset_id = ?`, goalID, assetID)
		return err
	}
	_, err := d.conn.Exec(`
		INSERT INTO goal_assets (goal_id, asset_id, ratio) VALUES (?, ?, ?)
		ON CONFLICT (goal_id, asset_id) DO UPDATE SET ratio = excluded.ratio`,
		goalID, assetID, ratio,
	)
	return err
}

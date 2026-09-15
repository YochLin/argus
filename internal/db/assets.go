package db

import "database/sql"

// Asset is one row of the Phase 9 wealth platform's spine table (migration
// 29, see docs/phase-9-asset-platform.md §9.1) — deposits, loans, and every
// future asset/liability type share this table; type-specific fields live in
// a detail table keyed by asset_id (deposit_details, loan_details, ...).
// Value is never stored here — see AssetSnapshot. AssetGroup and Venue are
// "" when unset (Venue displays as "手動登錄" per the design spec; AssetGroup
// is required by the drawer, never actually blank in practice).
type Asset struct {
	ID         int64
	Side       string // "asset" | "liability"
	Type       string // "deposit" | "loan" | "insurance" | ... free-form
	Name       string
	AssetGroup string // "liquid" | "growth" | "income" | "hard"
	Venue      string
	Currency   string
	Source     string // "manual" | "import" | "sync"
	CreatedAt  string
	ArchivedAt string // "" = not archived
}

// NewAsset is the caller-supplied subset of Asset's fields — CreatedAt is
// stamped by the DB and ArchivedAt/ID don't exist yet at creation time.
type NewAsset struct {
	Side       string
	Type       string
	Name       string
	AssetGroup string
	Venue      string
	Currency   string
	Source     string
}

// DepositDetails is deposit_details' one row per deposit-type asset.
type DepositDetails struct {
	Bank        string
	AccountNote string
}

// LoanDetails is loan_details' one row per loan-type asset (including
// revolving credit-card debt, which reuses this table rather than getting
// its own — docs/phase-9-asset-platform.md §8.5). SecuredAssetID is the
// collateral asset (e.g. a mortgage's underlying property), nil when
// unsecured.
type LoanDetails struct {
	Lender            string
	RatePct           *float64
	OriginalPrincipal *float64
	RemainingMonths   *int64
	SecuredAssetID    *int64
}

// AssetSnapshot is one dated value point for an asset (migration 29,
// asset_snapshots). Cost is nil unless the asset is investment-type
// (funds/bonds) and the caller tracks accumulated cost — nil means "not
// applicable", never 0. Value is always stored in the asset's own currency;
// conversion to a display currency happens at the read layer, never here.
type AssetSnapshot struct {
	AssetID int64
	Date    string
	Value   float64
	Cost    *float64
	Source  string
}

// AssetWithValue is an Asset joined to its latest snapshot, the shape every
// net-worth/dashboard read wants. Value/AsOf are the zero value when the
// asset has no snapshot yet (a freshly created asset before its first
// value is recorded) — callers must render "—", not treat it as a real 0.
type AssetWithValue struct {
	Asset
	Value *float64
	Cost  *float64
	AsOf  string
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableFloat(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func nullableInt64(i *int64) any {
	if i == nil {
		return nil
	}
	return *i
}

// CreateAsset inserts a spine-only row (no detail table) — for asset types
// that carry no computed-upon fields, e.g. real estate/gold/crypto/labor
// pension, which all share the generic "other" type per §8.5 rather than
// each getting their own detail table.
func (d *DB) CreateAsset(a NewAsset) (int64, error) {
	if a.Currency == "" {
		a.Currency = "TWD"
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	res, err := d.conn.Exec(`
		INSERT INTO assets (side, type, name, asset_group, venue, currency, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		a.Side, a.Type, a.Name, a.AssetGroup, nullableString(a.Venue), a.Currency, a.Source,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateDepositAsset creates a deposit-type asset and its deposit_details
// row atomically — the quick-add drawer's "one flow, one write" shape
// (§8.14.3).
func (d *DB) CreateDepositAsset(a NewAsset, det DepositDetails) (int64, error) {
	if a.Currency == "" {
		a.Currency = "TWD"
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO assets (side, type, name, asset_group, venue, currency, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		a.Side, a.Type, a.Name, a.AssetGroup, nullableString(a.Venue), a.Currency, a.Source,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
		INSERT INTO deposit_details (asset_id, bank, account_note) VALUES (?, ?, ?)`,
		id, nullableString(det.Bank), nullableString(det.AccountNote),
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// CreateLoanAsset creates a loan-type asset (side is normally "liability",
// but the drawer allows either — see LoanDetails' doc comment on
// credit-card debt) and its loan_details row atomically.
func (d *DB) CreateLoanAsset(a NewAsset, det LoanDetails) (int64, error) {
	if a.Currency == "" {
		a.Currency = "TWD"
	}
	if a.Source == "" {
		a.Source = "manual"
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO assets (side, type, name, asset_group, venue, currency, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		a.Side, a.Type, a.Name, a.AssetGroup, nullableString(a.Venue), a.Currency, a.Source,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
		INSERT INTO loan_details (asset_id, lender, rate_pct, original_principal, remaining_months, secured_asset_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, nullableString(det.Lender), nullableFloat(det.RatePct), nullableFloat(det.OriginalPrincipal),
		nullableInt64(det.RemainingMonths), nullableInt64(det.SecuredAssetID),
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func scanAsset(row interface {
	Scan(dest ...any) error
}) (Asset, error) {
	var a Asset
	var venue, archivedAt sql.NullString
	err := row.Scan(&a.ID, &a.Side, &a.Type, &a.Name, &a.AssetGroup, &venue, &a.Currency, &a.Source, &a.CreatedAt, &archivedAt)
	if err != nil {
		return Asset{}, err
	}
	a.Venue = venue.String
	a.ArchivedAt = archivedAt.String
	return a, err
}

// GetAsset returns one asset by id, or nil, nil if it doesn't exist
// (archived assets are still returned — callers that must exclude them
// check ArchivedAt themselves, same as ListAssets' includeArchived flag).
func (d *DB) GetAsset(id int64) (*Asset, error) {
	a, err := scanAsset(d.conn.QueryRow(`
		SELECT id, side, type, name, asset_group, venue, currency, source, created_at, archived_at
		FROM assets WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAssets returns every asset (spine only, no value), newest first.
// includeArchived=false filters archived_at IS NULL, the default for every
// list view per §9.1's "列表預設過濾 archived_at IS NULL".
func (d *DB) ListAssets(includeArchived bool) ([]Asset, error) {
	query := `SELECT id, side, type, name, asset_group, venue, currency, source, created_at, archived_at FROM assets`
	if !includeArchived {
		query += ` WHERE archived_at IS NULL`
	}
	query += ` ORDER BY id DESC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAssetsWithValue is ListAssets joined to each asset's latest snapshot —
// the shape the net-worth aggregate and dashboard both want, and per §2's
// design goal, the whole join never touches more than these two tables no
// matter how many detail tables exist. An asset with no snapshot yet comes
// back with Value == nil (render "—", not 0 — see AssetWithValue's doc
// comment).
func (d *DB) ListAssetsWithValue(includeArchived bool) ([]AssetWithValue, error) {
	query := `
		SELECT a.id, a.side, a.type, a.name, a.asset_group, a.venue, a.currency, a.source, a.created_at, a.archived_at,
			s.value, s.cost, s.date
		FROM assets a
		LEFT JOIN asset_snapshots s ON s.asset_id = a.id
			AND s.date = (SELECT MAX(date) FROM asset_snapshots WHERE asset_id = a.id)`
	if !includeArchived {
		query += ` WHERE a.archived_at IS NULL`
	}
	query += ` ORDER BY a.id DESC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AssetWithValue
	for rows.Next() {
		var w AssetWithValue
		var venue, archivedAt, asOf sql.NullString
		var value, cost sql.NullFloat64
		if err := rows.Scan(&w.ID, &w.Side, &w.Type, &w.Name, &w.AssetGroup, &venue, &w.Currency, &w.Source, &w.CreatedAt, &archivedAt,
			&value, &cost, &asOf); err != nil {
			return nil, err
		}
		w.Venue = venue.String
		w.ArchivedAt = archivedAt.String
		w.AsOf = asOf.String
		if value.Valid {
			v := value.Float64
			w.Value = &v
		}
		if cost.Valid {
			c := cost.Float64
			w.Cost = &c
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ArchiveAsset soft-deletes an asset (sets archived_at) — §9.1 rules out
// hard deletion because it would orphan the asset's snapshot history.
// Archiving an already-archived or nonexistent asset is a no-op, not an
// error: the caller (a delete button) doesn't need to distinguish those
// cases.
func (d *DB) ArchiveAsset(id int64) error {
	_, err := d.conn.Exec(`UPDATE assets SET archived_at = CURRENT_TIMESTAMP WHERE id = ? AND archived_at IS NULL`, id)
	return err
}

// GetDepositDetails returns nil, nil if the asset has no deposit_details row.
func (d *DB) GetDepositDetails(assetID int64) (*DepositDetails, error) {
	var det DepositDetails
	var bank, note sql.NullString
	err := d.conn.QueryRow(`SELECT bank, account_note FROM deposit_details WHERE asset_id = ?`, assetID).Scan(&bank, &note)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	det.Bank = bank.String
	det.AccountNote = note.String
	return &det, nil
}

// UpdateDepositDetails overwrites an existing deposit_details row — the
// asset itself must already have one (created via CreateDepositAsset).
func (d *DB) UpdateDepositDetails(assetID int64, det DepositDetails) error {
	_, err := d.conn.Exec(`UPDATE deposit_details SET bank = ?, account_note = ? WHERE asset_id = ?`,
		nullableString(det.Bank), nullableString(det.AccountNote), assetID)
	return err
}

// GetLoanDetails returns nil, nil if the asset has no loan_details row.
func (d *DB) GetLoanDetails(assetID int64) (*LoanDetails, error) {
	var det LoanDetails
	var lender sql.NullString
	var rate, principal sql.NullFloat64
	var months, secured sql.NullInt64
	err := d.conn.QueryRow(`
		SELECT lender, rate_pct, original_principal, remaining_months, secured_asset_id
		FROM loan_details WHERE asset_id = ?`, assetID,
	).Scan(&lender, &rate, &principal, &months, &secured)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	det.Lender = lender.String
	if rate.Valid {
		det.RatePct = &rate.Float64
	}
	if principal.Valid {
		det.OriginalPrincipal = &principal.Float64
	}
	if months.Valid {
		det.RemainingMonths = &months.Int64
	}
	if secured.Valid {
		det.SecuredAssetID = &secured.Int64
	}
	return &det, nil
}

// UpdateLoanDetails overwrites an existing loan_details row.
func (d *DB) UpdateLoanDetails(assetID int64, det LoanDetails) error {
	_, err := d.conn.Exec(`
		UPDATE loan_details SET lender = ?, rate_pct = ?, original_principal = ?, remaining_months = ?, secured_asset_id = ?
		WHERE asset_id = ?`,
		nullableString(det.Lender), nullableFloat(det.RatePct), nullableFloat(det.OriginalPrincipal),
		nullableInt64(det.RemainingMonths), nullableInt64(det.SecuredAssetID), assetID)
	return err
}

// UpsertAssetSnapshot writes (or overwrites) one asset's value for one date
// — this is the *only* write path for an asset's current value, whether
// that's the quick-add drawer's initial figure or the balance-sheet page's
// in-place edit (§9.1 rule 2: never UPDATE assets, an edit is always
// "today's value is now X"). Overwriting today's row is deliberate — a
// same-day correction shouldn't produce two rows.
func (d *DB) UpsertAssetSnapshot(s AssetSnapshot) error {
	if s.Source == "" {
		s.Source = "manual"
	}
	_, err := d.conn.Exec(`
		INSERT INTO asset_snapshots (asset_id, date, value, cost, source)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(asset_id, date) DO UPDATE SET
			value = excluded.value,
			cost = excluded.cost,
			source = excluded.source`,
		s.AssetID, s.Date, s.Value, nullableFloat(s.Cost), s.Source,
	)
	return err
}

// GetLatestAssetSnapshot returns an asset's most recent snapshot, or nil,
// nil if it has none yet.
func (d *DB) GetLatestAssetSnapshot(assetID int64) (*AssetSnapshot, error) {
	var s AssetSnapshot
	s.AssetID = assetID
	var cost sql.NullFloat64
	err := d.conn.QueryRow(`
		SELECT date, value, cost, source FROM asset_snapshots
		WHERE asset_id = ? ORDER BY date DESC LIMIT 1`, assetID,
	).Scan(&s.Date, &s.Value, &cost, &s.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if cost.Valid {
		s.Cost = &cost.Float64
	}
	return &s, nil
}

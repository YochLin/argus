package db

import "database/sql"

// FundDetails is fund_details' one row per fund-type asset (migration 33,
// see docs/phase-9-asset-platform.md §8.7/§9.2). MonthlyAmount nil or 0 means
// the DCA schedule has stopped (a lump-sum-only holding) — see the
// migration's doc comment.
type FundDetails struct {
	Code                 string
	Platform             string
	MonthlyAmount        *float64
	NextContributionDate string
}

// CreateFundAsset creates a fund-type asset and its fund_details row
// atomically, same "one flow, one write" shape as CreateDepositAsset/
// CreateLoanAsset/CreateInsuranceAsset. Side is always "asset".
func (d *DB) CreateFundAsset(a NewAsset, det FundDetails) (int64, error) {
	a.Side = "asset"
	a.Type = "fund"
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
		INSERT INTO fund_details (asset_id, code, platform, monthly_amount, next_contribution_date)
		VALUES (?, ?, ?, ?, ?)`,
		id, nullableString(det.Code), nullableString(det.Platform),
		nullableFloat(det.MonthlyAmount), nullableString(det.NextContributionDate),
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// GetFundDetails returns nil, nil if the asset has no fund_details row.
func (d *DB) GetFundDetails(assetID int64) (*FundDetails, error) {
	var det FundDetails
	var code, platform, next sql.NullString
	var monthly sql.NullFloat64
	err := d.conn.QueryRow(`
		SELECT code, platform, monthly_amount, next_contribution_date
		FROM fund_details WHERE asset_id = ?`, assetID,
	).Scan(&code, &platform, &monthly, &next)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	det.Code, det.Platform, det.NextContributionDate = code.String, platform.String, next.String
	if monthly.Valid {
		det.MonthlyAmount = &monthly.Float64
	}
	return &det, nil
}

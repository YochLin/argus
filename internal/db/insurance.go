package db

import "database/sql"

// InsuranceDetails is insurance_details' one row per insurance-type asset
// (migration 32, see docs/phase-9-asset-platform.md §8.6/§9.2).
type InsuranceDetails struct {
	Insurer        string
	Insured        string // e.g. "本人"/"配偶" — who the policy covers
	PolicyNo       string
	AnnualPremium  *float64
	PremiumYears   *int64
	MaturityDate   string
	SurrenderValue *float64
}

// InsuranceCoverage is one insurance_coverages row — a single benefit
// amount on a policy. PerPeriod is "" for a lump-sum benefit, "month" or
// "day" otherwise; per §8.6 amounts with different PerPeriod must never be
// summed together.
type InsuranceCoverage struct {
	ID        int64
	AssetID   int64
	Kind      string // "life"|"accident"|"ci"|"cancer"|"disability"|"hospital"
	Amount    float64
	PerPeriod string
}

// CreateInsuranceAsset creates an insurance-type asset plus its
// insurance_details and (single) insurance_coverages row atomically — the
// quick-add form's "one flow, one write" shape (§8.14.3), matching
// CreateDepositAsset/CreateLoanAsset. Side is always "asset" (a policy is
// never a liability); callers don't pass it.
func (d *DB) CreateInsuranceAsset(a NewAsset, det InsuranceDetails, cov InsuranceCoverage) (int64, error) {
	a.Side = "asset"
	a.Type = "insurance"
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
		INSERT INTO insurance_details (asset_id, insurer, insured, policy_no, annual_premium, premium_years, maturity_date, surrender_value)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, nullableString(det.Insurer), nullableString(det.Insured), nullableString(det.PolicyNo),
		nullableFloat(det.AnnualPremium), nullableInt64(det.PremiumYears), nullableString(det.MaturityDate), nullableFloat(det.SurrenderValue),
	); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
		INSERT INTO insurance_coverages (asset_id, kind, amount, per_period)
		VALUES (?, ?, ?, ?)`,
		id, cov.Kind, cov.Amount, nullableString(cov.PerPeriod),
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// GetInsuranceDetails returns nil, nil if the asset has no insurance_details
// row.
func (d *DB) GetInsuranceDetails(assetID int64) (*InsuranceDetails, error) {
	var det InsuranceDetails
	var insurer, insured, policyNo, maturity sql.NullString
	var premium, surrender sql.NullFloat64
	var years sql.NullInt64
	err := d.conn.QueryRow(`
		SELECT insurer, insured, policy_no, annual_premium, premium_years, maturity_date, surrender_value
		FROM insurance_details WHERE asset_id = ?`, assetID,
	).Scan(&insurer, &insured, &policyNo, &premium, &years, &maturity, &surrender)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	det.Insurer, det.Insured, det.PolicyNo, det.MaturityDate = insurer.String, insured.String, policyNo.String, maturity.String
	if premium.Valid {
		det.AnnualPremium = &premium.Float64
	}
	if years.Valid {
		det.PremiumYears = &years.Int64
	}
	if surrender.Valid {
		det.SurrenderValue = &surrender.Float64
	}
	return &det, nil
}

// ListInsuranceCoverages returns every coverage row across every
// insurance-type asset (archived or not — the caller cross-references
// against ListAssets(false) to exclude archived policies, same
// join-in-Go pattern wealth_retire.go's earmark lookup uses rather than a
// SQL join, since the wealth package's read layer already has both lists
// in memory for other reasons).
func (d *DB) ListInsuranceCoverages() ([]InsuranceCoverage, error) {
	rows, err := d.conn.Query(`SELECT id, asset_id, kind, amount, per_period FROM insurance_coverages ORDER BY asset_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InsuranceCoverage
	for rows.Next() {
		var c InsuranceCoverage
		var perPeriod sql.NullString
		if err := rows.Scan(&c.ID, &c.AssetID, &c.Kind, &c.Amount, &perPeriod); err != nil {
			return nil, err
		}
		c.PerPeriod = perPeriod.String
		out = append(out, c)
	}
	return out, rows.Err()
}

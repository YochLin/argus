package db

// WealthResetAtKey is the settings key ResetWealthData stamps, for the
// settings page's "last reset" line.
const WealthResetAtKey = "wealth.reset_at"

// ResetWealthData wipes the wealth account (Phase 28 D) — every asset and
// liability with its value history and details, the cash flows, the goals
// and their earmarks, the profile.* settings, and the assistant's drafts
// and conversation — then stamps WealthResetAtKey with resetAt. Trading data
// (positions, transactions, snapshots, cash_balance*) is never touched: the
// trading account's net-worth row comes from there, not from assets. fx_rates
// stays too, it's a cache of public rates rather than anything the user
// entered. The caller backs the database up first.
func (d *DB) ResetWealthData(resetAt string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"DELETE FROM asset_snapshots",
		"DELETE FROM deposit_details",
		"DELETE FROM loan_details",
		"DELETE FROM fund_details",
		"DELETE FROM insurance_details",
		"DELETE FROM insurance_coverages",
		"DELETE FROM goal_assets",
		"DELETE FROM goals",
		"DELETE FROM recurring_cashflows",
		"DELETE FROM assets",
		"DELETE FROM wealth_drafts",
		"DELETE FROM wealth_assistant_messages",
		"DELETE FROM settings WHERE key LIKE 'profile.%'",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		WealthResetAtKey, resetAt); err != nil {
		return err
	}
	return tx.Commit()
}

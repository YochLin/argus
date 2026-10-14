package db

// ResetTradingData wipes every positions/transactions/daily_snapshots/
// net_worth_snapshots row plus the cash_balance* settings keys. Not
// market-scoped — daily_snapshots has no market column to filter by (see
// migration 12's doc comment for which four tables got one) — so this is a
// full wipe, not a per-market one. Meant only for a dedicated
// single-purpose database like Phase 11's paper.db (see
// internal/bot/paper.go's handlePaper "reset confirm" path): calling this
// against the bot's real db.DB would destroy the user's actual trade
// history, and there is deliberately no other caller anywhere in this
// codebase.
func (d *DB) ResetTradingData() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"DELETE FROM positions",
		"DELETE FROM transactions",
		"DELETE FROM daily_snapshots",
		"DELETE FROM net_worth_snapshots",
		"DELETE FROM settings WHERE key IN ('cash_balance', 'cash_balance_twd')",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

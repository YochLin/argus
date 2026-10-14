package db

import "database/sql"

// GetSignalState returns the last recorded state for (ticker, signal), or ""
// if none has been recorded yet.
func (d *DB) GetSignalState(ticker, signal string) (string, error) {
	var state string
	err := d.conn.QueryRow(
		`SELECT state FROM signal_states WHERE ticker = ? AND signal = ?`,
		ticker, signal,
	).Scan(&state)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return state, nil
}

// SetSignalState upserts the current state for (ticker, signal).
func (d *DB) SetSignalState(ticker, signal, state string) error {
	_, err := d.conn.Exec(`
		INSERT INTO signal_states (ticker, signal, state, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker, signal) DO UPDATE SET
			state = excluded.state,
			updated_at = excluded.updated_at`,
		ticker, signal, state,
	)
	return err
}

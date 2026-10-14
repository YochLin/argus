package db

import "database/sql"

// GetSetting returns the stored value for key, or ok=false if it's never
// been set.
func (d *DB) GetSetting(key string) (string, bool, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSetting upserts value for key.
func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		key, value,
	)
	return err
}

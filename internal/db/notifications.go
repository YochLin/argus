package db

// Notification is one row from Phase 24 Stage 2's in-app notification
// history (migration 24) — the same events a Telegram push shows, kept so a
// future Web/App surface can list past alerts instead of only ever seeing
// them fly by in a chat log. Read defaults false; MarkNotificationRead is
// the only writer of it.
type Notification struct {
	ID        int64
	Type      string
	Text      string
	Level     string
	Read      bool
	CreatedAt string
}

// SaveNotification inserts one notification row.
func (d *DB) SaveNotification(n Notification) error {
	_, err := d.conn.Exec(
		`INSERT INTO notifications (type, text, level) VALUES (?, ?, ?)`,
		n.Type, n.Text, n.Level,
	)
	return err
}

// GetRecentNotifications returns the most recent limit notifications,
// newest first — mirrors GetRecentPriceEvents' shape.
func (d *DB) GetRecentNotifications(limit int) ([]Notification, error) {
	rows, err := d.conn.Query(
		`SELECT id, type, text, level, read, created_at FROM notifications ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.Text, &n.Level, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkNotificationRead flips one notification row to read.
func (d *DB) MarkNotificationRead(id int64) error {
	_, err := d.conn.Exec(`UPDATE notifications SET read = 1 WHERE id = ?`, id)
	return err
}

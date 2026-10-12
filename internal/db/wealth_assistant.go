package db

import "time"

// AssistantMessage is one row of wealth_assistant_messages (migration 39).
// Meta is the raw JSON object stored alongside Text — this table doesn't
// interpret it, internal/web does.
type AssistantMessage struct {
	ID        int64
	Role      string // "user" | "ai" | "sys" | "div"
	Text      string
	Meta      string
	CreatedAt string
}

// ListAssistantMessages returns the whole conversation, oldest first.
func (d *DB) ListAssistantMessages() ([]AssistantMessage, error) {
	rows, err := d.conn.Query(`SELECT id, role, text, meta, created_at FROM wealth_assistant_messages ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssistantMessage
	for rows.Next() {
		var m AssistantMessage
		if err := rows.Scan(&m.ID, &m.Role, &m.Text, &m.Meta, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddAssistantMessages appends msgs in one transaction and returns them with
// ID/CreatedAt filled in — a turn (the user's message and the reply, plus a
// divider when the session started over) lands whole or not at all. An
// empty Meta is stored as "{}".
func (d *DB) AddAssistantMessages(msgs []AssistantMessage) ([]AssistantMessage, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now().In(cst).Format(time.RFC3339)
	out := make([]AssistantMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Meta == "" {
			m.Meta = "{}"
		}
		m.CreatedAt = now
		res, err := tx.Exec(`INSERT INTO wealth_assistant_messages (role, text, meta, created_at) VALUES (?, ?, ?, ?)`, m.Role, m.Text, m.Meta, m.CreatedAt)
		if err != nil {
			return nil, err
		}
		if m.ID, err = res.LastInsertId(); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, tx.Commit()
}

// ClearAssistantMessages deletes the whole conversation (開新對話).
func (d *DB) ClearAssistantMessages() error {
	_, err := d.conn.Exec(`DELETE FROM wealth_assistant_messages`)
	return err
}

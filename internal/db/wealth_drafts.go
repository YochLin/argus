package db

import (
	"database/sql"
	"errors"
	"time"
)

// WealthDraft is one row of wealth_drafts (migration 40). Payload is the
// kind's raw JSON — internal/assets defines and validates it.
type WealthDraft struct {
	ID         int64
	Kind       string
	TargetID   *int64
	Payload    string
	ApplyError string
	CreatedAt  string
}

// ErrDraftNotFound is returned when a draft id doesn't exist (any more).
var ErrDraftNotFound = errors.New("draft not found")

// ListWealthDrafts returns every draft, oldest first.
func (d *DB) ListWealthDrafts() ([]WealthDraft, error) {
	rows, err := d.conn.Query(`SELECT id, kind, target_id, payload, apply_error, created_at FROM wealth_drafts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WealthDraft
	for rows.Next() {
		var w WealthDraft
		var target sql.NullInt64
		if err := rows.Scan(&w.ID, &w.Kind, &target, &w.Payload, &w.ApplyError, &w.CreatedAt); err != nil {
			return nil, err
		}
		if target.Valid {
			w.TargetID = &target.Int64
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// AddWealthDraft stores a new draft and returns its id.
func (d *DB) AddWealthDraft(kind string, targetID *int64, payload string) (int64, error) {
	res, err := d.conn.Exec(`INSERT INTO wealth_drafts (kind, target_id, payload, created_at) VALUES (?, ?, ?, ?)`,
		kind, targetID, payload, time.Now().In(cst).Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateWealthDraft replaces a draft's payload and clears its apply_error —
// the edit is the user's (or the model's) fix for whatever failed.
func (d *DB) UpdateWealthDraft(id int64, payload string) error {
	return draftChanged(d.conn.Exec(`UPDATE wealth_drafts SET payload = ?, apply_error = '' WHERE id = ?`, payload, id))
}

// SetWealthDraftError records why writing a draft failed.
func (d *DB) SetWealthDraftError(id int64, msg string) error {
	return draftChanged(d.conn.Exec(`UPDATE wealth_drafts SET apply_error = ? WHERE id = ?`, msg, id))
}

// DeleteWealthDraft removes one draft.
func (d *DB) DeleteWealthDraft(id int64) error {
	return draftChanged(d.conn.Exec(`DELETE FROM wealth_drafts WHERE id = ?`, id))
}

// ClearWealthDrafts removes every draft (清空草稿).
func (d *DB) ClearWealthDrafts() error {
	_, err := d.conn.Exec(`DELETE FROM wealth_drafts`)
	return err
}

func draftChanged(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDraftNotFound
	}
	return nil
}

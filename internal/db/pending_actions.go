package db

import (
	"database/sql"
	"time"
)

// Pending action statuses drive PLAN.md's Phase 4 "寫入把關基建": a write
// tool running in the MCP subprocess (see internal/mcptools) has no
// Telegram bot of its own, so it can only leave a proposal here as
// "pending". The main bot process is the one Telegram-connected reader that
// turns a pending row into a confirm/reject message (marking it "sent") and,
// once the user taps a button, resolves it into a terminal
// "confirmed"/"rejected" state — executing the underlying change only in
// the confirmed case.
const (
	PendingActionStatusPending   = "pending"
	PendingActionStatusSent      = "sent"
	PendingActionStatusConfirmed = "confirmed"
	PendingActionStatusRejected  = "rejected"
)

// Pending action types. Defined here — rather than in internal/mcptools
// (which creates them) or internal/bot (which interprets them) — because
// both of those packages already import internal/db; a shared constant
// means the writer and reader can never drift out of sync on the literal
// string, unlike this codebase's other necessary cross-package duplication
// (e.g. internal/mcptools's own copy of formatFundamentals) where no common
// import exists to hang a shared constant on.
const (
	PendingActionRecordBuy  = "record_buy"
	PendingActionRecordSell = "record_sell"
)

// PendingAction is one row of the write-gating queue. Payload is a
// JSON-encoded, action-type-specific blob (e.g. ticker/shares/price/fee/date
// for a trade proposal) — this table doesn't need to know its shape, only
// the bot-side executor for a given ActionType does.
type PendingAction struct {
	ID         int64
	ActionType string
	Payload    string
	Status     string
	CreatedAt  time.Time
}

// CreatePendingAction inserts a new proposal in "pending" status and returns
// its id.
func (d *DB) CreatePendingAction(actionType, payload string) (int64, error) {
	res, err := d.conn.Exec(
		`INSERT INTO pending_actions (action_type, payload, status) VALUES (?, ?, ?)`,
		actionType, payload, PendingActionStatusPending,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetPendingActionsByStatus returns every pending_actions row with the given
// status, oldest first.
func (d *DB) GetPendingActionsByStatus(status string) ([]PendingAction, error) {
	rows, err := d.conn.Query(
		`SELECT id, action_type, payload, status, created_at FROM pending_actions WHERE status = ? ORDER BY id`,
		status,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []PendingAction
	for rows.Next() {
		var a PendingAction
		if err := rows.Scan(&a.ID, &a.ActionType, &a.Payload, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

// GetPendingAction returns a single pending_actions row by id, or ok=false if
// it doesn't exist.
func (d *DB) GetPendingAction(id int64) (PendingAction, bool, error) {
	a := PendingAction{ID: id}
	err := d.conn.QueryRow(
		`SELECT action_type, payload, status, created_at FROM pending_actions WHERE id = ?`, id,
	).Scan(&a.ActionType, &a.Payload, &a.Status, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return PendingAction{}, false, nil
	}
	if err != nil {
		return PendingAction{}, false, err
	}
	return a, true, nil
}

// MarkPendingActionSent atomically transitions a row from "pending" to
// "sent" — guarded by the WHERE clause so this only ever succeeds once per
// row. ok=false means the row wasn't in "pending" state (already sent, or
// doesn't exist), which callers should treat as "nothing to do", not an
// error.
func (d *DB) MarkPendingActionSent(id int64) (bool, error) {
	return d.transitionPendingAction(id, PendingActionStatusPending, PendingActionStatusSent)
}

// ResolvePendingAction atomically transitions a row from "sent" to a
// terminal status ("confirmed"/"rejected"). This is what stops a double-tap
// on the same Telegram inline button (or a tap arriving after the row was
// already resolved) from executing the underlying action twice — ok=false
// means the row wasn't in "sent" state, so the caller lost the race (or the
// button is stale) and must not execute anything.
func (d *DB) ResolvePendingAction(id int64, status string) (bool, error) {
	return d.transitionPendingAction(id, PendingActionStatusSent, status)
}

func (d *DB) transitionPendingAction(id int64, fromStatus, toStatus string) (bool, error) {
	res, err := d.conn.Exec(
		`UPDATE pending_actions SET status = ? WHERE id = ? AND status = ?`,
		toStatus, id, fromStatus,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

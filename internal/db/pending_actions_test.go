package db

import "testing"

func TestPendingActionLifecycle(t *testing.T) {
	d := newTestDB(t)

	id, err := d.CreatePendingAction(PendingActionRecordBuy, `{"ticker":"AAPL"}`)
	if err != nil {
		t.Fatalf("CreatePendingAction() error = %v", err)
	}

	a, ok, err := d.GetPendingAction(id)
	if err != nil || !ok {
		t.Fatalf("GetPendingAction() = %v, %v, %v", a, ok, err)
	}
	if a.ActionType != PendingActionRecordBuy || a.Payload != `{"ticker":"AAPL"}` || a.Status != PendingActionStatusPending {
		t.Errorf("GetPendingAction() = %+v, want action_type=%s payload set status=%s", a, PendingActionRecordBuy, PendingActionStatusPending)
	}

	pending, err := d.GetPendingActionsByStatus(PendingActionStatusPending)
	if err != nil || len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("GetPendingActionsByStatus(pending) = %v, %v", pending, err)
	}

	sent, err := d.MarkPendingActionSent(id)
	if err != nil || !sent {
		t.Fatalf("MarkPendingActionSent() = %v, %v; want true, nil", sent, err)
	}

	stillPending, err := d.GetPendingActionsByStatus(PendingActionStatusPending)
	if err != nil || len(stillPending) != 0 {
		t.Fatalf("GetPendingActionsByStatus(pending) after send = %v, %v; want empty", stillPending, err)
	}

	resolved, err := d.ResolvePendingAction(id, PendingActionStatusConfirmed)
	if err != nil || !resolved {
		t.Fatalf("ResolvePendingAction() = %v, %v; want true, nil", resolved, err)
	}

	a, ok, err = d.GetPendingAction(id)
	if err != nil || !ok || a.Status != PendingActionStatusConfirmed {
		t.Fatalf("GetPendingAction() after resolve = %+v, %v, %v; want status=%s", a, ok, err, PendingActionStatusConfirmed)
	}
}

// TestMarkPendingActionSentGuardsAgainstNonPending exercises the atomic
// WHERE-clause guard: calling MarkPendingActionSent twice must only succeed
// the first time, since a row already "sent" isn't "pending" anymore.
func TestMarkPendingActionSentGuardsAgainstNonPending(t *testing.T) {
	d := newTestDB(t)
	id, err := d.CreatePendingAction(PendingActionRecordSell, `{}`)
	if err != nil {
		t.Fatal(err)
	}

	first, err := d.MarkPendingActionSent(id)
	if err != nil || !first {
		t.Fatalf("first MarkPendingActionSent() = %v, %v; want true, nil", first, err)
	}
	second, err := d.MarkPendingActionSent(id)
	if err != nil || second {
		t.Fatalf("second MarkPendingActionSent() = %v, %v; want false, nil", second, err)
	}
}

// TestResolvePendingActionGuardsAgainstDoubleTap is the race guard that
// matters most in production: two near-simultaneous taps on the same
// Telegram inline button (or a tap on an already-resolved message) must not
// both succeed, or a confirmed trade could be recorded twice.
func TestResolvePendingActionGuardsAgainstDoubleTap(t *testing.T) {
	d := newTestDB(t)
	id, err := d.CreatePendingAction(PendingActionRecordBuy, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.MarkPendingActionSent(id); err != nil {
		t.Fatal(err)
	}

	first, err := d.ResolvePendingAction(id, PendingActionStatusConfirmed)
	if err != nil || !first {
		t.Fatalf("first ResolvePendingAction() = %v, %v; want true, nil", first, err)
	}
	second, err := d.ResolvePendingAction(id, PendingActionStatusRejected)
	if err != nil || second {
		t.Fatalf("second ResolvePendingAction() = %v, %v; want false, nil", second, err)
	}

	a, ok, err := d.GetPendingAction(id)
	if err != nil || !ok || a.Status != PendingActionStatusConfirmed {
		t.Fatalf("GetPendingAction() = %+v, %v, %v; status should remain %s after the second call lost the race", a, ok, err, PendingActionStatusConfirmed)
	}
}

func TestResolvePendingActionBeforeSentFails(t *testing.T) {
	d := newTestDB(t)
	id, err := d.CreatePendingAction(PendingActionRecordBuy, `{}`)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := d.ResolvePendingAction(id, PendingActionStatusConfirmed)
	if err != nil || resolved {
		t.Fatalf("ResolvePendingAction() on a still-pending (not yet sent) row = %v, %v; want false, nil", resolved, err)
	}
}

func TestGetPendingActionNotFound(t *testing.T) {
	d := newTestDB(t)
	_, ok, err := d.GetPendingAction(999)
	if err != nil || ok {
		t.Fatalf("GetPendingAction(999) = _, %v, %v; want ok=false", ok, err)
	}
}

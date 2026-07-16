package bot

import (
	"path/filepath"
	"strings"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
)

func newPendingActionsTestBot(t *testing.T) (*Bot, *db.DB) {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return &Bot{db: d, lang: i18n.EN}, d
}

func TestParseCallbackData(t *testing.T) {
	cases := []struct {
		data        string
		wantID      int64
		wantConfirm bool
		wantOK      bool
	}{
		{"pa_confirm:42", 42, true, true},
		{"pa_reject:42", 42, false, true},
		{"pa_confirm:0", 0, true, true},
		{"unknown:42", 0, false, false},
		{"pa_confirm:abc", 0, false, false},
		{"", 0, false, false},
	}
	for _, c := range cases {
		id, confirm, ok := parseCallbackData(c.data)
		if id != c.wantID || confirm != c.wantConfirm || ok != c.wantOK {
			t.Errorf("parseCallbackData(%q) = (%d, %v, %v), want (%d, %v, %v)", c.data, id, confirm, ok, c.wantID, c.wantConfirm, c.wantOK)
		}
	}
}

func TestDescribePendingAction(t *testing.T) {
	b := &Bot{lang: i18n.EN}

	buy := db.PendingAction{ActionType: db.PendingActionRecordBuy, Payload: `{"ticker":"AAPL","shares":10,"price":200,"fee":1.5,"date":"2026-01-15"}`}
	text, ok := b.describePendingAction(buy)
	if !ok || !strings.Contains(text, "AAPL") || !strings.Contains(strings.ToUpper(text), "BUY") {
		t.Errorf("describePendingAction(buy) = %q, %v; want text mentioning AAPL/BUY", text, ok)
	}

	sell := db.PendingAction{ActionType: db.PendingActionRecordSell, Payload: `{"ticker":"MSFT","shares":5,"price":300,"fee":0,"date":"2026-01-15"}`}
	text, ok = b.describePendingAction(sell)
	if !ok || !strings.Contains(text, "MSFT") || !strings.Contains(strings.ToUpper(text), "SELL") {
		t.Errorf("describePendingAction(sell) = %q, %v; want text mentioning MSFT/SELL", text, ok)
	}

	if _, ok := b.describePendingAction(db.PendingAction{ActionType: "unknown_type", Payload: "{}"}); ok {
		t.Error("describePendingAction with unknown action_type should return ok=false")
	}
	if _, ok := b.describePendingAction(db.PendingAction{ActionType: db.PendingActionRecordBuy, Payload: "not json"}); ok {
		t.Error("describePendingAction with malformed payload should return ok=false")
	}
}

func TestResolvePendingActionConfirmExecutesBuy(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	id, err := d.CreatePendingAction(db.PendingActionRecordBuy, `{"ticker":"AAPL","shares":10,"price":200,"fee":1.5,"date":"2026-01-15"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.MarkPendingActionSent(id); err != nil {
		t.Fatal(err)
	}

	result := b.resolvePendingAction(id, true)
	if !strings.Contains(result, "AAPL") {
		t.Errorf("resolvePendingAction(confirm) = %q, want text mentioning AAPL", result)
	}

	pos, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok || pos.Shares != 10 {
		t.Fatalf("GetPosition(AAPL) = %+v, %v, %v; want a 10-share position after confirming", pos, ok, err)
	}

	a, _, err := d.GetPendingAction(id)
	if err != nil || a.Status != db.PendingActionStatusConfirmed {
		t.Errorf("pending action status = %q, %v; want %q", a.Status, err, db.PendingActionStatusConfirmed)
	}
}

func TestResolvePendingActionRejectDoesNotExecute(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	id, err := d.CreatePendingAction(db.PendingActionRecordBuy, `{"ticker":"AAPL","shares":10,"price":200,"fee":0,"date":"2026-01-15"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.MarkPendingActionSent(id); err != nil {
		t.Fatal(err)
	}

	result := b.resolvePendingAction(id, false)
	if result != i18n.T(i18n.EN, i18n.KeyPendingActionRejected) {
		t.Errorf("resolvePendingAction(reject) = %q, want the rejected message", result)
	}

	if _, ok, err := d.GetPosition("AAPL"); err != nil || ok {
		t.Errorf("GetPosition(AAPL) after reject = _, %v, %v; want ok=false (nothing executed)", ok, err)
	}
}

func TestResolvePendingActionDoubleTapGuard(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	id, err := d.CreatePendingAction(db.PendingActionRecordBuy, `{"ticker":"AAPL","shares":10,"price":200,"fee":0,"date":"2026-01-15"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.MarkPendingActionSent(id); err != nil {
		t.Fatal(err)
	}

	first := b.resolvePendingAction(id, true)
	if !strings.Contains(first, "AAPL") {
		t.Fatalf("first resolvePendingAction() = %q, want success text", first)
	}
	second := b.resolvePendingAction(id, true)
	if second != i18n.T(i18n.EN, i18n.KeyPendingActionAlreadyResolved) {
		t.Errorf("second resolvePendingAction() = %q, want the already-resolved message", second)
	}

	pos, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok || pos.Shares != 10 {
		t.Errorf("GetPosition(AAPL) = %+v, %v, %v; want exactly 10 shares (buy must not execute twice)", pos, ok, err)
	}
}

func TestResolvePendingActionNotYetSent(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	id, err := d.CreatePendingAction(db.PendingActionRecordBuy, `{"ticker":"AAPL","shares":10,"price":200,"fee":0,"date":"2026-01-15"}`)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately not calling MarkPendingActionSent — simulates a stale
	// callback arriving before/without the confirmation ever being sent.

	result := b.resolvePendingAction(id, true)
	if result != i18n.T(i18n.EN, i18n.KeyPendingActionAlreadyResolved) {
		t.Errorf("resolvePendingAction() on a still-pending row = %q, want the already-resolved message", result)
	}
	if _, ok, err := d.GetPosition("AAPL"); err != nil || ok {
		t.Error("a not-yet-sent action must never be executable")
	}
}

func TestExecutePendingActionUnknownType(t *testing.T) {
	b := &Bot{lang: i18n.EN}
	result := b.executePendingAction(db.PendingAction{ActionType: "unknown_type", Payload: "{}"})
	if result != i18n.T(i18n.EN, i18n.KeyPendingActionExecFailed) {
		t.Errorf("executePendingAction(unknown type) = %q, want the exec-failed message", result)
	}
}

func TestExecutePendingActionMalformedPayload(t *testing.T) {
	b := &Bot{lang: i18n.EN}
	result := b.executePendingAction(db.PendingAction{ActionType: db.PendingActionRecordSell, Payload: "not json"})
	if result != i18n.T(i18n.EN, i18n.KeyPendingActionExecFailed) {
		t.Errorf("executePendingAction(malformed payload) = %q, want the exec-failed message", result)
	}
}

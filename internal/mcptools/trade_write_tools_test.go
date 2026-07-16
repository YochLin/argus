package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
)

func TestRecordBuyCreatesPendingActionNotAPosition(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, writeDB: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "record_buy", map[string]any{
		"ticker": "aapl", "shares": 10.0, "price": 200.0, "fee": 1.5, "date": "2026-01-15",
	})
	if isError {
		t.Fatalf("record_buy returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") {
		t.Errorf("record_buy result missing ticker, got: %s", text)
	}

	// The whole point of the proposal gate: no position/transaction should
	// exist yet, only a pending_actions row.
	if _, ok, err := d.GetPosition("AAPL"); err != nil || ok {
		t.Errorf("GetPosition(AAPL) = _, %v, %v; want ok=false (no position until confirmed)", ok, err)
	}

	pending, err := d.GetPendingActionsByStatus(db.PendingActionStatusPending)
	if err != nil || len(pending) != 1 {
		t.Fatalf("GetPendingActionsByStatus(pending) = %v, %v; want exactly one row", pending, err)
	}
	a := pending[0]
	if a.ActionType != db.PendingActionRecordBuy {
		t.Errorf("ActionType = %q, want %q", a.ActionType, db.PendingActionRecordBuy)
	}
	var payload tradePayload
	if err := json.Unmarshal([]byte(a.Payload), &payload); err != nil {
		t.Fatalf("Payload didn't decode as tradePayload: %v", err)
	}
	if payload.Ticker != "AAPL" || payload.Shares != 10.0 || payload.Price != 200.0 || payload.Fee != 1.5 || payload.Date != "2026-01-15" {
		t.Errorf("Payload = %+v, want ticker=AAPL shares=10 price=200 fee=1.5 date=2026-01-15", payload)
	}
}

func TestRecordSellCreatesPendingAction(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, writeDB: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "record_sell", map[string]any{
		"ticker": "MSFT", "shares": 5.0, "price": 300.0,
	})
	if isError {
		t.Fatalf("record_sell returned an error result: %s", text)
	}
	if !strings.Contains(text, "MSFT") {
		t.Errorf("record_sell result missing ticker, got: %s", text)
	}

	pending, err := d.GetPendingActionsByStatus(db.PendingActionStatusPending)
	if err != nil || len(pending) != 1 || pending[0].ActionType != db.PendingActionRecordSell {
		t.Fatalf("GetPendingActionsByStatus(pending) = %v, %v; want one record_sell row", pending, err)
	}

	// date omitted should default to today, not be left blank.
	var payload tradePayload
	if err := json.Unmarshal([]byte(pending[0].Payload), &payload); err != nil {
		t.Fatalf("Payload didn't decode: %v", err)
	}
	if payload.Date == "" {
		t.Error("Payload.Date should default to today when omitted, got empty string")
	}
}

func TestRecordBuyInvalidInput(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, writeDB: d}
	session := connectTool(t, ts)

	cases := []map[string]any{
		{"ticker": "", "shares": 10.0, "price": 200.0},
		{"ticker": "AAPL", "shares": 0.0, "price": 200.0},
		{"ticker": "AAPL", "shares": -5.0, "price": 200.0},
		{"ticker": "AAPL", "shares": 10.0, "price": 0.0},
		{"ticker": "AAPL", "shares": 10.0, "price": 200.0, "fee": -1.0},
		{"ticker": "AAPL", "shares": 10.0, "price": 200.0, "date": "not-a-date"},
	}
	for _, args := range cases {
		if _, isError := callText(t, session, "record_buy", args); !isError {
			t.Errorf("record_buy(%v) should return IsError", args)
		}
	}

	if pending, err := d.GetPendingActionsByStatus(db.PendingActionStatusPending); err != nil || len(pending) != 0 {
		t.Errorf("invalid record_buy calls should not create any pending_actions rows, got %v, %v", pending, err)
	}
}

func TestTradeWriteToolsNotRegisteredWithoutWriteDB(t *testing.T) {
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}}
	session := connectTool(t, ts)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	names := make(map[string]bool)
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, notWant := range []string{"record_buy", "record_sell"} {
		if names[notWant] {
			t.Errorf("tools/list should not advertise %q when writeDB is nil", notWant)
		}
	}
}

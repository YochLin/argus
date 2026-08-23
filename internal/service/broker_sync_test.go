package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"argus/internal/db"
	"argus/internal/sinopac"
)

type fakeSinopacClient struct {
	positions  []sinopac.Position
	details    map[int][]sinopac.PositionDetail
	pnl        []sinopac.ProfitLoss
	balance    sinopac.AccountBalance
	balanceErr error
}

func (f *fakeSinopacClient) PositionUnit(ctx context.Context) ([]sinopac.Position, error) {
	return f.positions, nil
}

func (f *fakeSinopacClient) PositionDetail(ctx context.Context, detailID int) ([]sinopac.PositionDetail, error) {
	return f.details[detailID], nil
}

func (f *fakeSinopacClient) ProfitLoss(ctx context.Context, begin, end string) ([]sinopac.ProfitLoss, error) {
	return f.pnl, nil
}

func (f *fakeSinopacClient) AccountBalance(ctx context.Context) (sinopac.AccountBalance, error) {
	return f.balance, f.balanceErr
}

type mockBrokerSyncStore struct {
	actions     map[string][]db.PendingAction
	existingExt map[string]bool
	existsErr   error
	settings    map[string]string
}

func newMockBrokerSyncStore() *mockBrokerSyncStore {
	return &mockBrokerSyncStore{
		actions:     make(map[string][]db.PendingAction),
		existingExt: make(map[string]bool),
		settings:    make(map[string]string),
	}
}

func (m *mockBrokerSyncStore) GetPendingActionsByStatus(status string) ([]db.PendingAction, error) {
	return m.actions[status], nil
}

func (m *mockBrokerSyncStore) TransactionExtIDExists(extID string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return m.existingExt[extID], nil
}

func (m *mockBrokerSyncStore) SetSetting(key, value string) error {
	m.settings[key] = value
	return nil
}

func TestBrokerSyncServiceFetchTrades(t *testing.T) {
	client := &fakeSinopacClient{
		positions: []sinopac.Position{{ID: 1, Code: "2330"}},
		details: map[int][]sinopac.PositionDetail{
			1: {{Date: "2026-08-20", Code: "2330", Quantity: 1000, Price: 900, Dseq: "d1"}},
		},
		pnl: []sinopac.ProfitLoss{
			{Date: "2026-08-21", Code: "2330", Quantity: 1000, Price: 950, Dseq: "d2"},
		},
	}
	s := NewBrokerSyncService(client, newMockBrokerSyncStore())

	trades, err := s.FetchTrades(context.Background(), nil, time.Now())
	if err != nil {
		t.Fatalf("FetchTrades() error = %v", err)
	}
	if len(trades) != 2 {
		t.Fatalf("len(trades) = %d, want 2", len(trades))
	}
}

func TestBrokerSyncServiceFetchTradesSkipsTicker(t *testing.T) {
	client := &fakeSinopacClient{
		positions: []sinopac.Position{{ID: 1, Code: "2330"}},
		details: map[int][]sinopac.PositionDetail{
			1: {{Date: "2026-08-20", Code: "2330", Quantity: 1000, Price: 900, Dseq: "d1"}},
		},
	}
	s := NewBrokerSyncService(client, newMockBrokerSyncStore())

	trades, err := s.FetchTrades(context.Background(), map[string]bool{"2330": true}, time.Now())
	if err != nil {
		t.Fatalf("FetchTrades() error = %v", err)
	}
	if len(trades) != 0 {
		t.Fatalf("expected skipped ticker to be excluded, got %+v", trades)
	}
}

func TestBrokerSyncServiceQueuedExtIDs(t *testing.T) {
	store := newMockBrokerSyncStore()
	store.actions[db.PendingActionStatusPending] = []db.PendingAction{
		{ActionType: db.PendingActionRecordBuy, Payload: `{"ext_id":"d1"}`},
		{ActionType: db.PendingActionRecordSell, Payload: `{"ext_id":"d2"}`},
	}
	s := NewBrokerSyncService(&fakeSinopacClient{}, store)

	extract := func(payload string) (string, bool) {
		if payload == `{"ext_id":"d1"}` {
			return "d1", true
		}
		if payload == `{"ext_id":"d2"}` {
			return "d2", true
		}
		return "", false
	}
	queued, err := s.QueuedExtIDs(extract)
	if err != nil {
		t.Fatalf("QueuedExtIDs() error = %v", err)
	}
	if !queued["d1"] || !queued["d2"] {
		t.Fatalf("QueuedExtIDs() = %v, want d1 and d2 both true", queued)
	}
}

func TestBrokerSyncServiceNewTradesFiltersQueuedAndRecorded(t *testing.T) {
	store := newMockBrokerSyncStore()
	store.existingExt["already-recorded"] = true
	s := NewBrokerSyncService(&fakeSinopacClient{}, store)

	trades := []sinopac.Trade{
		{ExtID: "already-queued", Ticker: "AAPL"},
		{ExtID: "already-recorded", Ticker: "MSFT"},
		{ExtID: "brand-new", Ticker: "GOOG"},
	}
	out := s.NewTrades(trades, map[string]bool{"already-queued": true})
	if len(out) != 1 || out[0].ExtID != "brand-new" {
		t.Fatalf("NewTrades() = %+v, want only brand-new", out)
	}
}

func TestBrokerSyncServiceNewTradesSkipsOnStoreError(t *testing.T) {
	store := newMockBrokerSyncStore()
	store.existsErr = errors.New("db down")
	s := NewBrokerSyncService(&fakeSinopacClient{}, store)

	out := s.NewTrades([]sinopac.Trade{{ExtID: "x"}}, nil)
	if len(out) != 0 {
		t.Fatalf("NewTrades() = %+v, want empty on store error", out)
	}
}

func TestBrokerSyncServiceSyncCashBalance(t *testing.T) {
	client := &fakeSinopacClient{balance: sinopac.AccountBalance{AccBalance: 123456.78}}
	store := newMockBrokerSyncStore()
	s := NewBrokerSyncService(client, store)

	s.SyncCashBalance(context.Background())

	if got := store.settings[CashSettingKeyTWD]; got != "123456.78" {
		t.Errorf("settings[%s] = %q, want 123456.78", CashSettingKeyTWD, got)
	}
}

func TestBrokerSyncServiceSyncCashBalanceNoopOnError(t *testing.T) {
	client := &fakeSinopacClient{balanceErr: errors.New("network down")}
	store := newMockBrokerSyncStore()
	s := NewBrokerSyncService(client, store)

	s.SyncCashBalance(context.Background())

	if _, ok := store.settings[CashSettingKeyTWD]; ok {
		t.Errorf("settings should be untouched on AccountBalance error, got %v", store.settings)
	}
}

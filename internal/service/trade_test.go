package service

import (
	"errors"
	"testing"

	"argus/internal/db"
)

type fakeTradeStore struct {
	position    db.Position
	positionOK  bool
	buys        int
	sells       int
	watchlist   []string
	realizedPnL float64
}

func (f *fakeTradeStore) GetPosition(string) (db.Position, bool, error) {
	return f.position, f.positionOK, nil
}

func (f *fakeTradeStore) RecordBuyExt(ticker string, shares, price, fee float64, date, extID string) (db.Position, error) {
	f.buys++
	f.position = db.Position{Ticker: ticker, Shares: shares, AvgCost: price + fee/shares}
	f.positionOK = true
	return f.position, nil
}

func (f *fakeTradeStore) RecordSellExt(ticker string, shares, price, fee float64, date, extID string) (db.Position, float64, error) {
	f.sells++
	f.position.Ticker = ticker
	f.position.Shares -= shares
	f.realizedPnL = shares*price - fee
	return f.position, f.realizedPnL, nil
}

func (f *fakeTradeStore) AddTicker(ticker string) error {
	f.watchlist = append(f.watchlist, ticker)
	return nil
}

func (f *fakeTradeStore) DeleteTransaction(id int64) (db.Transaction, error) {
	return db.Transaction{ID: id}, nil
}

func TestTradeServiceBuyAutoFeeAndWatchlist(t *testing.T) {
	store := &fakeTradeStore{}
	svc := NewTradeService(store, 1)

	result, err := svc.Buy(BuyInput{Ticker: " aapl ", Shares: 10, Price: 200, Date: "2026-08-14"})
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if result.Ticker != "AAPL" || !result.FeeAuto || result.Fee < 0 {
		t.Errorf("Buy() result = %+v, want normalized ticker and auto fee", result)
	}
	if store.buys != 1 || len(store.watchlist) != 1 || store.watchlist[0] != "AAPL" {
		t.Errorf("store writes = buys:%d watchlist:%v, want one buy and AAPL", store.buys, store.watchlist)
	}
}

func TestTradeServiceSellReturnsCloseStateAndStopPrice(t *testing.T) {
	store := &fakeTradeStore{position: db.Position{Ticker: "AAPL", Shares: 10, StopPrice: 180}, positionOK: true}
	svc := NewTradeService(store, 1)

	result, err := svc.Sell(SellInput{Ticker: "AAPL", Shares: 10, Price: 220, Fee: float64Ptr(1), Date: "2026-08-14"})
	if err != nil {
		t.Fatalf("Sell() error = %v", err)
	}
	if !result.Closed || result.StopPrice != 180 || result.RealizedPnL != 2199 {
		t.Errorf("Sell() result = %+v, want closed result with stop price and P&L", result)
	}
}

func TestTradeServiceRejectsInvalidInputBeforeStore(t *testing.T) {
	store := &fakeTradeStore{}
	svc := NewTradeService(store, 1)

	_, err := svc.Buy(BuyInput{Ticker: "AAPL", Shares: 0, Price: 200})
	if !errors.Is(err, ErrInvalidTrade) {
		t.Errorf("Buy() error = %v, want ErrInvalidTrade", err)
	}
	if store.buys != 0 {
		t.Errorf("store.buys = %d, want 0", store.buys)
	}
}

func float64Ptr(v float64) *float64 { return &v }

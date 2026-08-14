package service

import (
	"testing"

	"argus/internal/market"
)

type fakeWatchlistStore struct {
	added   []string
	removed []string
	all     []string
}

func (f *fakeWatchlistStore) AddTicker(ticker string) error {
	f.added = append(f.added, ticker)
	return nil
}

func (f *fakeWatchlistStore) RemoveTicker(ticker string) error {
	f.removed = append(f.removed, ticker)
	return nil
}

func (f *fakeWatchlistStore) GetWatchlist() ([]string, error) {
	return f.all, nil
}

func (f *fakeWatchlistStore) GetWatchlistByMarket(market.MarketID) ([]string, error) {
	return f.all, nil
}

func TestWatchlistServiceNormalizesWrites(t *testing.T) {
	store := &fakeWatchlistStore{}
	svc := NewWatchlistService(store)

	added, err := svc.Add("  aapl ")
	if err != nil || added != "AAPL" {
		t.Fatalf("AddTicker() = %q, %v, want AAPL", added, err)
	}
	removed, err := svc.Remove(" 2330 ")
	if err != nil || removed != "2330" {
		t.Fatalf("RemoveTicker() = %q, %v, want 2330", removed, err)
	}
	if len(store.added) != 1 || store.added[0] != "AAPL" || len(store.removed) != 1 || store.removed[0] != "2330" {
		t.Errorf("store writes = added:%v removed:%v", store.added, store.removed)
	}
}

func TestWatchlistServiceRejectsEmptyTicker(t *testing.T) {
	store := &fakeWatchlistStore{}
	svc := NewWatchlistService(store)

	if _, err := svc.Add("  "); err != ErrInvalidTicker {
		t.Errorf("AddTicker() error = %v, want ErrInvalidTicker", err)
	}
	if len(store.added) != 0 {
		t.Errorf("store.added = %v, want no writes", store.added)
	}
}

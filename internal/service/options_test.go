package service

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/option"
)

// fakeOptionChain implements data.OptionChainProvider with a fixed chain —
// local to this file rather than shared across service test files, same
// reasoning as technicals_test.go's helpers (independently-mergeable PR
// branches shouldn't depend on each other's test-only symbols).
type fakeOptionChain struct {
	expirations []time.Time
	quotes      map[time.Time][]data.OptionQuote
}

func (f fakeOptionChain) GetOptionExpirations(string) ([]time.Time, error) {
	return f.expirations, nil
}
func (f fakeOptionChain) GetOptionChain(_ string, expiry time.Time) ([]data.OptionQuote, error) {
	return f.quotes[expiry], nil
}

var testExpiry = time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)

func TestOptionMarkFindsContract(t *testing.T) {
	chain := fakeOptionChain{
		expirations: []time.Time{testExpiry},
		quotes: map[time.Time][]data.OptionQuote{
			testExpiry: {{ContractSymbol: "AAPL260918C00320000", Bid: 4.9, Ask: 5.1}},
		},
	}
	pos := db.OptionPosition{ContractSymbol: "AAPL260918C00320000", Underlying: "AAPL", Expiry: "2026-09-18"}

	mark, _, err := OptionMark(chain, pos)
	if err != nil {
		t.Fatalf("OptionMark() error = %v", err)
	}
	if mark != 5.0 {
		t.Errorf("mark = %v, want 5.0 (bid/ask mid)", mark)
	}
}

func TestOptionMarkContractNotFound(t *testing.T) {
	chain := fakeOptionChain{expirations: []time.Time{testExpiry}, quotes: map[time.Time][]data.OptionQuote{testExpiry: {}}}
	pos := db.OptionPosition{ContractSymbol: "AAPL260918C00320000", Underlying: "AAPL", Expiry: "2026-09-18"}

	if _, _, err := OptionMark(chain, pos); err == nil {
		t.Error("OptionMark() error = nil, want an error for a missing contract")
	}
}

func TestOptionMarkNilProvider(t *testing.T) {
	if _, _, err := OptionMark(nil, db.OptionPosition{}); err == nil {
		t.Error("OptionMark() error = nil, want an error for a nil chain provider")
	}
}

func TestGatherOptionCandidatesFiltersByDTEBand(t *testing.T) {
	now := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	nearExpiry := now.AddDate(0, 0, 45) // inside LongCall's 30-60 DTE band
	farExpiry := now.AddDate(0, 0, 200) // outside it
	chain := fakeOptionChain{
		expirations: []time.Time{nearExpiry, farExpiry},
		quotes: map[time.Time][]data.OptionQuote{
			nearExpiry: {{ContractSymbol: "NEAR", Right: "C", Strike: 100, Bid: 4.9, Ask: 5.1, OpenInterest: 500, Volume: 100, ImpliedVolatility: 0.3, Expiration: nearExpiry}},
			farExpiry:  {{ContractSymbol: "FAR", Right: "C", Strike: 100, Bid: 4.9, Ask: 5.1, OpenInterest: 500, Volume: 100, ImpliedVolatility: 0.3, Expiration: farExpiry}},
		},
	}
	profile := option.LongCall

	got, err := GatherOptionCandidates(chain, "AAPL", 100, profile, now)
	if err != nil {
		t.Fatalf("GatherOptionCandidates() error = %v", err)
	}
	if len(got) != 1 || got[0].Quote.ContractSymbol != "NEAR" {
		t.Errorf("GatherOptionCandidates() = %+v, want exactly the in-band NEAR contract", got)
	}
}

type mockATMIVStore struct {
	saved map[string]struct {
		iv  float64
		dte int
	}
}

func newMockATMIVStore() *mockATMIVStore {
	return &mockATMIVStore{saved: map[string]struct {
		iv  float64
		dte int
	}{}}
}

func (m *mockATMIVStore) SaveATMIV(ticker, date string, iv float64, dte int) error {
	m.saved[ticker] = struct {
		iv  float64
		dte int
	}{iv, dte}
	return nil
}

func TestRecordDailyATMIVPicksClosestStrikeToSpot(t *testing.T) {
	expiry := time.Now().AddDate(0, 0, 30)
	chain := fakeOptionChain{
		expirations: []time.Time{expiry},
		quotes: map[time.Time][]data.OptionQuote{
			expiry: {
				{Right: "C", Strike: 90, ImpliedVolatility: 0.20},
				{Right: "C", Strike: 100, ImpliedVolatility: 0.35}, // closest to spot 101
				{Right: "P", Strike: 100, ImpliedVolatility: 0.99}, // wrong side, ignored
			},
		},
	}
	store := newMockATMIVStore()

	RecordDailyATMIV(chain, store, []string{"AAPL"}, map[string]float64{"AAPL": 101}, "2026-08-22")

	got, ok := store.saved["AAPL"]
	if !ok {
		t.Fatal("RecordDailyATMIV() did not save a row for AAPL")
	}
	if got.iv != 0.35 {
		t.Errorf("saved IV = %v, want 0.35 (from the closest-to-spot strike)", got.iv)
	}
}

func TestRecordDailyATMIVNilProviderNoop(t *testing.T) {
	store := newMockATMIVStore()
	RecordDailyATMIV(nil, store, []string{"AAPL"}, map[string]float64{"AAPL": 101}, "2026-08-22")
	if len(store.saved) != 0 {
		t.Errorf("saved = %+v, want no writes when the chain provider is nil", store.saved)
	}
}

func TestRecordDailyATMIVSkipsTickerWithoutPrice(t *testing.T) {
	chain := fakeOptionChain{expirations: []time.Time{testExpiry}}
	store := newMockATMIVStore()
	RecordDailyATMIV(chain, store, []string{"AAPL"}, map[string]float64{}, "2026-08-22")
	if len(store.saved) != 0 {
		t.Errorf("saved = %+v, want no writes for a ticker with no quoted price", store.saved)
	}
}

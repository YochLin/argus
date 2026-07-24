package web

import (
	"errors"
	"testing"

	"argus/internal/db"
)

// fakeNames implements data.CompanyNameProvider for tests.
type fakeNames struct {
	names map[string]string
}

func (f *fakeNames) GetCompanyName(ticker string) (string, error) {
	if name, ok := f.names[ticker]; ok {
		return name, nil
	}
	return "", errors.New("no name")
}

func TestBuildCompanyNames_NilProvider(t *testing.T) {
	got := buildCompanyNames(&fakeDB{watchlist: []string{"2330"}}, nil)
	if got.Names == nil {
		t.Fatal("Names is nil, want empty map (frontend indexes it unconditionally)")
	}
	if len(got.Names) != 0 {
		t.Fatalf("Names = %v, want empty", got.Names)
	}
}

func TestBuildCompanyNames_TWOnlyAcrossSources(t *testing.T) {
	fdb := &fakeDB{
		watchlist: []string{"2330", "AAPL"},
		positions: []db.Position{{Ticker: "0050"}},
		txs:       []db.Transaction{{Ticker: "5274"}, {Ticker: "NVDA"}},
	}
	provider := &fakeNames{names: map[string]string{
		"2330": "台積電",
		"0050": "元大台灣50",
		"5274": "信驊",
		// AAPL/NVDA deliberately resolvable-looking here would be a bug if
		// looked up — the US skip happens before the provider is consulted.
		"AAPL": "should never be looked up",
	}}

	got := buildCompanyNames(fdb, provider)
	want := map[string]string{"2330": "台積電", "0050": "元大台灣50", "5274": "信驊"}
	if len(got.Names) != len(want) {
		t.Fatalf("Names = %v, want %v", got.Names, want)
	}
	for ticker, name := range want {
		if got.Names[ticker] != name {
			t.Errorf("Names[%q] = %q, want %q", ticker, got.Names[ticker], name)
		}
	}
}

func TestBuildCompanyNames_LookupFailureSkipsTicker(t *testing.T) {
	fdb := &fakeDB{watchlist: []string{"2330", "9999"}}
	provider := &fakeNames{names: map[string]string{"2330": "台積電"}}

	got := buildCompanyNames(fdb, provider)
	if len(got.Names) != 1 || got.Names["2330"] != "台積電" {
		t.Fatalf("Names = %v, want only 2330", got.Names)
	}
}

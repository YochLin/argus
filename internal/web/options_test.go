package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// fakeOptionChain implements data.OptionChainProvider with a fixed chain —
// no live Yahoo call.
type fakeOptionChain struct {
	quotes map[string][]data.OptionQuote // keyed by underlying
}

func (f fakeOptionChain) GetOptionExpirations(string) ([]time.Time, error) { return nil, nil }
func (f fakeOptionChain) GetOptionChain(underlying string, _ time.Time) ([]data.OptionQuote, error) {
	return f.quotes[underlying], nil
}

func TestBuildOptions_PositionWithMarkAndGreeks(t *testing.T) {
	expiry := time.Now().AddDate(0, 0, 40).Format("2006-01-02")
	fdb := &fakeDB{
		optionPositions: []db.OptionPosition{
			{ContractSymbol: "AAPL-CALL", Underlying: "AAPL", Right: "C", Strike: 315, Expiry: expiry, Multiplier: 100, Contracts: 2, AvgPremium: 5.40},
		},
	}
	expiryTime, _ := time.Parse("2006-01-02", expiry)
	chain := fakeOptionChain{quotes: map[string][]data.OptionQuote{
		"AAPL": {{ContractSymbol: "AAPL-CALL", Right: "C", Strike: 315, Bid: 6.0, Ask: 6.2, ImpliedVolatility: 0.30, Expiration: expiryTime}},
	}}
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{"AAPL": {Price: 315}}}

	resp, err := buildOptions(fdb, chain, quotes, market.US)
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if len(resp.Positions) != 1 {
		t.Fatalf("Positions = %+v, want 1", resp.Positions)
	}
	p := resp.Positions[0]
	if p.Mark != 6.1 {
		t.Errorf("Mark = %v, want 6.1 (mid of 6.0/6.2)", p.Mark)
	}
	if p.MarketValue != 6.1*2*100 {
		t.Errorf("MarketValue = %v, want %v", p.MarketValue, 6.1*2*100)
	}
	if p.Delta <= 0 {
		t.Errorf("Delta = %v, want positive (ATM long call)", p.Delta)
	}
	if len(resp.Calendar) != 1 || resp.Calendar[0].Date != expiry {
		t.Errorf("Calendar = %+v, want one entry dated %s", resp.Calendar, expiry)
	}
}

func TestBuildOptions_ClosedExcludesOpens(t *testing.T) {
	fdb := &fakeDB{
		optionTransactions: []db.OptionTransaction{
			{ContractSymbol: "A", Action: db.OptionActionBuyToOpen, RealizedPnL: 0},
			{ContractSymbol: "A", Action: db.OptionActionSellToClose, RealizedPnL: 180},
			{ContractSymbol: "B", Action: db.OptionActionExpired, RealizedPnL: -100},
		},
	}
	resp, err := buildOptions(fdb, nil, &fakeQuotes{}, market.US)
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if len(resp.Closed) != 2 {
		t.Fatalf("Closed = %+v, want 2 (BTO excluded)", resp.Closed)
	}
	for _, c := range resp.Closed {
		if c.Action == db.OptionActionBuyToOpen {
			t.Errorf("Closed contains an open action: %+v", c)
		}
	}
}

// TestBuildOptions_EmptyUSCollateralNotNil confirms Collateral.Positions is
// an empty slice (not nil) when a US account has no open option positions —
// the everyday "nothing to show" case. json.Marshal turns a nil slice into
// `null`, and OptionsView.tsx does `collateral.positions.length` on it
// unguarded, which throws and blanks the entire page (no error boundary).
func TestBuildOptions_EmptyUSCollateralNotNil(t *testing.T) {
	fdb := &fakeDB{}
	resp, err := buildOptions(fdb, nil, &fakeQuotes{}, market.US)
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if resp.Collateral.Positions == nil {
		t.Fatal("Collateral.Positions is nil, want empty slice (would marshal to JSON null)")
	}
	if b, _ := json.Marshal(resp.Collateral); string(b) != `{"lockedCash":0,"positions":[]}` {
		t.Errorf("Collateral JSON = %s, want positions: []", b)
	}
}

func TestBuildOptions_TWReturnsEmpty(t *testing.T) {
	fdb := &fakeDB{optionPositions: []db.OptionPosition{
		{ContractSymbol: "AAPL-CALL", Underlying: "AAPL", Right: "C", Strike: 315, Expiry: "2026-09-18", Multiplier: 100, Contracts: 1},
	}}
	resp, err := buildOptions(fdb, nil, &fakeQuotes{}, market.TW)
	if err != nil {
		t.Fatalf("buildOptions(TW): %v", err)
	}
	if len(resp.Positions) != 0 || len(resp.Closed) != 0 || len(resp.Calendar) != 0 {
		t.Errorf("buildOptions(TW) = %+v, want all-empty", resp)
	}
	if resp.Collateral.Positions == nil {
		t.Fatal("Collateral.Positions is nil, want empty slice (would marshal to JSON null)")
	}
	if b, _ := json.Marshal(resp.Collateral); string(b) != `{"lockedCash":0,"positions":[]}` {
		t.Errorf("Collateral JSON = %s, want positions: []", b)
	}
}

func TestBuildOptions_ChainFetchFailureDegrades(t *testing.T) {
	fdb := &fakeDB{
		optionPositions: []db.OptionPosition{
			{ContractSymbol: "AAPL-CALL", Underlying: "AAPL", Right: "C", Strike: 315, Expiry: "2026-09-18", Multiplier: 100, Contracts: 1, AvgPremium: 5.40},
		},
	}
	// No matching quote in the chain for this contract symbol.
	chain := fakeOptionChain{quotes: map[string][]data.OptionQuote{}}
	resp, err := buildOptions(fdb, chain, &fakeQuotes{}, market.US)
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if len(resp.Positions) != 1 {
		t.Fatalf("Positions = %+v, want 1 (degraded, not dropped)", resp.Positions)
	}
	if resp.Positions[0].Mark != 0 || resp.Positions[0].AvgPremium != 5.40 {
		t.Errorf("Positions[0] = %+v, want zeroed live fields but intact ledger fields", resp.Positions[0])
	}
}

func TestHandleOptions(t *testing.T) {
	s := testServer()
	s.db = &fakeDB{optionPositions: []db.OptionPosition{
		{ContractSymbol: "AAPL-CALL", Underlying: "AAPL", Right: "C", Strike: 315, Expiry: "2026-09-18", Multiplier: 100, Contracts: 1},
	}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/options", s.handleOptions)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/options", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got optionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Positions) != 1 {
		t.Errorf("Positions = %+v, want 1", got.Positions)
	}
}

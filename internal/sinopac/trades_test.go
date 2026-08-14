package sinopac

import "testing"

func TestTrades(t *testing.T) {
	details := []PositionDetail{
		{Date: "2026-08-10", Code: "2330", Quantity: 1000, Price: 950, Dseq: "d1", Direction: "Buy"},
		{Date: "2026-08-11", Code: "0050", Quantity: 4000, Price: 152.3, Dseq: "d2", Direction: "Buy"},
		{Date: "2026-08-12", Code: "0050", Quantity: 1000, Price: 153, Dseq: ""}, // empty dseq -> synthetic
	}
	pnl := []ProfitLoss{
		{Code: "2317", Quantity: 2000, Price: 105, Date: "2026-08-13", Dseq: "d3"},
	}
	skip := map[string]bool{"0050": true}

	got := Trades(details, pnl, skip)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (0050 rows skipped)", len(got))
	}
	if got[0].Ticker != "2330" || got[0].Side != "BUY" || got[0].ExtID != "d1" || got[0].Synthetic {
		t.Errorf("got[0] = %+v, want 2330 BUY d1 non-synthetic", got[0])
	}
	if got[1].Ticker != "2317" || got[1].Side != "SELL" || got[1].ExtID != "d3" {
		t.Errorf("got[1] = %+v, want 2317 SELL d3", got[1])
	}
	if got[0].Date > got[1].Date {
		t.Errorf("results not sorted ascending by date: %s > %s", got[0].Date, got[1].Date)
	}
}

func TestTradesSyntheticExtID(t *testing.T) {
	details := []PositionDetail{
		{Date: "2026-08-12", Code: "2330", Quantity: 1000, Price: 950, Dseq: ""},
	}
	got := Trades(details, nil, nil)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if !got[0].Synthetic || got[0].ExtID == "" {
		t.Errorf("got[0] = %+v, want non-empty synthetic ExtID", got[0])
	}
}

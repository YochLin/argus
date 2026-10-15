package sinopac

import "testing"

// The 6669/2368 rows are live daemon output for 2026-10-13, checked against
// the user's own records: 6669 bought 5 @2205, 2368 bought 10 @1175 and
// sold 15 @1305 (the sell was 10-07, moved to today here to exercise it).
func TestTrades(t *testing.T) {
	const today = "2026-10-13"
	positions := []Position{
		{Code: "6669", Quantity: 15, YdQuantity: 10},
		{Code: "2368", Quantity: 25, YdQuantity: 30},
		{Code: "2330", Quantity: 1000, YdQuantity: 1000}, // no trade today
		{Code: "0050", Quantity: 5099, YdQuantity: 5000},
	}
	details := []PositionDetail{
		{Date: "2026-10-06", Code: "6669", Quantity: 0, Price: 22832, Fee: 32, Dseq: "X0AJV"},
		{Date: today, Code: "6669", Quantity: 0, Price: 11040, Fee: 15, Dseq: "X0F5E"},
		{Date: "2026-09-24", Code: "2368", Quantity: 0, Price: 16523, Fee: 23, Dseq: "X054J"},
		{Date: today, Code: "2368", Quantity: 0, Price: 11766, Fee: 16, Dseq: "X0FA5"},
		{Date: "2026-08-01", Code: "2330", Quantity: 1, Price: 950000, Dseq: "d1"},
		{Date: today, Code: "0050", Quantity: 0, Price: 10000, Dseq: "d2"},
	}
	pnl := []ProfitLoss{
		{Code: "2368", Quantity: 15, Price: 1305, Date: today, Dseq: "X03JZ"},
		{Code: "2327", Quantity: 35, Price: 635, Date: "2026-10-08", Dseq: "X03MX"}, // not today
	}

	got := Trades(positions, details, pnl, map[string]bool{"0050": true}, today)

	want := []Trade{
		{ExtID: "X0FA5", Date: today, Ticker: "2368", Side: "BUY", Shares: 10, Price: 1175},
		{ExtID: "X03JZ", Date: today, Ticker: "2368", Side: "SELL", Shares: 15, Price: 1305},
		{ExtID: "X0F5E", Date: today, Ticker: "6669", Side: "BUY", Shares: 5, Price: 2205},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %+v in %+v", w, got)
		}
	}
}

// Two buys of one ticker today become one Trade; an empty dseq makes it
// synthetic.
func TestTradesMergesTodaysLots(t *testing.T) {
	const today = "2026-10-13"
	positions := []Position{{Code: "2330", Quantity: 30, YdQuantity: 0}}
	details := []PositionDetail{
		{Date: today, Code: "2330", Price: 10010, Fee: 10, Dseq: "a"},
		{Date: today, Code: "2330", Price: 20020, Fee: 20, Dseq: ""},
	}
	got := Trades(positions, details, nil, nil, today)
	if len(got) != 1 || got[0].Shares != 30 || got[0].Price != 1000 || !got[0].Synthetic {
		t.Errorf("got %+v, want one synthetic BUY 30 @1000", got)
	}
}

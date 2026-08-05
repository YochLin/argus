package option

import (
	"testing"
	"time"

	"argus/internal/data"
)

func TestSelect_LiquidityGate(t *testing.T) {
	asOf := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	expiry := asOf.AddDate(0, 0, 40) // within LongCall's 30-60 DTE band

	base := data.OptionQuote{
		ContractSymbol:    "AAPL260914C00320000",
		Right:             "C",
		Strike:            320,
		Bid:               4.90,
		Ask:               5.10,
		Volume:            100,
		OpenInterest:      1000,
		ImpliedVolatility: 0.30,
		Expiration:        expiry,
	}

	cases := []struct {
		name   string
		modify func(data.OptionQuote) data.OptionQuote
		want   bool
	}{
		{"passes as-is", func(q data.OptionQuote) data.OptionQuote { return q }, true},
		{"low open interest fails", func(q data.OptionQuote) data.OptionQuote { q.OpenInterest = 100; return q }, false},
		{"low volume fails", func(q data.OptionQuote) data.OptionQuote { q.Volume = 1; return q }, false},
		{"wide spread fails", func(q data.OptionQuote) data.OptionQuote { q.Bid, q.Ask = 3.0, 7.0; return q }, false},
		{"no bid/ask and no last fails", func(q data.OptionQuote) data.OptionQuote { q.Bid, q.Ask, q.LastPrice = 0, 0, 0; return q }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chain := []data.OptionQuote{c.modify(base)}
			got := Select(chain, 315, asOf, LongCall)
			if (len(got) == 1) != c.want {
				t.Errorf("Select() len = %d, want present=%v", len(got), c.want)
			}
		})
	}
}

func TestSelect_DTEAndDeltaBand(t *testing.T) {
	asOf := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	spot := 315.0

	mk := func(dte int, strike float64) data.OptionQuote {
		return data.OptionQuote{
			ContractSymbol:    "AAPL",
			Right:             "C",
			Strike:            strike,
			Bid:               4.9,
			Ask:               5.1,
			Volume:            100,
			OpenInterest:      1000,
			ImpliedVolatility: 0.30,
			Expiration:        asOf.AddDate(0, 0, dte),
		}
	}

	// Too short DTE (below LongCall's 30 floor) is excluded even though
	// everything else about it would pass.
	tooShort := []data.OptionQuote{mk(10, 315)}
	if got := Select(tooShort, spot, asOf, LongCall); len(got) != 0 {
		t.Errorf("Select() with DTE=10 = %d candidates, want 0 (below DTEMin)", len(got))
	}

	// Deep ITM (delta near 1.0) is excluded by LongCall's 0.35-0.60 band.
	deepITM := []data.OptionQuote{mk(40, 200)}
	if got := Select(deepITM, spot, asOf, LongCall); len(got) != 0 {
		t.Errorf("Select() deep ITM = %d candidates, want 0 (delta out of band)", len(got))
	}

	// Near-the-money, mid-band DTE should pass and carry populated greeks.
	atTheMoney := []data.OptionQuote{mk(40, spot)}
	got := Select(atTheMoney, spot, asOf, LongCall)
	if len(got) != 1 {
		t.Fatalf("Select() ATM = %d candidates, want 1", len(got))
	}
	if got[0].Greeks.Delta <= 0 || got[0].DTE != 40 {
		t.Errorf("candidate = %+v, want positive delta and DTE 40", got[0])
	}
}

func TestSelect_SortedByDTEThenSpread(t *testing.T) {
	asOf := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	spot := 315.0

	mk := func(symbol string, dte int, spread float64) data.OptionQuote {
		mid := 5.0
		return data.OptionQuote{
			ContractSymbol:    symbol,
			Right:             "C",
			Strike:            spot,
			Bid:               mid - spread/2,
			Ask:               mid + spread/2,
			Volume:            100,
			OpenInterest:      1000,
			ImpliedVolatility: 0.30,
			Expiration:        asOf.AddDate(0, 0, dte),
		}
	}

	chain := []data.OptionQuote{
		mk("later-wide", 50, 0.40),
		mk("sooner", 35, 0.20),
		mk("later-tight", 50, 0.10),
	}
	got := Select(chain, spot, asOf, LongCall)
	if len(got) != 3 {
		t.Fatalf("Select() = %d candidates, want 3", len(got))
	}
	wantOrder := []string{"sooner", "later-tight", "later-wide"}
	for i, w := range wantOrder {
		if got[i].Quote.ContractSymbol != w {
			t.Errorf("candidate[%d] = %s, want %s", i, got[i].Quote.ContractSymbol, w)
		}
	}
}

func TestSelect_RightMismatchExcluded(t *testing.T) {
	asOf := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	put := data.OptionQuote{
		ContractSymbol: "AAPL260914P00320000", Right: "P", Strike: 320,
		Bid: 4.9, Ask: 5.1, Volume: 100, OpenInterest: 1000,
		ImpliedVolatility: 0.30, Expiration: asOf.AddDate(0, 0, 40),
	}
	// LongCall only wants calls.
	if got := Select([]data.OptionQuote{put}, 315, asOf, LongCall); len(got) != 0 {
		t.Errorf("Select(LongCall) on a put = %d candidates, want 0", len(got))
	}
}

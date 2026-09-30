package web

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

// flatThenLast is n daily candles all closing at 100 except the last one, so
// every trailing window (20/60/120) reads (last-100)% as long as n covers it.
func flatThenLast(n int, last float64) []data.Candle {
	out := make([]data.Candle, n)
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range out {
		c := 100.0
		if i == n-1 {
			c = last
		}
		out[i] = data.Candle{Date: d.AddDate(0, 0, i), Open: c, High: c, Low: c, Close: c, Volume: 1000}
	}
	return out
}

type countingSector struct {
	industry map[string]string
	err      map[string]error
	calls    atomic.Int32
}

func (c *countingSector) GetSector(ticker string) (data.SectorInfo, error) {
	c.calls.Add(1)
	if err := c.err[ticker]; err != nil {
		return data.SectorInfo{}, err
	}
	return data.SectorInfo{Industry: c.industry[ticker]}, nil
}

func peerByTicker(t *testing.T, rows []peerRow, ticker string) peerRow {
	t.Helper()
	for _, r := range rows {
		if r.Ticker == ticker {
			return r
		}
	}
	t.Fatalf("no row for %s in %+v", ticker, rows)
	return peerRow{}
}

func TestBuildPeers_US(t *testing.T) {
	s := &Server{
		db: &fakeDB{watchlist: []string{"AAPL", "MSFT", "NVDA", "XOM"}},
		history: &fakeHistory{candles: map[string][]data.Candle{
			"AAPL": flatThenLast(130, 110),
			"MSFT": flatThenLast(130, 130),
			"NVDA": flatThenLast(50, 105), // too short for the 60/120-day windows
			"XOM":  flatThenLast(130, 90),
		}},
		sector: &countingSector{industry: map[string]string{
			"AAPL": "Technology", "MSFT": "Technology", "NVDA": "Technology", "XOM": "Energy",
		}},
	}

	got, err := s.buildPeers("AAPL")
	if err != nil {
		t.Fatalf("buildPeers() error = %v", err)
	}
	if got.Sector != "Technology" {
		t.Errorf("Sector = %q, want Technology", got.Sector)
	}
	// XOM is another sector; NVDA has no 60-day figure so it sorts last.
	var order []string
	for _, r := range got.Peers {
		order = append(order, r.Ticker)
	}
	if want := []string{"MSFT", "AAPL", "NVDA"}; !equalStrings(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}

	self := peerByTicker(t, got.Peers, "AAPL")
	if !self.Self || self.Rel != nil || self.Price != 110 {
		t.Errorf("self row = %+v, want Self, nil Rel, Price 110", self)
	}
	msft := peerByTicker(t, got.Peers, "MSFT")
	if msft.Self || msft.Chg60 == nil || *msft.Chg60 != 30 || msft.Rel == nil || *msft.Rel != 20 {
		t.Errorf("MSFT row = %+v, want Chg60 30 and Rel 20", msft)
	}
	nvda := peerByTicker(t, got.Peers, "NVDA")
	if nvda.Chg20 == nil || *nvda.Chg20 != 5 || nvda.Chg60 != nil || nvda.Chg120 != nil || nvda.Rel != nil {
		t.Errorf("NVDA row = %+v, want only Chg20 (5) set", nvda)
	}
}

func TestBuildPeers_HiddenWhenAlone(t *testing.T) {
	s := &Server{
		db:      &fakeDB{watchlist: []string{"AAPL", "XOM"}},
		history: &fakeHistory{candles: map[string][]data.Candle{"AAPL": flatThenLast(130, 110), "XOM": flatThenLast(130, 90)}},
		sector:  &countingSector{industry: map[string]string{"AAPL": "Technology", "XOM": "Energy"}},
	}
	got, err := s.buildPeers("AAPL")
	if err != nil {
		t.Fatalf("buildPeers() error = %v", err)
	}
	if got.Peers == nil || len(got.Peers) != 0 {
		t.Errorf("Peers = %v, want empty non-nil slice", got.Peers)
	}
}

// A ticker reached by URL that is neither watched nor held still gets a card
// against the tracked names of its sector.
func TestBuildPeers_ViewedTickerNotTracked(t *testing.T) {
	s := &Server{
		db: &fakeDB{
			watchlist: []string{"MSFT"},
			positions: []db.Position{{Ticker: "MSFT", Shares: 1}},
		},
		history: &fakeHistory{candles: map[string][]data.Candle{"TSLA": flatThenLast(130, 120), "MSFT": flatThenLast(130, 110)}},
		sector:  &countingSector{industry: map[string]string{"TSLA": "Auto", "MSFT": "Auto"}},
	}
	got, err := s.buildPeers("TSLA")
	if err != nil {
		t.Fatalf("buildPeers() error = %v", err)
	}
	if len(got.Peers) != 2 || got.Peers[0].Ticker != "TSLA" || !got.Peers[0].Self {
		t.Errorf("Peers = %+v, want TSLA (self) then MSFT", got.Peers)
	}
}

func TestBuildPeers_TW(t *testing.T) {
	s := &Server{
		db: &fakeDB{watchlist: []string{"2330", "2454", "2317"}},
		history: &fakeHistory{candles: map[string][]data.Candle{
			"2330": flatThenLast(130, 120), "2454": flatThenLast(130, 100), "2317": flatThenLast(130, 200),
		}},
		industryMap: &fakeIndustryMap{m: map[string]string{"2330": "半導體業", "2454": "半導體業", "2317": "其他電子業"}},
	}
	got, err := s.buildPeers("2330")
	if err != nil {
		t.Fatalf("buildPeers() error = %v", err)
	}
	if got.Sector != "半導體業" || len(got.Peers) != 2 {
		t.Fatalf("got %+v, want 2 rows in 半導體業", got)
	}
	if r := peerByTicker(t, got.Peers, "2454"); r.Rel == nil || *r.Rel != -20 {
		t.Errorf("2454 Rel = %v, want -20", r.Rel)
	}
}

func TestBuildPeers_NoSectorSource(t *testing.T) {
	s := &Server{db: &fakeDB{watchlist: []string{"AAPL", "MSFT"}}, history: &fakeHistory{}}
	got, err := s.buildPeers("AAPL")
	if err != nil {
		t.Fatalf("buildPeers() error = %v", err)
	}
	if len(got.Peers) != 0 {
		t.Errorf("Peers = %v, want none without a sector provider", got.Peers)
	}
}

// Sectors are cached across requests, including a failed lookup (an ETF with
// no industry), so a page view costs no repeat Finnhub calls.
func TestBuildPeers_SectorLookupsAreCached(t *testing.T) {
	sec := &countingSector{
		industry: map[string]string{"AAPL": "Technology", "MSFT": "Technology"},
		err:      map[string]error{"VOO": errors.New("no industry")},
	}
	s := &Server{
		db: &fakeDB{watchlist: []string{"AAPL", "MSFT", "VOO"}},
		history: &fakeHistory{candles: map[string][]data.Candle{
			"AAPL": flatThenLast(130, 110), "MSFT": flatThenLast(130, 120),
		}},
		sector: sec,
	}
	for i := 0; i < 3; i++ {
		got, err := s.buildPeers("AAPL")
		if err != nil || len(got.Peers) != 2 {
			t.Fatalf("call %d: peers = %+v, err = %v", i, got.Peers, err)
		}
	}
	if n := sec.calls.Load(); n != 3 {
		t.Errorf("GetSector called %d times over 3 requests, want 3 (one per ticker)", n)
	}
}

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/signals"
)

// TestUniverseScanChunkFullCoverage verifies UniverseScanChunk rotates
// through every ticker exactly once over a full chunkCount-day cycle, with
// no gaps or duplicates — the property the daily scan job actually depends
// on for eventual full universe coverage.
func TestUniverseScanChunkFullCoverage(t *testing.T) {
	var tickers []string
	for i := 0; i < 503; i++ {
		tickers = append(tickers, fmt.Sprintf("T%03d", i))
	}

	const chunkCount = 1 // matches bot.scanChunkCount in production
	seen := make(map[string]int)
	for day := 0; day < chunkCount; day++ {
		for _, ticker := range UniverseScanChunk(tickers, chunkCount, day) {
			seen[ticker]++
		}
	}

	if len(seen) != len(tickers) {
		t.Fatalf("covered %d/%d tickers over a full cycle, want all of them", len(seen), len(tickers))
	}
	for ticker, n := range seen {
		if n != 1 {
			t.Errorf("ticker %s scanned %d times over a full cycle, want exactly 1", ticker, n)
		}
	}
}

func TestUniverseScanChunkEmptyAndNegativeDay(t *testing.T) {
	if got := UniverseScanChunk(nil, 1, 0); got != nil {
		t.Errorf("UniverseScanChunk(nil, ...) = %v, want nil", got)
	}
	tickers := []string{"A", "B", "C", "D", "E"}
	// A negative dayIndex must still resolve to a valid, in-range chunk
	// rather than panicking on a negative slice index.
	got := UniverseScanChunk(tickers, 1, -1)
	if len(got) == 0 {
		t.Errorf("UniverseScanChunk(..., -1) = %v, want a non-empty chunk", got)
	}
}

func TestScanServiceRevenueGrowthOK(t *testing.T) {
	tests := []struct {
		name         string
		fundamentals func(string) (*data.Fundamentals, error)
		ticker       string
		minPct       float64
		want         bool
	}{
		{"no fundamentals reader configured", nil, "AAPL", 5, false},
		{"fetch error fails closed", func(string) (*data.Fundamentals, error) { return nil, errors.New("boom") }, "AAPL", 5, false},
		{"US ticker uses RevenueGrowthYoY, above threshold", func(string) (*data.Fundamentals, error) {
			return &data.Fundamentals{RevenueGrowthYoY: 10}, nil
		}, "AAPL", 5, true},
		{"US ticker below threshold", func(string) (*data.Fundamentals, error) {
			return &data.Fundamentals{RevenueGrowthYoY: 3}, nil
		}, "AAPL", 5, false},
		{"TW ticker uses MonthRevenueYoYPct, above threshold", func(string) (*data.Fundamentals, error) {
			return &data.Fundamentals{MonthRevenueYoYPct: 10}, nil
		}, "2330", 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ScanService{fundamentals: tt.fundamentals}
			if got := s.revenueGrowthOK(tt.ticker, tt.minPct); got != tt.want {
				t.Errorf("revenueGrowthOK() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestScanServiceCheckStatefulSignalsRSIDedup exercises the signal_states
// persistence loop CheckStatefulSignals moved from bot.checkStatefulSignals
// (Phase 24 Stage 1 Scan & Strategy Service extraction): the same
// overbought RSI reading fires once and then goes silent until state
// changes, proving the store round-trip survived the move.
func TestScanServiceCheckStatefulSignalsRSIDedup(t *testing.T) {
	store := &fakeScanStore{mockRiskStore: newMockRiskStore()}
	s := NewScanService(ScanConfig{Store: store, Detector: signals.NewDetector(i18n.EN)})

	candles := make([]data.Candle, 15)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range candles {
		candles[i] = data.Candle{Date: base.AddDate(0, 0, i), Close: 100 + float64(i)}
	}

	first := s.CheckStatefulSignals("AAPL", candles)
	if !hasSignalType(first, "rsi_overbought") {
		t.Fatalf("CheckStatefulSignals() first call = %v, want an rsi_overbought signal", first)
	}
	if got := store.signalStates["AAPL:"+signals.FamilyRSI]; got != signals.StateOverbought {
		t.Errorf("persisted RSI state = %q, want %q", got, signals.StateOverbought)
	}

	second := s.CheckStatefulSignals("AAPL", candles)
	if hasSignalType(second, "rsi_overbought") {
		t.Errorf("CheckStatefulSignals() second call = %v, want no repeat rsi_overbought signal", second)
	}
}

// fakeScanStore adds RunUniverseScan's universe/watchlist/scan_hits methods
// to the existing signal_states fake, so both halves of ScanStore come from
// one hand-written value.
type fakeScanStore struct {
	*mockRiskStore
	universe  []db.UniverseEntry
	watchlist map[market.MarketID][]string
	hits      []string // "TICKER|date|reason"
}

func (f *fakeScanStore) GetUniverse() ([]db.UniverseEntry, error) { return f.universe, nil }

func (f *fakeScanStore) GetWatchlistByMarket(m market.MarketID) ([]string, error) {
	return f.watchlist[m], nil
}

func (f *fakeScanStore) SaveScanHit(ticker, date, reason string) error {
	f.hits = append(f.hits, ticker+"|"+date+"|"+reason)
	return nil
}

// fakeHistory/fakeQuotes stand in for Yahoo and the provider chain.
type fakeHistory struct {
	candles map[string][]data.Candle
	asked   []string
}

func (f *fakeHistory) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	f.asked = append(f.asked, ticker)
	c, ok := f.candles[ticker]
	if !ok {
		return nil, errors.New("no history")
	}
	return c, nil
}

type fakeQuotes struct{ quote data.Quote }

func (f *fakeQuotes) GetQuote(ticker string) (*data.Quote, error) {
	q := f.quote
	q.Ticker = ticker
	return &q, nil
}

// TestRunUniverseScanSelection pins down which tickers RunUniverseScan
// actually fetches history for — the three filters that decide it (wrong
// market, already on the watchlist, TW-restricted) are the whole reason this
// job isn't just "scan everything", and getting any of them backwards would
// silently cost either coverage or a wasted ~500 requests.
func TestRunUniverseScanSelection(t *testing.T) {
	// A Wednesday, so the US trading-day gate (which checks *yesterday*)
	// sees a Tuesday and lets the scan through.
	now := time.Date(2026, 8, 19, 5, 45, 0, 0, cst)
	rising := make([]data.Candle, 15)
	for i := range rising {
		rising[i] = data.Candle{Date: now.AddDate(0, 0, i-15), Close: 100 + float64(i)}
	}

	store := &fakeScanStore{
		mockRiskStore: newMockRiskStore(),
		universe: []db.UniverseEntry{
			{Ticker: "AAPL"}, {Ticker: "MSFT"}, {Ticker: "2330"},
		},
		watchlist: map[market.MarketID][]string{market.US: {"MSFT"}},
	}
	history := &fakeHistory{candles: map[string][]data.Candle{"AAPL": rising}}
	s := NewScanService(ScanConfig{
		Store:    store,
		Detector: signals.NewDetector(i18n.EN),
		History:  history,
		Quotes:   &fakeQuotes{quote: data.Quote{Price: 100, Timestamp: now}},
		Lang:     i18n.EN,
		Now:      func() time.Time { return now },
	})

	res, err := s.RunUniverseScan(context.Background(), market.US)
	if err != nil {
		t.Fatalf("RunUniverseScan: %v", err)
	}
	if res.Skipped {
		t.Fatal("scan skipped on a trading day")
	}
	// MSFT is on the watchlist, 2330 belongs to the TW market: only AAPL is
	// this market's un-watched universe. SPY is the market-regime benchmark,
	// fetched once per scan rather than per ticker, so it isn't a selection.
	var scanned []string
	for _, t := range history.asked {
		if t != BenchmarkFor(market.US) {
			scanned = append(scanned, t)
		}
	}
	if len(scanned) != 1 || scanned[0] != "AAPL" {
		t.Errorf("fetched history for %v, want [AAPL]", scanned)
	}
	if res.Scanned != 1 {
		t.Errorf("Scanned = %d, want 1", res.Scanned)
	}
	if res.Hits == 0 {
		t.Fatal("expected at least one scan hit from the rising AAPL fixture (RSI overbought)")
	}
	if res.Hits != len(store.hits) {
		t.Errorf("Hits = %d but %d rows written", res.Hits, len(store.hits))
	}
	for _, h := range store.hits {
		if !strings.Contains(h, "2026-08-19") {
			t.Errorf("scan hit %q not dated from the injected clock", h)
		}
	}
}

// fakeRestricted is RestrictedProvider's test double.
type fakeRestricted struct {
	punish map[string]string
	notice map[string]string
}

func (f *fakeRestricted) RegulatoryPunish(ctx context.Context) (map[string]string, error) {
	return f.punish, nil
}

func (f *fakeRestricted) RegulatoryNotice(ctx context.Context) (map[string]string, error) {
	return f.notice, nil
}

// TestRunUniverseScanRestrictedFilter covers the one selection filter
// TestRunUniverseScanSelection's name (and the PR description) claimed but
// never actually exercised — market.US never reaches RestrictedTickers'
// TW-only path at all. A disposition-listed (處置) TW ticker must be
// skipped before a history fetch, not scanned and potentially surfaced as a
// candidate.
func TestRunUniverseScanRestrictedFilter(t *testing.T) {
	now := time.Date(2026, 8, 19, 5, 45, 0, 0, cst)
	rising := make([]data.Candle, 15)
	for i := range rising {
		rising[i] = data.Candle{Date: now.AddDate(0, 0, i-15), Close: 100 + float64(i)}
	}

	store := &fakeScanStore{
		mockRiskStore: newMockRiskStore(),
		universe: []db.UniverseEntry{
			{Ticker: "2330"}, {Ticker: "2454"},
		},
	}
	history := &fakeHistory{candles: map[string][]data.Candle{"2330": rising, "2454": rising}}
	s := NewScanService(ScanConfig{
		Store:    store,
		Detector: signals.NewDetector(i18n.EN),
		History:  history,
		// marketClosed's TW path stales this off the real wall clock
		// (time.Since), not the injected Now above — a Timestamp pinned to
		// the fixture's 2026-08-19 clock would read as stale relative to
		// whenever the test actually runs.
		Quotes:     &fakeQuotes{quote: data.Quote{Price: 100, Timestamp: time.Now()}},
		Restricted: &fakeRestricted{punish: map[string]string{"2330": "處置"}},
		Lang:       i18n.EN,
		Now:        func() time.Time { return now },
	})

	res, err := s.RunUniverseScan(context.Background(), market.TW)
	if err != nil {
		t.Fatalf("RunUniverseScan: %v", err)
	}
	if res.Skipped {
		t.Fatal("scan skipped on a trading day")
	}
	var scanned []string
	for _, t := range history.asked {
		if t != BenchmarkFor(market.TW) { // 0050, fetched once for the regime check, not a selection
			scanned = append(scanned, t)
		}
	}
	if len(scanned) != 1 || scanned[0] != "2454" {
		t.Errorf("fetched history for %v, want [2454] (2330 is restricted)", scanned)
	}
}

// TestRunUniverseScanSkipsClosedMarket covers the gate that keeps a holiday
// rerun from writing a duplicate scan_hits row off stale data.
func TestRunUniverseScanSkipsClosedMarket(t *testing.T) {
	store := &fakeScanStore{mockRiskStore: newMockRiskStore(), universe: []db.UniverseEntry{{Ticker: "AAPL"}}}
	history := &fakeHistory{}
	// A Sunday: yesterday was Saturday, not a US trading day.
	sunday := time.Date(2026, 8, 23, 5, 45, 0, 0, cst)
	s := NewScanService(ScanConfig{
		Store:    store,
		Detector: signals.NewDetector(i18n.EN),
		History:  history,
		Now:      func() time.Time { return sunday },
	})

	res, err := s.RunUniverseScan(context.Background(), market.US)
	if err != nil {
		t.Fatalf("RunUniverseScan: %v", err)
	}
	if !res.Skipped || len(history.asked) != 0 {
		t.Errorf("closed-market run = %+v with %d fetches, want skipped and none", res, len(history.asked))
	}
}

func TestDecorateStrategyHits(t *testing.T) {
	sigs := []signals.Signal{{Type: "strategy_squeeze", Message: "hit"}, {Type: "rsi_overbought", Message: "rsi"}}
	got := DecorateStrategyHits(sigs, false, i18n.EN)
	if got[0].Message != "hit" {
		t.Errorf("non-bear regime decorated %q", got[0].Message)
	}
	got = DecorateStrategyHits(sigs, true, i18n.EN)
	if !strings.Contains(got[0].Message, "\n") {
		t.Errorf("strategy hit not decorated in a bear regime: %q", got[0].Message)
	}
	if got[1].Message != "rsi" {
		t.Errorf("non-strategy signal decorated: %q", got[1].Message)
	}
}

// 網 3's §4.4 downgrade notice is unconditional — it is not a market-regime
// caveat, so a bull regime must not silence it.
func TestDecorateStrategyHitsTrendBreakoutAlwaysDowngraded(t *testing.T) {
	sigs := []signals.Signal{{Type: signals.TypeTrendBreakout, Message: "hit"}, {Type: "strategy_squeeze", Message: "other"}}
	got := DecorateStrategyHits(sigs, false, i18n.EN)
	want := i18n.T(i18n.EN, i18n.KeyStrategyUnvalidated)
	if !strings.Contains(got[0].Message, want) {
		t.Errorf("trend breakout missing the downgrade notice in a bull regime: %q", got[0].Message)
	}
	if strings.Contains(got[1].Message, want) {
		t.Errorf("downgrade notice leaked onto another screen: %q", got[1].Message)
	}
}

func TestDecorateStrategyHitsBoxBottomAlwaysDowngraded(t *testing.T) {
	sigs := []signals.Signal{{Type: signals.TypeBoxBottom, Message: "hit"}, {Type: "strategy_squeeze", Message: "other"}}
	got := DecorateStrategyHits(sigs, false, i18n.EN)
	want := i18n.T(i18n.EN, i18n.KeyStrategyUnvalidatedBoxBottom)
	if !strings.Contains(got[0].Message, want) {
		t.Errorf("box bottom missing the downgrade notice in a bull regime: %q", got[0].Message)
	}
	if strings.Contains(got[1].Message, want) {
		t.Errorf("downgrade notice leaked onto another screen: %q", got[1].Message)
	}
}

func TestDecorateStrategyHitsTrustFollowAlwaysDowngraded(t *testing.T) {
	sigs := []signals.Signal{{Type: signals.TypeTrustFollow, Message: "hit"}, {Type: "strategy_squeeze", Message: "other"}}
	got := DecorateStrategyHits(sigs, false, i18n.EN)
	want := i18n.T(i18n.EN, i18n.KeyStrategyUnvalidatedTrustFollow)
	if !strings.Contains(got[0].Message, want) {
		t.Errorf("trust follow missing the downgrade notice in a bull regime: %q", got[0].Message)
	}
	if strings.Contains(got[1].Message, want) {
		t.Errorf("downgrade notice leaked onto another screen: %q", got[1].Message)
	}
}

func hasSignalType(sigs []signals.Signal, typ string) bool {
	for _, s := range sigs {
		if s.Type == typ {
			return true
		}
	}
	return false
}

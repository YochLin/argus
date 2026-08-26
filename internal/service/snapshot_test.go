package service

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

type mockSnapshotStore struct {
	saved         []db.DailySnapshot
	recentCloses  map[string][]float64
	positions     []db.Position
	netWorthSaves map[string]float64 // key: date+":"+market
	saveErr       error
}

func newMockSnapshotStore() *mockSnapshotStore {
	return &mockSnapshotStore{
		recentCloses:  make(map[string][]float64),
		netWorthSaves: make(map[string]float64),
	}
}

func (m *mockSnapshotStore) SaveSnapshot(s db.DailySnapshot) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, s)
	return nil
}

func (m *mockSnapshotStore) GetRecentCloses(ticker string, n int) ([]float64, error) {
	return m.recentCloses[ticker], nil
}

func (m *mockSnapshotStore) GetPositions() ([]db.Position, error) {
	return m.positions, nil
}

func (m *mockSnapshotStore) SaveNetWorthSnapshot(date string, mkt market.MarketID, total float64) error {
	m.netWorthSaves[date+":"+string(mkt)] = total
	return nil
}

type mockQuoteStore struct {
	quotes map[string]*data.Quote
	err    error
}

func (m mockQuoteStore) GetQuote(ticker string) (*data.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	q, ok := m.quotes[ticker]
	if !ok {
		return nil, errors.New("no quote for " + ticker)
	}
	return q, nil
}

func TestFetchClosingQuotesSkipsStaleAndSavesFresh(t *testing.T) {
	store := newMockSnapshotStore()
	quotes := mockQuoteStore{quotes: map[string]*data.Quote{
		"FRESH": {Ticker: "FRESH", Price: 100, Timestamp: time.Now()},
		"STALE": {Ticker: "STALE", Price: 50, Timestamp: time.Now().Add(-24 * time.Hour)},
	}}
	s := NewSnapshotService(store, quotes)

	got := s.FetchClosingQuotes([]string{"FRESH", "STALE", "MISSING"}, market.US, "2026-08-21")
	if _, ok := got.Prices["FRESH"]; !ok {
		t.Errorf("Prices = %v, want FRESH present", got.Prices)
	}
	if _, ok := got.Prices["STALE"]; ok {
		t.Errorf("Prices = %v, want STALE excluded (quote >12h old)", got.Prices)
	}
	if _, ok := got.Prices["MISSING"]; ok {
		t.Errorf("Prices = %v, want MISSING excluded (fetch error)", got.Prices)
	}
	if len(store.saved) != 1 || store.saved[0].Ticker != "FRESH" {
		t.Errorf("saved = %v, want exactly one FRESH row", store.saved)
	}
}

func TestFetchClosingQuotesDetectsBigMoveEvent(t *testing.T) {
	store := newMockSnapshotStore()
	quotes := mockQuoteStore{quotes: map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 110, PrevClose: 100, ChangePercent: 10, Timestamp: time.Now()},
	}}
	s := NewSnapshotService(store, quotes)

	got := s.FetchClosingQuotes([]string{"AAPL"}, market.US, "2026-08-21")
	if len(got.PriceEvents) != 1 || got.PriceEvents[0].Ticker != "AAPL" {
		t.Fatalf("PriceEvents = %v, want one AAPL event (10%% change >= US 7%% threshold)", got.PriceEvents)
	}
	if got.PriceEvents[0].ChangePct != 10 {
		t.Errorf("ChangePct = %v, want 10", got.PriceEvents[0].ChangePct)
	}
}

func TestSnapshotBenchmarkToSkipsStaleAndSaves(t *testing.T) {
	t.Run("stale quote is not saved", func(t *testing.T) {
		store := newMockSnapshotStore()
		quotes := mockQuoteStore{quotes: map[string]*data.Quote{
			"SPY": {Ticker: "SPY", Price: 500, Timestamp: time.Now().Add(-24 * time.Hour)},
		}}
		s := NewSnapshotService(store, quotes)
		s.SnapshotBenchmarkTo(store, "SPY", "2026-08-21")
		if len(store.saved) != 0 {
			t.Errorf("saved = %v, want none (stale quote)", store.saved)
		}
	})

	t.Run("fresh quote is saved", func(t *testing.T) {
		store := newMockSnapshotStore()
		quotes := mockQuoteStore{quotes: map[string]*data.Quote{
			"SPY": {Ticker: "SPY", Price: 500, Timestamp: time.Now()},
		}}
		s := NewSnapshotService(store, quotes)
		s.SnapshotBenchmarkTo(store, "SPY", "2026-08-21")
		if len(store.saved) != 1 || store.saved[0].Ticker != "SPY" || store.saved[0].Close != 500 {
			t.Errorf("saved = %v, want one SPY row at 500", store.saved)
		}
	})
}

func TestRecordNetWorthSnapshotFiltersByMarketAndFallsBackToQuote(t *testing.T) {
	store := newMockSnapshotStore()
	store.positions = []db.Position{
		{Ticker: "AAPL", Shares: 10},  // US, price prefetched
		{Ticker: "MSFT", Shares: 2},   // US, needs quote fallback
		{Ticker: "2330", Shares: 100}, // TW, excluded from a US snapshot
	}
	quotes := mockQuoteStore{quotes: map[string]*data.Quote{
		"MSFT": {Ticker: "MSFT", Price: 300},
	}}
	s := NewSnapshotService(store, quotes)

	s.RecordNetWorthSnapshot(market.US, "2026-08-21", map[string]float64{"AAPL": 200})

	want := 10*200.0 + 2*300.0
	got, ok := store.netWorthSaves["2026-08-21:us"]
	if !ok || got != want {
		t.Errorf("net worth saved = %v, ok=%v, want %v (AAPL prefetched + MSFT fallback, TW excluded)", got, ok, want)
	}
}

func TestRecordNetWorthSnapshotNoPositionsInMarketIsNoop(t *testing.T) {
	store := newMockSnapshotStore()
	store.positions = []db.Position{{Ticker: "2330", Shares: 100}}
	s := NewSnapshotService(store, mockQuoteStore{})

	s.RecordNetWorthSnapshot(market.US, "2026-08-21", nil)

	if len(store.netWorthSaves) != 0 {
		t.Errorf("net worth saved = %v, want none (no US positions)", store.netWorthSaves)
	}
}

func TestFilterNewsNearDate(t *testing.T) {
	at := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	news := []data.NewsItem{
		{Headline: "same day", PublishedAt: at("2026-08-25T14:00:00+08:00")},
		{Headline: "us session, next cst day", PublishedAt: at("2026-08-26T03:30:00+08:00")},
		{Headline: "day before", PublishedAt: at("2026-08-24T09:00:00+08:00")},
		{Headline: "two days old", PublishedAt: at("2026-08-23T23:59:00+08:00")},
		{Headline: "a week old", PublishedAt: at("2026-08-18T10:00:00+08:00")},
		{Headline: "undated"},
	}

	got := FilterNewsNearDate(news, "2026-08-25")
	want := []string{"same day", "us session, next cst day", "day before"}
	if len(got) != len(want) {
		t.Fatalf("kept %d items %v, want %v", len(got), headlines(got), want)
	}
	for i, h := range want {
		if got[i].Headline != h {
			t.Errorf("item %d = %q, want %q", i, got[i].Headline, h)
		}
	}

	if got := FilterNewsNearDate(news, "not-a-date"); len(got) != len(news) {
		t.Errorf("unparseable date filtered %d items away, want the news left untouched", len(news)-len(got))
	}
	if got := FilterNewsNearDate(news, "2026-01-05"); len(got) != 0 {
		t.Errorf("kept %v for an unrelated date, want none (the prompt's 'cause unknown' branch)", headlines(got))
	}
}

func TestDedupeHeadlines(t *testing.T) {
	news := []data.NewsItem{
		// The TW case: one wire story, three outlets, Google News' " - 媒體"
		// tag being the only difference.
		{Source: "經濟日報", Headline: "台積電法說會釋出樂觀展望 外資調高目標價 - 經濟日報"},
		{Source: "工商時報", Headline: "台積電法說會釋出樂觀展望 外資調高目標價 - 工商時報"},
		{Source: "鉅亨網", Headline: "台積電法說會釋出樂觀展望，外資調高目標價"},
		// Same company, genuinely different story — must survive.
		{Source: "中央社", Headline: "台積電宣布高雄二廠動工時程"},
		{Source: "Reuters", Headline: "TSMC lifts capex guidance for 2026"},
		{Source: "Bloomberg", Headline: "TSMC lifts capex guidance for 2026 - Bloomberg"},
	}

	got := DedupeHeadlines(news)
	want := []string{"經濟日報", "中央社", "Reuters"}
	if len(got) != len(want) {
		t.Fatalf("kept %d items %v, want %v", len(got), headlines(got), want)
	}
	for i, src := range want {
		if got[i].Source != src {
			t.Errorf("item %d from %q, want the first copy from %q", i, got[i].Source, src)
		}
	}

	if got := DedupeHeadlines(nil); got != nil {
		t.Errorf("DedupeHeadlines(nil) = %v, want nil", got)
	}
}

func headlines(news []data.NewsItem) []string {
	out := make([]string, len(news))
	for i, n := range news {
		out[i] = n.Headline
	}
	return out
}

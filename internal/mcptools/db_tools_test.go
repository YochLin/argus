package mcptools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
)

// newTestDB opens a fresh, real sqlite file (modernc.org/sqlite is pure Go,
// no cgo/mocking needed) — mirrors internal/db's own test helper.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.New(path)
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestGetWatchlist(t *testing.T) {
	d := newTestDB(t)
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddTicker("MSFT"); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_watchlist", map[string]any{})
	if isError {
		t.Fatalf("get_watchlist returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") || !strings.Contains(text, "MSFT") {
		t.Errorf("get_watchlist result missing tickers, got: %s", text)
	}
}

func TestGetWatchlistEmpty(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "get_watchlist", map[string]any{})
	if !isError {
		t.Fatal("get_watchlist with an empty watchlist should return IsError, not a silent empty success")
	}
}

func TestGetPortfolio(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.RecordBuy("AAPL", 10, 150, 0, "2026-01-01"); err != nil {
		t.Fatal(err)
	}

	fp := &fakeProvider{quote: &data.Quote{Ticker: "AAPL", Price: 200}}
	ts := &toolset{lang: i18n.EN, provider: fp, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_portfolio", map[string]any{})
	if isError {
		t.Fatalf("get_portfolio returned an error result: %s", text)
	}
	for _, want := range []string{"AAPL", "150.00", "200.00", "2000.00"} {
		if !strings.Contains(text, want) {
			t.Errorf("get_portfolio result missing %q, got:\n%s", want, text)
		}
	}
}

func TestGetPortfolioEmpty(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "get_portfolio", map[string]any{})
	if !isError {
		t.Fatal("get_portfolio with no positions should return IsError, not a silent empty success")
	}
}

func TestGetRecommendationStats(t *testing.T) {
	d := newTestDB(t)
	today := time.Now().In(cst).Format("2006-01-02")
	if err := d.SaveRecommendations(today, []db.Recommendation{
		{Date: today, Ticker: "AAPL", Action: "BUY", Reason: "test", Price: 150, Source: "watchlist"},
	}); err != nil {
		t.Fatal(err)
	}

	fp := &fakeProvider{quote: &data.Quote{Ticker: "AAPL", Price: 165}}
	ts := &toolset{lang: i18n.EN, provider: fp, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_recommendation_stats", map[string]any{"days": 90})
	if isError {
		t.Fatalf("get_recommendation_stats returned an error result: %s", text)
	}
	for _, want := range []string{"AAPL", "BUY", "150.00", "165.00", "Hit rate: 1/1"} {
		if !strings.Contains(text, want) {
			t.Errorf("get_recommendation_stats result missing %q, got:\n%s", want, text)
		}
	}
}

func TestGetRecommendationStatsEmpty(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "get_recommendation_stats", map[string]any{})
	if !isError {
		t.Fatal("get_recommendation_stats with no recommendations in window should return IsError")
	}
}

func TestGetUniverseSummary(t *testing.T) {
	d := newTestDB(t)
	if err := d.AddUniverseTicker("ZZZZ", "manual"); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_universe_summary", map[string]any{})
	if isError {
		t.Fatalf("get_universe_summary returned an error result: %s", text)
	}
	if !strings.Contains(text, "manual") {
		t.Errorf("get_universe_summary result missing manual source breakdown, got:\n%s", text)
	}
}

func TestDBToolsNotRegisteredWithoutDB(t *testing.T) {
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}}
	session := connectTool(t, ts)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	names := make(map[string]bool)
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, notWant := range []string{"get_watchlist", "get_portfolio", "get_recommendation_stats", "get_universe_summary"} {
		if names[notWant] {
			t.Errorf("tools/list should not advertise %q when db is nil", notWant)
		}
	}
}

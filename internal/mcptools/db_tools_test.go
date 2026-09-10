package mcptools

import (
	"context"
	"fmt"
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
	for _, want := range []string{"AAPL", "$150", "$200", "$2,000"} {
		if !strings.Contains(text, want) {
			t.Errorf("get_portfolio result missing %q, got:\n%s", want, text)
		}
	}
}

// TestGetPortfolioMarketSubtotals is Phase 6's mcptools-side counterpart to
// internal/bot's TestBuildDashboard_MarketFilter — get_portfolio must never
// mix a TW position's NT$ value into the USD subtotal (or vice versa), see
// docs/phase-6-tw-market.md §4.5.
func TestGetPortfolioMarketSubtotals(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.RecordBuy("AAPL", 10, 150, 0, "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RecordBuy("2330", 1000, 900, 0, "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.RecordSell("AAPL", 2, 180, 0, "2026-01-05"); err != nil {
		t.Fatal(err)
	}

	fp := &fakeProvider{quotes: map[string]*data.Quote{
		"AAPL": {Ticker: "AAPL", Price: 200},
		"2330": {Ticker: "2330", Price: 950},
	}}
	ts := &toolset{lang: i18n.EN, provider: fp, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_portfolio", map[string]any{})
	if isError {
		t.Fatalf("get_portfolio returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") || !strings.Contains(text, "2330") {
		t.Fatalf("get_portfolio result missing a ticker, got:\n%s", text)
	}
	// AAPL's realized P&L ((180-150)*2=60) must land in the USD section, not
	// bleed into 2330's TWD subtotal.
	usIdx := strings.Index(text, "AAPL")
	twIdx := strings.Index(text, "2330")
	if usIdx > twIdx {
		t.Fatalf("expected US section (AAPL) before TW section (2330), got:\n%s", text)
	}
	usSection := text[:twIdx]
	twSection := text[twIdx:]
	if !strings.Contains(usSection, "60.00") {
		t.Errorf("US section missing AAPL's realized P&L 60.00, got:\n%s", usSection)
	}
	if strings.Contains(twSection, "60.00") {
		t.Errorf("TW section leaked AAPL's realized P&L, got:\n%s", twSection)
	}
	if !strings.Contains(twSection, "NT$950") {
		t.Errorf("TW section missing 2330's price NT$950, got:\n%s", twSection)
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

func TestGetPortfolioQuoteFailureDegradesPosition(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.RecordBuy("AAPL", 10, 150, 0, "2026-01-01"); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{
		lang:     i18n.EN,
		provider: &fakeProvider{quoteErr: fmt.Errorf("quote unavailable")},
		history:  &fakeHistory{},
		db:       d,
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_portfolio", map[string]any{})
	if isError {
		t.Fatalf("get_portfolio should degrade a quote failure, got error: %s", text)
	}
	if !strings.Contains(text, "AAPL") || !strings.Contains(text, "quote unavailable") {
		t.Errorf("get_portfolio should retain the ticker with an unavailable quote, got:\n%s", text)
	}
}

// testCall mirrors internal/db/options_test.go's fixture symbol.
const testCall = "AAPL260918C00320000"

func TestGetPortfolioIncludesOptionPositions(t *testing.T) {
	d := newTestDB(t)
	if _, _, err := d.RecordOption(testCall, "BUY", 2, 5.40, 0, "2026-08-01"); err != nil {
		t.Fatal(err)
	}

	expiry := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	ts := &toolset{
		lang:     i18n.EN,
		provider: &fakeProvider{quote: &data.Quote{Ticker: "AAPL", Price: 300}},
		history:  &fakeHistory{},
		db:       d,
		optionChain: fakeOptionChain{
			expirations: []time.Time{expiry},
			quotes: map[time.Time][]data.OptionQuote{
				expiry: {{ContractSymbol: testCall, Bid: 8.0, Ask: 8.4, Expiration: expiry}},
			},
		},
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_portfolio", map[string]any{})
	if isError {
		t.Fatalf("get_portfolio returned an error result: %s", text)
	}
	if !strings.Contains(text, testCall) || !strings.Contains(text, "8.20") {
		t.Errorf("get_portfolio missing option position/mark, got:\n%s", text)
	}
}

// TestGetPortfolioOptionsOnlyNotEmpty covers a user who holds only option
// contracts, no stock positions — get_portfolio must not report "empty".
func TestGetPortfolioOptionsOnlyNotEmpty(t *testing.T) {
	d := newTestDB(t)
	if _, _, err := d.RecordOption(testCall, "BUY", 1, 5.40, 0, "2026-08-01"); err != nil {
		t.Fatal(err)
	}

	expiry := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	ts := &toolset{
		lang:     i18n.EN,
		provider: &fakeProvider{},
		history:  &fakeHistory{},
		db:       d,
		optionChain: fakeOptionChain{
			expirations: []time.Time{expiry},
			quotes: map[time.Time][]data.OptionQuote{
				expiry: {{ContractSymbol: testCall, Bid: 8.0, Ask: 8.4, Expiration: expiry}},
			},
		},
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_portfolio", map[string]any{})
	if isError {
		t.Fatalf("get_portfolio with only an option position should not report empty, got error: %s", text)
	}
	if !strings.Contains(text, testCall) {
		t.Errorf("get_portfolio missing the option position, got:\n%s", text)
	}
}

// TestGetPortfolioOptionsSectionSkippedWithoutChainProvider covers the
// nil-degrade case — an option position exists but ts.optionChain is nil
// (no data.OptionChainProvider wired in), same as get_option_chain simply
// not being registered.
func TestGetPortfolioOptionsSectionSkippedWithoutChainProvider(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.RecordBuy("AAPL", 10, 150, 0, "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.RecordOption(testCall, "BUY", 1, 5.40, 0, "2026-08-01"); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{
		lang:     i18n.EN,
		provider: &fakeProvider{quote: &data.Quote{Ticker: "AAPL", Price: 200}},
		history:  &fakeHistory{},
		db:       d,
		// optionChain left nil.
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_portfolio", map[string]any{})
	if isError {
		t.Fatalf("get_portfolio returned an error result: %s", text)
	}
	if strings.Contains(text, testCall) {
		t.Errorf("get_portfolio should omit the option position without a chain provider, got:\n%s", text)
	}
}

func TestGetThesis(t *testing.T) {
	d := newTestDB(t)
	if err := d.SetThesis("AAPL", "long-term compounder, hold through volatility"); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_thesis", map[string]any{"ticker": "aapl"})
	if isError {
		t.Fatalf("get_thesis returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") || !strings.Contains(text, "long-term compounder") {
		t.Errorf("get_thesis result missing expected content, got:\n%s", text)
	}
}

func TestGetThesisNotSet(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "get_thesis", map[string]any{"ticker": "AAPL"})
	if !isError {
		t.Fatal("get_thesis for a ticker with no recorded thesis should return IsError")
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
	for _, want := range []string{"AAPL", "BUY", "$150", "$165", "Hit rate: 1/1"} {
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

func TestGetRecentRecommendations(t *testing.T) {
	d := newTestDB(t)
	today := time.Now().In(cst).Format("2006-01-02")
	if err := d.SaveRecommendations(today, []db.Recommendation{
		{Date: today, Ticker: "AAPL", Action: "BUY", Reason: "strong momentum", Price: 150, Source: "watchlist"},
		{Date: today, Ticker: "NVDA", Action: "HOLD", Reason: "wait for earnings", Price: 900, Source: "movers"},
	}); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_recent_recommendations", map[string]any{"days": 90})
	if isError {
		t.Fatalf("get_recent_recommendations returned an error result: %s", text)
	}
	for _, want := range []string{"AAPL", "BUY", "150.00", "strong momentum", "NVDA", "HOLD", "wait for earnings", "movers"} {
		if !strings.Contains(text, want) {
			t.Errorf("get_recent_recommendations result missing %q, got:\n%s", want, text)
		}
	}
}

func TestGetRecentRecommendationsTickerFilter(t *testing.T) {
	d := newTestDB(t)
	today := time.Now().In(cst).Format("2006-01-02")
	if err := d.SaveRecommendations(today, []db.Recommendation{
		{Date: today, Ticker: "AAPL", Action: "BUY", Reason: "strong momentum", Price: 150, Source: "watchlist"},
		{Date: today, Ticker: "NVDA", Action: "HOLD", Reason: "wait for earnings", Price: 900, Source: "movers"},
	}); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_recent_recommendations", map[string]any{"days": 90, "ticker": "nvda"})
	if isError {
		t.Fatalf("get_recent_recommendations returned an error result: %s", text)
	}
	if !strings.Contains(text, "NVDA") {
		t.Errorf("filtered result missing NVDA, got:\n%s", text)
	}
	if strings.Contains(text, "AAPL") {
		t.Errorf("filtered result should not include AAPL, got:\n%s", text)
	}

	_, isError = callText(t, session, "get_recent_recommendations", map[string]any{"days": 90, "ticker": "TSLA"})
	if !isError {
		t.Fatal("get_recent_recommendations with a ticker that has no rows should return IsError")
	}
}

func TestGetRecentRecommendationsEmpty(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: d}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "get_recent_recommendations", map[string]any{})
	if !isError {
		t.Fatal("get_recent_recommendations with no recommendations in window should return IsError")
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
	for _, notWant := range []string{"get_watchlist", "get_portfolio", "get_recommendation_stats", "get_recent_recommendations", "get_universe_summary", "get_thesis"} {
		if names[notWant] {
			t.Errorf("tools/list should not advertise %q when db is nil", notWant)
		}
	}
}

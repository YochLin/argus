package mcptools

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/data"
	"argus/internal/i18n"
)

// fakeProvider is a minimal data.Provider double, mirroring
// internal/data/provider_test.go's fakeProvider. quoteCalls counts
// GetQuote invocations (atomically, since MCP call handling isn't
// guaranteed single-goroutine) so cache-hit tests can assert the provider
// was skipped rather than just that the result matched.
type fakeProvider struct {
	quote      *data.Quote
	quoteErr   error
	quoteCalls int32
	news       []data.NewsItem
	newsErr    error
	movers     []string
	moversErr  error
	// quotes, when non-nil, overrides quote for the tickers it contains —
	// used by tests needing distinct per-ticker quotes (Phase 6's
	// get_portfolio market-subtotal test needs a US and a TW ticker priced
	// differently in the same call).
	quotes map[string]*data.Quote
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) GetQuote(ticker string) (*data.Quote, error) {
	atomic.AddInt32(&f.quoteCalls, 1)
	if q, ok := f.quotes[ticker]; ok {
		return q, nil
	}
	return f.quote, f.quoteErr
}
func (f *fakeProvider) GetNews(string, int) ([]data.NewsItem, error) { return f.news, f.newsErr }
func (f *fakeProvider) GetMarketMovers() ([]string, error)           { return f.movers, f.moversErr }

type fakeHistory struct {
	candles []data.Candle
	err     error
	// byTicker, when non-nil, overrides candles/err for the tickers it
	// contains — used by tests that need e.g. SPY's history to differ from
	// the ticker under test (get_technicals' RS63 line).
	byTicker map[string][]data.Candle
}

func (f *fakeHistory) GetHistory(ticker, _ string) ([]data.Candle, error) {
	if f.byTicker != nil {
		if c, ok := f.byTicker[ticker]; ok {
			return c, nil
		}
	}
	return f.candles, f.err
}

type fakeFundamentals struct {
	fd      *data.Fundamentals
	fdErr   error
	stmt    *data.FinancialStatement
	stmtErr error
}

func (f *fakeFundamentals) GetFundamentals(string) (*data.Fundamentals, error) { return f.fd, f.fdErr }
func (f *fakeFundamentals) GetFinancialStatements(string, string) (*data.FinancialStatement, error) {
	return f.stmt, f.stmtErr
}

type fakeEarnings struct {
	events map[string]data.EarningsEvent
	err    error
}

func (f *fakeEarnings) GetUpcomingEarnings([]string, int) (map[string]data.EarningsEvent, error) {
	return f.events, f.err
}

// connectTool builds a server with the given toolset and connects an
// in-memory client to it, mirroring the SDK's own example test pattern
// (mcp.NewInMemoryTransports) rather than going through stdio.
func connectTool(t *testing.T, ts *toolset) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "argus-test", Version: "0.0.0"}, nil)
	registerTools(server, ts)

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callText(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (text string, isError bool) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s) transport error = %v", name, err)
	}
	if len(res.Content) == 0 {
		return "", res.IsError
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("CallTool(%s) content[0] is not TextContent: %#v", name, res.Content[0])
	}
	return tc.Text, res.IsError
}

func TestGetQuote(t *testing.T) {
	ts := &toolset{
		lang: i18n.EN,
		provider: &fakeProvider{quote: &data.Quote{
			Ticker: "AAPL", Price: 200, ChangePercent: 1.5,
			Open: 198, High: 201, Low: 197, Volume: 1000, PrevClose: 197,
			Timestamp: time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC),
		}},
		history: &fakeHistory{},
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_quote", map[string]any{"ticker": "aapl"})
	if isError {
		t.Fatalf("get_quote returned an error result: %s", text)
	}
	for _, want := range []string{"AAPL", "200.00", "1.50%", "198.00", "201.00", "197.00"} {
		if !strings.Contains(text, want) {
			t.Errorf("get_quote result missing %q, got:\n%s", want, text)
		}
	}
}

func TestGetQuoteError(t *testing.T) {
	ts := &toolset{
		lang:     i18n.EN,
		provider: &fakeProvider{quoteErr: errors.New("yahoo: no data for BADTICKER")},
		history:  &fakeHistory{},
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_quote", map[string]any{"ticker": "BADTICKER"})
	if !isError {
		t.Fatalf("get_quote with a failing provider should return IsError, got text: %s", text)
	}
	if strings.Contains(text, "yahoo:") {
		t.Errorf("get_quote error text leaked the raw provider error instead of the i18n message: %s", text)
	}
	if !strings.Contains(text, "BADTICKER") {
		t.Errorf("get_quote error text should name the ticker, got: %s", text)
	}
}

func TestGetHistory(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 7, d, 0, 0, 0, 0, time.UTC) }
	ts := &toolset{
		provider: &fakeProvider{},
		history: &fakeHistory{candles: []data.Candle{
			{Date: day(1), Open: 99.5, High: 101, Low: 99, Close: 100, Volume: 1200},
			{Date: day(2), Open: 100.25, High: 102, Low: 100, Close: 101.5, Volume: 1500},
			{Date: day(3), Open: 101, High: 101.75, Low: 98.75, Close: 99.25, Volume: 1800},
		}},
		lang: i18n.ZH,
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_history", map[string]any{"ticker": "MSFT"})
	if isError {
		t.Fatalf("get_history returned an error result: %s", text)
	}
	for _, want := range []string{"MSFT", "3", "2026-07-03", "101.00", "101.75", "98.75", "99.25", "1800"} {
		if !strings.Contains(text, want) {
			t.Errorf("get_history result missing %q, got:\n%s", want, text)
		}
	}
}

func TestGetNewsEmptyIsError(t *testing.T) {
	ts := &toolset{
		provider: &fakeProvider{news: nil},
		history:  &fakeHistory{},
		lang:     i18n.EN,
	}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "get_news", map[string]any{"ticker": "TSLA"})
	if !isError {
		t.Fatal("get_news with zero results should return IsError, not a silent empty success")
	}
}

func TestGetMarketMovers(t *testing.T) {
	ts := &toolset{
		provider: &fakeProvider{movers: []string{"AAPL", "MSFT"}},
		history:  &fakeHistory{},
		lang:     i18n.EN,
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_market_movers", map[string]any{})
	if isError {
		t.Fatalf("get_market_movers returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") || !strings.Contains(text, "MSFT") {
		t.Errorf("get_market_movers result missing tickers, got: %s", text)
	}
}

func TestFundamentalsToolsNotRegisteredWithoutProvider(t *testing.T) {
	ts := &toolset{
		provider: &fakeProvider{},
		history:  &fakeHistory{},
		lang:     i18n.EN,
		// fundamentals and earnings left nil, as when FINNHUB_API_KEY isn't set.
	}
	session := connectTool(t, ts)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	names := make(map[string]bool)
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"get_quote", "get_history", "get_news", "get_market_movers"} {
		if !names[want] {
			t.Errorf("tools/list missing always-on tool %q", want)
		}
	}
	for _, notWant := range []string{"get_fundamentals", "get_financial_statements", "get_upcoming_earnings"} {
		if names[notWant] {
			t.Errorf("tools/list should not advertise %q when its provider is nil", notWant)
		}
	}
}

func TestGetFundamentals(t *testing.T) {
	ts := &toolset{
		provider: &fakeProvider{},
		history:  &fakeHistory{},
		lang:     i18n.EN,
		fundamentals: &fakeFundamentals{fd: &data.Fundamentals{
			Ticker: "AAPL", PE: 30.5, ROE: 45.2,
		}},
	}
	session := connectTool(t, ts)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	found := false
	for _, tool := range res.Tools {
		if tool.Name == "get_fundamentals" {
			found = true
		}
	}
	if !found {
		t.Fatal("get_fundamentals should be registered when fundamentals provider is non-nil")
	}

	text, isError := callText(t, session, "get_fundamentals", map[string]any{"ticker": "aapl"})
	if isError {
		t.Fatalf("get_fundamentals returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") || !strings.Contains(text, "30.5") || !strings.Contains(text, "45.2") {
		t.Errorf("get_fundamentals result missing expected fields, got:\n%s", text)
	}
}

func TestGetUpcomingEarnings(t *testing.T) {
	ts := &toolset{
		provider: &fakeProvider{},
		history:  &fakeHistory{},
		lang:     i18n.EN,
		earnings: &fakeEarnings{events: map[string]data.EarningsEvent{
			"AAPL": {Ticker: "AAPL", Date: "2026-01-15", Hour: "amc"},
		}},
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_upcoming_earnings", map[string]any{"tickers": []string{"AAPL", "MSFT"}})
	if isError {
		t.Fatalf("get_upcoming_earnings returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") || !strings.Contains(text, "2026-01-15") {
		t.Errorf("get_upcoming_earnings result missing expected fields, got:\n%s", text)
	}
	if strings.Contains(text, "MSFT") {
		t.Errorf("get_upcoming_earnings should omit tickers with no scheduled earnings, got:\n%s", text)
	}
}

func TestGetQuoteCacheHitSkipsProvider(t *testing.T) {
	fp := &fakeProvider{quote: &data.Quote{
		Ticker: "AAPL", Price: 200, ChangePercent: 1.5,
		Timestamp: time.Date(2026, 1, 2, 21, 0, 0, 0, time.UTC),
	}}
	ts := &toolset{
		lang:     i18n.EN,
		provider: fp,
		history:  &fakeHistory{},
		cache:    newTTLCache(),
		limiter:  newTokenBucket(10, 10),
	}
	session := connectTool(t, ts)

	for i := 0; i < 3; i++ {
		text, isError := callText(t, session, "get_quote", map[string]any{"ticker": "AAPL"})
		if isError {
			t.Fatalf("get_quote call #%d returned an error result: %s", i+1, text)
		}
	}

	if calls := atomic.LoadInt32(&fp.quoteCalls); calls != 1 {
		t.Errorf("expected the provider to be hit once and the rest served from cache, got %d calls", calls)
	}
}

func TestGetQuoteDistinctTickersBothHitProvider(t *testing.T) {
	fp := &fakeProvider{quote: &data.Quote{Ticker: "AAPL", Price: 200}}
	ts := &toolset{
		lang:     i18n.EN,
		provider: fp,
		history:  &fakeHistory{},
		cache:    newTTLCache(),
		limiter:  newTokenBucket(10, 10),
	}
	session := connectTool(t, ts)

	callText(t, session, "get_quote", map[string]any{"ticker": "AAPL"})
	callText(t, session, "get_quote", map[string]any{"ticker": "MSFT"})

	if calls := atomic.LoadInt32(&fp.quoteCalls); calls != 2 {
		t.Errorf("distinct tickers should each be a separate cache key and both hit the provider, got %d calls", calls)
	}
}

func TestGetQuoteFailedCallIsNotCached(t *testing.T) {
	fp := &fakeProvider{quoteErr: errors.New("yahoo: no data for AAPL")}
	ts := &toolset{
		lang:     i18n.EN,
		provider: fp,
		history:  &fakeHistory{},
		cache:    newTTLCache(),
		limiter:  newTokenBucket(10, 10),
	}
	session := connectTool(t, ts)

	callText(t, session, "get_quote", map[string]any{"ticker": "AAPL"})
	callText(t, session, "get_quote", map[string]any{"ticker": "AAPL"})

	if calls := atomic.LoadInt32(&fp.quoteCalls); calls != 2 {
		t.Errorf("a failed call should not be cached, so a retry should hit the provider again, got %d calls", calls)
	}
}

func TestGetQuoteRateLimiterThrottlesCacheMisses(t *testing.T) {
	fp := &fakeProvider{quote: &data.Quote{Ticker: "AAPL", Price: 200}}
	ts := &toolset{
		lang:     i18n.EN,
		provider: fp,
		history:  &fakeHistory{},
		cache:    newTTLCache(),
		limiter:  newTokenBucket(1, 50), // 1 burst, refills every 20ms
	}
	session := connectTool(t, ts)

	start := time.Now()
	// Two distinct tickers, both cache misses, so the second must wait on
	// the rate limiter rather than hit the provider immediately.
	callText(t, session, "get_quote", map[string]any{"ticker": "AAPL"})
	callText(t, session, "get_quote", map[string]any{"ticker": "MSFT"})
	elapsed := time.Since(start)

	if elapsed < 10*time.Millisecond {
		t.Errorf("second cache-miss call should have been throttled by the rate limiter, took only %v", elapsed)
	}
}

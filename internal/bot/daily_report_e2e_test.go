package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
)

// fakeE2EProvider is a minimal data.Provider + data.HistoryProvider stub for
// the daily-report E2E test: AAPL resolves to a real quote so the
// recommendation pipeline has something to render and save; everything else
// (news, movers, history) degrades exactly like a live API error would,
// which every call site along RunDailyReport's path already handles
// gracefully (see pipeline.go's fetchStockData/computeTechnicals).
type fakeE2EProvider struct{}

func (fakeE2EProvider) Name() string { return "fake" }

func (fakeE2EProvider) GetQuote(ticker string) (*data.Quote, error) {
	if ticker != "AAPL" {
		return nil, fmt.Errorf("fake: no quote for %s", ticker)
	}
	return &data.Quote{
		Ticker:        "AAPL",
		Price:         200.0,
		Open:          198.0,
		High:          201.0,
		Low:           197.0,
		PrevClose:     197.5,
		ChangePercent: 1.27,
		Timestamp:     time.Now(),
	}, nil
}

func (fakeE2EProvider) GetNews(ticker string, limit int) ([]data.NewsItem, error) {
	return nil, nil
}

func (fakeE2EProvider) GetMarketMovers() ([]string, error) { return nil, nil }

func (fakeE2EProvider) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	return nil, errors.New("fake: no history")
}

// fakeE2ELLMProvider is an llm.Provider stub that always returns the same
// canned recommendation reply, standing in for a real claude-agent-acp
// subprocess so RunDailyReport's E2E test can run offline in CI.
type fakeE2ELLMProvider struct{}

func (fakeE2ELLMProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	return "[TICKER: AAPL]\nAction: BUY\nReason: Strong momentum and a recent earnings beat.\n", nil
}

func (fakeE2ELLMProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (llm.ChatSession, error) {
	return nil, errors.New("fake: chat not supported")
}

// capturedTelegramCall is one sendMessage call the fake Telegram server
// received.
type capturedTelegramCall struct {
	text string
}

// newFakeTelegramServer starts an httptest server answering exactly the
// tgbotapi calls RunDailyReport's path makes: getMe (called once by
// tgbotapi.NewBotAPIWithAPIEndpoint during construction) and sendMessage
// (Bot.Send). Every sendMessage's "text" form value is captured for the
// test to assert against.
func newFakeTelegramServer(t *testing.T) (*httptest.Server, func() []capturedTelegramCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []capturedTelegramCall

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "getMe":
			w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"test","username":"test_bot"}}`))
		case "sendMessage":
			mu.Lock()
			calls = append(calls, capturedTelegramCall{text: r.FormValue("text")})
			mu.Unlock()
			w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1}}}`))
		default:
			w.Write([]byte(`{"ok":true,"result":true}`))
		}
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	getCalls := func() []capturedTelegramCall {
		mu.Lock()
		defer mu.Unlock()
		out := make([]capturedTelegramCall, len(calls))
		copy(out, calls)
		return out
	}
	return server, getCalls
}

// TestRunDailyReportE2E drives RunDailyReport end to end against fake
// data/LLM providers and a fake Telegram endpoint (no real network calls,
// no real claude-agent-acp subprocess) and asserts on the outward-facing
// contract: a Telegram message mentioning the recommendation, and a
// matching row persisted to the recommendations table. See PLAN.md's "UX
// 與可靠性優化" section for why this test exists — RunDailyReport had no
// integration coverage beyond unit tests of its pure helpers.
func TestRunDailyReportE2E(t *testing.T) {
	server, getCalls := newFakeTelegramServer(t)

	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	b, err := New(Config{
		Token:       "test-token",
		ChatID:      12345,
		DB:          d,
		Provider:    fakeE2EProvider{},
		History:     fakeE2EProvider{},
		LLM:         llm.NewClientWithProvider(fakeE2ELLMProvider{}, "", "", "", i18n.EN),
		Lang:        i18n.EN,
		APIEndpoint: server.URL + "/bot%s/%s",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// Pin the market.IsTradingDay guard to a known ordinary Wednesday
	// trading day, independent of whatever real date the test happens to
	// run on (see Bot.now's doc comment).
	b.now = func() time.Time { return time.Date(2026, time.March, 4, 23, 30, 0, 0, cst) }

	b.RunDailyReport(context.Background())

	calls := getCalls()
	if len(calls) == 0 {
		t.Fatal("RunDailyReport sent no Telegram messages")
	}
	var sawRecommendation bool
	for _, c := range calls {
		if strings.Contains(c.text, "AAPL") && strings.Contains(strings.ToUpper(c.text), "BUY") {
			sawRecommendation = true
		}
	}
	if !sawRecommendation {
		t.Errorf("no sendMessage payload mentioned AAPL/BUY; got %d messages: %+v", len(calls), calls)
	}

	recs, err := d.GetRecommendationsSince("2000-01-01")
	if err != nil {
		t.Fatalf("GetRecommendationsSince() error = %v", err)
	}
	var found *db.Recommendation
	for i := range recs {
		if recs[i].Ticker == "AAPL" {
			found = &recs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no AAPL recommendation was saved to the DB; got %+v", recs)
	}
	if found.Action != "BUY" {
		t.Errorf("saved recommendation action = %q, want BUY", found.Action)
	}
	if found.Price != 200.0 {
		t.Errorf("saved recommendation price = %v, want 200.0", found.Price)
	}
	if found.Source != "watchlist" {
		t.Errorf("saved recommendation source = %q, want watchlist", found.Source)
	}
}

// TestRunDailyReportE2E_MarketHoliday exercises the opposite branch: on a
// non-trading day, RunDailyReport must send exactly the light "market
// closed" notice and neither call the LLM nor touch the DB.
func TestRunDailyReportE2E_MarketHoliday(t *testing.T) {
	server, getCalls := newFakeTelegramServer(t)

	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	b, err := New(Config{
		Token:       "test-token",
		ChatID:      12345,
		DB:          d,
		Provider:    fakeE2EProvider{},
		History:     fakeE2EProvider{},
		LLM:         llm.NewClientWithProvider(fakeE2ELLMProvider{}, "", "", "", i18n.EN),
		Lang:        i18n.EN,
		APIEndpoint: server.URL + "/bot%s/%s",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// 2026-07-04 is a Saturday (Independence Day observed the preceding
	// Friday, but either way this date is not a trading day).
	b.now = func() time.Time { return time.Date(2026, time.July, 4, 23, 30, 0, 0, cst) }

	b.RunDailyReport(context.Background())

	calls := getCalls()
	if len(calls) != 1 {
		t.Fatalf("RunDailyReport sent %d messages on a market holiday, want exactly 1: %+v", len(calls), calls)
	}
	if !strings.Contains(calls[0].text, i18n.T(i18n.EN, i18n.KeyDailyReportMarketClosed)) {
		t.Errorf("message = %q, want the market-closed notice", calls[0].text)
	}

	recs, err := d.GetRecommendationsSince("2000-01-01")
	if err != nil {
		t.Fatalf("GetRecommendationsSince() error = %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected no recommendations saved on a market holiday, got %+v", recs)
	}
}

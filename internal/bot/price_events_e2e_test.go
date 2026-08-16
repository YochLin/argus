package bot

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/market"
)

// priceEventTestQuotes has five US watchlist tickers: three big movers, one
// just under the overflow cap, and one that doesn't trigger at all — enough
// to exercise priceEventWriteupCap's top-3 ranking plus the overflow branch
// (see recordPriceEvents' doc comment).
var priceEventTestQuotes = map[string]*data.Quote{
	"AAAA": {Ticker: "AAAA", Price: 120, Open: 121, PrevClose: 100, ChangePercent: 20, Timestamp: time.Now()},
	"BBBB": {Ticker: "BBBB", Price: 115, Open: 116, PrevClose: 100, ChangePercent: 15, Timestamp: time.Now()},
	"CCCC": {Ticker: "CCCC", Price: 110, Open: 111, PrevClose: 100, ChangePercent: 10, Timestamp: time.Now()},
	"DDDD": {Ticker: "DDDD", Price: 108, Open: 109, PrevClose: 100, ChangePercent: 8, Timestamp: time.Now()},
	"EEEE": {Ticker: "EEEE", Price: 102, Open: 101, PrevClose: 100, ChangePercent: 2, Timestamp: time.Now()},
}

type priceEventProvider struct{}

func (priceEventProvider) Name() string { return "fake" }
func (priceEventProvider) GetQuote(ticker string) (*data.Quote, error) {
	if q, ok := priceEventTestQuotes[ticker]; ok {
		return q, nil
	}
	return nil, errors.New("fake: no quote")
}
func (priceEventProvider) GetNews(ticker string, limit int) ([]data.NewsItem, error) { return nil, nil }
func (priceEventProvider) GetMarketMovers() ([]string, error)                        { return nil, nil }
func (priceEventProvider) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	return nil, errors.New("fake: no history")
}

// countingE2ELLMProvider counts how many times Prompt is called (so a test
// can assert the cap held and dedup on rerun actually skipped the LLM).
type countingE2ELLMProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingE2ELLMProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return "a plausible explanation of the move", nil
}
func (p *countingE2ELLMProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (llm.ChatSession, error) {
	return nil, errors.New("fake: chat not supported")
}
func (p *countingE2ELLMProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// TestRunClosingSnapshot_PriceEvents is Phase 20's acceptance test §7 item
// 2/3: of five triggering/non-triggering watchlist tickers, only the top
// priceEventWriteupCap (3) get an LLM writeup + push, the rest are recorded
// with no summary plus one overflow notice, and a same-day rerun produces no
// new pushes or LLM calls at all (HasPriceEvent's dedup).
func TestRunClosingSnapshot_PriceEvents(t *testing.T) {
	server, getCalls := newFakeTelegramServer(t)
	llmProvider := &countingE2ELLMProvider{}

	d, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	for ticker := range priceEventTestQuotes {
		if err := d.AddTicker(ticker); err != nil {
			t.Fatalf("AddTicker(%s) error = %v", ticker, err)
		}
	}

	b, err := New(Config{
		Token:       "test-token",
		ChatID:      12345,
		DB:          d,
		Provider:    priceEventProvider{},
		History:     priceEventProvider{},
		LLM:         llm.NewClientWithProvider(llmProvider, "", "", "", i18n.EN),
		Lang:        i18n.EN,
		APIEndpoint: server.URL + "/bot%s/%s",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	b.RunClosingSnapshot(context.Background(), market.US)

	if got := llmProvider.callCount(); got != 3 {
		t.Fatalf("LLM calls after first run = %d, want 3 (priceEventWriteupCap)", got)
	}

	events, err := d.GetRecentPriceEvents(20)
	if err != nil {
		t.Fatalf("GetRecentPriceEvents() error = %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("price_events rows = %d, want 4 (AAAA/BBBB/CCCC/DDDD; EEEE never triggers)", len(events))
	}
	var withSummary, withoutSummary int
	for _, ev := range events {
		if ev.Ticker == "EEEE" {
			t.Errorf("EEEE (2%% move) got a price_events row, want none")
		}
		if ev.Summary == "" {
			withoutSummary++
		} else {
			withSummary++
		}
	}
	if withSummary != 3 || withoutSummary != 1 {
		t.Errorf("with/without summary = %d/%d, want 3/1", withSummary, withoutSummary)
	}

	calls := getCalls()
	var writeupPushes, overflowPushes int
	for _, c := range calls {
		if strings.Contains(c.text, "Price Event") {
			writeupPushes++
		}
		if strings.Contains(c.text, "also triggered a price event") {
			overflowPushes++
		}
	}
	if writeupPushes != 3 {
		t.Errorf("writeup pushes = %d, want 3", writeupPushes)
	}
	if overflowPushes != 1 {
		t.Errorf("overflow pushes = %d, want 1", overflowPushes)
	}

	// Rerun the same day: HasPriceEvent must dedup everything — no new LLM
	// calls, no new Telegram pushes, no new rows.
	b.RunClosingSnapshot(context.Background(), market.US)

	if got := llmProvider.callCount(); got != 3 {
		t.Errorf("LLM calls after rerun = %d, want still 3 (dedup should skip the LLM entirely)", got)
	}
	eventsAfterRerun, err := d.GetRecentPriceEvents(20)
	if err != nil {
		t.Fatalf("GetRecentPriceEvents() error = %v", err)
	}
	if len(eventsAfterRerun) != 4 {
		t.Errorf("price_events rows after rerun = %d, want still 4", len(eventsAfterRerun))
	}
}

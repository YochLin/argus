package bot

import (
	"context"
	"errors"
	"fmt"
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

// dropProvider serves history in which every ticker's last three bars fall,
// each ticker a little further than the last, plus one headline dated on the
// final bar; "QUIET" has no drop and "NONEWS" no headline.
type dropProvider struct {
	last time.Time
}

func (p dropProvider) Name() string { return "fake" }
func (p dropProvider) GetQuote(ticker string) (*data.Quote, error) {
	return nil, errors.New("fake: no quote")
}
func (p dropProvider) GetMarketMovers() ([]string, error) { return nil, nil }
func (p dropProvider) GetNews(ticker string, limit int) ([]data.NewsItem, error) {
	if ticker == "NONEWS" {
		return nil, nil
	}
	return []data.NewsItem{{Headline: ticker + " cuts guidance", Source: "Reuters", PublishedAt: p.last}}, nil
}
func (p dropProvider) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	closes := []float64{100, 100, 100, 100, 100}
	if ticker != "QUIET" {
		depth := float64(ticker[len(ticker)-1]-'0') * 0.5
		closes = []float64{100, 100, 97, 95, 92 - depth}
	}
	out := make([]data.Candle, len(closes))
	for i, c := range closes {
		out[i] = data.Candle{Date: p.last.AddDate(0, 0, i-len(closes)+1), Open: c, High: c + 1, Low: c - 1, Close: c, Volume: 1}
	}
	return out, nil
}

// dropLLM answers one attribution and counts the calls.
type dropLLM struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (p *dropLLM) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.fail {
		return "", errors.New("fake: llm down")
	}
	return "CAUSE: earn\nALSO: flow\nCONFIDENCE: high\nSUMMARY: Guidance was cut.", nil
}
func (p *dropLLM) NewChatSession(ctx context.Context, systemPrompt, model string, scope llm.MCPScope) (llm.ChatSession, error) {
	return nil, errors.New("fake: chat not supported")
}

func newDropBot(t *testing.T, provider *dropLLM, last time.Time) (*Bot, *db.DB) {
	t.Helper()
	server, _ := newFakeTelegramServer(t)
	d, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	p := dropProvider{last: last}
	b, err := New(Config{
		Token: "test-token", ChatID: 12345, DB: d,
		Provider: p, History: p,
		LLM:         llm.NewClientWithProvider(provider, "", "", "", i18n.EN),
		Lang:        i18n.EN,
		APIEndpoint: server.URL + "/bot%s/%s",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return b, d
}

func TestRecordDropCauses_CapsLLMAndKeepsEveryHeadline(t *testing.T) {
	last := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	date := last.Format("2006-01-02")
	provider := &dropLLM{}
	b, d := newDropBot(t, provider, last)

	tickers := []string{"QUIET", "NONEWS"}
	for i := 1; i <= 7; i++ {
		tickers = append(tickers, fmt.Sprintf("TK%d", i))
	}
	b.recordDropCauses(context.Background(), tickers, market.US, date)

	if provider.calls != dropCauseCap {
		t.Fatalf("LLM calls = %d, want %d", provider.calls, dropCauseCap)
	}
	for i := 1; i <= 7; i++ {
		ticker := fmt.Sprintf("TK%d", i)
		rows, err := d.DropCausesFor(ticker)
		if err != nil || len(rows) != 1 {
			t.Fatalf("%s rows = %+v, err = %v", ticker, rows, err)
		}
		r := rows[0]
		if len(r.News) != 1 || !strings.Contains(r.News[0].Headline, ticker) {
			t.Errorf("%s news = %+v", ticker, r.News)
		}
		// Deepest first: TK7..TK3 fell furthest.
		if wantCause := i >= 3; (r.Cause == "earn") != wantCause {
			t.Errorf("%s cause = %q, want attributed=%v", ticker, r.Cause, wantCause)
		}
	}
	for _, ticker := range []string{"QUIET", "NONEWS"} {
		if rows, _ := d.DropCausesFor(ticker); len(rows) != 0 {
			t.Errorf("%s rows = %+v, want none", ticker, rows)
		}
	}

	b.recordDropCauses(context.Background(), tickers, market.US, date)
	if provider.calls != dropCauseCap {
		t.Errorf("rerun made LLM calls: %d", provider.calls)
	}
}

func TestRecordDropCauses_LLMFailureKeepsHeadlines(t *testing.T) {
	last := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	b, d := newDropBot(t, &dropLLM{fail: true}, last)

	b.recordDropCauses(context.Background(), []string{"TK1"}, market.US, last.Format("2006-01-02"))

	rows, err := d.DropCausesFor("TK1")
	if err != nil || len(rows) != 1 || rows[0].Cause != "" || len(rows[0].News) != 1 {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
}

func TestRecordDropCauses_NothingOnADayWithoutACandle(t *testing.T) {
	last := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	provider := &dropLLM{}
	b, d := newDropBot(t, provider, last)

	b.recordDropCauses(context.Background(), []string{"TK1"}, market.US, "2026-03-21")

	if rows, _ := d.DropCausesFor("TK1"); len(rows) != 0 || provider.calls != 0 {
		t.Errorf("rows = %+v, calls = %d", rows, provider.calls)
	}
}

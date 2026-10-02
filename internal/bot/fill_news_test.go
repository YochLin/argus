package bot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
)

type fillNewsProvider struct{ news []data.NewsItem }

func (fillNewsProvider) Name() string                                   { return "fake" }
func (fillNewsProvider) GetQuote(string) (*data.Quote, error)           { return nil, errors.New("no quote") }
func (p fillNewsProvider) GetNews(string, int) ([]data.NewsItem, error) { return p.news, nil }
func (fillNewsProvider) GetMarketMovers() ([]string, error)             { return nil, nil }

// fillNewsLLM answers every prompt with reply, or fails while err is set.
type fillNewsLLM struct {
	mu    sync.Mutex
	reply string
	err   error
	calls int
}

func (p *fillNewsLLM) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.reply, p.err
}
func (p *fillNewsLLM) NewChatSession(ctx context.Context, systemPrompt, model string) (llm.ChatSession, error) {
	return nil, errors.New("fake: chat not supported")
}

func TestCaptureFillNews(t *testing.T) {
	server, _ := newFakeTelegramServer(t)
	d, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	cst := time.FixedZone("CST", 8*3600)
	fill := time.Date(2026, 9, 30, 10, 0, 0, 0, cst)
	date := fill.Format("2006-01-02")
	prov := fillNewsProvider{news: []data.NewsItem{
		{Headline: "NVDA reports record revenue", Source: "Reuters", URL: "https://example.com/a", PublishedAt: fill},
		{Headline: "Analyst lifts NVDA target", Source: "WSJ", URL: "https://example.com/b", PublishedAt: fill.Add(time.Hour)},
		{Headline: "Old story from last month", Source: "Blog", URL: "https://example.com/old", PublishedAt: fill.AddDate(0, -1, 0)},
	}}
	lp := &fillNewsLLM{err: errors.New("acp down")}
	b, err := New(Config{
		Token: "test-token", ChatID: 12345, DB: d, Provider: prov, History: nil,
		LLM: llm.NewClientWithProvider(lp, "", "", "", i18n.EN), Lang: i18n.EN,
		APIEndpoint: server.URL + "/bot%s/%s",
	})
	if err != nil {
		t.Fatal(err)
	}

	// LLM down: headlines and links are still kept, unlabelled; the month-old
	// story falls outside the date window.
	b.captureFillNews("NVDA", date)
	rows, _ := d.FillNewsFor("NVDA", date)
	if len(rows) != 2 {
		t.Fatalf("stored %d rows, want 2 (old story filtered): %+v", len(rows), rows)
	}
	if rows[0].URL != "https://example.com/a" || rows[0].Sentiment != "" {
		t.Fatalf("want link kept and row unlabelled, got %+v", rows[0])
	}

	// The next capture that day (a second fill) labels what is still open.
	lp.mu.Lock()
	lp.err = nil
	lp.reply = "1. sentiment=bull tag=earn major=1\n2. sentiment=bull tag=analyst major=0\n"
	lp.mu.Unlock()
	b.captureFillNews("NVDA", date)
	rows, _ = d.FillNewsFor("NVDA", date)
	if len(rows) != 2 || rows[0].Sentiment != "bull" || rows[0].Tag != "earn" || !rows[0].Major || rows[1].Tag != "analyst" || rows[1].Major {
		t.Fatalf("labels not applied: %+v", rows)
	}

	// A third capture has nothing left to classify, so the LLM is not called.
	lp.mu.Lock()
	before := lp.calls
	lp.mu.Unlock()
	b.captureFillNews("NVDA", date)
	if lp.calls != before {
		t.Errorf("LLM called again with every row already labelled")
	}

	// A fill from long ago (CSV import) gets no snapshot, and a bad date is ignored.
	b.captureFillNews("NVDA", "2026-01-05")
	b.captureFillNews("NVDA", "")
	if r, _ := d.FillNewsFor("NVDA", "2026-01-05"); len(r) != 0 {
		t.Errorf("old fill got news: %+v", r)
	}
	if r, _ := d.FillNewsFor("NVDA", ""); len(r) != 0 {
		t.Errorf("empty date got news: %+v", r)
	}
}

// An empty news result leaves nothing stored, so the log is the only way to
// tell "the provider returned nothing" (Yahoo does that now and then) from
// "headlines came back but none was near the fill".
func TestStoreFillNewsLogsEmptyResult(t *testing.T) {
	d, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	var buf bytes.Buffer
	prev := slog.Default()
	logger.Configure(&buf, slog.LevelInfo)
	t.Cleanup(func() { slog.SetDefault(prev) })

	cst := time.FixedZone("CST", 8*3600)
	fill := time.Date(2026, 9, 30, 10, 0, 0, 0, cst)
	date := fill.Format("2006-01-02")

	(&Bot{db: d, provider: fillNewsProvider{}}).storeFillNews("AAPL", date)
	if !strings.Contains(buf.String(), "provider returned no headlines") {
		t.Errorf("empty fetch not logged: %q", buf.String())
	}

	buf.Reset()
	old := fillNewsProvider{news: []data.NewsItem{{Headline: "Old story", PublishedAt: fill.AddDate(0, -1, 0)}}}
	(&Bot{db: d, provider: old}).storeFillNews("AAPL", date)
	if !strings.Contains(buf.String(), "none within the date window") || strings.Contains(buf.String(), "provider returned no headlines") {
		t.Errorf("filtered-out fetch logged wrong: %q", buf.String())
	}

	buf.Reset()
	fresh := fillNewsProvider{news: []data.NewsItem{{Headline: "Today", PublishedAt: fill}}}
	(&Bot{db: d, provider: fresh}).storeFillNews("AAPL", date)
	if buf.Len() != 0 {
		t.Errorf("a normal capture should log nothing, got %q", buf.String())
	}
}

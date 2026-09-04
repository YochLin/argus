package bot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/market"
)

func candleOn(date string, high, low, close float64) data.Candle {
	d, _ := time.Parse("2006-01-02", date)
	return data.Candle{Date: d, High: high, Low: low, Close: close}
}

func TestPostExitWindow(t *testing.T) {
	exitDate := "2026-08-14"

	// The exit-day candle itself must be excluded.
	sameDay := []data.Candle{candleOn(exitDate, 100, 90, 95)}
	if _, _, _, ok := postExitWindow(sameDay, exitDate, 5); ok {
		t.Error("postExitWindow() with only the exit-day candle = ok, want not ok")
	}

	// Fewer than n post-exit candles.
	four := []data.Candle{
		candleOn("2026-08-15", 101, 99, 100),
		candleOn("2026-08-16", 102, 98, 101),
		candleOn("2026-08-17", 103, 97, 102),
		candleOn("2026-08-18", 104, 96, 103),
	}
	if _, _, _, ok := postExitWindow(four, exitDate, 5); ok {
		t.Error("postExitWindow() with 4 post-exit candles = ok, want not ok (need 5)")
	}

	// Empty input.
	if _, _, _, ok := postExitWindow(nil, exitDate, 5); ok {
		t.Error("postExitWindow(nil) = ok, want not ok")
	}

	// 8 post-exit candles: only the first 5 must count, even though later
	// candles have a higher high / lower low / different close — this is
	// the "same conclusion no matter how late the job actually runs"
	// guarantee (§4.2).
	eight := []data.Candle{
		candleOn("2026-08-15", 105, 95, 100),
		candleOn("2026-08-16", 110, 90, 105), // window high
		candleOn("2026-08-17", 108, 85, 103), // window low
		candleOn("2026-08-18", 107, 92, 104),
		candleOn("2026-08-19", 106, 93, 109), // window close (5th)
		candleOn("2026-08-20", 200, 1, 999),  // must NOT be counted
		candleOn("2026-08-21", 200, 1, 999),
		candleOn("2026-08-22", 200, 1, 999),
	}
	last, high, low, ok := postExitWindow(eight, exitDate, 5)
	if !ok {
		t.Fatal("postExitWindow(8 candles) = not ok, want ok")
	}
	if last != 109 {
		t.Errorf("last = %v, want 109 (5th post-exit candle's close)", last)
	}
	if high != 110 {
		t.Errorf("high = %v, want 110 (within first 5 only)", high)
	}
	if low != 85 {
		t.Errorf("low = %v, want 85 (within first 5 only)", low)
	}
}

func TestFollowupVerdict(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
		m    market.MarketID
		want string
	}{
		{"US just under threshold", 4.99, market.US, "neutral"},
		{"US exactly at threshold", 5.0, market.US, "sold_early"},
		{"US above threshold", 8, market.US, "sold_early"},
		{"US exactly at negative threshold", -5.0, market.US, "good_exit"},
		{"US below negative threshold", -8, market.US, "good_exit"},
		{"US neutral", 1, market.US, "neutral"},
		{"TW just under threshold", 6.99, market.TW, "neutral"},
		{"TW exactly at threshold", 7.0, market.TW, "sold_early"},
		{"TW exactly at negative threshold", -7.0, market.TW, "good_exit"},
		{"TW neutral", -2, market.TW, "neutral"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := followupVerdict(tt.pct, tt.m); got != tt.want {
				t.Errorf("followupVerdict(%v, %v) = %q, want %q", tt.pct, tt.m, got, tt.want)
			}
		})
	}
}

// sellFollowupHistoryProvider is a fake data.HistoryProvider keyed by
// ticker, ignoring rangeParam — followupClosedTrade always asks for "3mo".
type sellFollowupHistoryProvider struct {
	byTicker map[string][]data.Candle
}

func (p sellFollowupHistoryProvider) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	return p.byTicker[ticker], nil
}

// lessonLLMProvider replies with a fixed review that carries a real
// "Lesson:" marker, so parseLesson/saveLesson has something to persist —
// countingE2ELLMProvider (price_events_e2e_test.go) doesn't, since
// ExplainPriceEvent never parses a lesson out of its reply.
type lessonLLMProvider struct{ calls int }

func (p *lessonLLMProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	p.calls++
	return "It kept rising after the exit.\nLesson: Use a trailing stop instead of a fixed target.", nil
}
func (p *lessonLLMProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (llm.ChatSession, error) {
	return nil, errors.New("fake: chat not supported")
}

// TestCheckSellFollowups is Phase 26's acceptance test (docs/phase-26-sell-followup.md
// §7 item 3): a ticker with a closed round trip inside the age window and 5
// trading days of post-exit history gets a follow-up push + a sell_followups
// row + a trade_lessons row, a rerun doesn't duplicate any of it, a ticker
// bought back into after its exit is skipped entirely, and a ticker with
// fewer than 5 post-exit candles is skipped (left for a future retry) rather
// than erroring.
func TestCheckSellFollowups(t *testing.T) {
	server, getCalls := newFakeTelegramServer(t)
	llmProvider := &lessonLLMProvider{}

	d, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })

	const exitDate = "2026-08-14"
	const today = "2026-08-20" // 6 calendar days later, inside [5,30]

	// SOLD: a clean closed round trip with 5 (and then some) trading days
	// of post-exit history, priced to land in the "sold_early" band.
	if _, err := d.RecordBuy("SOLD", 10, 100, 0, "2026-08-01"); err != nil {
		t.Fatalf("RecordBuy(SOLD) error = %v", err)
	}
	if _, _, err := d.RecordSell("SOLD", 10, 110, 0, exitDate); err != nil {
		t.Fatalf("RecordSell(SOLD) error = %v", err)
	}

	// REBUY: closed, then bought back into — must be skipped entirely.
	if _, err := d.RecordBuy("REBUY", 10, 100, 0, "2026-08-01"); err != nil {
		t.Fatalf("RecordBuy(REBUY) error = %v", err)
	}
	if _, _, err := d.RecordSell("REBUY", 10, 110, 0, exitDate); err != nil {
		t.Fatalf("RecordSell(REBUY) error = %v", err)
	}
	if _, err := d.RecordBuy("REBUY", 5, 115, 0, "2026-08-17"); err != nil {
		t.Fatalf("RecordBuy(REBUY) re-entry error = %v", err)
	}

	// THIN: closed, but only 4 trading days of post-exit history so far.
	if _, err := d.RecordBuy("THIN", 10, 100, 0, "2026-08-01"); err != nil {
		t.Fatalf("RecordBuy(THIN) error = %v", err)
	}
	if _, _, err := d.RecordSell("THIN", 10, 110, 0, exitDate); err != nil {
		t.Fatalf("RecordSell(THIN) error = %v", err)
	}

	history := sellFollowupHistoryProvider{byTicker: map[string][]data.Candle{
		"SOLD": {
			candleOn("2026-08-15", 112, 108, 110),
			candleOn("2026-08-16", 115, 109, 112),
			candleOn("2026-08-17", 118, 111, 115),
			candleOn("2026-08-18", 122, 114, 118),
			candleOn("2026-08-19", 122, 108, 120), // 5th trading day close
		},
		"THIN": {
			candleOn("2026-08-15", 112, 108, 110),
			candleOn("2026-08-16", 115, 109, 112),
			candleOn("2026-08-17", 118, 111, 115),
			candleOn("2026-08-18", 122, 114, 118),
		},
	}}

	b, err := New(Config{
		Token: "test-token", ChatID: 12345, DB: d,
		Provider: priceEventProvider{}, History: history,
		LLM:         llm.NewClientWithProvider(llmProvider, "", "", "", i18n.EN),
		Lang:        i18n.EN,
		APIEndpoint: server.URL + "/bot%s/%s",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	b.checkSellFollowups(context.Background(), market.US, today)

	if llmProvider.calls != 1 {
		t.Fatalf("LLM calls = %d, want 1 (only SOLD is due and complete)", llmProvider.calls)
	}

	has, err := d.HasSellFollowup("SOLD", exitDate)
	if err != nil {
		t.Fatalf("HasSellFollowup(SOLD) error = %v", err)
	}
	if !has {
		t.Error("HasSellFollowup(SOLD) = false, want true after a successful run")
	}
	if has, _ := d.HasSellFollowup("REBUY", exitDate); has {
		t.Error("HasSellFollowup(REBUY) = true, want false (bought back in, must be skipped)")
	}
	if has, _ := d.HasSellFollowup("THIN", exitDate); has {
		t.Error("HasSellFollowup(THIN) = true, want false (not enough post-exit history yet)")
	}

	lessons, err := d.GetLessonsForTickers([]string{"SOLD"})
	if err != nil {
		t.Fatalf("GetLessonsForTickers(SOLD) error = %v", err)
	}
	if len(lessons["SOLD"]) != 1 {
		t.Fatalf("trade_lessons rows for SOLD = %d, want 1", len(lessons["SOLD"]))
	}

	var followupPushes int
	for _, c := range getCalls() {
		if strings.Contains(c.text, "Sell Follow-up") {
			followupPushes++
		}
	}
	if followupPushes != 1 {
		t.Errorf("follow-up pushes = %d, want 1", followupPushes)
	}

	// Rerun: SOLD must not fire again (dedup), and THIN still isn't due for
	// a real retry since its fake history didn't grow a 5th candle.
	b.checkSellFollowups(context.Background(), market.US, today)
	if llmProvider.calls != 1 {
		t.Errorf("LLM calls after rerun = %d, want still 1", llmProvider.calls)
	}
}

package mcptools

import (
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
)

// syntheticCandles builds n daily candles with a gentle uptrend and a
// deterministic sawtooth on top, starting at `start` — enough history for
// every indicator formatTechnicals renders (MA200, MACD, Bollinger, etc.)
// to have real, non-sentinel values rather than all skipping out.
func syntheticCandles(n int, start float64) []data.Candle {
	candles := make([]data.Candle, n)
	day := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	price := start
	for i := 0; i < n; i++ {
		price += 0.15
		wiggle := float64(i%5) * 0.3
		open := price - 0.2 + wiggle
		high := price + 1 + wiggle
		low := price - 1
		close := price + wiggle
		candles[i] = data.Candle{
			Date:   day.AddDate(0, 0, i),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: int64(1_000_000 + i*1000),
		}
	}
	return candles
}

func TestGetTechnicals(t *testing.T) {
	ts := &toolset{
		provider: &fakeProvider{},
		history: &fakeHistory{
			byTicker: map[string][]data.Candle{
				"NVDA": syntheticCandles(260, 100),
				"SPY":  syntheticCandles(260, 400),
			},
		},
		lang: i18n.EN,
	}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "get_technicals", map[string]any{"ticker": "nvda"})
	if isError {
		t.Fatalf("get_technicals returned an error result: %s", text)
	}
	for _, want := range []string{"NVDA", "RSI", "MACD", "MA5", "MA20", "MA50", "MA200", "ATR", "Bollinger", "RS63"} {
		if !strings.Contains(text, want) {
			t.Errorf("get_technicals result missing %q, got:\n%s", want, text)
		}
	}
}

func TestGetTechnicalsNoHistoryIsError(t *testing.T) {
	ts := &toolset{
		provider: &fakeProvider{},
		history:  &fakeHistory{candles: nil},
		lang:     i18n.EN,
	}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "get_technicals", map[string]any{"ticker": "ZZZZ"})
	if !isError {
		t.Fatal("get_technicals with no history should return IsError")
	}
}

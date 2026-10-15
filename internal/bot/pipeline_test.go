package bot

import (
	"reflect"
	"testing"

	"argus/internal/i18n"
	"argus/internal/llm"
)

func TestCapScanHitTickers(t *testing.T) {
	if got := capScanHitTickers(nil, maxScanHitFundamentals); got != nil {
		t.Errorf("capScanHitTickers(nil) = %v, want nil", got)
	}
	if got := capScanHitTickers(map[string]string{}, maxScanHitFundamentals); got != nil {
		t.Errorf("capScanHitTickers(empty map) = %v, want nil", got)
	}

	underCap := map[string]string{"AAPL": "x", "MSFT": "y"}
	got := capScanHitTickers(underCap, maxScanHitFundamentals)
	if len(got) != 2 || !got["AAPL"] || !got["MSFT"] {
		t.Errorf("capScanHitTickers(under cap) = %v, want all included", got)
	}

	overCap := map[string]string{
		"AAA": "x", "BBB": "x", "CCC": "x", "DDD": "x", "EEE": "x", "FFF": "x", "GGG": "x",
	}
	got = capScanHitTickers(overCap, 5)
	if len(got) != 5 {
		t.Fatalf("capScanHitTickers(over cap) = %v, want exactly 5", got)
	}
	// Lexical order must pick the first 5 alphabetically, deterministically.
	want := map[string]bool{"AAA": true, "BBB": true, "CCC": true, "DDD": true, "EEE": true}
	for ticker := range want {
		if !got[ticker] {
			t.Errorf("capScanHitTickers(over cap) missing expected %s, got %v", ticker, got)
		}
	}
	if got["FFF"] || got["GGG"] {
		t.Errorf("capScanHitTickers(over cap) = %v, should exclude FFF/GGG past the cap", got)
	}
}

func TestSplitRecsBySource(t *testing.T) {
	recs := []llm.Recommendation{
		{Ticker: "AAPL", Action: "HOLD"},
		{Ticker: "NVDA", Action: "BUY"},
		{Ticker: "TSLA", Action: "BUY"},
		{Ticker: "ZZZZ", Action: "BUY"}, // missing from sources
	}
	sources := map[string]string{
		"AAPL": "watchlist",
		"NVDA": "scan",
		"TSLA": "movers",
	}

	gotWatchlist, gotCandidates := splitRecsBySource(recs, sources)

	wantWatchlist := []llm.Recommendation{{Ticker: "AAPL", Action: "HOLD"}}
	if !reflect.DeepEqual(gotWatchlist, wantWatchlist) {
		t.Errorf("splitRecsBySource() watchlist = %+v, want %+v", gotWatchlist, wantWatchlist)
	}

	wantCandidates := []llm.Recommendation{
		{Ticker: "NVDA", Action: "BUY"},
		{Ticker: "TSLA", Action: "BUY"},
		{Ticker: "ZZZZ", Action: "BUY"},
	}
	if !reflect.DeepEqual(gotCandidates, wantCandidates) {
		t.Errorf("splitRecsBySource() candidates = %+v, want %+v", gotCandidates, wantCandidates)
	}
}

func TestFilterRecsForDisplay(t *testing.T) {
	recs := []llm.Recommendation{
		{Ticker: "AAPL", Action: "HOLD"},
		{Ticker: "MSFT", Action: "HOLD"},
		{Ticker: "NVDA", Action: "BUY"},
		{Ticker: "TSLA", Action: "SELL"},
	}
	held := map[string]bool{"AAPL": true}

	got := filterRecsForDisplay(recs, held)
	want := []llm.Recommendation{
		{Ticker: "AAPL", Action: "HOLD"},
		{Ticker: "NVDA", Action: "BUY"},
		{Ticker: "TSLA", Action: "SELL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterRecsForDisplay() = %+v, want %+v", got, want)
	}
}

func TestFormatRecLine(t *testing.T) {
	t.Run("includes the action separator when Action is set", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "MSFT", Action: "BUY", Reason: "cloud growth."}
		got := formatRecLine(i18n.EN, r, nil, r.Ticker)
		want := "*MSFT* — BUY\ncloud growth.\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})

	t.Run("empty action omits the action separator", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "AAPL", Reason: "no action line."}
		got := formatRecLine(i18n.EN, r, nil, r.Ticker)
		want := "*AAPL*\nno action line.\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})

	t.Run("a sizing line for a ticker in the map is appended after the reason", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "AAPL", Action: "BUY", Reason: "breakout."}
		sizing := map[string]string{"AAPL": "sizing info\n"}
		got := formatRecLine(i18n.EN, r, sizing, r.Ticker)
		want := "*AAPL* — BUY\nbreakout.\nsizing info\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})

	t.Run("a ticker missing from sizing renders no sizing line", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "TSLA", Action: "SELL", Reason: "overextended."}
		got := formatRecLine(i18n.EN, r, map[string]string{"AAPL": "sizing info\n"}, r.Ticker)
		want := "*TSLA* — SELL\noverextended.\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})
}

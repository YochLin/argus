package data

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// twChartServer serves the Yahoo chart-API shape: a bare-zero meta (Yahoo's
// real behavior for a suffix that doesn't match any listing) for any symbol
// in zeroFor, and a populated quote for everything else. hits counts
// requests per symbol so tests can assert the cache actually avoids a
// second network call.
func twChartServer(t *testing.T, zeroFor map[string]bool, hits map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Path[len("/v8/finance/chart/"):]
		hits[symbol]++
		w.Header().Set("Content-Type", "application/json")
		if zeroFor[symbol] {
			fmt.Fprint(w, `{"chart":{"result":[{"meta":{"symbol":"`+symbol+`","regularMarketPrice":0,"chartPreviousClose":0,"regularMarketTime":0},"indicators":{"quote":[{}]}}]}}`)
			return
		}
		fmt.Fprint(w, `{"chart":{"result":[{"meta":{"symbol":"`+symbol+`","regularMarketPrice":123.45,"chartPreviousClose":120,"regularMarketVolume":1000,"regularMarketTime":1700000000},"indicators":{"quote":[{"open":[121],"high":[124],"low":[120]}]}}]}}`)
	}))
}

func TestYahooGetQuote_TWSuffixFallback(t *testing.T) {
	hits := map[string]int{}
	srv := twChartServer(t, map[string]bool{"2330.TW": true}, hits)
	defer srv.Close()

	y := NewYahoo()
	y.chartBaseURL = srv.URL

	q, err := y.GetQuote("2330")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if q.Ticker != "2330" {
		t.Errorf("Quote.Ticker = %q, want bare %q (not the resolved suffix)", q.Ticker, "2330")
	}
	if q.Price != 123.45 {
		t.Errorf("Price = %v, want 123.45", q.Price)
	}
	if hits["2330.TW"] != 1 || hits["2330.TWO"] != 1 {
		t.Fatalf("expected one try of each suffix on first call, got %v", hits)
	}

	// Second call for the same ticker should go straight to the cached
	// suffix — only one more request total, and it must be .TWO again.
	if _, err := y.GetQuote("2330"); err != nil {
		t.Fatalf("GetQuote (cached): %v", err)
	}
	if hits["2330.TW"] != 1 {
		t.Errorf(".TW should not be retried once .TWO is cached, got %d hits", hits["2330.TW"])
	}
	if hits["2330.TWO"] != 2 {
		t.Errorf(".TWO should be hit again from cache, got %d hits", hits["2330.TWO"])
	}
}

func TestYahooGetQuote_TWListedSuffixSucceedsFirstTry(t *testing.T) {
	hits := map[string]int{}
	srv := twChartServer(t, nil, hits)
	defer srv.Close()

	y := NewYahoo()
	y.chartBaseURL = srv.URL

	if _, err := y.GetQuote("2330"); err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if hits["2330.TW"] != 1 {
		t.Errorf("expected exactly one .TW request, got %d", hits["2330.TW"])
	}
	if hits["2330.TWO"] != 0 {
		t.Errorf(".TWO should never be tried when .TW succeeds, got %d hits", hits["2330.TWO"])
	}
}

func TestIsUSEquitySymbol(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"AAPL", true},
		{"MSFT", true},
		{"A", true},
		{"GOOGL", true}, // 5 chars, still plain letters
		{"", false},
		{"TOOLONG", false}, // > 5 chars
		{"BTC-USD", false}, // crypto pair
		{"SHOP.TO", false}, // foreign listing
		{"brk.b", false},   // lowercase, punctuation
		{"BRK.B", false},   // punctuation even if otherwise plain
		// Known limitation documented in CLAUDE.md: a plain 1-5 uppercase
		// letter symbol can't be distinguished from a real equity ticker by
		// shape alone, so a stablecoin ticker like USDE slips through. This
		// test pins down that this is current, known behavior rather than
		// an accident.
		{"USDE", true},
	}

	for _, tt := range tests {
		if got := IsUSEquitySymbol(tt.symbol); got != tt.want {
			t.Errorf("IsUSEquitySymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
		}
	}
}

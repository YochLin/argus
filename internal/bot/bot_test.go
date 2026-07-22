package bot

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
)

func TestDedup(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []string
	}{
		{
			name: "removes overlap",
			a:    []string{"AAPL", "MSFT", "NVDA"},
			b:    []string{"MSFT"},
			want: []string{"AAPL", "NVDA"},
		},
		{
			name: "no overlap returns a unchanged",
			a:    []string{"AAPL", "MSFT"},
			b:    []string{"TSLA"},
			want: []string{"AAPL", "MSFT"},
		},
		{
			name: "everything overlaps returns nil",
			a:    []string{"AAPL", "MSFT"},
			b:    []string{"AAPL", "MSFT"},
			want: nil,
		},
		{
			name: "empty a returns nil",
			a:    nil,
			b:    []string{"AAPL"},
			want: nil,
		},
		{
			name: "empty b returns a unchanged",
			a:    []string{"AAPL", "MSFT"},
			b:    nil,
			want: []string{"AAPL", "MSFT"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedup(tt.a, tt.b)
			if len(got) != len(tt.want) {
				t.Fatalf("dedup(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("dedup(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
				}
			}
		})
	}
}

func TestFormatQuote(t *testing.T) {
	t.Run("positive change shows up arrow", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", Price: 200, ChangePercent: 1.5, Open: 198, High: 201, Low: 197}
		out := formatQuote(i18n.EN, q)
		if !strings.Contains(out, "▲") {
			t.Errorf("formatQuote() = %q, want it to contain up arrow", out)
		}
		if !strings.Contains(out, "AAPL") {
			t.Errorf("formatQuote() = %q, want it to contain ticker", out)
		}
	})

	t.Run("negative change shows down arrow", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", Price: 200, ChangePercent: -1.5, Open: 198, High: 201, Low: 197}
		out := formatQuote(i18n.EN, q)
		if !strings.Contains(out, "▼") {
			t.Errorf("formatQuote() = %q, want it to contain down arrow", out)
		}
	})
}

func TestDaysUntil(t *testing.T) {
	today := time.Now().In(cst).Format("2006-01-02")
	tomorrow := time.Now().In(cst).AddDate(0, 0, 1).Format("2006-01-02")
	yesterday := time.Now().In(cst).AddDate(0, 0, -1).Format("2006-01-02")
	nextWeek := time.Now().In(cst).AddDate(0, 0, 7).Format("2006-01-02")

	tests := []struct {
		date string
		want int
	}{
		{today, 0},
		{tomorrow, 1},
		{yesterday, -1},
		{nextWeek, 7},
		{"not-a-date", 0},
	}
	for _, tt := range tests {
		if got := daysUntil(tt.date); got != tt.want {
			t.Errorf("daysUntil(%q) = %d, want %d", tt.date, got, tt.want)
		}
	}
}

func TestParseTradeArgs(t *testing.T) {
	t.Run("ticker shares price", func(t *testing.T) {
		ticker, shares, price, fee, date, err := parseTradeArgs("aapl 10 205.5")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "AAPL" || shares != 10 || price != 205.5 || fee != 0 || date != "" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %q; want AAPL, 10, 205.5, 0, \"\"", ticker, shares, price, fee, date)
		}
	})

	t.Run("with fee", func(t *testing.T) {
		ticker, shares, price, fee, date, err := parseTradeArgs("MSFT 5 400 1.5")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "MSFT" || shares != 5 || price != 400 || fee != 1.5 || date != "" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %q; want MSFT, 5, 400, 1.5, \"\"", ticker, shares, price, fee, date)
		}
	})

	t.Run("with date, no fee", func(t *testing.T) {
		ticker, shares, price, fee, date, err := parseTradeArgs("AAPL 10 200 2026-01-15")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "AAPL" || shares != 10 || price != 200 || fee != 0 || date != "2026-01-15" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %q; want AAPL, 10, 200, 0, 2026-01-15", ticker, shares, price, fee, date)
		}
	})

	t.Run("with fee and date, either order", func(t *testing.T) {
		for _, args := range []string{"AAPL 10 200 1.5 2026-01-15", "AAPL 10 200 2026-01-15 1.5"} {
			ticker, shares, price, fee, date, err := parseTradeArgs(args)
			if err != nil {
				t.Fatalf("parseTradeArgs(%q) error = %v", args, err)
			}
			if ticker != "AAPL" || shares != 10 || price != 200 || fee != 1.5 || date != "2026-01-15" {
				t.Errorf("parseTradeArgs(%q) = %q, %v, %v, %v, %q; want AAPL, 10, 200, 1.5, 2026-01-15", args, ticker, shares, price, fee, date)
			}
		}
	})

	for _, args := range []string{
		"",
		"AAPL",
		"AAPL 10",
		"AAPL 10 200 1 2",
		"AAPL 10 200 2026-01-15 2026-02-01",
		"AAPL 10 200 1 2 2026-01-15",
		"AAPL 0 200",
		"AAPL -1 200",
		"AAPL 10 0",
		"AAPL 10 -5",
		"AAPL 10 200 -1",
		"AAPL abc 200",
		"AAPL 10 200 2026-13-40",
	} {
		if _, _, _, _, _, err := parseTradeArgs(args); err == nil {
			t.Errorf("parseTradeArgs(%q) error = nil, want error", args)
		}
	}
}

func TestParseStopArgs(t *testing.T) {
	t.Run("ticker only", func(t *testing.T) {
		ticker, price, hasPrice, err := parseStopArgs("aapl")
		if err != nil {
			t.Fatalf("parseStopArgs() error = %v", err)
		}
		if ticker != "AAPL" || price != 0 || hasPrice {
			t.Errorf("parseStopArgs() = %q, %v, %v; want AAPL, 0, false", ticker, price, hasPrice)
		}
	})

	t.Run("ticker and price", func(t *testing.T) {
		ticker, price, hasPrice, err := parseStopArgs("aapl 190.5")
		if err != nil {
			t.Fatalf("parseStopArgs() error = %v", err)
		}
		if ticker != "AAPL" || price != 190.5 || !hasPrice {
			t.Errorf("parseStopArgs() = %q, %v, %v; want AAPL, 190.5, true", ticker, price, hasPrice)
		}
	})

	for _, args := range []string{"", "AAPL 190 extra", "AAPL 0", "AAPL -5", "AAPL abc"} {
		if _, _, _, err := parseStopArgs(args); err == nil {
			t.Errorf("parseStopArgs(%q) error = nil, want error", args)
		}
	}
}

func TestFormatChatContext(t *testing.T) {
	t.Run("empty tickers returns empty string", func(t *testing.T) {
		if got := formatChatContext(i18n.EN, nil, nil, nil); got != "" {
			t.Errorf("formatChatContext(nil) = %q, want \"\"", got)
		}
	})

	t.Run("watch-only ticker with no position", func(t *testing.T) {
		snapshots := map[string]db.DailySnapshot{
			"AAPL": {Date: "2026-07-05", Close: 210, ChangePercent: 1.5},
		}
		out := formatChatContext(i18n.EN, []string{"AAPL"}, nil, snapshots)
		if !strings.Contains(out, "AAPL") || !strings.Contains(out, "210.00") {
			t.Errorf("formatChatContext() = %q, want it to contain ticker and close price", out)
		}
		if strings.Contains(out, "holding") {
			t.Errorf("formatChatContext() = %q, want no position line for a ticker with no position", out)
		}
	})

	t.Run("held ticker includes cost basis and unrealized pct", func(t *testing.T) {
		snapshots := map[string]db.DailySnapshot{
			"AAPL": {Date: "2026-07-05", Close: 220, ChangePercent: 1.5},
		}
		positions := map[string]db.Position{
			"AAPL": {Ticker: "AAPL", Shares: 10, AvgCost: 200},
		}
		out := formatChatContext(i18n.EN, []string{"AAPL"}, positions, snapshots)
		if !strings.Contains(out, "holding") || !strings.Contains(out, "+10.00%") {
			t.Errorf("formatChatContext() = %q, want a position line with +10.00%% unrealized", out)
		}
	})

	t.Run("ticker with no snapshot yet", func(t *testing.T) {
		out := formatChatContext(i18n.EN, []string{"NEWCO"}, nil, nil)
		if !strings.Contains(out, "NEWCO") || !strings.Contains(out, "no closing data") {
			t.Errorf("formatChatContext() = %q, want a no-data line for NEWCO", out)
		}
	})
}

// TestUniverseScanChunkFullCoverage verifies universeScanChunk rotates
// through every ticker exactly once over a full chunkCount-day cycle, with
// no gaps or duplicates — the property the daily scan job actually depends
// on for eventual full universe coverage.
func TestUniverseScanChunkFullCoverage(t *testing.T) {
	var tickers []string
	for i := 0; i < 503; i++ {
		tickers = append(tickers, fmt.Sprintf("T%03d", i))
	}

	seen := make(map[string]int)
	for day := 0; day < scanChunkCount; day++ {
		for _, ticker := range universeScanChunk(tickers, scanChunkCount, day) {
			seen[ticker]++
		}
	}

	if len(seen) != len(tickers) {
		t.Fatalf("covered %d/%d tickers over a full cycle, want all of them", len(seen), len(tickers))
	}
	for ticker, n := range seen {
		if n != 1 {
			t.Errorf("ticker %s scanned %d times over a full cycle, want exactly 1", ticker, n)
		}
	}
}

func TestUniverseScanChunkEmptyAndNegativeDay(t *testing.T) {
	if got := universeScanChunk(nil, scanChunkCount, 0); got != nil {
		t.Errorf("universeScanChunk(nil, ...) = %v, want nil", got)
	}
	tickers := []string{"A", "B", "C", "D", "E"}
	// A negative dayIndex must still resolve to a valid, in-range chunk
	// rather than panicking on a negative slice index.
	got := universeScanChunk(tickers, scanChunkCount, -1)
	if len(got) == 0 {
		t.Errorf("universeScanChunk(..., -1) = %v, want a non-empty chunk", got)
	}
}

func TestBreachAlertDecision(t *testing.T) {
	tests := []struct {
		name           string
		adverseMovePct float64
		thresholdPct   float64
		prevState      string
		wantBreached   bool
		wantAlert      bool
		wantNewState   string
	}{
		{"under threshold, never breached", 5, 10, "", false, false, ""},
		{"fresh breach alerts", 12, 10, "", true, true, "breached"},
		{"exactly at threshold counts as breached", 10, 10, "", true, true, "breached"},
		{"already breached does not re-alert", 15, 10, "breached", true, false, "breached"},
		{"still under threshold with no prior breach stays quiet", 3, 10, "", false, false, ""},
		{"recovering under threshold resets state", 8, 10, "breached", false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breached, alert, newState := breachAlertDecision(tt.adverseMovePct, tt.thresholdPct, tt.prevState)
			if breached != tt.wantBreached || alert != tt.wantAlert || newState != tt.wantNewState {
				t.Errorf("breachAlertDecision(%v, %v, %q) = %v, %v, %q; want %v, %v, %q",
					tt.adverseMovePct, tt.thresholdPct, tt.prevState,
					breached, alert, newState, tt.wantBreached, tt.wantAlert, tt.wantNewState)
			}
		})
	}
}

func TestStopBreachDecision(t *testing.T) {
	tests := []struct {
		name         string
		close        float64
		stopPrice    float64
		prevState    string
		wantBreached bool
		wantAlert    bool
		wantNewState string
	}{
		{"above stop, never breached", 105, 100, "", false, false, ""},
		{"exactly at stop does not breach (long stop is a floor, not a ceiling)", 100, 100, "", false, false, ""},
		{"fresh breach alerts", 95, 100, "", true, true, "breached"},
		{"already breached does not re-alert", 90, 100, "breached", true, false, "breached"},
		{"recovering back at or above stop resets state", 100, 100, "breached", false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breached, alert, newState := stopBreachDecision(tt.close, tt.stopPrice, tt.prevState)
			if breached != tt.wantBreached || alert != tt.wantAlert || newState != tt.wantNewState {
				t.Errorf("stopBreachDecision(%v, %v, %q) = %v, %v, %q; want %v, %v, %q",
					tt.close, tt.stopPrice, tt.prevState,
					breached, alert, newState, tt.wantBreached, tt.wantAlert, tt.wantNewState)
			}
		})
	}
}

func TestTargetReachedDecision(t *testing.T) {
	tests := []struct {
		name         string
		close        float64
		targetPrice  float64
		prevState    string
		wantReached  bool
		wantAlert    bool
		wantNewState string
	}{
		{"below target, never reached", 95, 100, "", false, false, ""},
		{"exactly at target reaches (upward threshold, not a floor)", 100, 100, "", true, true, "hit"},
		{"fresh reach alerts", 105, 100, "", true, true, "hit"},
		{"already hit does not re-alert", 110, 100, "hit", true, false, "hit"},
		{"falling back under target resets state", 95, 100, "hit", false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached, alert, newState := targetReachedDecision(tt.close, tt.targetPrice, tt.prevState)
			if reached != tt.wantReached || alert != tt.wantAlert || newState != tt.wantNewState {
				t.Errorf("targetReachedDecision(%v, %v, %q) = %v, %v, %q; want %v, %v, %q",
					tt.close, tt.targetPrice, tt.prevState,
					reached, alert, newState, tt.wantReached, tt.wantAlert, tt.wantNewState)
			}
		})
	}
}

func TestSuggestShares(t *testing.T) {
	tests := []struct {
		name                               string
		accountValue, riskPct, price, stop float64
		want                               int
	}{
		{"normal case: $10000 * 1% = $100 risk / $5 per-share risk = 20 shares", 10000, 1, 50, 45, 20},
		{"disabled: riskPct <= 0", 10000, 0, 50, 45, 0},
		{"disabled: non-positive account value", 0, 1, 50, 45, 0},
		{"invalid: stop at price (zero per-share risk)", 10000, 1, 50, 50, 0},
		{"invalid: stop above price", 10000, 1, 50, 55, 0},
		{"invalid: non-positive stop", 10000, 1, 50, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestShares(tt.accountValue, tt.riskPct, tt.price, tt.stop); got != tt.want {
				t.Errorf("suggestShares(%v, %v, %v, %v) = %d, want %d", tt.accountValue, tt.riskPct, tt.price, tt.stop, got, tt.want)
			}
		})
	}
}

func TestTrailingStopThreshold(t *testing.T) {
	tests := []struct {
		name              string
		fixedPct, atrMult float64
		atr, peak         float64
		wantThresholdPct  float64
		wantATRBased      bool
		wantOK            bool
	}{
		{"fixed only, mult disabled (default)", 15, 0, 2, 100, 15, false, true},
		{"fixed only, mult set but ATR unavailable", 15, 3, 0, 100, 15, false, true},
		{"fixed only, mult set but no peak", 15, 3, 2, 0, 15, false, true},
		{"low volatility: ATR tightens below fixed", 15, 3, 2, 100, 6, true, true},       // 3*2/100*100=6 < 15
		{"high volatility: fixed caps the ATR distance", 15, 3, 8, 100, 15, false, true}, // 3*8/100*100=24 > 15
		{"pure ATR, fixed disabled", 0, 3, 2, 100, 6, true, true},
		{"fixed disabled, ATR unavailable: no usable threshold", 0, 3, 0, 100, 0, false, false},
		{"both disabled: check off entirely", 0, 0, 2, 100, 0, false, false},
		{"negative fixed treated as disabled", -5, 3, 2, 100, 6, true, true},
		{"negative mult treated as disabled", 15, -3, 2, 100, 15, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotThreshold, gotATRBased, gotOK := trailingStopThreshold(tt.fixedPct, tt.atrMult, tt.atr, tt.peak)
			if gotOK != tt.wantOK || gotATRBased != tt.wantATRBased || (gotOK && gotThreshold != tt.wantThresholdPct) {
				t.Errorf("trailingStopThreshold(%v, %v, %v, %v) = %v, %v, %v; want %v, %v, %v",
					tt.fixedPct, tt.atrMult, tt.atr, tt.peak,
					gotThreshold, gotATRBased, gotOK, tt.wantThresholdPct, tt.wantATRBased, tt.wantOK)
			}
		})
	}
}

func TestComputeVsSPY(t *testing.T) {
	tests := []struct {
		name                                      string
		currentPrice, avgCost, spyPrice, spyEntry float64
		wantTickerPct, wantSPYPct                 float64
	}{
		{"beats the market", 200, 150, 550, 500, 33.333333333333336, 10},
		{"underperforms while still up", 165, 150, 550, 500, 10, 10},
		{"down position", 120, 150, 480, 500, -20, -4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeVsSPY(tt.currentPrice, tt.avgCost, tt.spyPrice, tt.spyEntry)
			if !floatsClose(got.TickerPct, tt.wantTickerPct) || !floatsClose(got.SPYPct, tt.wantSPYPct) {
				t.Errorf("computeVsSPY(%v, %v, %v, %v) = %+v, want {%v %v}",
					tt.currentPrice, tt.avgCost, tt.spyPrice, tt.spyEntry, got, tt.wantTickerPct, tt.wantSPYPct)
			}
		})
	}
}

func floatsClose(a, b float64) bool {
	d := a - b
	return d > -1e-9 && d < 1e-9
}

func TestPositionsSlice(t *testing.T) {
	positions := map[string]db.Position{
		"MSFT": {Ticker: "MSFT", Shares: 1, AvgCost: 400},
		"AAPL": {Ticker: "AAPL", Shares: 2, AvgCost: 200},
	}
	got := positionsSlice(positions)
	if len(got) != 2 || got[0].Ticker != "AAPL" || got[1].Ticker != "MSFT" {
		t.Errorf("positionsSlice() = %+v, want [AAPL, MSFT] order", got)
	}

	if got := positionsSlice(nil); len(got) != 0 {
		t.Errorf("positionsSlice(nil) = %+v, want empty", got)
	}
}

func TestMergeCandidates(t *testing.T) {
	movers := []string{"AAPL", "MSFT"}
	scanHits := map[string]string{
		"MSFT": "RSI oversold (28.0)", // also a mover: must not duplicate
		"NVDA": "MACD golden cross",
	}
	watchlist := []string{"TSLA"} // excluded even if it somehow appears

	got := mergeCandidates(movers, scanHits, watchlist)

	want := map[string]bool{"AAPL": true, "MSFT": true, "NVDA": true}
	if len(got) != len(want) {
		t.Fatalf("mergeCandidates() = %v, want exactly %v", got, want)
	}
	seen := make(map[string]bool)
	for _, ticker := range got {
		if seen[ticker] {
			t.Errorf("mergeCandidates() contains duplicate %s", ticker)
		}
		seen[ticker] = true
		if !want[ticker] {
			t.Errorf("mergeCandidates() contains unexpected %s", ticker)
		}
	}
}

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

func TestRecommendationSources(t *testing.T) {
	watchlist := []string{"AAPL", "MSFT"}
	// MSFT also appears as a candidate — shouldn't happen in practice since
	// mergeCandidates already excludes watchlist tickers, but recommendationSources
	// guards it anyway: watchlist attribution must win regardless.
	candidates := []string{"MSFT", "NVDA", "TSLA", "SNOW"}
	scanHits := map[string]string{
		"NVDA": "RSI oversold (28.0)",
	}
	explore := map[string]string{
		"SNOW": "LLM nomination: named in a cloud-spend story",
	}

	got := recommendationSources(watchlist, candidates, scanHits, explore)

	want := map[string]string{
		"AAPL": "watchlist",
		"MSFT": "watchlist",
		"NVDA": "scan",
		"TSLA": "movers",
		"SNOW": "explore",
	}
	for ticker, wantSource := range want {
		if got[ticker] != wantSource {
			t.Errorf("recommendationSources()[%s] = %q, want %q", ticker, got[ticker], wantSource)
		}
	}
}

func TestRecommendationSourcesScanBeatsExplore(t *testing.T) {
	// Shouldn't happen in practice — exploreCandidates' dedup step already
	// excludes anything already a candidate — but scan must win over explore
	// defensively, same reasoning as watchlist winning over both.
	candidates := []string{"NVDA"}
	scanHits := map[string]string{"NVDA": "MACD golden cross"}
	explore := map[string]string{"NVDA": "LLM nomination: also mentioned in the news"}

	got := recommendationSources(nil, candidates, scanHits, explore)

	if got["NVDA"] != "scan" {
		t.Errorf("recommendationSources()[NVDA] = %q, want %q", got["NVDA"], "scan")
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

func TestFormatRecLine(t *testing.T) {
	t.Run("includes the action separator when Action is set", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "MSFT", Action: "BUY", Reason: "cloud growth."}
		got := formatRecLine(i18n.EN, r, nil)
		want := "*MSFT* — BUY\ncloud growth.\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})

	t.Run("empty action omits the action separator", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "AAPL", Reason: "no action line."}
		got := formatRecLine(i18n.EN, r, nil)
		want := "*AAPL*\nno action line.\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})

	t.Run("a sizing line for a ticker in the map is appended after the reason", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "AAPL", Action: "BUY", Reason: "breakout."}
		sizing := map[string]string{"AAPL": "sizing info\n"}
		got := formatRecLine(i18n.EN, r, sizing)
		want := "*AAPL* — BUY\nbreakout.\nsizing info\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})

	t.Run("a ticker missing from sizing renders no sizing line", func(t *testing.T) {
		r := llm.Recommendation{Ticker: "TSLA", Action: "SELL", Reason: "overextended."}
		got := formatRecLine(i18n.EN, r, map[string]string{"AAPL": "sizing info\n"})
		want := "*TSLA* — SELL\noverextended.\n"
		if got != want {
			t.Errorf("formatRecLine() = %q, want %q", got, want)
		}
	})
}

func TestTrackHit(t *testing.T) {
	tests := []struct {
		name                          string
		action                        string
		tickerChangePct, spyChangePct float64
		haveSPY                       bool
		want                          bool
	}{
		{"BUY beats SPY is a hit", "BUY", 10, 4, true, true},
		{"BUY behind SPY is not a hit even though price rose", "BUY", 3, 4, true, false},
		{"BUY without SPY data falls back to absolute direction (up)", "BUY", 3, 0, false, true},
		{"BUY without SPY data falls back to absolute direction (down)", "BUY", -1, 0, false, false},
		{"SELL underperforming SPY is a hit", "SELL", -10, -2, true, true},
		{"SELL merely tracking SPY down is not a hit", "SELL", -2, -2, true, false},
		{"SELL without SPY data falls back to absolute direction (down)", "SELL", -3, 0, false, true},
		{"HOLD never counts as a hit", "HOLD", 10, -10, true, false},
		{"unset action never counts as a hit", "", 10, -10, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trackHit(tt.action, tt.tickerChangePct, tt.spyChangePct, tt.haveSPY); got != tt.want {
				t.Errorf("trackHit(%q, %v, %v, %v) = %v, want %v",
					tt.action, tt.tickerChangePct, tt.spyChangePct, tt.haveSPY, got, tt.want)
			}
		})
	}
}

func TestDisplaySource(t *testing.T) {
	if got := displaySource(""); got != "watchlist" {
		t.Errorf(`displaySource("") = %q, want "watchlist"`, got)
	}
	if got := displaySource("scan"); got != "scan" {
		t.Errorf(`displaySource("scan") = %q, want "scan"`, got)
	}
}

func TestSummarizeTrack(t *testing.T) {
	rows := []trackRow{
		{Action: "BUY", Source: "watchlist", ChangePct: 10, Hit: true},
		{Action: "BUY", Source: "watchlist", ChangePct: -2, Hit: false},
		{Action: "SELL", Source: "scan", ChangePct: -5, Hit: true},
		{Action: "BUY", Source: "scan", ChangePct: 4, Hit: true},
	}

	overall, bySource := summarizeTrack(rows)

	if overall.Evaluated != 4 || overall.Hits != 3 {
		t.Fatalf("summarizeTrack() overall = %+v, want Evaluated=4 Hits=3", overall)
	}
	if got, want := overall.HitRate(), 75.0; got != want {
		t.Errorf("overall.HitRate() = %v, want %v", got, want)
	}
	// BUY avg: (10 + -2 + 4) / 3 = 4; SELL avg: -5 / 1 = -5
	if got, want := overall.AvgBuyPct(), 4.0; got != want {
		t.Errorf("overall.AvgBuyPct() = %v, want %v", got, want)
	}
	if got, want := overall.AvgSellPct(), -5.0; got != want {
		t.Errorf("overall.AvgSellPct() = %v, want %v", got, want)
	}

	if len(bySource) != 2 {
		t.Fatalf("summarizeTrack() bySource = %+v, want exactly 2 groups", bySource)
	}
	watchlistStats := bySource["watchlist"]
	if watchlistStats.Evaluated != 2 || watchlistStats.Hits != 1 {
		t.Errorf("bySource[watchlist] = %+v, want Evaluated=2 Hits=1", watchlistStats)
	}
	scanStats := bySource["scan"]
	if scanStats.Evaluated != 2 || scanStats.Hits != 2 {
		t.Errorf("bySource[scan] = %+v, want Evaluated=2 Hits=2", scanStats)
	}
}

func TestTrackSourceStatsZeroDivision(t *testing.T) {
	var s trackSourceStats
	if s.HitRate() != 0 || s.AvgBuyPct() != 0 || s.AvgSellPct() != 0 {
		t.Errorf("zero-value trackSourceStats methods = %v, %v, %v; want all 0", s.HitRate(), s.AvgBuyPct(), s.AvgSellPct())
	}
}

func TestSortedSourceKeys(t *testing.T) {
	bySource := map[string]trackSourceStats{
		"watchlist": {},
		"scan":      {},
		"movers":    {},
	}
	got := sortedSourceKeys(bySource)
	want := []string{"movers", "scan", "watchlist"}
	if len(got) != len(want) {
		t.Fatalf("sortedSourceKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedSourceKeys() = %v, want %v", got, want)
		}
	}
}

func TestRenderTrackSummaryEmptyWhenNothingEvaluated(t *testing.T) {
	if got := renderTrackSummary(i18n.EN, trackSourceStats{}, nil); got != "" {
		t.Errorf("renderTrackSummary() with Evaluated=0 = %q, want \"\"", got)
	}
}

func TestRenderTrackSummaryOmitsBySourceBreakdownForSingleSource(t *testing.T) {
	overall := trackSourceStats{Hits: 3, Evaluated: 4, BuySum: 12, BuyCount: 3}
	bySource := map[string]trackSourceStats{"watchlist": overall}

	got := renderTrackSummary(i18n.EN, overall, bySource)

	if !strings.Contains(got, "3/4") {
		t.Errorf("renderTrackSummary() missing hit rate, got:\n%s", got)
	}
	if strings.Contains(got, "By source") {
		t.Errorf("renderTrackSummary() with a single source should omit the by-source breakdown, got:\n%s", got)
	}
}

func TestRenderTrackSummaryIncludesBySourceBreakdownForMultipleSources(t *testing.T) {
	overall := trackSourceStats{Hits: 3, Evaluated: 4}
	bySource := map[string]trackSourceStats{
		"watchlist": {Hits: 1, Evaluated: 2},
		"scan":      {Hits: 2, Evaluated: 2},
	}

	got := renderTrackSummary(i18n.EN, overall, bySource)

	if !strings.Contains(got, "By source") {
		t.Errorf("renderTrackSummary() with 2 sources should include the by-source breakdown, got:\n%s", got)
	}
	if !strings.Contains(got, "watchlist") || !strings.Contains(got, "scan") {
		t.Errorf("renderTrackSummary() missing a source name, got:\n%s", got)
	}
}

func TestRenderEarningsPreviewFiltersByWindowAndSorts(t *testing.T) {
	in3Days := time.Now().In(cst).AddDate(0, 0, 3).Format("2006-01-02")
	in1Day := time.Now().In(cst).AddDate(0, 0, 1).Format("2006-01-02")
	in30Days := time.Now().In(cst).AddDate(0, 0, 30).Format("2006-01-02")
	yesterday := time.Now().In(cst).AddDate(0, 0, -1).Format("2006-01-02")

	earnings := map[string]data.EarningsEvent{
		"AAPL": {Ticker: "AAPL", Date: in3Days},
		"MSFT": {Ticker: "MSFT", Date: in1Day},
		"NVDA": {Ticker: "NVDA", Date: in30Days}, // outside the 7-day window
		"OLD":  {Ticker: "OLD", Date: yesterday}, // already past
	}

	got := renderEarningsPreview(i18n.EN, earnings, 7)

	if strings.Contains(got, "NVDA") || strings.Contains(got, "OLD") {
		t.Errorf("renderEarningsPreview() should exclude out-of-window tickers, got:\n%s", got)
	}
	msftIdx := strings.Index(got, "MSFT")
	aaplIdx := strings.Index(got, "AAPL")
	if msftIdx == -1 || aaplIdx == -1 || msftIdx > aaplIdx {
		t.Errorf("renderEarningsPreview() should sort soonest-first (MSFT before AAPL), got:\n%s", got)
	}
}

func TestRenderEarningsPreviewEmptyWhenNothingInWindow(t *testing.T) {
	if got := renderEarningsPreview(i18n.EN, nil, 7); got != "" {
		t.Errorf("renderEarningsPreview() with no earnings = %q, want \"\"", got)
	}
}

func TestSplitMessage(t *testing.T) {
	t.Run("short text is returned as a single chunk", func(t *testing.T) {
		got := splitMessage("hello\nworld\n", 100)
		if len(got) != 1 || got[0] != "hello\nworld\n" {
			t.Errorf("splitMessage() = %v, want single unchanged chunk", got)
		}
	})

	t.Run("splits on line boundaries, never mid-line", func(t *testing.T) {
		// Each line is 6 runes ("AAAA\n"/"BBBB\n" etc are 5, use 6 to be
		// explicit); limit of 10 fits one line per chunk, not two.
		text := "aaaaa\nbbbbb\nccccc\n"
		got := splitMessage(text, 10)
		if len(got) != 3 {
			t.Fatalf("splitMessage() = %v (len %d), want 3 chunks", got, len(got))
		}
		for i, want := range []string{"aaaaa\n", "bbbbb\n", "ccccc\n"} {
			if got[i] != want {
				t.Errorf("chunk %d = %q, want %q", i, got[i], want)
			}
		}
		// Reassembling every chunk must reproduce the original text exactly —
		// splitting must never drop or duplicate content.
		if strings.Join(got, "") != text {
			t.Errorf("chunks don't reassemble to the original text")
		}
	})

	t.Run("packs multiple short lines into one chunk up to the limit", func(t *testing.T) {
		text := "ab\ncd\nef\ngh\n"
		got := splitMessage(text, 6)
		if len(got) != 2 || got[0] != "ab\ncd\n" || got[1] != "ef\ngh\n" {
			t.Errorf("splitMessage() = %v, want [\"ab\\ncd\\n\" \"ef\\ngh\\n\"]", got)
		}
	})

	t.Run("a single line longer than the limit is hard-split rather than dropped", func(t *testing.T) {
		text := "abcdefghij"
		got := splitMessage(text, 4)
		if strings.Join(got, "") != text {
			t.Errorf("splitMessage() chunks %v don't reassemble to %q", got, text)
		}
		for _, c := range got {
			if utf8.RuneCountInString(c) > 4 {
				t.Errorf("chunk %q exceeds limit 4", c)
			}
		}
	})
}

func TestLastClosedRound(t *testing.T) {
	t.Run("no transactions at all", func(t *testing.T) {
		_, ok := lastClosedRound(nil)
		if ok {
			t.Errorf("lastClosedRound(nil) ok = true, want false")
		}
	})

	t.Run("still-open position is not a closed round", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-07-01"},
		}
		_, ok := lastClosedRound(txs)
		if ok {
			t.Errorf("lastClosedRound() ok = true for a still-open position, want false")
		}
	})

	t.Run("single buy and sell", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-07-01"},
			{Side: "SELL", Shares: 10, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-07-01" || round.EndDate != "2026-07-10" || len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() = %+v, want start 2026-07-01, end 2026-07-10, 2 legs", round)
		}
	})

	t.Run("multiple buys and partial sells closing out", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 5, Date: "2026-07-01"},
			{Side: "BUY", Shares: 5, Date: "2026-07-02"},
			{Side: "SELL", Shares: 3, Date: "2026-07-05"},
			{Side: "SELL", Shares: 7, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-07-01" || round.EndDate != "2026-07-10" || len(round.Legs) != 4 {
			t.Errorf("lastClosedRound() = %+v, want start 2026-07-01, end 2026-07-10, 4 legs", round)
		}
	})

	t.Run("closed then re-entered picks the latest round", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-01-01"},
			{Side: "SELL", Shares: 10, Date: "2026-01-15"},
			{Side: "BUY", Shares: 5, Date: "2026-07-01"},
			{Side: "SELL", Shares: 5, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-07-01" || round.EndDate != "2026-07-10" || len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() = %+v, want the second (latest) round only", round)
		}
	})

	t.Run("closed then re-entered and still open returns the prior closed round", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-01-01"},
			{Side: "SELL", Shares: 10, Date: "2026-01-15"},
			{Side: "BUY", Shares: 5, Date: "2026-07-01"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-01-01" || round.EndDate != "2026-01-15" || len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() = %+v, want the first (only closed) round", round)
		}
	})

	t.Run("float dust residue counts as closed", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-07-01"},
			{Side: "SELL", Shares: 9.9999999995, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true (residue within float tolerance)")
		}
		if len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() legs = %d, want 2", len(round.Legs))
		}
	})
}

func TestRecordSellClosedFlag(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if err := d.SetStopPrice("AAPL", 180); err != nil {
		t.Fatalf("SetStopPrice() error = %v", err)
	}

	msg, closed, stopPrice := b.recordSell("AAPL", 4, 220, 0, "2026-06-10")
	if closed {
		t.Errorf("recordSell() closed = true for a partial sell, want false; msg = %q", msg)
	}
	if stopPrice != 180 {
		t.Errorf("recordSell() stopPrice = %v, want 180 (read before the sell)", stopPrice)
	}

	msg, closed, stopPrice = b.recordSell("AAPL", 6, 230, 0, "2026-06-20")
	if !closed {
		t.Errorf("recordSell() closed = false for a sell down to 0 shares, want true; msg = %q", msg)
	}
	if stopPrice != 180 {
		t.Errorf("recordSell() stopPrice = %v, want 180 for the sell that fully closed the position", stopPrice)
	}

	msg, closed, stopPrice = b.recordSell("AAPL", 1, 200, 0, "2026-06-21")
	if closed {
		t.Errorf("recordSell() closed = true on an error path (no position left), want false; msg = %q", msg)
	}
	if stopPrice != 0 {
		t.Errorf("recordSell() stopPrice = %v, want 0 on an error path", stopPrice)
	}
}

func TestBuildClosedTradeReviewNoTransactions(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)

	_, ok, err := b.buildClosedTradeReview("AAPL", 0)
	if err != nil || ok {
		t.Fatalf("buildClosedTradeReview() = _, %v, %v; want ok=false, err=nil for a never-traded ticker", ok, err)
	}
}

func TestBuildClosedTradeReviewFull(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	if _, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 10, 220, 1, "2026-06-20"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	for _, s := range []db.DailySnapshot{
		{Ticker: "AAPL", Date: "2026-06-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-06-10", Close: 235}, // period high
		{Ticker: "AAPL", Date: "2026-06-15", Close: 190}, // period low
		{Ticker: "AAPL", Date: "2026-06-20", Close: 220},
		{Ticker: "SPY", Date: "2026-06-01", Close: 500},
		{Ticker: "SPY", Date: "2026-06-20", Close: 525},
	} {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot() error = %v", err)
		}
	}

	if err := d.SetThesis("AAPL", "long-term compounder"); err != nil {
		t.Fatalf("SetThesis() error = %v", err)
	}

	if err := d.SaveRecommendations("2026-06-10", []db.Recommendation{
		{Ticker: "AAPL", Action: "HOLD", Reason: "still in range"},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}
	// Outside the holding window — must not leak into the review.
	if err := d.SaveRecommendations("2026-07-01", []db.Recommendation{
		{Ticker: "AAPL", Action: "BUY", Reason: "after the round closed"},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}

	trade, ok, err := b.buildClosedTradeReview("AAPL", 0)
	if err != nil || !ok {
		t.Fatalf("buildClosedTradeReview() = _, %v, %v; want ok=true, err=nil", ok, err)
	}

	if len(trade.Legs) != 2 {
		t.Fatalf("buildClosedTradeReview() legs = %d, want 2", len(trade.Legs))
	}
	// realized P&L: (220-200)*10 - 1(buy fee, folded into avg cost) - 1(sell fee) = 198.
	if trade.RealizedPnL < 197.9 || trade.RealizedPnL > 198.1 {
		t.Errorf("buildClosedTradeReview() RealizedPnL = %v, want ~198", trade.RealizedPnL)
	}
	if trade.HoldingDays != 19 {
		t.Errorf("buildClosedTradeReview() HoldingDays = %d, want 19", trade.HoldingDays)
	}
	if trade.PeriodHigh != 235 || trade.PeriodLow != 190 {
		t.Errorf("buildClosedTradeReview() period high/low = %v/%v, want 235/190", trade.PeriodHigh, trade.PeriodLow)
	}
	if trade.VsSPY == nil {
		t.Fatal("buildClosedTradeReview() VsSPY = nil, want a comparison (both endpoints have SPY snapshots)")
	}
	// Ticker: (220-200)/200*100 = 10%; SPY: (525-500)/500*100 = 5%.
	if trade.VsSPY.TickerPct != 10 || trade.VsSPY.SPYPct != 5 {
		t.Errorf("buildClosedTradeReview() VsSPY = %+v, want {10 5}", trade.VsSPY)
	}
	if trade.Thesis == nil || *trade.Thesis != "long-term compounder" {
		t.Errorf("buildClosedTradeReview() Thesis = %v, want \"long-term compounder\"", trade.Thesis)
	}
	if len(trade.Recommendations) != 1 || trade.Recommendations[0].Action != "HOLD" {
		t.Errorf("buildClosedTradeReview() Recommendations = %+v, want exactly the in-window HOLD", trade.Recommendations)
	}
}

func TestSparklineEmpty(t *testing.T) {
	if got := sparkline(nil); got != "" {
		t.Errorf("sparkline(nil) = %q, want empty", got)
	}
}

func TestSparklineSinglePoint(t *testing.T) {
	got := sparkline([]float64{1000})
	want := string(sparklineChars[len(sparklineChars)/2])
	if got != want {
		t.Errorf("sparkline([1000]) = %q, want %q (mid-level char)", got, want)
	}
}

func TestSparklineFlatSeries(t *testing.T) {
	got := sparkline([]float64{1000, 1000, 1000})
	want := strings.Repeat(string(sparklineChars[len(sparklineChars)/2]), 3)
	if got != want {
		t.Errorf("sparkline(flat) = %q, want %q", got, want)
	}
}

func TestSparklineRange(t *testing.T) {
	got := sparkline([]float64{0, 50, 100})
	runes := []rune(got)
	if len(runes) != 3 {
		t.Fatalf("sparkline() = %q, want 3 runes", got)
	}
	if runes[0] != sparklineChars[0] {
		t.Errorf("sparkline() first char = %q, want lowest level %q", string(runes[0]), string(sparklineChars[0]))
	}
	if runes[2] != sparklineChars[len(sparklineChars)-1] {
		t.Errorf("sparkline() last char = %q, want highest level %q", string(runes[2]), string(sparklineChars[len(sparklineChars)-1]))
	}
}

func TestMaxDrawdownPctEmptyAndSinglePoint(t *testing.T) {
	if got := maxDrawdownPct(nil); got != 0 {
		t.Errorf("maxDrawdownPct(nil) = %v, want 0", got)
	}
	if got := maxDrawdownPct([]float64{1000}); got != 0 {
		t.Errorf("maxDrawdownPct(single point) = %v, want 0", got)
	}
}

func TestMaxDrawdownPctMonotonicallyUpIsZero(t *testing.T) {
	got := maxDrawdownPct([]float64{1000, 1050, 1100, 1200})
	if got != 0 {
		t.Errorf("maxDrawdownPct(monotonic up) = %v, want 0", got)
	}
}

func TestMaxDrawdownPctPicksWorstDipFromRunningPeak(t *testing.T) {
	// Peak 1200 -> trough 900 (25% drawdown) -> partial recovery to 1100
	// (still only a ~8.3% drawdown from 1200) -> the 25% dip must win, not
	// just a first-vs-last or last-seen-peak comparison.
	got := maxDrawdownPct([]float64{1000, 1200, 900, 1100})
	want := 25.0
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("maxDrawdownPct() = %v, want %v", got, want)
	}
}

func TestMonthRange(t *testing.T) {
	t.Run("ordinary month", func(t *testing.T) {
		now := time.Date(2026, time.July, 17, 9, 30, 0, 0, cst)
		from, to := monthRange(now)
		if from != "2026-06-01" || to != "2026-06-30" {
			t.Errorf("monthRange(2026-07-17) = %q, %q, want 2026-06-01, 2026-06-30", from, to)
		}
	})

	t.Run("january rolls back to december of the prior year", func(t *testing.T) {
		now := time.Date(2026, time.January, 1, 9, 30, 0, 0, cst)
		from, to := monthRange(now)
		if from != "2025-12-01" || to != "2025-12-31" {
			t.Errorf("monthRange(2026-01-01) = %q, %q, want 2025-12-01, 2025-12-31", from, to)
		}
	})
}

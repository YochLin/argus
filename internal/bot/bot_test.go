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
	"argus/internal/market"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantCmd  string
		wantArgs string
		wantOK   bool
	}{
		{"bare command", "/status", "status", "", true},
		{"command with args", "/add AAPL", "add", "AAPL", true},
		{"command with multi-word args", "/buy AAPL 10 200", "buy", "AAPL 10 200", true},
		{"botname suffix stripped", "/status@my_bot", "status", "", true},
		{"botname suffix with args", "/add@my_bot AAPL", "add", "AAPL", true},
		{"plain text is not a command", "hello there", "", "", false},
		{"empty text is not a command", "", "", "", false},
		{"bare slash is not a command", "/", "", "", false},
		{"args get trimmed", "/add   AAPL  ", "add", "AAPL", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, ok := parseCommand(tt.text)
			if cmd != tt.wantCmd || args != tt.wantArgs || ok != tt.wantOK {
				t.Errorf("parseCommand(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.text, cmd, args, ok, tt.wantCmd, tt.wantArgs, tt.wantOK)
			}
		})
	}
}

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
		out := formatQuote(i18n.EN, q, q.Ticker)
		if !strings.Contains(out, "▲") {
			t.Errorf("formatQuote() = %q, want it to contain up arrow", out)
		}
		if !strings.Contains(out, "AAPL") {
			t.Errorf("formatQuote() = %q, want it to contain ticker", out)
		}
	})

	t.Run("negative change shows down arrow", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", Price: 200, ChangePercent: -1.5, Open: 198, High: 201, Low: 197}
		out := formatQuote(i18n.EN, q, q.Ticker)
		if !strings.Contains(out, "▼") {
			t.Errorf("formatQuote() = %q, want it to contain down arrow", out)
		}
	})
}

// fakeCompanyNames is a minimal data.CompanyNameProvider stub for
// TestTickerLabel/TestCompanyName — a static map avoids pulling in FinMind's
// HTTP-mocking machinery for what's just Bot's TW-gating logic.
type fakeCompanyNames struct {
	names map[string]string
}

func (f fakeCompanyNames) GetCompanyName(ticker string) (string, error) {
	if name, ok := f.names[ticker]; ok {
		return name, nil
	}
	return "", fmt.Errorf("no name for %s", ticker)
}

// fakeEarnings is a minimal data.EarningsProvider stub for TestLoadEarnings.
type fakeEarnings struct {
	events map[string]data.EarningsEvent
	err    error
}

func (f fakeEarnings) GetUpcomingEarnings(tickers []string, days int) (map[string]data.EarningsEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]data.EarningsEvent)
	for _, t := range tickers {
		if e, ok := f.events[t]; ok {
			out[t] = e
		}
	}
	return out, nil
}

func (f fakeEarnings) GetEarningsInRange([]string, time.Time, time.Time) ([]data.EarningsEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	events := make([]data.EarningsEvent, 0, len(f.events))
	for _, e := range f.events {
		events = append(events, e)
	}
	return events, nil
}

func TestLoadEarningsMergesUSAndTWSources(t *testing.T) {
	b := &Bot{
		earnings: fakeEarnings{events: map[string]data.EarningsEvent{
			"AAPL": {Ticker: "AAPL", Date: "2026-08-01"},
		}},
		now: func() time.Time { return time.Date(2026, time.August, 1, 12, 0, 0, 0, cst) },
	}

	got := b.loadEarnings([]string{"AAPL", "2330"})

	aapl, ok := got["AAPL"]
	if !ok || aapl.Estimated {
		t.Errorf("loadEarnings()[AAPL] = %+v, want Finnhub's real (non-estimated) event", aapl)
	}
	tw, ok := got["2330"]
	if !ok || !tw.Estimated {
		t.Errorf("loadEarnings()[2330] = %+v, want the TW statutory-deadline proxy (Estimated=true)", tw)
	}
}

func TestLoadEarningsWorksWithoutFinnhubConfigured(t *testing.T) {
	// b.earnings is nil, same as no FINNHUB_API_KEY.
	b := &Bot{now: func() time.Time { return time.Date(2026, time.August, 1, 12, 0, 0, 0, cst) }}

	got := b.loadEarnings([]string{"2330"})

	if _, ok := got["2330"]; !ok {
		t.Error("loadEarnings() with nil b.earnings should still return the TW proxy entry")
	}
}

func TestBotTickerLabel(t *testing.T) {
	b := &Bot{companyNames: fakeCompanyNames{names: map[string]string{"2330": "台積電"}}}

	if got := b.tickerLabel("2330"); got != "台積電(2330)" {
		t.Errorf("tickerLabel(2330) = %q, want 台積電(2330)", got)
	}
	if got := b.tickerLabel("AAPL"); got != "AAPL" {
		t.Errorf("tickerLabel(AAPL) = %q, want AAPL unchanged (US ticker, no lookup)", got)
	}
	if got := b.tickerLabel("2454"); got != "2454" {
		t.Errorf("tickerLabel(2454) = %q, want bare ticker (unresolvable TW name degrades gracefully)", got)
	}

	bNoProvider := &Bot{}
	if got := bNoProvider.tickerLabel("2330"); got != "2330" {
		t.Errorf("tickerLabel(2330) with nil companyNames = %q, want bare ticker", got)
	}
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
		ticker, shares, price, fee, feeSet, date, err := parseTradeArgs("aapl 10 205.5")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "AAPL" || shares != 10 || price != 205.5 || fee != 0 || feeSet || date != "" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %v, %q; want AAPL, 10, 205.5, 0, false, \"\"", ticker, shares, price, fee, feeSet, date)
		}
	})

	t.Run("with fee", func(t *testing.T) {
		ticker, shares, price, fee, feeSet, date, err := parseTradeArgs("MSFT 5 400 1.5")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "MSFT" || shares != 5 || price != 400 || fee != 1.5 || !feeSet || date != "" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %v, %q; want MSFT, 5, 400, 1.5, true, \"\"", ticker, shares, price, fee, feeSet, date)
		}
	})

	t.Run("with date, no fee", func(t *testing.T) {
		ticker, shares, price, fee, feeSet, date, err := parseTradeArgs("AAPL 10 200 2026-01-15")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "AAPL" || shares != 10 || price != 200 || fee != 0 || feeSet || date != "2026-01-15" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %v, %q; want AAPL, 10, 200, 0, false, 2026-01-15", ticker, shares, price, fee, feeSet, date)
		}
	})

	t.Run("with fee and date, either order", func(t *testing.T) {
		for _, args := range []string{"AAPL 10 200 1.5 2026-01-15", "AAPL 10 200 2026-01-15 1.5"} {
			ticker, shares, price, fee, feeSet, date, err := parseTradeArgs(args)
			if err != nil {
				t.Fatalf("parseTradeArgs(%q) error = %v", args, err)
			}
			if ticker != "AAPL" || shares != 10 || price != 200 || fee != 1.5 || !feeSet || date != "2026-01-15" {
				t.Errorf("parseTradeArgs(%q) = %q, %v, %v, %v, %v, %q; want AAPL, 10, 200, 1.5, true, 2026-01-15", args, ticker, shares, price, fee, feeSet, date)
			}
		}
	})

	// Phase 13 §11.3: an explicit 0 must be distinguishable from an omitted
	// fee, since only the latter triggers TW's fee auto-calc.
	t.Run("explicit zero fee is feeSet, unlike an omitted one", func(t *testing.T) {
		_, _, _, fee, feeSet, _, err := parseTradeArgs("2330 1000 100")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if fee != 0 || feeSet {
			t.Errorf("parseTradeArgs(no fee) = fee %v, feeSet %v; want 0, false", fee, feeSet)
		}

		_, _, _, fee, feeSet, _, err = parseTradeArgs("2330 1000 100 0")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if fee != 0 || !feeSet {
			t.Errorf("parseTradeArgs(fee=0) = fee %v, feeSet %v; want 0, true", fee, feeSet)
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
		if _, _, _, _, _, _, err := parseTradeArgs(args); err == nil {
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

func TestParseCashArgs(t *testing.T) {
	t.Run("bare amount defaults to USD", func(t *testing.T) {
		m, amount, err := parseCashArgs("1000")
		if err != nil || m != market.US || amount != 1000 {
			t.Errorf("parseCashArgs(1000) = %v, %v, %v; want us, 1000, nil", m, amount, err)
		}
	})

	t.Run("explicit usd", func(t *testing.T) {
		m, amount, err := parseCashArgs("usd 500.50")
		if err != nil || m != market.US || amount != 500.50 {
			t.Errorf("parseCashArgs(usd 500.50) = %v, %v, %v; want us, 500.5, nil", m, amount, err)
		}
	})

	t.Run("explicit twd is case-insensitive", func(t *testing.T) {
		m, amount, err := parseCashArgs("TWD 30000")
		if err != nil || m != market.TW || amount != 30000 {
			t.Errorf("parseCashArgs(TWD 30000) = %v, %v, %v; want tw, 30000, nil", m, amount, err)
		}
	})

	for _, args := range []string{"", "gbp 100", "usd", "usd 100 extra", "usd -5", "usd abc"} {
		if _, _, err := parseCashArgs(args); err == nil {
			t.Errorf("parseCashArgs(%q) error = nil, want error", args)
		}
	}
}

func TestLotSuffix(t *testing.T) {
	cases := []struct {
		name   string
		m      market.MarketID
		shares float64
		want   string
	}{
		{"US never gets a lot note", market.US, 1000, ""},
		{"TW under one lot", market.TW, 500, ""},
		{"TW exact multiple of a lot", market.TW, 2000, i18n.T(i18n.EN, i18n.KeyPortfolioLotSuffix, 2)},
		{"TW non-round share count", market.TW, 1500, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lotSuffix(i18n.EN, c.m, c.shares); got != c.want {
				t.Errorf("lotSuffix(%s, %v) = %q, want %q", c.m, c.shares, got, c.want)
			}
		})
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
		if !strings.Contains(out, "AAPL") || !strings.Contains(out, "$210") {
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

// TestUniverseScanChunkFullCoverage/TestUniverseScanChunkEmptyAndNegativeDay
// moved to internal/service/scan_test.go (Phase 24 Stage 1) along with
// universeScanChunk itself — see service.UniverseScanChunk.

// suggestShares/trailingStopThreshold moved to internal/paper (Phase 11
// PR1, see paper.SuggestShares/paper.TrailingStopThreshold) — their tests
// moved with them to internal/paper/paper_test.go.
//
// TestBreachAlertDecision/TestStopBreachDecision/TestTargetReachedDecision
// moved to internal/service/risk_test.go (Phase 24 tech debt 1) — the
// bot-package wrapper functions they tested were orphan copies of
// service.BreachAlertDecision/StopBreachDecision/TargetReachedDecision left
// behind by Stage 1.1's RiskService extraction, unused by anything except
// these tests; deleting the wrappers moved the tests to where the real
// implementation lives instead of dropping the edge-case coverage.
//
// TestRestrictedAlertDecision moved to internal/service/risk_test.go the
// same way (Phase 24 tech debt 2) — restrictedAlertDecision itself moved
// into RiskService.EvaluateRestrictedAlerts.

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
			got := computeVsSPY(tt.currentPrice, tt.avgCost, tt.spyPrice, tt.spyEntry, "SPY")
			if !floatsClose(got.TickerPct, tt.wantTickerPct) || !floatsClose(got.SPYPct, tt.wantSPYPct) || got.Bench != "SPY" {
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

	overall, bySource, _ := summarizeTrack(rows)

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
	if got := renderTrackSummary(i18n.EN, trackSourceStats{}, nil, nil); got != "" {
		t.Errorf("renderTrackSummary() with Evaluated=0 = %q, want \"\"", got)
	}
}

func TestRenderTrackSummaryOmitsBySourceBreakdownForSingleSource(t *testing.T) {
	overall := trackSourceStats{Hits: 3, Evaluated: 4, BuySum: 12, BuyCount: 3}
	bySource := map[string]trackSourceStats{"watchlist": overall}

	got := renderTrackSummary(i18n.EN, overall, bySource, nil)

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

	got := renderTrackSummary(i18n.EN, overall, bySource, nil)

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

func TestRenderEarningsPreviewEstimatedWording(t *testing.T) {
	in3Days := time.Now().In(cst).AddDate(0, 0, 3).Format("2006-01-02")
	earnings := map[string]data.EarningsEvent{
		"2330": {Ticker: "2330", Date: in3Days, Estimated: true},
	}
	got := renderEarningsPreview(i18n.EN, earnings, 7)
	if !strings.Contains(got, "statutory filing deadline") {
		t.Errorf("renderEarningsPreview() missing estimated wording for a TW proxy entry, got:\n%s", got)
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

	msg, closed, stopPrice, err := b.recordSell("AAPL", 4, 220, 0, false, "2026-06-10", "")
	if err != nil {
		t.Errorf("recordSell() error = %v, want nil for a valid partial sell", err)
	}
	if closed {
		t.Errorf("recordSell() closed = true for a partial sell, want false; msg = %q", msg)
	}
	if stopPrice != 180 {
		t.Errorf("recordSell() stopPrice = %v, want 180 (read before the sell)", stopPrice)
	}

	msg, closed, stopPrice, err = b.recordSell("AAPL", 6, 230, 0, false, "2026-06-20", "")
	if err != nil {
		t.Errorf("recordSell() error = %v, want nil for a valid closing sell", err)
	}
	if !closed {
		t.Errorf("recordSell() closed = false for a sell down to 0 shares, want true; msg = %q", msg)
	}
	if stopPrice != 180 {
		t.Errorf("recordSell() stopPrice = %v, want 180 for the sell that fully closed the position", stopPrice)
	}

	msg, closed, stopPrice, err = b.recordSell("AAPL", 1, 200, 0, false, "2026-06-21", "")
	if err == nil {
		t.Errorf("recordSell() error = nil, want ErrNoPosition (no position left)")
	}
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

func TestParseRecommendMarketArg(t *testing.T) {
	cases := []struct {
		args string
		want []market.MarketID
		ok   bool
	}{
		{"", []market.MarketID{market.US, market.TW}, true},
		{"  ", []market.MarketID{market.US, market.TW}, true},
		{"us", []market.MarketID{market.US}, true},
		{"US", []market.MarketID{market.US}, true},
		{"tw", []market.MarketID{market.TW}, true},
		{"TW", []market.MarketID{market.TW}, true},
		{"jp", nil, false},
	}
	for _, c := range cases {
		got, ok := parseRecommendMarketArg(c.args)
		if ok != c.ok {
			t.Errorf("parseRecommendMarketArg(%q) ok = %v, want %v", c.args, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseRecommendMarketArg(%q) = %v, want %v", c.args, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseRecommendMarketArg(%q) = %v, want %v", c.args, got, c.want)
				break
			}
		}
	}
}

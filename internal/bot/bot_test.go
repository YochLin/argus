package bot

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestRecommendationSources(t *testing.T) {
	watchlist := []string{"AAPL", "MSFT"}
	// MSFT also appears as a candidate — shouldn't happen in practice since
	// mergeCandidates already excludes watchlist tickers, but recommendationSources
	// guards it anyway: watchlist attribution must win regardless.
	candidates := []string{"MSFT", "NVDA", "TSLA"}
	scanHits := map[string]string{
		"NVDA": "RSI oversold (28.0)",
	}

	got := recommendationSources(watchlist, candidates, scanHits)

	want := map[string]string{
		"AAPL": "watchlist",
		"MSFT": "watchlist",
		"NVDA": "scan",
		"TSLA": "movers",
	}
	for ticker, wantSource := range want {
		if got[ticker] != wantSource {
			t.Errorf("recommendationSources()[%s] = %q, want %q", ticker, got[ticker], wantSource)
		}
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

func TestWriteRecGroup(t *testing.T) {
	t.Run("empty recs writes nothing, not even the title", func(t *testing.T) {
		var sb strings.Builder
		writeRecGroup(&sb, i18n.EN, i18n.KeyRecWatchlistSectionTitle, nil)
		if sb.String() != "" {
			t.Errorf("writeRecGroup() = %q, want empty", sb.String())
		}
	})

	t.Run("numbers from 1 within the group and includes the title", func(t *testing.T) {
		var sb strings.Builder
		recs := []llm.Recommendation{
			{Ticker: "AAPL", Action: "HOLD", Reason: "fairly valued."},
			{Ticker: "MSFT", Action: "BUY", Reason: "cloud growth."},
		}
		writeRecGroup(&sb, i18n.EN, i18n.KeyRecWatchlistSectionTitle, recs)
		got := sb.String()
		want := i18n.T(i18n.EN, i18n.KeyRecWatchlistSectionTitle) +
			"1. *AAPL* — HOLD\nfairly valued.\n\n" +
			"2. *MSFT* — BUY\ncloud growth.\n\n"
		if got != want {
			t.Errorf("writeRecGroup() = %q, want %q", got, want)
		}
	})

	t.Run("empty action omits the action separator", func(t *testing.T) {
		var sb strings.Builder
		recs := []llm.Recommendation{{Ticker: "AAPL", Reason: "no action line."}}
		writeRecGroup(&sb, i18n.EN, i18n.KeyRecCandidatesSectionTitle, recs)
		got := sb.String()
		want := i18n.T(i18n.EN, i18n.KeyRecCandidatesSectionTitle) + "1. *AAPL*\nno action line.\n\n"
		if got != want {
			t.Errorf("writeRecGroup() = %q, want %q", got, want)
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

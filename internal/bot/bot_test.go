package bot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
)

func TestCommaf(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{4321020, "4,321,020"},
		{1234567.89, "1,234,568"}, // rounds to nearest integer
		{-1234567, "-1,234,567"},
		{-500, "-500"},
		{100, "100"},
	}
	for _, tt := range tests {
		if got := commaf(tt.in); got != tt.want {
			t.Errorf("commaf(%v) = %q, want %q", tt.in, got, tt.want)
		}
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

func TestFormatFundamentals(t *testing.T) {
	fd := &data.Fundamentals{
		Ticker:           "AAPL",
		PE:               28.5,
		MarketCapMillion: 3200000,
	}
	out := formatFundamentals(i18n.EN, fd)
	if !strings.Contains(out, "3,200,000") {
		t.Errorf("formatFundamentals() = %q, want market cap formatted with thousands separators", out)
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

func TestFormatFinancialStatement(t *testing.T) {
	st := &data.FinancialStatement{
		Form:       "10-K",
		FiscalYear: 2025,
		PeriodEnd:  "2025-09-27",
		Revenue:    391035000000,
		NetIncome:  93736000000,
	}
	out := formatFinancialStatement(i18n.EN, st)
	if !strings.Contains(out, "10-K") || !strings.Contains(out, "2025-09-27") {
		t.Errorf("formatFinancialStatement() = %q, want it to contain form and period end", out)
	}
	// Revenue/NetIncome are formatted in millions with thousands separators.
	if !strings.Contains(out, "391,035") {
		t.Errorf("formatFinancialStatement() = %q, want revenue formatted in millions", out)
	}
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

package bot

import (
	"strings"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
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

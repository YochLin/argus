package llm

import (
	"strings"
	"testing"

	"argus/internal/data"
	"argus/internal/i18n"
)

func TestBuildInsightPromptTotalValueAndCashLine(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
		{Quote: &data.Quote{Ticker: "MSFT", Price: 300}, Position: &Position{Shares: 5, AvgCost: 250}},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 500, true)

	// Total position value: 10*200 + 5*300 = 3500; grand total with cash: 4000.
	if !strings.Contains(prompt, "3500.00") {
		t.Errorf("buildInsightPrompt() missing total position value 3500.00, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "500.00") {
		t.Errorf("buildInsightPrompt() missing cash balance 500.00, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "4000.00") {
		t.Errorf("buildInsightPrompt() missing grand total 4000.00, got:\n%s", prompt)
	}
	for _, want := range []string{"AAPL", "MSFT"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildInsightPrompt() missing ticker %q, got:\n%s", want, prompt)
		}
	}
}

func TestBuildInsightPromptOmitsCashLineWhenUnset(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 0, false)

	if strings.Contains(prompt, "Cash balance") {
		t.Errorf("buildInsightPrompt() with haveCash=false should omit the cash line entirely, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "2000.00") {
		t.Errorf("buildInsightPrompt() missing total position value 2000.00, got:\n%s", prompt)
	}
}

func TestBuildInsightPromptSkipsValueForPositionWithoutQuote(t *testing.T) {
	// A position with a nil Quote shouldn't happen in practice (writeStockSection
	// itself early-returns without one), but the total-value accumulation
	// guards against it explicitly, so make sure it doesn't panic or miscount.
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
		{Quote: nil, Position: &Position{Shares: 5, AvgCost: 100}},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 0, false)

	if !strings.Contains(prompt, "2000.00") {
		t.Errorf("buildInsightPrompt() missing total position value 2000.00 (should ignore the quote-less position), got:\n%s", prompt)
	}
}

func TestBuildInsightPromptRendersThesisAndVsSPY(t *testing.T) {
	thesis := "long-term compounder, services growth"
	positions := []StockData{
		{
			Quote:    &data.Quote{Ticker: "AAPL", Price: 200},
			Position: &Position{Shares: 10, AvgCost: 150},
			Thesis:   &thesis,
			VsSPY:    &VsSPYReturn{TickerPct: 33.3, SPYPct: 10.0},
		},
	}

	prompt := buildInsightPrompt(i18n.EN, positions, 0, false)

	if !strings.Contains(prompt, thesis) {
		t.Errorf("buildInsightPrompt() missing thesis text, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "+33.3%") || !strings.Contains(prompt, "+10.0%") {
		t.Errorf("buildInsightPrompt() missing vs-SPY percentages, got:\n%s", prompt)
	}
}

func TestBuildWeeklyReviewPromptIncludesTrackSummaryWhenPresent(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
	}

	prompt := buildWeeklyReviewPrompt(i18n.EN, positions, 0, false, "Hit rate: 3/5 (60%)")

	if !strings.Contains(prompt, "Hit rate: 3/5 (60%)") {
		t.Errorf("buildWeeklyReviewPrompt() missing track summary text, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "This week's recommendation tracking") {
		t.Errorf("buildWeeklyReviewPrompt() missing track summary header, got:\n%s", prompt)
	}
}

func TestBuildWeeklyReviewPromptOmitsTrackSectionWhenEmpty(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
	}

	prompt := buildWeeklyReviewPrompt(i18n.EN, positions, 0, false, "")

	if strings.Contains(prompt, "This week's recommendation tracking") {
		t.Errorf("buildWeeklyReviewPrompt() with empty trackSummary should omit the track section entirely, got:\n%s", prompt)
	}
}

func TestBuildWeeklyReviewPromptTotalValueAndCashLine(t *testing.T) {
	positions := []StockData{
		{Quote: &data.Quote{Ticker: "AAPL", Price: 200}, Position: &Position{Shares: 10, AvgCost: 150}},
		{Quote: &data.Quote{Ticker: "MSFT", Price: 300}, Position: &Position{Shares: 5, AvgCost: 250}},
	}

	prompt := buildWeeklyReviewPrompt(i18n.EN, positions, 500, true, "")

	// Total position value: 10*200 + 5*300 = 3500; grand total with cash: 4000.
	if !strings.Contains(prompt, "3500.00") || !strings.Contains(prompt, "500.00") || !strings.Contains(prompt, "4000.00") {
		t.Errorf("buildWeeklyReviewPrompt() missing total/cash/grand-total values, got:\n%s", prompt)
	}
}

func TestWriteStockSectionOmitsThesisAndVsSPYWhenNil(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{Quote: &data.Quote{Ticker: "AAPL", Price: 200}})

	got := sb.String()
	if strings.Contains(got, "Holding thesis") || strings.Contains(got, "vs. market") {
		t.Errorf("writeStockSection() should omit thesis/vs-SPY lines when both are nil, got:\n%s", got)
	}
}

func TestBuildTradeReviewPromptMinimal(t *testing.T) {
	trade := ClosedTrade{
		Ticker: "AAPL",
		Legs: []TradeLeg{
			{Side: "BUY", Shares: 10, Price: 150, Date: "2026-06-01"},
			{Side: "SELL", Shares: 10, Price: 180, Date: "2026-06-20"},
		},
		RealizedPnL: 300,
		HoldingDays: 19,
	}

	prompt := buildTradeReviewPrompt(i18n.EN, trade)

	if !strings.Contains(prompt, "AAPL") {
		t.Errorf("buildTradeReviewPrompt() missing ticker, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "BUY 10") || !strings.Contains(prompt, "SELL 10") {
		t.Errorf("buildTradeReviewPrompt() missing leg lines, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "+300.00") || !strings.Contains(prompt, "19 days") {
		t.Errorf("buildTradeReviewPrompt() missing P&L/holding-days line, got:\n%s", prompt)
	}
	for _, absent := range []string{"Price range", "vs. market", "Holding thesis", "Recommendations during"} {
		if strings.Contains(prompt, absent) {
			t.Errorf("buildTradeReviewPrompt() should omit %q when no data is set, got:\n%s", absent, prompt)
		}
	}
}

func TestBuildTradeReviewPromptFull(t *testing.T) {
	thesis := "long-term compounder"
	trade := ClosedTrade{
		Ticker: "AAPL",
		Legs: []TradeLeg{
			{Side: "BUY", Shares: 10, Price: 150, Date: "2026-06-01"},
			{Side: "SELL", Shares: 10, Price: 180, Date: "2026-06-20"},
		},
		RealizedPnL: 300,
		HoldingDays: 19,
		VsSPY:       &VsSPYReturn{TickerPct: 20, SPYPct: 5},
		PeriodHigh:  195,
		PeriodLow:   145,
		Thesis:      &thesis,
		Recommendations: []TradeRecommendation{
			{Date: "2026-06-15", Action: "HOLD", Reason: "still above support"},
		},
	}

	prompt := buildTradeReviewPrompt(i18n.EN, trade)

	for _, want := range []string{"195.00", "145.00", "+20.0%", "+5.0%", thesis, "HOLD", "still above support"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildTradeReviewPrompt() missing %q, got:\n%s", want, prompt)
		}
	}
}

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

func TestWriteStockSectionOmitsThesisAndVsSPYWhenNil(t *testing.T) {
	var sb strings.Builder
	writeStockSection(&sb, i18n.EN, StockData{Quote: &data.Quote{Ticker: "AAPL", Price: 200}})

	got := sb.String()
	if strings.Contains(got, "Holding thesis") || strings.Contains(got, "vs. market") {
		t.Errorf("writeStockSection() should omit thesis/vs-SPY lines when both are nil, got:\n%s", got)
	}
}

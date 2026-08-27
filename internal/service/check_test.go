package service

import (
	"errors"
	"testing"

	"argus/internal/data"
)

type mockFundamentalsProvider struct {
	fundamentals map[string]*data.Fundamentals
	statements   map[string]*data.FinancialStatement
	err          error
}

func (m *mockFundamentalsProvider) GetFundamentals(ticker string) (*data.Fundamentals, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fundamentals[ticker], nil
}

func (m *mockFundamentalsProvider) GetFinancialStatements(ticker, freq string) (*data.FinancialStatement, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.statements[ticker], nil
}

type mockAnalystRatingProvider struct {
	ratings map[string]*data.AnalystRating
	err     error
}

func (m *mockAnalystRatingProvider) GetAnalystRating(ticker string) (*data.AnalystRating, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ratings[ticker], nil
}

func TestCheckStockDataReturnsErrorOnQuoteFailure(t *testing.T) {
	quotes := &mockBriefingQuotes{quoteErrTickers: map[string]bool{"AAPL": true}}

	_, err := CheckStockData(quotes, nil, nil, nil, "AAPL")
	if err == nil {
		t.Fatal("CheckStockData() error = nil, want the quote error")
	}
}

func TestCheckStockDataAttachesFundamentalsAndAnalystRating(t *testing.T) {
	quotes := &mockBriefingQuotes{
		quotes: map[string]*data.Quote{"AAPL": {Ticker: "AAPL", Price: 200}},
		news:   map[string][]data.NewsItem{"AAPL": {{Headline: "AAPL news"}}},
	}
	fundamentals := &mockFundamentalsProvider{
		fundamentals: map[string]*data.Fundamentals{"AAPL": {PE: 30}},
		statements:   map[string]*data.FinancialStatement{"AAPL": {}},
	}
	analystRating := &mockAnalystRatingProvider{
		ratings: map[string]*data.AnalystRating{"AAPL": {StrongBuy: 10}},
	}

	stock, err := CheckStockData(quotes, nil, fundamentals, analystRating, "AAPL")
	if err != nil {
		t.Fatalf("CheckStockData() error = %v", err)
	}
	if stock.Quote == nil || stock.Quote.Price != 200 {
		t.Errorf("Quote = %+v, want AAPL @ 200", stock.Quote)
	}
	if len(stock.News) != 1 {
		t.Errorf("News = %+v, want 1 item", stock.News)
	}
	if stock.Fundamentals == nil || stock.Fundamentals.PE != 30 {
		t.Errorf("Fundamentals = %+v, want PE=30", stock.Fundamentals)
	}
	if stock.Statement == nil {
		t.Error("Statement = nil, want attached")
	}
	if stock.AnalystRating == nil || stock.AnalystRating.StrongBuy != 10 {
		t.Errorf("AnalystRating = %+v, want StrongBuy=10", stock.AnalystRating)
	}
}

func TestCheckStockDataDegradesWithNilProviders(t *testing.T) {
	quotes := &mockBriefingQuotes{quotes: map[string]*data.Quote{"AAPL": {Ticker: "AAPL", Price: 200}}}

	stock, err := CheckStockData(quotes, nil, nil, nil, "AAPL")
	if err != nil {
		t.Fatalf("CheckStockData() error = %v", err)
	}
	if stock.Fundamentals != nil || stock.Statement != nil || stock.AnalystRating != nil {
		t.Errorf("stock = %+v, want no fundamentals/statement/analyst rating with nil providers", stock)
	}
}

func TestCheckStockDataLogsAndSkipsOnFundamentalsError(t *testing.T) {
	quotes := &mockBriefingQuotes{quotes: map[string]*data.Quote{"AAPL": {Ticker: "AAPL", Price: 200}}}
	fundamentals := &mockFundamentalsProvider{err: errors.New("finnhub down")}
	analystRating := &mockAnalystRatingProvider{err: errors.New("finnhub down")}

	stock, err := CheckStockData(quotes, nil, fundamentals, analystRating, "AAPL")
	if err != nil {
		t.Fatalf("CheckStockData() error = %v, want nil (a fundamentals failure degrades, doesn't fail the call)", err)
	}
	if stock.Fundamentals != nil || stock.Statement != nil || stock.AnalystRating != nil {
		t.Errorf("stock = %+v, want no fundamentals/statement/analyst rating on provider error", stock)
	}
}

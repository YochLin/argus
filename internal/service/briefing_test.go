package service

import (
	"errors"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
)

type mockBriefingQuotes struct {
	quotes          map[string]*data.Quote
	news            map[string][]data.NewsItem
	quoteErrTickers map[string]bool
}

func (m *mockBriefingQuotes) GetQuote(ticker string) (*data.Quote, error) {
	if m.quoteErrTickers[ticker] {
		return nil, errors.New("quote unavailable")
	}
	q, ok := m.quotes[ticker]
	if !ok {
		return nil, errors.New("no quote")
	}
	return q, nil
}

func (m *mockBriefingQuotes) GetNews(ticker string, limit int) ([]data.NewsItem, error) {
	return m.news[ticker], nil
}

func TestFetchIndexQuotesSkipsFailedTickers(t *testing.T) {
	quotes := &mockBriefingQuotes{
		quotes:          map[string]*data.Quote{"SPY": {Price: 500, ChangePercent: 1.2}},
		quoteErrTickers: map[string]bool{"QQQ": true},
	}
	idx := []IndexProxy{{Ticker: "SPY", Label: "S&P 500"}, {Ticker: "QQQ", Label: "Nasdaq"}}

	got := FetchIndexQuotes(quotes, idx)
	if len(got) != 1 || got[0].Label != "S&P 500" || got[0].Price != 500 {
		t.Errorf("FetchIndexQuotes() = %+v, want just the SPY entry", got)
	}
}

func TestLoadQuoteHighlightsSkipsFailedQuotesAndAttachesPosition(t *testing.T) {
	quotes := &mockBriefingQuotes{
		quotes: map[string]*data.Quote{
			"AAPL": {Ticker: "AAPL", Price: 200},
			"2330": {Ticker: "2330", Price: 1000},
		},
		quoteErrTickers: map[string]bool{"MISSING": true},
	}
	positions := map[string]db.Position{"AAPL": {Ticker: "AAPL", Shares: 10, AvgCost: 150}}

	got := LoadQuoteHighlights(quotes, nil, []string{"AAPL", "MISSING", "2330"}, positions)
	if len(got) != 2 {
		t.Fatalf("LoadQuoteHighlights() returned %d entries, want 2 (MISSING skipped)", len(got))
	}
	if got[0].Position == nil || got[0].Position.Shares != 10 {
		t.Errorf("AAPL entry Position = %+v, want Shares=10", got[0].Position)
	}
	if got[1].Position != nil {
		t.Errorf("2330 entry Position = %+v, want nil (no held position)", got[1].Position)
	}
}

func TestCompanyNameForSkipsUSTickersAndNilProvider(t *testing.T) {
	if got := companyNameFor(nil, "2330"); got != "" {
		t.Errorf("companyNameFor(nil, 2330) = %q, want empty (nil provider)", got)
	}
	if got := companyNameFor(&fakeCompanyNames{names: map[string]string{"AAPL": "should not be looked up"}}, "AAPL"); got != "" {
		t.Errorf("companyNameFor(_, AAPL) = %q, want empty (US ticker, no lookup)", got)
	}
}

type fakeCompanyNames struct {
	names map[string]string
}

func (f *fakeCompanyNames) GetCompanyName(ticker string) (string, error) {
	name, ok := f.names[ticker]
	if !ok {
		return "", errors.New("not found")
	}
	return name, nil
}

func TestCompanyNameForResolvesTWTicker(t *testing.T) {
	names := &fakeCompanyNames{names: map[string]string{"2330": "台積電"}}
	if got := companyNameFor(names, "2330"); got != "台積電" {
		t.Errorf("companyNameFor(_, 2330) = %q, want 台積電", got)
	}
}

func TestLoadQuoteHighlightsDedupesNewsAcrossTickers(t *testing.T) {
	quotes := &mockBriefingQuotes{
		quotes: map[string]*data.Quote{
			"AAPL": {Ticker: "AAPL", Price: 200},
			"MSFT": {Ticker: "MSFT", Price: 300},
		},
		news: map[string][]data.NewsItem{
			"AAPL": {{Headline: "The Deep Unknowns Of AI"}},
			"MSFT": {{Headline: "The Deep Unknowns Of AI"}, {Headline: "MSFT earnings beat"}},
		},
	}

	got := LoadQuoteHighlights(quotes, nil, []string{"AAPL", "MSFT"}, nil)
	if len(got) != 2 {
		t.Fatalf("LoadQuoteHighlights() returned %d entries, want 2", len(got))
	}
	if len(got[0].News) != 1 {
		t.Fatalf("AAPL News = %+v, want 1 item", got[0].News)
	}
	if len(got[1].News) != 1 || got[1].News[0].Headline != "MSFT earnings beat" {
		t.Errorf("MSFT News = %+v, want just the distinct story (shared headline already used by AAPL)", got[1].News)
	}
}

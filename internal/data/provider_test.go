package data

import (
	"errors"
	"testing"
)

// fakeProvider is a minimal Provider double for exercising Multi's
// fallback-on-error behavior without hitting a real API.
type fakeProvider struct {
	name string

	quote    *Quote
	quoteErr error

	news    []NewsItem
	newsErr error

	movers    []string
	moversErr error

	calls int // number of times any method was called, to assert short-circuiting
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) GetQuote(ticker string) (*Quote, error) {
	f.calls++
	return f.quote, f.quoteErr
}

func (f *fakeProvider) GetNews(ticker string, limit int) ([]NewsItem, error) {
	f.calls++
	return f.news, f.newsErr
}

func (f *fakeProvider) GetMarketMovers() ([]string, error) {
	f.calls++
	return f.movers, f.moversErr
}

func TestMultiGetQuote(t *testing.T) {
	t.Run("returns first provider's result when it succeeds", func(t *testing.T) {
		primary := &fakeProvider{name: "primary", quote: &Quote{Ticker: "AAPL", Price: 200}}
		fallback := &fakeProvider{name: "fallback", quote: &Quote{Ticker: "AAPL", Price: 999}}
		m := NewMulti(primary, fallback)

		q, err := m.GetQuote("AAPL")
		if err != nil {
			t.Fatalf("GetQuote() error = %v", err)
		}
		if q.Price != 200 {
			t.Errorf("GetQuote() = %+v, want primary's quote", q)
		}
		if fallback.calls != 0 {
			t.Errorf("fallback provider was called %d times, want 0 (should short-circuit)", fallback.calls)
		}
	})

	t.Run("falls back to next provider on error", func(t *testing.T) {
		primary := &fakeProvider{name: "primary", quoteErr: errors.New("primary down")}
		fallback := &fakeProvider{name: "fallback", quote: &Quote{Ticker: "AAPL", Price: 200}}
		m := NewMulti(primary, fallback)

		q, err := m.GetQuote("AAPL")
		if err != nil {
			t.Fatalf("GetQuote() error = %v", err)
		}
		if q.Price != 200 {
			t.Errorf("GetQuote() = %+v, want fallback's quote", q)
		}
	})

	t.Run("returns the last error when every provider fails", func(t *testing.T) {
		primary := &fakeProvider{name: "primary", quoteErr: errors.New("primary down")}
		fallback := &fakeProvider{name: "fallback", quoteErr: errors.New("fallback down")}
		m := NewMulti(primary, fallback)

		_, err := m.GetQuote("AAPL")
		if err == nil || err.Error() != "fallback down" {
			t.Errorf("GetQuote() error = %v, want %q", err, "fallback down")
		}
	})
}

func TestMultiGetNews(t *testing.T) {
	primary := &fakeProvider{name: "primary", newsErr: errors.New("primary down")}
	fallback := &fakeProvider{name: "fallback", news: []NewsItem{{Headline: "ok"}}}
	m := NewMulti(primary, fallback)

	items, err := m.GetNews("AAPL", 5)
	if err != nil {
		t.Fatalf("GetNews() error = %v", err)
	}
	if len(items) != 1 || items[0].Headline != "ok" {
		t.Errorf("GetNews() = %+v, want fallback's items", items)
	}
}

func TestMultiGetMarketMovers(t *testing.T) {
	primary := &fakeProvider{name: "primary", moversErr: errors.New("primary down")}
	fallback := &fakeProvider{name: "fallback", movers: []string{"AAPL", "MSFT"}}
	m := NewMulti(primary, fallback)

	tickers, err := m.GetMarketMovers()
	if err != nil {
		t.Fatalf("GetMarketMovers() error = %v", err)
	}
	if len(tickers) != 2 || tickers[0] != "AAPL" {
		t.Errorf("GetMarketMovers() = %v, want fallback's tickers", tickers)
	}
}

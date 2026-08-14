package web

import (
	"sync"
	"time"

	"argus/internal/data"
	"argus/internal/logger"
)

// quoteCacheTTL mirrors internal/mcptools' quoteCacheTTL in spirit (live
// quotes are cheap to serve stale for a few seconds), but is duplicated
// here rather than imported — mcptools' cache/rate-limit types are all
// unexported and its own package-boundary convention is to keep its
// dependency graph narrow, not to be imported by other long-running
// processes. Unlike mcptools (which rate-limits because a chat session can
// call it repeatedly), a dashboard page load only fetches each held
// ticker's quote once, so there's no token-bucket here — just the cache.
const quoteCacheTTL = 30 * time.Second

// quoteGetter is the subset of data.Provider the dashboard needs — narrow
// enough that both *quoteCache and a bare data.Provider satisfy it, and
// easy to fake in tests.
type quoteGetter interface {
	GetQuote(ticker string) (*data.Quote, error)
}

type cachedQuote struct {
	quote     *data.Quote
	expiresAt time.Time
}

// quoteCache is a small TTL cache around a data.Provider's GetQuote, used
// by the positions list so repeated dashboard page loads within the TTL
// window don't each burn a live API call per held ticker.
type quoteCache struct {
	provider data.Provider

	mu    sync.Mutex
	cache map[string]cachedQuote
}

func newQuoteCache(provider data.Provider) *quoteCache {
	return &quoteCache{provider: provider, cache: make(map[string]cachedQuote)}
}

func (c *quoteCache) GetQuote(ticker string) (*data.Quote, error) {
	c.mu.Lock()
	if cached, ok := c.cache[ticker]; ok && time.Now().Before(cached.expiresAt) {
		c.mu.Unlock()
		return cached.quote, nil
	}
	c.mu.Unlock()

	q, err := c.provider.GetQuote(ticker)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[ticker] = cachedQuote{quote: q, expiresAt: time.Now().Add(quoteCacheTTL)}
	c.mu.Unlock()
	return q, nil
}

// fetchQuotes resolves tickers' quotes concurrently instead of one round
// trip at a time — a page with N held positions used to take N sequential
// GetQuote calls (each a real Finnhub/Yahoo HTTP round trip) to render;
// fetching them in parallel collapses that to ~1 round trip regardless of N.
// A ticker whose fetch fails is simply absent from the result (logged under
// label, e.g. "dashboard"/"risk", matching each call site's prior log
// prefix) — same "attach what's available" degrade every caller already
// used for a single failed quote. nil quotes (test doubles that don't wire
// one up) or an empty tickers list short-circuits to an empty map.
func fetchQuotes(quotes quoteGetter, tickers []string, label string) map[string]*data.Quote {
	result := make(map[string]*data.Quote, len(tickers))
	if quotes == nil || len(tickers) == 0 {
		return result
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range tickers {
		wg.Add(1)
		go func(ticker string) {
			defer wg.Done()
			q, err := quotes.GetQuote(ticker)
			if err != nil {
				logger.Errorf("web: %s: get quote for %s: %v", label, ticker, err)
				return
			}
			mu.Lock()
			result[ticker] = q
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	return result
}

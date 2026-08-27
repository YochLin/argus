package service

import (
	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/llm"
	"argus/internal/logger"
	"argus/internal/market"
)

// IndexProxy is one ETF-proxy ticker/label pair for a morning briefing's
// broad-index summary — there's no direct index quote in data.Provider, but
// a liquid ETF (SPY/QQQ/DIA/IWM, 0050/0051) tracks its index closely enough
// for a narrative recap (not a precise index-point figure). See
// internal/bot's usIndices/twIndices for the concrete lists, including why
// TW uses ETF proxies too rather than raw ^TWII/^TWOII index symbols.
type IndexProxy struct {
	Ticker string
	Label  string
}

// FetchIndexQuotes fetches each proxy's current quote, skipping (logging)
// any that fail rather than failing the whole briefing — same per-field-
// degrades convention as ComputeMarketRegime's own VIX fetch.
func FetchIndexQuotes(quotes QuoteReader, idx []IndexProxy) []llm.IndexQuote {
	var out []llm.IndexQuote
	for _, i := range idx {
		q, err := quotes.GetQuote(i.Ticker)
		if err != nil {
			logger.Errorf("morning briefing: %s quote: %v", i.Ticker, err)
			continue
		}
		out = append(out, llm.IndexQuote{Label: i.Label, Price: q.Price, ChangePercent: q.ChangePercent})
	}
	return out
}

// companyNameFor resolves ticker's display name — "" (not an error) for a US
// ticker (already human-readable, no lookup needed), a nil provider
// (FINMIND_TOKEN unset), or a failed lookup, matching bot.companyName's
// contract so every caller can treat "" as "no name available" without a
// separate error check.
func companyNameFor(names data.CompanyNameProvider, ticker string) string {
	if names == nil || market.Of(ticker) != market.TW {
		return ""
	}
	name, err := names.GetCompanyName(ticker)
	if err != nil {
		return ""
	}
	return name
}

// QuoteNewsReader is the data-source boundary LoadQuoteHighlights needs —
// narrowed to the two reads it performs, same convention as QuoteReader.
type QuoteNewsReader interface {
	QuoteReader
	GetNews(ticker string, limit int) ([]data.NewsItem, error)
}

// briefingNewsFetch/briefingNewsSlots are LoadQuoteHighlights' fetch/slot
// widths — fetch matches the per-ticker prompt's own tickerNewsFetch (see
// bot.tickerNewsFetch) so the newsPicker dedup has the same room to refill a
// skipped duplicate; slots is 3 rather than 5 because the briefing covers
// the watchlist and ~20 movers in one narrative recap, not a per-ticker
// decision.
const (
	briefingNewsFetch = 10
	briefingNewsSlots = 3
)

// LoadQuoteHighlights is a lighter sibling of the daily report's full stock
// data assembly, for a morning briefing's watchlist/mover sections: quote +
// a few news items + company name + position (if held), deliberately
// skipping a technicals/history fetch — a narrative recap doesn't need
// RSI/MACD/candles, and fetching full technicals for both the watchlist and
// ~20 mover tickers every morning would double the daily history-fetch
// volume for no benefit here. The caller's llm rendering degrades safely
// regardless (see llm.StockData's per-field degradation convention). News is
// run through one newsPicker for the whole call so a story repeated across
// tickers (a market-wide piece Finnhub tags onto several symbols) only fills
// one slot, not one per ticker — same dedup the per-ticker prompt sections
// use, see newsPicker.
func LoadQuoteHighlights(quotes QuoteNewsReader, names data.CompanyNameProvider, tickers []string, positions map[string]db.Position) []llm.StockData {
	var result []llm.StockData
	picker := &newsPicker{}
	for _, t := range tickers {
		q, err := quotes.GetQuote(t)
		if err != nil {
			logger.Errorf("morning briefing: quote %s: %v", t, err)
			continue
		}
		fetched, _ := quotes.GetNews(t, briefingNewsFetch)
		stock := llm.StockData{Quote: q, News: picker.pick(fetched, briefingNewsSlots), CompanyName: companyNameFor(names, t)}
		if p, ok := positions[t]; ok {
			stock.Position = &llm.Position{Shares: p.Shares, AvgCost: p.AvgCost}
		}
		result = append(result, stock)
	}
	return result
}

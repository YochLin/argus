package data

import "time"

type Quote struct {
	Ticker        string
	Price         float64
	Open          float64
	High          float64
	Low           float64
	PrevClose     float64
	Volume        int64
	ChangePercent float64
	Timestamp     time.Time
}

type NewsItem struct {
	Headline    string
	Summary     string
	Source      string
	URL         string
	PublishedAt time.Time
}

type Provider interface {
	Name() string
	GetQuote(ticker string) (*Quote, error)
	GetNews(ticker string, limit int) ([]NewsItem, error)
	GetMarketMovers() ([]string, error) // top gainers/active tickers from S&P500/NASDAQ100
}

// HistoryProvider supplies historical closing prices for technical
// indicators (RSI/MACD/moving averages). Finnhub's free tier blocks
// /stock/candle entirely ("You don't have access to this resource"), so
// unlike Provider this has no Finnhub implementation or Multi wrapper —
// Yahoo's chart endpoint is the only source, same one GetQuote already uses.
type HistoryProvider interface {
	// GetHistory returns ~1 year of daily closes/highs/lows/volumes for
	// ticker, all oldest first and index-aligned (closes[i]/highs[i]/lows[i]/
	// volumes[i] are the same trading day) — enough closes for a 200-day
	// moving average, and enough volumes for a trailing-average "unusual
	// volume" read (see signals.VolumeRatio). highs/lows exist for
	// signals.ATR, which needs the daily range (and the previous day's
	// close), not just the closing price. Volume for Finnhub-quoted tickers
	// is otherwise unavailable (Finnhub's /quote has no volume field at
	// all — see Finnhub.GetQuote), so this is the only reliable volume
	// source in the system, not just a technicals convenience.
	GetHistory(ticker string) (closes, highs, lows []float64, volumes []int64, err error)
}

// Multi is a provider that tries each provider in order, falling back on error.
type Multi struct {
	providers []Provider
}

func NewMulti(providers ...Provider) *Multi {
	return &Multi{providers: providers}
}

func (m *Multi) Name() string { return "multi" }

func (m *Multi) GetQuote(ticker string) (*Quote, error) {
	var lastErr error
	for _, p := range m.providers {
		q, err := p.GetQuote(ticker)
		if err == nil {
			return q, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (m *Multi) GetNews(ticker string, limit int) ([]NewsItem, error) {
	var lastErr error
	for _, p := range m.providers {
		items, err := p.GetNews(ticker, limit)
		if err == nil {
			return items, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (m *Multi) GetMarketMovers() ([]string, error) {
	var lastErr error
	for _, p := range m.providers {
		tickers, err := p.GetMarketMovers()
		if err == nil {
			return tickers, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

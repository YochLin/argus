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

// Candle is one daily OHLCV bar. Date is the trading day (exchange-local
// instant from Yahoo's timestamp array; format it as a date, don't compare
// clock times). Open can be 0 on the rare day Yahoo has a close but no open
// tick — render-side code should treat 0 fields as "no data", same sentinel
// convention as Quote.
type Candle struct {
	Date                   time.Time
	Open, High, Low, Close float64
	Volume                 int64
}

// Closes/Highs/Lows/Volumes extract one field's series from candles (oldest
// first, same order), for indicator functions (signals.RSI/MACD/MA/ATR/
// VolumeRatio) that take plain slices rather than candles.
func Closes(candles []Candle) []float64 {
	out := make([]float64, len(candles))
	for i, c := range candles {
		out[i] = c.Close
	}
	return out
}

func Highs(candles []Candle) []float64 {
	out := make([]float64, len(candles))
	for i, c := range candles {
		out[i] = c.High
	}
	return out
}

func Lows(candles []Candle) []float64 {
	out := make([]float64, len(candles))
	for i, c := range candles {
		out[i] = c.Low
	}
	return out
}

func Volumes(candles []Candle) []int64 {
	out := make([]int64, len(candles))
	for i, c := range candles {
		out[i] = c.Volume
	}
	return out
}

// HistoryProvider supplies historical daily bars for technical indicators
// (RSI/MACD/moving averages) and for the raw-candle context fed to LLM
// prompts (llm.StockData.Candles). Finnhub's free tier blocks /stock/candle
// entirely ("You don't have access to this resource"), so unlike Provider
// this has no Finnhub implementation or Multi wrapper — Yahoo's chart
// endpoint is the only source, same one GetQuote already uses.
type HistoryProvider interface {
	// GetHistory returns daily OHLCV candles for ticker, oldest first, over
	// the window rangeParam selects (Yahoo chart API values, e.g. "1y"/"2y"/
	// "5y"/"max" — empty defaults to "1y" in the Yahoo implementation).
	// Existing prompt/technicals callers all pass "1y": enough closes for a
	// 200-day moving average, and enough volumes for a trailing-average
	// "unusual volume" read (see signals.VolumeRatio). Highs/lows exist for
	// signals.ATR, which needs the daily range (and the previous day's
	// close), not just the closing price. Volume for Finnhub-quoted tickers
	// is otherwise unavailable (Finnhub's /quote has no volume field at all
	// — see Finnhub.GetQuote), so this is the only reliable volume source in
	// the system, not just a technicals convenience. internal/web's round
	// detail page (Phase 5 PR3) is the first caller to need a wider window —
	// an old closed-out round can predate a fixed 1y lookback.
	GetHistory(ticker, rangeParam string) ([]Candle, error)
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

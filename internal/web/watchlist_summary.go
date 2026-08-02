package web

import (
	"log"
	"sync"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/signals"
)

type watchlistSummaryItem struct {
	Ticker     string    `json:"ticker"`
	Price      float64   `json:"price"`
	ChangePct  float64   `json:"changePct"`
	Sparkline  []float64 `json:"sparkline"`
	Support    *float64  `json:"support"`
	Resistance *float64  `json:"resistance"`
	HeldShares float64   `json:"heldShares"`
}

type watchlistSummaryResponse struct {
	Tickers []watchlistSummaryItem `json:"tickers"`
}

func buildWatchlistSummary(database dbReader, quotes quoteGetter, history data.HistoryProvider, m market.MarketID) (watchlistSummaryResponse, error) {
	tickersResp, err := buildTickers(database, m)
	if err != nil {
		return watchlistSummaryResponse{}, err
	}

	positions, err := database.GetPositions()
	if err != nil {
		return watchlistSummaryResponse{}, err
	}

	posMap := make(map[string]float64)
	for _, p := range positions {
		posMap[p.Ticker] = p.Shares
	}

	quoteMap := fetchQuotes(quotes, tickersResp.Tickers, "watchlist-summary")
	historyMap := fetchHistories(history, tickersResp.Tickers)

	items := make([]watchlistSummaryItem, 0, len(tickersResp.Tickers))
	for _, ticker := range tickersResp.Tickers {
		item := watchlistSummaryItem{
			Ticker:     ticker,
			Sparkline:  []float64{},
			HeldShares: posMap[ticker],
		}

		if q, ok := quoteMap[ticker]; ok {
			item.Price = q.Price
			item.ChangePct = q.ChangePercent
		}

		if candles, ok := historyMap[ticker]; ok && len(candles) > 0 {
			closes := make([]float64, 0, len(candles))
			for _, c := range candles {
				closes = append(closes, c.Close)
			}
			item.Sparkline = closes

			refPrice := item.Price
			if refPrice == 0 {
				refPrice = candles[len(candles)-1].Close
			}

			levels := signals.PriceLevels(candles)
			var bestSup, bestRes *float64
			for _, l := range levels {
				if l.Price < refPrice {
					p := l.Price
					if bestSup == nil || p > *bestSup {
						pVal := p
						bestSup = &pVal
					}
				} else if l.Price > refPrice {
					p := l.Price
					if bestRes == nil || p < *bestRes {
						pVal := p
						bestRes = &pVal
					}
				}
			}
			item.Support = bestSup
			item.Resistance = bestRes
		}

		items = append(items, item)
	}

	return watchlistSummaryResponse{Tickers: items}, nil
}

// fetchHistories concurrently resolves each ticker's candle history — same
// rationale as fetchQuotes in quotes.go (N sequential API round trips
// collapsed to ~1), kept separate because data.HistoryProvider's
// GetHistory(ticker, range) shape doesn't fit the quoteGetter interface.
func fetchHistories(history data.HistoryProvider, tickers []string) map[string][]data.Candle {
	result := make(map[string][]data.Candle, len(tickers))
	if history == nil || len(tickers) == 0 {
		return result
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range tickers {
		wg.Add(1)
		go func(ticker string) {
			defer wg.Done()
			candles, err := history.GetHistory(ticker, "3mo")
			if err != nil {
				log.Printf("web: watchlist-summary: get history for %s: %v", ticker, err)
				return
			}
			mu.Lock()
			result[ticker] = candles
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	return result
}

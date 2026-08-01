package web

import (
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

	items := make([]watchlistSummaryItem, 0, len(tickersResp.Tickers))
	for _, ticker := range tickersResp.Tickers {
		item := watchlistSummaryItem{
			Ticker:     ticker,
			Sparkline:  []float64{},
			HeldShares: posMap[ticker],
		}

		if quotes != nil {
			if q, qErr := quotes.GetQuote(ticker); qErr == nil && q != nil {
				item.Price = q.Price
				item.ChangePct = q.ChangePercent
			}
		}

		if history != nil {
			candles, hErr := history.GetHistory(ticker, "3mo")
			if hErr == nil && len(candles) > 0 {
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
		}

		items = append(items, item)
	}

	return watchlistSummaryResponse{Tickers: items}, nil
}

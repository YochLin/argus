package web

import (
	"sort"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/signals"
)

// buildChart assembles /api/chart: ticker's ~1y of daily candles plus the
// support/resistance levels computed from that same slice (docs/phase-7-
// support-resistance.md §4.1) - deliberately the same window the chart draws,
// so there's no lookahead and no level drawn outside the visible K-line
// range. Candles/levels are never nil (JSON "[]", not "null" - same
// dashboard.go convention).
func buildChart(history data.HistoryProvider, ticker string) (chartResponse, error) {
	candles, err := history.GetHistory(ticker, "1y")
	if err != nil {
		return chartResponse{}, err
	}

	levels := signals.PriceLevels(candles)

	resp := chartResponse{
		Ticker:  ticker,
		Candles: make([]candleResponse, 0, len(candles)),
		Levels:  make([]levelResponse, 0, len(levels)),
	}
	for _, c := range candles {
		resp.Candles = append(resp.Candles, candleResponse{
			Date:   c.Date.Format("2006-01-02"),
			Open:   c.Open,
			High:   c.High,
			Low:    c.Low,
			Close:  c.Close,
			Volume: c.Volume,
		})
	}
	for _, l := range levels {
		resp.Levels = append(resp.Levels, levelResponse{
			Price:     l.Price,
			Touches:   l.Touches,
			FirstDate: l.FirstDate.Format("2006-01-02"),
			LastDate:  l.LastDate.Format("2006-01-02"),
		})
	}
	return resp, nil
}

// buildTickers assembles /api/tickers: the union of watchlist and held
// tickers restricted to market m, deduped and sorted - the /chart list
// page's ticker picker (docs/phase-7-support-resistance.md §4.2). Watchlist
// is included alongside positions since a ticker the user is watching but
// hasn't bought yet is exactly where support/resistance is most useful
// (waiting for a pullback to support before entering).
func buildTickers(database dbReader, m market.MarketID) (tickersResponse, error) {
	watchlist, err := database.GetWatchlist()
	if err != nil {
		return tickersResponse{}, err
	}
	positions, err := database.GetPositions()
	if err != nil {
		return tickersResponse{}, err
	}

	set := make(map[string]bool)
	for _, t := range watchlist {
		if market.Of(t) == m {
			set[t] = true
		}
	}
	for _, p := range positions {
		if market.Of(p.Ticker) == m {
			set[p.Ticker] = true
		}
	}

	tickers := make([]string, 0, len(set))
	for t := range set {
		tickers = append(tickers, t)
	}
	sort.Strings(tickers)

	return tickersResponse{Tickers: tickers}, nil
}

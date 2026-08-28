package web

import (
	"sort"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// buildChart assembles /api/chart: ticker's ~1y of daily candles plus the
// support/resistance levels computed from that same slice (docs/phase-7-
// support-resistance.md §4.1), open position risk info if held, and historical
// rounds for this ticker. A history-fetch failure (mistyped/delisted/
// unresolvable ticker) degrades to empty candles/levels rather than erroring
// out entirely — the DB-backed position/rounds section below has nothing to
// do with price history, and a held position for a ticker Yahoo can't
// resolve must still be reachable (and deletable) from this page.
func buildChart(database dbReader, quotes quoteGetter, history data.HistoryProvider, ticker string) (chartResponse, error) {
	candles, err := history.GetHistory(ticker, "1y")
	if err != nil {
		logger.Errorf("web: build chart for %s: history unavailable: %v", ticker, err)
		candles = nil
	}

	levels := signals.PriceLevels(candles)

	resp := chartResponse{
		Ticker:  ticker,
		Candles: make([]candleResponse, 0, len(candles)),
		Levels:  make([]levelResponse, 0, len(levels)),
		Rounds:  []roundSummary{},
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

	if database != nil {
		allPositions, err := database.GetPositions()
		if err == nil {
			var pos *db.Position
			for _, p := range allPositions {
				if p.Ticker == ticker && p.Shares > 0 {
					pCopy := p
					pos = &pCopy
					break
				}
			}
			if pos != nil {
				m := market.Of(ticker)
				mPositions := filterPositionsByMarket(allPositions, m)
				cash, _ := loadCash(database, m)

				mTickers := make([]string, len(mPositions))
				for i, p := range mPositions {
					mTickers[i] = p.Ticker
				}
				quoteMap := fetchQuotes(quotes, mTickers, "chart")

				var totalVal float64
				for _, p := range mPositions {
					if q, ok := quoteMap[p.Ticker]; ok {
						totalVal += q.Price * p.Shares
					}
				}
				accountValue := totalVal + cash
				rp := computeSinglePositionRisk(*pos, quoteMap[ticker], accountValue)
				resp.Position = &rp
			}
		}

		allTxs, err := database.GetAllTransactions()
		if err == nil {
			var tickerTxs []db.Transaction
			for _, tx := range allTxs {
				if tx.Ticker == ticker {
					tickerTxs = append(tickerTxs, tx)
				}
			}
			segmented := segmentRounds(tickerTxs)
			resp.Rounds = make([]roundSummary, 0, len(segmented))
			for _, r := range segmented {
				resp.Rounds = append(resp.Rounds, roundSummary{
					Ticker:      ticker,
					Start:       r.StartDate,
					End:         r.EndDate,
					Open:        r.EndDate == "",
					Shares:      roundBuyShares(r.Legs),
					RealizedPnL: roundRealizedPnL(r.Legs),
				})
			}
			sort.Slice(resp.Rounds, func(i, j int) bool {
				return resp.Rounds[i].Start > resp.Rounds[j].Start
			})
		}
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

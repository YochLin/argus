package web

import (
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// chartHistoryRange is how much history /api/chart serves. Two years, not one:
// the pattern statistics (how a pattern has played out on this ticker) need
// the sample, and a round up to two years old still gets its MAE/MFE.
const chartHistoryRange = "2y"

// buildChart assembles /api/chart: ticker's ~2y of daily candles, the
// support/resistance levels computed from the latest year of them (docs/
// phase-7-support-resistance.md §4.1 tuned PriceLevels on 1y; widening it to the
// whole slice would move every line), the candlestick/volume/gap patterns
// (signals.DetectPatterns), open position risk info if held, and historical
// rounds for this ticker. A history-fetch failure (mistyped/delisted/
// unresolvable ticker) degrades to empty candles/levels rather than erroring
// out entirely — the DB-backed position/rounds section below has nothing to
// do with price history, and a held position for a ticker Yahoo can't
// resolve must still be reachable (and deletable) from this page.
func buildChart(database dbReader, quotes quoteGetter, history data.HistoryProvider, ticker string) (chartResponse, error) {
	candles, err := history.GetHistory(ticker, chartHistoryRange)
	if err != nil {
		logger.Errorf("web: build chart for %s: history unavailable: %v", ticker, err)
		candles = nil
	}

	levels := signals.PriceLevels(trailingYear(candles))
	patterns := signals.DetectPatterns(candles)

	resp := chartResponse{
		Ticker:   ticker,
		Candles:  make([]candleResponse, 0, len(candles)),
		Levels:   make([]levelResponse, 0, len(levels)),
		Patterns: make([]patternResponse, 0, len(patterns)),
		Rounds:   []roundSummary{},
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

	for _, p := range patterns {
		pr := patternResponse{
			Type: string(p.Type), Cat: p.Cat, Dir: p.Dir,
			Start: candles[p.Start].Date.Format("2006-01-02"), End: candles[p.End].Date.Format("2006-01-02"),
			Conf: p.Conf, BodyRatio: p.BodyRatio, VolRatio: p.VolRatio, Prior5Pct: p.Prior5Pct, Extra: p.Extra,
			Fwd5: p.Fwd5Pct,
		}
		if p.RefIdx >= 0 {
			pr.RefDate = candles[p.RefIdx].Date.Format("2006-01-02")
		}
		if p.Cat == signals.PatCatGap {
			pr.Gap = &gapResponse{Lo: p.GapLo, Hi: p.GapHi}
			if p.FillIdx >= 0 {
				pr.Gap.FillDate = candles[p.FillIdx].Date.Format("2006-01-02")
			}
		}
		resp.Patterns = append(resp.Patterns, pr)
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
			now := time.Now()
			for _, r := range segmented {
				rs := roundSummary{
					Ticker:      ticker,
					Start:       r.StartDate,
					End:         r.EndDate,
					Open:        r.EndDate == "",
					Shares:      roundBuyShares(r.Legs),
					RealizedPnL: roundRealizedPnL(r.Legs),
				}
				// Only when the candles reach back to the round's start: a
				// round older than the 1y window would otherwise report the
				// excursion of just its covered tail, silently understated.
				if len(candles) > 0 && candles[0].Date.Format("2006-01-02") <= r.StartDate {
					if mm := roundMAEMFE(candles, r.Legs, r.StartDate, r.EndDate, now); mm.OK {
						rs.MAEPct, rs.MFEPct, rs.HasMAEMFE = mm.MAEPct, mm.MFEPct, true
					}
				}
				resp.Rounds = append(resp.Rounds, rs)
			}
			sort.Slice(resp.Rounds, func(i, j int) bool {
				return resp.Rounds[i].Start > resp.Rounds[j].Start
			})
		}
	}

	return resp, nil
}

// trailingYear is the last year of candles ending at the latest one.
func trailingYear(candles []data.Candle) []data.Candle {
	if len(candles) == 0 {
		return nil
	}
	cutoff := candles[len(candles)-1].Date.AddDate(-1, 0, 0)
	return candles[sort.Search(len(candles), func(i int) bool { return !candles[i].Date.Before(cutoff) }):]
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

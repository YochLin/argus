package web

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// errRoundNotFound distinguishes "no such round" (404) from any other
// buildRoundDetail failure (a DB or Yahoo error, 500) for handleRoundDetail
// to branch on via errors.Is.
var errRoundNotFound = errors.New("web: round not found")

// round is one position round trip in a ticker's transaction history.
// Re-exported from internal/service for web backwards compatibility.
type round = service.Round

var (
	segmentRounds    = service.SegmentRounds
	roundRealizedPnL = service.RoundRealizedPnL
	roundBuyShares   = service.RoundBuyShares
)

// buildRounds assembles /api/rounds: every round in market m, across every
// ticker ever transacted in it, most-recently-started first — the flat list
// a frontend picker groups/filters by ticker. Restricted to one market
// (Phase 6) for the same reason buildDashboard/buildCalendar are: a round's
// realized P&L is a currency amount, and TWD/USD rounds must never mix in
// one list.
func buildRounds(database dbReader, m market.MarketID) (roundsResponse, error) {
	allTxs, err := database.GetAllTransactions()
	if err != nil {
		return roundsResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)

	byTicker := make(map[string][]db.Transaction)
	for _, t := range txs {
		byTicker[t.Ticker] = append(byTicker[t.Ticker], t)
	}

	resp := roundsResponse{Rounds: []roundSummary{}}
	for ticker, tickerTxs := range byTicker {
		for _, r := range segmentRounds(tickerTxs) {
			resp.Rounds = append(resp.Rounds, roundSummary{
				Ticker:      ticker,
				Start:       r.StartDate,
				End:         r.EndDate,
				Open:        r.EndDate == "",
				Shares:      roundBuyShares(r.Legs),
				RealizedPnL: roundRealizedPnL(r.Legs),
			})
		}
	}

	sort.Slice(resp.Rounds, func(i, j int) bool {
		if resp.Rounds[i].Start != resp.Rounds[j].Start {
			return resp.Rounds[i].Start > resp.Rounds[j].Start
		}
		return resp.Rounds[i].Ticker < resp.Rounds[j].Ticker
	})
	return resp, nil
}

// roundHistoryRange picks the coarsest Yahoo chart range value that still
// comfortably covers "start through now" — GetHistory's window has to reach
// back far enough for an old closed-out round, which a fixed "1y" (every
// other caller's window) can't guarantee. Pure and separately tested so the
// bucket boundaries are pinned down without a network call.
func roundHistoryRange(start string, now time.Time) string {
	t, err := time.Parse("2006-01-02", start)
	if err != nil {
		return "1y"
	}
	days := now.Sub(t).Hours() / 24
	switch {
	case days <= 300:
		return "1y"
	case days <= 650:
		return "2y"
	case days <= 1700:
		return "5y"
	default:
		return "max"
	}
}

// buildRoundDetail assembles /api/round-detail: the requested round's daily
// candles (padded ~15 calendar days before/after so the chart shows context
// leading into the entry and out of the exit, not just the bare trade
// window) plus its own legs as trades for the markers.
func buildRoundDetail(database dbReader, history data.HistoryProvider, ticker, start string) (roundDetailResponse, error) {
	txs, err := database.GetAllTransactions()
	if err != nil {
		return roundDetailResponse{}, err
	}

	var tickerTxs []db.Transaction
	for _, t := range txs {
		if t.Ticker == ticker {
			tickerTxs = append(tickerTxs, t)
		}
	}

	var found *round
	for _, r := range segmentRounds(tickerTxs) {
		if r.StartDate == start {
			r := r
			found = &r
			break
		}
	}
	if found == nil {
		return roundDetailResponse{}, fmt.Errorf("%w: %s starting %s", errRoundNotFound, ticker, start)
	}

	now := time.Now()
	startT, err := time.Parse("2006-01-02", found.StartDate)
	if err != nil {
		return roundDetailResponse{}, err
	}
	endT := now
	if found.EndDate != "" {
		endT, err = time.Parse("2006-01-02", found.EndDate)
		if err != nil {
			return roundDetailResponse{}, err
		}
	}
	windowStart := startT.AddDate(0, 0, -15)
	windowEnd := endT.AddDate(0, 0, 15)

	candles, err := history.GetHistory(ticker, roundHistoryRange(found.StartDate, now))
	if err != nil {
		// Same rationale as buildChart: a ticker Yahoo can't resolve must not
		// take down trades/theses/lessons below, which are DB-only and have
		// nothing to do with price history.
		logger.Errorf("web: round detail: history unavailable for %s: %v", ticker, err)
		candles = nil
	}

	mm := roundMAEMFE(candles, found.Legs, found.StartDate, found.EndDate, now)

	resp := roundDetailResponse{
		Ticker:    ticker,
		Start:     found.StartDate,
		End:       found.EndDate,
		Candles:   []candleResponse{},
		Trades:    []transactionResponse{},
		MAEPct:    mm.MAEPct,
		MFEPct:    mm.MFEPct,
		HasMAEMFE: mm.OK,
	}
	for _, c := range candles {
		if c.Date.Before(windowStart) || c.Date.After(windowEnd) {
			continue
		}
		resp.Candles = append(resp.Candles, candleResponse{
			Date:   c.Date.Format("2006-01-02"),
			Open:   c.Open,
			High:   c.High,
			Low:    c.Low,
			Close:  c.Close,
			Volume: c.Volume,
		})
	}
	for _, l := range found.Legs {
		resp.Trades = append(resp.Trades, transactionResponse{
			ID:          l.ID,
			Date:        l.Date,
			Ticker:      l.Ticker,
			Side:        l.Side,
			Shares:      l.Shares,
			Price:       l.Price,
			Fee:         l.Fee,
			RealizedPnL: l.RealizedPnL,
		})
	}

	resp.Editable = found.EndDate == ""
	resp.Theses = []thesisEntryResponse{}
	if entries, err := database.GetThesisEntriesInRange(ticker, found.StartDate, found.EndDate); err != nil {
		logger.Errorf("web: round detail: get thesis entries for %s: %v", ticker, err)
	} else {
		for _, e := range entries {
			resp.Theses = append(resp.Theses, thesisEntryResponse{Date: e.CreatedAt, Text: e.Text})
		}
	}

	resp.Lessons = []lessonResponse{}
	if byTicker, err := database.GetLessonsForTickers([]string{ticker}); err != nil {
		logger.Errorf("web: round detail: get lessons for %s: %v", ticker, err)
	} else {
		for _, l := range byTicker[ticker] {
			resp.Lessons = append(resp.Lessons, lessonResponse{Date: l.Date, Lesson: l.Lesson})
		}
	}

	return resp, nil
}

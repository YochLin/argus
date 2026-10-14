package web

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// roundsResponse is /api/rounds' body — every position round trip across
// every ticker ever transacted (rounds.go's segmentRounds), most-recently-
// started first. The frontend's round picker groups these by ticker.
type roundsResponse struct {
	Rounds []roundSummary `json:"rounds"`
}

type roundSummary struct {
	Ticker      string  `json:"ticker"`
	Start       string  `json:"start"`
	End         string  `json:"end"` // "" while still open
	Open        bool    `json:"open"`
	Shares      float64 `json:"shares"`
	RealizedPnL float64 `json:"realizedPnL"`
	// MAEPct/MFEPct are only filled by /api/chart (Phase 27 P2), which already
	// holds ~1y of candles — /api/rounds has none, so it leaves HasMAEMFE
	// false. HasMAEMFE is also false for a round that starts before the
	// first candle, since a window-truncated excursion would understate it.
	MAEPct    float64 `json:"maePct"`
	MFEPct    float64 `json:"mfePct"`
	HasMAEMFE bool    `json:"hasMaeMfe"`
}

// roundDetailResponse is /api/round-detail's body: one round's daily
// candles (padded before/after — see buildRoundDetail) plus its own legs as
// trades, for the round detail page's lightweight-charts K-line and buy/
// sell markers. MAEPct/MFEPct (Phase 5 PR4, design doc §A2) are the round's
// own approximate max adverse/favorable excursion, computed from the same
// candles already fetched here — HasMAEMFE is false only when the round's
// window has no matching candle data at all (shouldn't normally happen for
// a real round, but a delisted/renamed ticker could hit it).
type roundDetailResponse struct {
	Ticker    string                `json:"ticker"`
	Start     string                `json:"start"`
	End       string                `json:"end"`
	Candles   []candleResponse      `json:"candles"`
	Trades    []transactionResponse `json:"trades"`
	MAEPct    float64               `json:"maePct"`
	MFEPct    float64               `json:"mfePct"`
	HasMAEMFE bool                  `json:"hasMaeMfe"`
	// Thesis/Lessons (Phase 8 PR4, docs/phase-8-trader-analytics.md §6.2) are
	// attachments from service.RoundThesis/db.GetLessonsForTickers, each
	// degrading independently (a query failure logs and leaves the field at
	// its zero value) rather than failing the whole response, same
	// attach-what's-available convention as buildClosedTradeReview. Thesis is
	// the round's single overwritable text (Phase 27 P1b; "" = none written) —
	// editable for open and closed rounds alike. ThesisEdited is false when the
	// text is only the legacy journal's last entry for the round, shown as a
	// default until the first save.
	Thesis       string           `json:"thesis"`
	ThesisEdited bool             `json:"thesisEdited"`
	Lessons      []lessonResponse `json:"lessons"`
}

type lessonResponse struct {
	Date   string `json:"date"`
	Lesson string `json:"lesson"`
}

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
	// roundHistoryRange picks the coarsest chart range covering "start through
	// now" (service.HistoryRangeSince has the bucket boundaries).
	roundHistoryRange = service.HistoryRangeSince
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
	tickerRounds := segmentRounds(tickerTxs)
	for _, r := range tickerRounds {
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

	if th, ok, err := service.RoundThesis(database, ticker, tickerRounds, found.StartDate); err != nil {
		logger.Errorf("web: round detail: get thesis for %s: %v", ticker, err)
	} else if ok {
		resp.Thesis, resp.ThesisEdited = th.Text, th.Edited
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

func (s *Server) handleRounds(w http.ResponseWriter, r *http.Request) {
	resp, err := buildRounds(s.db, marketParam(r))
	if err != nil {
		logger.Errorf("web: build rounds: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build rounds")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRoundDetail(w http.ResponseWriter, r *http.Request) {
	ticker := r.URL.Query().Get("ticker")
	start := r.URL.Query().Get("start")
	if ticker == "" || start == "" {
		writeError(w, http.StatusBadRequest, "ticker and start are required")
		return
	}

	resp, err := buildRoundDetail(s.db, s.history, ticker, start)
	if errors.Is(err, errRoundNotFound) {
		writeError(w, http.StatusNotFound, "round not found")
		return
	}
	if err != nil {
		logger.Errorf("web: build round detail: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build round detail")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

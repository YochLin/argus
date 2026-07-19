package web

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

// errRoundNotFound distinguishes "no such round" (404) from any other
// buildRoundDetail failure (a DB or Yahoo error, 500) for handleRoundDetail
// to branch on via errors.Is.
var errRoundNotFound = errors.New("web: round not found")

// round is one position round trip in a ticker's transaction history: share
// balance goes from 0 up (one or more BUYs) and, for a closed round, back
// down to 0 (one or more SELLs) — mirrors internal/bot's tradeRound/
// lastClosedRound (handlers.go), duplicated here rather than imported since
// internal/web doesn't import internal/bot (transport-layer package; same
// can't-share-an-import boundary as formatFundamentals elsewhere in this
// project). Unlike lastClosedRound (which only returns the most recent
// *closed* round, for the sell-review feature), segmentRounds below returns
// every round including a still-open trailing one — the design doc's
// "未清倉的持倉就是「進行中的回合」" — since the round detail page needs to
// list and open all of them, not just the latest closed one.
type round struct {
	Legs      []db.Transaction
	StartDate string
	EndDate   string // "" if still open
}

// segmentRounds walks txs (must be date-ordered, as db.GetAllTransactions/
// GetTransactions already return them) into 0→positive→0 round trips,
// exactly like internal/bot's lastClosedRound, but keeps every round instead
// of just the latest closed one, and returns a still-open trailing round
// (EndDate "") instead of omitting it. Balances within 1e-9 of 0 count as
// closed, the same float-dust threshold db.RecordSell uses.
func segmentRounds(txs []db.Transaction) []round {
	var rounds []round
	balance := 0.0
	start := -1
	for i, tx := range txs {
		if start == -1 {
			start = i
		}
		switch tx.Side {
		case "BUY":
			balance += tx.Shares
		case "SELL":
			balance -= tx.Shares
		}
		if math.Abs(balance) < 1e-9 {
			rounds = append(rounds, round{
				Legs:      append([]db.Transaction{}, txs[start:i+1]...),
				StartDate: txs[start].Date,
				EndDate:   tx.Date,
			})
			start = -1
			balance = 0
		}
	}
	if start != -1 {
		rounds = append(rounds, round{
			Legs:      append([]db.Transaction{}, txs[start:]...),
			StartDate: txs[start].Date,
			EndDate:   "",
		})
	}
	return rounds
}

// roundRealizedPnL sums the realized_pnl of every SELL leg — meaningful for
// both a closed round (its final total) and an in-progress one with partial
// sells along the way (db.RecordSell computes realized_pnl per sell
// regardless of whether that sell fully closes the position).
func roundRealizedPnL(legs []db.Transaction) float64 {
	var total float64
	for _, l := range legs {
		if l.Side == "SELL" {
			total += l.RealizedPnL
		}
	}
	return total
}

// roundBuyShares sums the BUY legs — the round's total position size, for
// the picker list's display.
func roundBuyShares(legs []db.Transaction) float64 {
	var total float64
	for _, l := range legs {
		if l.Side == "BUY" {
			total += l.Shares
		}
	}
	return total
}

// buildRounds assembles /api/rounds: every round, across every ticker ever
// transacted, most-recently-started first — the flat list a frontend picker
// groups/filters by ticker.
func buildRounds(database dbReader) (roundsResponse, error) {
	txs, err := database.GetAllTransactions()
	if err != nil {
		return roundsResponse{}, err
	}

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
		return roundDetailResponse{}, err
	}

	resp := roundDetailResponse{
		Ticker:  ticker,
		Start:   found.StartDate,
		End:     found.EndDate,
		Candles: []candleResponse{},
		Trades:  []transactionResponse{},
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
			Date:        l.Date,
			Ticker:      l.Ticker,
			Side:        l.Side,
			Shares:      l.Shares,
			Price:       l.Price,
			Fee:         l.Fee,
			RealizedPnL: l.RealizedPnL,
		})
	}
	return resp, nil
}

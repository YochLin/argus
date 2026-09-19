package service

import (
	"math"
	"time"

	"argus/internal/db"
)

// Round is one position round trip in a ticker's transaction history:
// share balance goes from 0 up to some positive amount (via one or more BUYs)
// and, for a closed round, back down to 0 (via one or more SELLs).
type Round struct {
	Legs      []db.Transaction
	StartDate string // first BUY date
	EndDate   string // date balance returned to 0 ("" if still open)
}

// SegmentRounds walks txs (must be date-ordered, as db.GetAllTransactions/
// GetTransactions already return them) into 0->positive->0 round trips,
// returning every round including a still-open trailing one (EndDate "").
// Balances within 1e-9 of 0 count as closed, the same float-dust threshold
// db.RecordSell uses.
func SegmentRounds(txs []db.Transaction) []Round {
	var rounds []Round
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
			rounds = append(rounds, Round{
				Legs:      append([]db.Transaction{}, txs[start:i+1]...),
				StartDate: txs[start].Date,
				EndDate:   tx.Date,
			})
			start = -1
			balance = 0
		}
	}
	if start != -1 {
		rounds = append(rounds, Round{
			Legs:      append([]db.Transaction{}, txs[start:]...),
			StartDate: txs[start].Date,
			EndDate:   "",
		})
	}
	return rounds
}

// LastClosedRound returns the most recent round in txs that closed back to 0.
// Returns ok=false when there is no closed round.
func LastClosedRound(txs []db.Transaction) (Round, bool) {
	rounds := SegmentRounds(txs)
	for i := len(rounds) - 1; i >= 0; i-- {
		if rounds[i].EndDate != "" {
			return rounds[i], true
		}
	}
	return Round{}, false
}

// RoundRealizedPnL sums the realized_pnl of every SELL leg in legs.
func RoundRealizedPnL(legs []db.Transaction) float64 {
	var total float64
	for _, l := range legs {
		if l.Side == "SELL" {
			total += l.RealizedPnL
		}
	}
	return total
}

// RoundBuyShares sums the shares across every BUY leg in legs.
func RoundBuyShares(legs []db.Transaction) float64 {
	var total float64
	for _, l := range legs {
		if l.Side == "BUY" {
			total += l.Shares
		}
	}
	return total
}

// RoundEntryPrice calculates the round's weighted-average entry price across its BUY
// legs only, including transaction fees — matching db.RecordBuy's cost basis formula.
// Returns ok=false if there are no BUY legs or total shares <= 0.
func RoundEntryPrice(legs []db.Transaction) (float64, bool) {
	var shares, cost float64
	for _, l := range legs {
		if l.Side == "BUY" {
			cost = (shares*cost + l.Shares*l.Price + l.Fee) / (shares + l.Shares)
			shares += l.Shares
		}
	}
	if shares <= 0 {
		return 0, false
	}
	return cost, true
}

// WeightedAvgPrice returns the shares-weighted average price across every leg matching side
// ("BUY" or "SELL") without fee. Returns 0 if there are no matching legs or total shares is 0.
func WeightedAvgPrice(legs []db.Transaction, side string) float64 {
	var shares, cost float64
	for _, l := range legs {
		if l.Side == side {
			shares += l.Shares
			cost += l.Shares * l.Price
		}
	}
	if shares == 0 {
		return 0
	}
	return cost / shares
}

// RoundHoldingDays returns a closed round's calendar-day holding period (StartDate to EndDate).
// Returns ok=false if either date fails to parse or the round is still open.
func RoundHoldingDays(r Round) (int, bool) {
	if r.StartDate == "" || r.EndDate == "" {
		return 0, false
	}
	startT, err := time.Parse("2006-01-02", r.StartDate)
	if err != nil {
		return 0, false
	}
	endT, err := time.Parse("2006-01-02", r.EndDate)
	if err != nil {
		return 0, false
	}
	days := int(endT.Sub(startT).Hours() / 24)
	return days, true
}

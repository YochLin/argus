package web

import (
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
)

// distSample is one closed round's R-multiple. Only rounds whose closing
// SELL recorded a non-zero stop_price (Phase 8 PR1's migration 13) and whose
// weighted entry price sits above that stop are included — everything else
// (no stop set, or a stop at/above entry, the same division guard
// internal/llm's buildTradeReviewPrompt already applies) folds into
// NoStopCount instead of a fabricated R.
type rMultipleSample struct {
	Ticker string  `json:"ticker"`
	R      float64 `json:"r"`
}

// holdingReturnSample is one closed round: how long it was held vs. its
// realized dollar P&L — "did winners get cut short and losers held too
// long," a behavioral question a scatter answers more honestly than an
// average (docs/phase-8-trader-analytics.md §6.1.2).
type holdingReturnSample struct {
	Ticker      string  `json:"ticker"`
	HoldingDays int     `json:"holdingDays"`
	RealizedPnL float64 `json:"realizedPnL"`
}

// maeReturnSample is one closed round's deepest adverse close (%, from
// weighted entry price) against its final realized return (%) — a
// close-only approximation of intraday MAE (§6.1.3's documented ceiling,
// there's no intraday data in this daily-snapshot architecture).
type maeReturnSample struct {
	Ticker    string  `json:"ticker"`
	MAEPct    float64 `json:"maePct"`
	ReturnPct float64 `json:"returnPct"`
}

// distributionsResponse is /api/distributions' body (Phase 8 PR4, see
// docs/phase-8-trader-analytics.md §6.1) — three independent scatter/
// histogram datasets, all derived from the same closed-round unit
// (segmentRounds) rather than reports.go's per-SELL sellRecord: a round
// exited across several partial sells is one sample here, not several.
type distributionsResponse struct {
	RMultiples []rMultipleSample `json:"rMultiples"`
	// NoStopCount is closed rounds with no usable R (no stop_price on
	// record, or an invalid stop at/above entry) — shown alongside
	// RMultiples so the chart never silently implies a smaller universe than
	// what actually closed.
	NoStopCount int `json:"noStopCount"`
	// EarliestRDate is the StartDate of the earliest round with a computable
	// R, or "" if there are none yet — the frontend renders "R data
	// accumulating since {date}" from this rather than a hardcoded migration
	// deploy date, since the exact production deploy timestamp isn't known
	// to this code.
	EarliestRDate  string                `json:"earliestRDate"`
	HoldingReturns []holdingReturnSample `json:"holdingReturns"`
	MAEReturns     []maeReturnSample     `json:"maeReturns"`
	// SkippedMAECount is closed rounds with no usable MAE/MFE (a
	// daily_snapshots gap over the round's window — e.g. a movers-sourced
	// buy with no snapshot before the position existed) — same "show what
	// was skipped" honesty as NoStopCount.
	SkippedMAECount int `json:"skippedMaeCount"`
}

// roundEntryPrice is the round's weighted-average entry price across its BUY
// legs only — the same formula db.RecordBuy/maefe.go's dailyCostBasis use,
// but collapsed to the round's final value (SELL legs never change cost
// basis) since R-multiple only needs the one number, not a per-date
// checkpoint series. ok is false for a round with no BUY legs (shouldn't
// happen — segmentRounds only ever starts a round at a BUY).
func roundEntryPrice(legs []db.Transaction) (float64, bool) {
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

// roundHoldingDays is a closed round's calendar-day holding period —
// StartDate to EndDate, both guaranteed non-empty for a closed round.
func roundHoldingDays(r round) (int, bool) {
	startT, err := time.Parse("2006-01-02", r.StartDate)
	if err != nil {
		return 0, false
	}
	endT, err := time.Parse("2006-01-02", r.EndDate)
	if err != nil {
		return 0, false
	}
	return int(endT.Sub(startT).Hours() / 24), true
}

// buildDistributions assembles /api/distributions: every closed round across
// market m, reduced three ways. One ticker's history fetch failing (for the
// MAE/MFE scatter only) degrades that ticker's rounds out of the dataset
// (logged, counted in SkippedMAECount) rather than failing the whole
// response — same attach-what's-available convention as buildMAEMFESummary,
// which this function shares its candle-fetch cache shape with.
func buildDistributions(database dbReader, history data.HistoryProvider, m market.MarketID) (distributionsResponse, error) {
	allTxs, err := database.GetAllTransactions()
	if err != nil {
		return distributionsResponse{}, err
	}
	txs := filterTransactionsByMarket(allTxs, m)

	byTicker := make(map[string][]db.Transaction)
	for _, t := range txs {
		byTicker[t.Ticker] = append(byTicker[t.Ticker], t)
	}

	resp := distributionsResponse{
		RMultiples:     []rMultipleSample{},
		HoldingReturns: []holdingReturnSample{},
		MAEReturns:     []maeReturnSample{},
	}
	historyCache := make(map[string][]data.Candle)

	for ticker, tickerTxs := range byTicker {
		for _, r := range segmentRounds(tickerTxs) {
			if r.EndDate == "" {
				continue // only closed rounds have a final realized outcome
			}

			if days, ok := roundHoldingDays(r); ok {
				resp.HoldingReturns = append(resp.HoldingReturns, holdingReturnSample{
					Ticker: ticker, HoldingDays: days, RealizedPnL: roundRealizedPnL(r.Legs),
				})
			}

			if rSample, ok := computeRMultiple(r); ok {
				resp.RMultiples = append(resp.RMultiples, rMultipleSample{Ticker: ticker, R: rSample})
				if resp.EarliestRDate == "" || r.StartDate < resp.EarliestRDate {
					resp.EarliestRDate = r.StartDate
				}
			} else {
				resp.NoStopCount++
			}

			candles, cached := historyCache[ticker]
			if !cached {
				var err error
				candles, err = history.GetHistory(ticker, "max")
				if err != nil {
					logger.Errorf("web: distributions: get history for %s: %v", ticker, err)
					candles = nil
				}
				historyCache[ticker] = candles
			}
			if sample, ok := computeMAESample(ticker, r, candles); ok {
				resp.MAEReturns = append(resp.MAEReturns, sample)
			} else {
				resp.SkippedMAECount++
			}
		}
	}

	return resp, nil
}

// computeRMultiple returns r's R-multiple (total realized P&L over dollar
// risk at entry) — ok is false when there's no usable stop (unset, or at/
// above the weighted entry price, the same guard internal/llm's
// buildTradeReviewPrompt applies for the same reason).
func computeRMultiple(r round) (float64, bool) {
	stopPrice := r.Legs[len(r.Legs)-1].StopPrice // the round's final SELL — always the last leg of a closed round
	if stopPrice <= 0 {
		return 0, false
	}
	entryPrice, ok := roundEntryPrice(r.Legs)
	if !ok || entryPrice <= stopPrice {
		return 0, false
	}
	shares := roundBuyShares(r.Legs)
	risk := (entryPrice - stopPrice) * shares
	if risk <= 0 {
		return 0, false
	}
	return roundRealizedPnL(r.Legs) / risk, true
}

// computeMAESample returns r's MAE/return sample from candles — ok is false
// when the round's window has no matching candle data at all (a
// daily_snapshots-style gap, e.g. a movers-sourced buy with no history
// before the fetch window, or the ticker's history fetch itself failed).
func computeMAESample(ticker string, r round, candles []data.Candle) (maeReturnSample, bool) {
	if candles == nil {
		return maeReturnSample{}, false
	}
	retPct, ok := roundReturnPct(r.Legs)
	if !ok {
		return maeReturnSample{}, false
	}
	startT, err := time.Parse("2006-01-02", r.StartDate)
	if err != nil {
		return maeReturnSample{}, false
	}
	endT, err := time.Parse("2006-01-02", r.EndDate)
	if err != nil {
		return maeReturnSample{}, false
	}
	mm := computeMAEMFE(candles, r.Legs, startT, endT)
	if !mm.OK {
		return maeReturnSample{}, false
	}
	return maeReturnSample{Ticker: ticker, MAEPct: mm.MAEPct, ReturnPct: retPct}, true
}

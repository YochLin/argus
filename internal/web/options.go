package web

import (
	"log"
	"math"
	"sort"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
	"argus/internal/option"
)

// optionsResponse is /api/options's body (Phase 12 PR4, US-only —
// options are US-only everywhere in this phase): open positions (with
// live mark/greeks/DTE when the chain fetch succeeds), the full closed-
// position history, an expiry calendar, and the same collateral summary
// risk.go's /api/risk already computes (buildOptionCollateral, PR3) —
// reused rather than duplicated. Read-only: doesn't touch Phase 10's write
// endpoints, same boundary as Phase 11 PR4's read-only paper account page.
type optionsResponse struct {
	Positions  []optionPositionResponse `json:"positions"`
	Closed     []optionClosedResponse   `json:"closed"`
	Calendar   []optionExpiryCalEntry   `json:"calendar"`
	Collateral optionCollateralSummary  `json:"collateral"`
}

type optionCollateralSummary struct {
	LockedCash float64                    `json:"lockedCash"`
	Positions  []optionCollateralResponse `json:"positions"`
}

type optionPositionResponse struct {
	ContractSymbol string  `json:"contractSymbol"`
	Underlying     string  `json:"underlying"`
	Right          string  `json:"right"`
	Strike         float64 `json:"strike"`
	Expiry         string  `json:"expiry"`
	DTE            int     `json:"dte"`
	Multiplier     int64   `json:"multiplier"`
	Contracts      float64 `json:"contracts"`
	AvgPremium     float64 `json:"avgPremium"`
	StopPremium    float64 `json:"stopPremium"`
	// Mark/MarketValue/Delta/Gamma/Theta/Vega are 0 when the live chain
	// fetch for this contract failed — a degraded row, not an error for
	// the whole endpoint (same "attach what's available" convention as
	// buildDashboard's own positions list).
	Mark        float64 `json:"mark"`
	MarketValue float64 `json:"marketValue"`
	Delta       float64 `json:"delta"`
	Gamma       float64 `json:"gamma"`
	Theta       float64 `json:"theta"`
	Vega        float64 `json:"vega"`
}

type optionClosedResponse struct {
	ContractSymbol string  `json:"contractSymbol"`
	Underlying     string  `json:"underlying"`
	Right          string  `json:"right"`
	Strike         float64 `json:"strike"`
	Expiry         string  `json:"expiry"`
	Action         string  `json:"action"` // STC/BTC/EXPIRED/ASSIGNED/EXERCISED — never BTO/STO, see buildOptions
	Contracts      float64 `json:"contracts"`
	Premium        float64 `json:"premium"`
	Fee            float64 `json:"fee"`
	Date           string  `json:"date"`
	RealizedPnL    float64 `json:"realizedPnL"`
}

type optionExpiryCalEntry struct {
	Date    string   `json:"date"`
	Symbols []string `json:"symbols"`
}

// buildOptions assembles /api/options's response for market m. Empty
// (zero-value slices, not nil, and never an error) for m=tw — options are
// US-only, same "correct, not degraded" empty result as
// buildOptionCollateral's own m!=US branch.
func buildOptions(database dbReader, optionChain data.OptionChainProvider, quotes quoteGetter, m market.MarketID) (optionsResponse, error) {
	resp := optionsResponse{
		Positions: []optionPositionResponse{},
		Closed:    []optionClosedResponse{},
		Calendar:  []optionExpiryCalEntry{},
	}
	if m != market.US {
		return resp, nil
	}

	positions, err := database.GetOptionPositions()
	if err != nil {
		return resp, err
	}

	stockPositions, err := database.GetPositions()
	if err != nil {
		return resp, err
	}
	lockedCash, collateral, err := buildOptionCollateral(database, stockPositions, m)
	if err != nil {
		log.Printf("web: options: collateral: %v", err)
	}
	resp.Collateral = optionCollateralSummary{LockedCash: lockedCash, Positions: collateral}

	calendarSymbols := make(map[string][]string)
	for _, p := range positions {
		posResp := optionPositionResponse{
			ContractSymbol: p.ContractSymbol,
			Underlying:     p.Underlying,
			Right:          p.Right,
			Strike:         p.Strike,
			Expiry:         p.Expiry,
			Multiplier:     p.Multiplier,
			Contracts:      p.Contracts,
			AvgPremium:     p.AvgPremium,
			StopPremium:    p.StopPremium,
		}
		if optionChain != nil {
			if mark, dte, g, ok := fetchOptionMarkAndGreeks(optionChain, quotes, p); ok {
				posResp.Mark = mark
				posResp.DTE = dte
				posResp.MarketValue = mark * p.Contracts * float64(p.Multiplier)
				posResp.Delta, posResp.Gamma, posResp.Theta, posResp.Vega = g.Delta, g.Gamma, g.Theta, g.Vega
			}
		}
		resp.Positions = append(resp.Positions, posResp)
		calendarSymbols[p.Expiry] = append(calendarSymbols[p.Expiry], p.ContractSymbol)
	}
	sort.Slice(resp.Positions, func(i, j int) bool {
		if resp.Positions[i].Expiry != resp.Positions[j].Expiry {
			return resp.Positions[i].Expiry < resp.Positions[j].Expiry
		}
		return resp.Positions[i].Underlying < resp.Positions[j].Underlying
	})

	for date, symbols := range calendarSymbols {
		sort.Strings(symbols)
		resp.Calendar = append(resp.Calendar, optionExpiryCalEntry{Date: date, Symbols: symbols})
	}
	sort.Slice(resp.Calendar, func(i, j int) bool { return resp.Calendar[i].Date < resp.Calendar[j].Date })

	txs, err := database.GetAllOptionTransactions()
	if err != nil {
		return resp, err
	}
	for _, t := range txs {
		if t.Action == db.OptionActionBuyToOpen || t.Action == db.OptionActionSellToOpen {
			continue // an open, not a close — not part of this list
		}
		resp.Closed = append(resp.Closed, optionClosedResponse{
			ContractSymbol: t.ContractSymbol,
			Underlying:     t.Underlying,
			Right:          t.Right,
			Strike:         t.Strike,
			Expiry:         t.Expiry,
			Action:         t.Action,
			Contracts:      t.Contracts,
			Premium:        t.Premium,
			Fee:            t.Fee,
			Date:           t.Date,
			RealizedPnL:    t.RealizedPnL,
		})
	}
	return resp, nil
}

// fetchOptionMarkAndGreeks fetches p's live chain and returns its mark
// price, days-to-expiry, and Black-Scholes greeks — ok=false on any fetch
// failure or if the contract has since dropped out of the chain (e.g. the
// expiry passed), which the caller treats as a degraded row, not a hard
// error. Mirrors internal/bot's Bot.optionMark; not shared between the two
// packages for the same can't-share-an-import reason as
// cashSettingKeyFor/loadCash elsewhere in this package.
func fetchOptionMarkAndGreeks(optionChain data.OptionChainProvider, quotes quoteGetter, p db.OptionPosition) (mark float64, dte int, g option.Greeks, ok bool) {
	expiry, err := time.Parse("2006-01-02", p.Expiry)
	if err != nil {
		return 0, 0, option.Greeks{}, false
	}
	chainQuotes, err := optionChain.GetOptionChain(p.Underlying, expiry)
	if err != nil {
		return 0, 0, option.Greeks{}, false
	}
	for _, q := range chainQuotes {
		if q.ContractSymbol != p.ContractSymbol {
			continue
		}
		mark = option.Mark(q.Bid, q.Ask, q.LastPrice)
		dte = int(math.Ceil(time.Until(expiry).Hours() / 24))

		var spot float64
		if sq, err := quotes.GetQuote(p.Underlying); err == nil {
			spot = sq.Price
		}
		right := option.Call
		if p.Right == "P" {
			right = option.Put
		}
		g = option.BlackScholesGreeks(spot, p.Strike, float64(dte)/365, option.RiskFreeRate, q.ImpliedVolatility, 0, right)
		return mark, dte, g, true
	}
	return 0, 0, option.Greeks{}, false
}

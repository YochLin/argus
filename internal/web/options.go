package web

import (
	"net/http"
	"sort"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/option"
	"argus/internal/service"
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
		Positions:  []optionPositionResponse{},
		Closed:     []optionClosedResponse{},
		Calendar:   []optionExpiryCalEntry{},
		Collateral: optionCollateralSummary{Positions: []optionCollateralResponse{}},
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
		logger.Errorf("web: options: collateral: %v", err)
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

// optionOpenRequest is POST /api/options/open's body — the web dashboard's
// counterpart to /obuy and /osell, dispatched by side rather than two
// separate routes since the OCC symbol already carries call/put/strike/
// expiry (see option.Parse).
type optionOpenRequest struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"` // "BUY" or "SELL"
	Contracts float64 `json:"contracts"`
	Premium   float64 `json:"premium"`
	Fee       float64 `json:"fee"`
	Date      string  `json:"date"`
}

// optionCloseRequest is POST /api/options/close's body — Action is one of
// db.OptionActionBuyToClose/SellToClose (Contracts/Premium/Fee required) or
// db.OptionActionExpired/Assigned/Exercised (ignored — resolveOption always
// closes the whole remaining position at premium 0).
type optionCloseRequest struct {
	Symbol    string  `json:"symbol"`
	Action    string  `json:"action"`
	Contracts float64 `json:"contracts"`
	Premium   float64 `json:"premium"`
	Fee       float64 `json:"fee"`
	Date      string  `json:"date"`
}

// handleOptionOpen and handleOptionClose follow trade.go's execBuy/execSell
// shape exactly (decode -> resolve date -> call the TradeExecutor seam ->
// Notify regardless of outcome -> map err to 400) — see trade.go's doc
// comment on that seam for why bot-layer behavior (the naked-call warning,
// the ASSIGNED/EXERCISED stock-side trade) can't be skipped by writing
// db.RecordOption directly here.
func (s *Server) handleOptionOpen(w http.ResponseWriter, r *http.Request) {
	var req optionOpenRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	date, ok := resolveTradeDate(req.Date)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}
	msg, err := s.trade.ExecuteOptionOpen(req.Symbol, req.Side, req.Contracts, req.Premium, req.Fee, date)
	s.trade.Notify(msg)
	if err != nil {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: msg})
}

func (s *Server) handleOptionClose(w http.ResponseWriter, r *http.Request) {
	var req optionCloseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	date, ok := resolveTradeDate(req.Date)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}
	msg, err := s.trade.ExecuteOptionClose(req.Symbol, req.Action, req.Contracts, req.Premium, req.Fee, date)
	s.trade.Notify(msg)
	if err != nil {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: msg})
}

// fetchOptionMarkAndGreeks fetches p's live chain via service.FindOptionQuote
// (the same chain-fetch-and-match step internal/bot's Bot.optionMark uses
// for mark/dte alone) and layers Black-Scholes greeks on top, since that IV
// input only this package's Greeks display needs. ok=false on any fetch
// failure or if the contract has since dropped out of the chain (e.g. the
// expiry passed), which the caller treats as a degraded row, not a hard
// error.
func fetchOptionMarkAndGreeks(optionChain data.OptionChainProvider, quotes quoteGetter, p db.OptionPosition) (mark float64, dte int, g option.Greeks, ok bool) {
	q, dte, err := service.FindOptionQuote(optionChain, p)
	if err != nil {
		return 0, 0, option.Greeks{}, false
	}
	mark = option.Mark(q.Bid, q.Ask, q.LastPrice)

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

package web

import (
	"log"

	"argus/internal/data"
	"argus/internal/market"
)

// buildCompanyNames assembles /api/company-names: every TW ticker the user
// has any relationship with (watchlist ∪ positions ∪ transacted tickers —
// the union of what the other endpoints can ever display) mapped to its
// Chinese short name via data.CompanyNameProvider (FinMind's
// TaiwanStockInfo, cached forever in-process — see finmind.go's
// GetCompanyName — so only a ticker's first-ever lookup costs a request).
// One map endpoint rather than a name field on every response struct: the
// frontend fetches it once at the app shell level and applies it wherever
// a ticker renders, so the other builders/response shapes stay untouched.
//
// provider is nil when FINMIND_TOKEN isn't set (same optionality as
// bot.Config.CompanyNames); that and any per-ticker lookup failure degrade
// to the bare ticker on the frontend, never an error — a missing name is
// cosmetic. US tickers are skipped entirely, same reasoning as
// data.CompanyNameProvider's doc comment: AAPL is already human-readable.
func buildCompanyNames(database dbReader, provider data.CompanyNameProvider) companyNamesResponse {
	resp := companyNamesResponse{Names: map[string]string{}}
	if provider == nil {
		return resp
	}

	set := map[string]bool{}
	watchlist, err := database.GetWatchlist()
	if err != nil {
		log.Printf("web: company names: get watchlist: %v", err)
	}
	for _, t := range watchlist {
		set[t] = true
	}
	positions, err := database.GetPositions()
	if err != nil {
		log.Printf("web: company names: get positions: %v", err)
	}
	for _, p := range positions {
		set[p.Ticker] = true
	}
	txs, err := database.GetAllTransactions()
	if err != nil {
		log.Printf("web: company names: get transactions: %v", err)
	}
	for _, t := range txs {
		set[t.Ticker] = true
	}

	for ticker := range set {
		if market.Of(ticker) != market.TW {
			continue
		}
		name, err := provider.GetCompanyName(ticker)
		if err != nil {
			log.Printf("web: company names: %s: %v", ticker, err)
			continue
		}
		resp.Names[ticker] = name
	}
	return resp
}

package data

import "fmt"

// CompanyNameProvider resolves a ticker's human-readable company name.
// FinMind is the only implementation (see finmind.go's GetCompanyName) —
// unlike FundamentalsProvider/EarningsProvider there's no US-side
// implementation to route to, since a US ticker symbol (AAPL) is already
// human-readable and needs no lookup; only Taiwan's numeric tickers do.
type CompanyNameProvider interface {
	GetCompanyName(ticker string) (string, error)
}

// TickerLabel formats ticker for display with its resolved company name
// (e.g. "台積電(2330)"), matching the format the user asked for. name is
// expected to already be scoped to tickers that need this treatment (TW
// tickers with a successful CompanyNameProvider lookup) — an empty name
// returns the bare ticker unchanged, so callers can pass through a failed
// or skipped lookup without a separate branch.
func TickerLabel(ticker, name string) string {
	if name == "" {
		return ticker
	}
	return fmt.Sprintf("%s(%s)", name, ticker)
}

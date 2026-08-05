package data

import "time"

// OptionQuote is one contract from a Yahoo option chain response — see
// yahoo.go's GetOptionChain for the live JSON shape this is decoded from.
type OptionQuote struct {
	ContractSymbol    string
	Right             string // "C" or "P" — matches option.Call/option.Put's string values without importing internal/option (avoids a cycle, since internal/option/select.go takes []OptionQuote)
	Strike            float64
	LastPrice         float64
	Bid               float64
	Ask               float64
	Volume            int64
	OpenInterest      int64
	ImpliedVolatility float64
	InTheMoney        bool
	Expiration        time.Time
	LastTradeDate     time.Time
}

// OptionChainProvider is implemented only by Yahoo — nil-check optional
// provider, same convention as EarningsProvider (Finnhub-only) and
// FundamentalsProvider: no second source for this data.
type OptionChainProvider interface {
	// GetOptionExpirations lists the available expiry dates for underlying.
	GetOptionExpirations(underlying string) ([]time.Time, error)
	// GetOptionChain returns every call and put contract at expiry (an
	// exact value from GetOptionExpirations — Yahoo's endpoint snaps an
	// arbitrary date to the nearest listed expiry rather than erroring).
	GetOptionChain(underlying string, expiry time.Time) ([]OptionQuote, error)
}

package main

import (
	"argus/internal/data"
	"argus/internal/sinopac"
)

// coreProviders is the provider bundle main() and runMCPServer() build
// identically — see newCoreProviders' doc comment (Phase 24 tech debt 6).
// Each caller layers its own extra fields on top by reading Finnhub/FinMind
// directly off this struct (nil-guarded, since assigning a nil *data.Finnhub/
// *data.FinMind to an interface field would produce a non-nil interface
// wrapping a nil pointer — every `if x != nil` check on that field would
// then wrongly see it as present).
type coreProviders struct {
	Provider     data.Provider
	Yahoo        *data.Yahoo
	Finnhub      *data.Finnhub
	FinMind      *data.FinMind
	Sinopac      *sinopac.Client
	Fundamentals data.FundamentalsProvider
	CompanyNames data.CompanyNameProvider
	InsiderTx    data.InsiderTransactionProvider
	Earnings     data.EarningsProvider
}

// newCoreProviders builds the provider chain main() and runMCPServer() both
// need: Finnhub (if finnhubKey set) → Shioaji (if shioajiAddr set) →
// GoogleNews → Yahoo, merged into a Multi and wrapped in
// data.NewNewsFilter(newsBlocked) — same ordering rationale in both
// callers: Shioaji sits ahead of GoogleNews/Yahoo for broker-grade TW quotes,
// and GoogleNews sits ahead of Yahoo so TW tickers get Chinese-language news
// (Finnhub's /company-news is US-only, Yahoo's search API answers a .TW
// symbol with mostly English wire coverage) — constructed after FinMind
// because it takes CompanyNames (with a token it searches "台積電", without
// one the bare "2330"). fundamentalsRouter (and so Fundamentals) stays nil,
// not a router wrapping two nil fields, when neither key is set — preserving
// every `if x.fundamentals != nil` check's behavior.
//
// Only the Finnhub/FinMind-derived fields both callers actually use
// (InsiderTx/Earnings from Finnhub, CompanyNames from FinMind) are returned;
// main()'s extra Finnhub fields (AnalystRating/MarketNews/Sector/
// EarningsSurprise) and extra FinMind fields (TrustNet/IndustryMap/
// TWValuation) — which runMCPServer() has no use for — are assigned by main()
// itself off the returned Finnhub/FinMind pointers.
func newCoreProviders(finnhubKey, finMindToken, shioajiAddr string, newsBlocked func(source string) bool) coreProviders {
	var providers []data.Provider
	var cp coreProviders

	fundamentalsRouter := &data.FundamentalsRouter{}
	if finnhubKey != "" {
		cp.Finnhub = data.NewFinnhub(finnhubKey)
		providers = append(providers, cp.Finnhub)
		fundamentalsRouter.US = cp.Finnhub
		cp.InsiderTx = cp.Finnhub
		cp.Earnings = cp.Finnhub
	}
	if finMindToken != "" {
		cp.FinMind = data.NewFinMind(finMindToken)
		fundamentalsRouter.TW = cp.FinMind
		cp.CompanyNames = cp.FinMind
	}
	if fundamentalsRouter.US != nil || fundamentalsRouter.TW != nil {
		cp.Fundamentals = fundamentalsRouter
	}

	if shioajiAddr != "" {
		cp.Sinopac = sinopac.New(shioajiAddr)
		providers = append(providers, data.NewShioaji(cp.Sinopac))
	}
	providers = append(providers, data.NewGoogleNews(cp.CompanyNames))
	cp.Yahoo = data.NewYahoo()
	providers = append(providers, cp.Yahoo)

	cp.Provider = data.NewNewsFilter(data.NewMulti(providers...), newsBlocked)
	return cp
}

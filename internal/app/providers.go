package app

import (
	"argus/internal/data"
	"argus/internal/sinopac"
)

// CoreProviders is the provider bundle Boot and runMCPServer build
// identically — see NewCoreProviders' doc comment (Phase 24 tech debt 6).
// Each caller layers its own extra fields on top by reading Finnhub/FinMind
// directly off this struct (nil-guarded, since assigning a nil *data.Finnhub/
// *data.FinMind to an interface field would produce a non-nil interface
// wrapping a nil pointer — every `if x != nil` check on that field would
// then wrongly see it as present).
type CoreProviders struct {
	Provider     data.Provider
	Yahoo        *data.Yahoo
	Finnhub      *data.Finnhub
	FinMind      *data.FinMind
	Sinopac      *sinopac.Client
	Cnyes        *data.Cnyes
	Fundamentals data.FundamentalsProvider
	CompanyNames data.CompanyNameProvider
	InsiderTx    data.InsiderTransactionProvider
	Earnings     data.EarningsProvider
}

// NewCoreProviders builds the provider chain Boot and runMCPServer both
// need: Finnhub (if finnhubKey set) → Shioaji (if shioajiAddr set) → Cnyes →
// GoogleNews → Yahoo, merged into a Multi and wrapped in
// data.NewNewsFilter(newsBlocked) — same ordering rationale in both
// callers: Shioaji sits ahead of the news sources for broker-grade TW
// quotes; Cnyes sits ahead of GoogleNews because its category-feed tags are
// editor-assigned (not a full-text keyword match) and a TW ticker it has no
// news for deliberately returns no fallback rather than GoogleNews's
// noisier full-text hits (Phase 19 後續 PR5-2, see cnyes.go's GetNews doc
// comment); GoogleNews sits ahead of Yahoo so TW tickers still get
// Chinese-language coverage for whatever Cnyes doesn't tag (Finnhub's
// /company-news is US-only, Yahoo's search API answers a .TW symbol with
// mostly English wire coverage) — constructed after FinMind because it
// takes CompanyNames (with a token it searches "台積電", without one the
// bare "2330"). The returned Cnyes instance is also Boot's TWMarketNews
// source (app.go) — one shared instance, not two, so its per-ticker news
// cache (cnyes.go's stockNewsCache) isn't built twice for no reason.
// fundamentalsRouter (and so Fundamentals) stays nil, not a router wrapping
// two nil fields, when neither key is set — preserving every
// `if x.fundamentals != nil` check's behavior.
//
// Only the Finnhub/FinMind-derived fields both callers actually use
// (InsiderTx/Earnings from Finnhub, CompanyNames from FinMind) are returned;
// Boot's extra Finnhub fields (AnalystRating/MarketNews/Sector/
// EarningsSurprise) and extra FinMind fields (TrustNet/IndustryMap/
// TWValuation) — which runMCPServer has no use for — are assigned by Boot
// itself off the returned Finnhub/FinMind pointers.
func NewCoreProviders(finnhubKey, finMindToken, shioajiAddr string, newsBlocked func(source string) bool) CoreProviders {
	var providers []data.Provider
	var cp CoreProviders

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
	cp.Cnyes = data.NewCnyes()
	providers = append(providers, cp.Cnyes)
	providers = append(providers, data.NewGoogleNews(cp.CompanyNames))
	cp.Yahoo = data.NewYahoo()
	providers = append(providers, cp.Yahoo)

	cp.Provider = data.NewNewsFilter(data.NewMulti(providers...), newsBlocked)
	return cp
}

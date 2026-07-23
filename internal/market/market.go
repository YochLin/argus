package market

// MarketID identifies which exchange/market a ticker trades on. This
// project only distinguishes two (see Of) — Phase 6's "台美雙市場" scope
// explicitly excludes Hong Kong and other markets (0700.HK would collide
// with this package's shape-based classification; see docs/phase-6-tw-market.md
// §1), so a third MarketID isn't introduced speculatively.
type MarketID string

const (
	US MarketID = "us"
	TW MarketID = "tw"
)

// Of classifies ticker by shape: a leading digit means a Taiwan listing
// (2330, 0050, 00679B), anything else (including empty string) a US one.
// This is the single source of truth for market classification across the
// codebase — db writes, provider routing, bot filtering, and the web
// dashboard all call this rather than storing or accepting a
// user-supplied market value. No validation beyond the leading character;
// an invalid/empty ticker classifies as US, same "garbage in, garbage
// through" contract as the rest of this package.
func Of(ticker string) MarketID {
	if len(ticker) > 0 && ticker[0] >= '0' && ticker[0] <= '9' {
		return TW
	}
	return US
}

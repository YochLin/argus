package option

// Mark is the single entry point for "what is this contract worth right
// now" — mid-price first, falling back to last only when bid/ask is
// unusable. A thin/illiquid contract's lastPrice can be a stale trade from
// days ago (a "zombie price"); using it would systematically misprice
// exactly the contracts this matters most for.
func Mark(bid, ask, last float64) float64 {
	if bid > 0 && ask > 0 && ask >= bid {
		return (bid + ask) / 2
	}
	return last
}

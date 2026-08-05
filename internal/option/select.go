package option

import (
	"math"
	"sort"
	"time"

	"argus/internal/data"
)

// Profile parameterizes Select's delta/DTE screening band and liquidity
// gate. Every threshold is a field rather than a package constant
// (docs/phase-12-options.md §4 PR3) — these are real-world calibration
// knobs (how wide a delta band, how much DTE slack) nobody can tune
// correctly before actually trading through them.
type Profile struct {
	Name     string
	Right    Right
	DeltaMin float64 // signed, same convention as Greeks.Delta (a put's is negative)
	DeltaMax float64
	DTEMin   int
	DTEMax   int

	// Liquidity gate — checked before delta, since it matters more: a
	// contract with a 30% bid/ask spread loses money even when the
	// direction call is right.
	MinOpenInterest int64
	MinVolume       int64
	MaxSpreadPct    float64 // (ask-bid)/mid
}

// The four strategies this phase supports (docs/phase-12-options.md §1):
// buying calls/puts outright, or selling them against cash (CSP) or stock
// (covered call). DTE/delta bands are the design doc's own worked numbers.
var (
	LongCall    = Profile{Name: "LongCall", Right: Call, DeltaMin: 0.35, DeltaMax: 0.60, DTEMin: 30, DTEMax: 60, MinOpenInterest: 500, MinVolume: 10, MaxSpreadPct: 0.10}
	LongPut     = Profile{Name: "LongPut", Right: Put, DeltaMin: -0.60, DeltaMax: -0.35, DTEMin: 30, DTEMax: 60, MinOpenInterest: 500, MinVolume: 10, MaxSpreadPct: 0.10}
	CSP         = Profile{Name: "CSP", Right: Put, DeltaMin: -0.30, DeltaMax: -0.15, DTEMin: 30, DTEMax: 45, MinOpenInterest: 500, MinVolume: 10, MaxSpreadPct: 0.10}
	CoveredCall = Profile{Name: "CoveredCall", Right: Call, DeltaMin: 0.15, DeltaMax: 0.30, DTEMin: 30, DTEMax: 45, MinOpenInterest: 500, MinVolume: 10, MaxSpreadPct: 0.10}
)

// Candidate is one contract that passed Select's liquidity and delta/DTE
// screen, with its priced greeks attached so a caller never has to
// recompute them.
type Candidate struct {
	Quote     data.OptionQuote
	Mark      float64
	Greeks    Greeks
	DTE       int
	SpreadPct float64
}

// Select screens chain (every contract at every expiry a caller cares to
// pass in — Yahoo's chain endpoint is per-expiry, so gathering the ones
// within p's DTE band is the caller's job, not this pure function's) down
// to Candidates matching p's liquidity gate and delta/DTE band, sorted by
// DTE ascending then tightest spread — nearer-term, more liquid contracts
// first. asOf is the reference "now" for DTE/greeks (not time.Now()
// directly) so this stays a deterministic pure function to test.
func Select(chain []data.OptionQuote, spot float64, asOf time.Time, p Profile) []Candidate {
	var out []Candidate
	for _, q := range chain {
		if q.Right != string(p.Right) {
			continue
		}
		if q.OpenInterest < p.MinOpenInterest || q.Volume < p.MinVolume {
			continue
		}
		mark := Mark(q.Bid, q.Ask, q.LastPrice)
		if mark <= 0 {
			continue
		}
		spreadPct := math.Inf(1) // no two-sided market -> fails the gate below
		if q.Bid > 0 && q.Ask > 0 && q.Ask >= q.Bid {
			mid := (q.Bid + q.Ask) / 2
			spreadPct = (q.Ask - q.Bid) / mid
		}
		if spreadPct > p.MaxSpreadPct {
			continue
		}

		dte := int(math.Ceil(q.Expiration.Sub(asOf).Hours() / 24))
		if dte < p.DTEMin || dte > p.DTEMax {
			continue
		}

		t := float64(dte) / 365
		g := BlackScholesGreeks(spot, q.Strike, t, RiskFreeRate, q.ImpliedVolatility, 0, p.Right)
		if g.Delta < p.DeltaMin || g.Delta > p.DeltaMax {
			continue
		}

		out = append(out, Candidate{Quote: q, Mark: mark, Greeks: g, DTE: dte, SpreadPct: spreadPct})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].DTE != out[j].DTE {
			return out[i].DTE < out[j].DTE
		}
		return out[i].SpreadPct < out[j].SpreadPct
	})
	return out
}

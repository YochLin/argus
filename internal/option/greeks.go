package option

import "math"

// RiskFreeRate is a package-level constant rather than a live ^IRX quote.
// ponytail: short-dated delta is nearly insensitive to r; wire up a real
// rate feed only if a long-dated (LEAPS) position makes the approximation
// visibly wrong.
const RiskFreeRate = 0.04

// Greeks holds the Black-Scholes sensitivities used for contract selection
// and risk display. Vega is per 1 vol point (sigma +0.01); Theta is per
// calendar day (annual theta / 365) — both match how traders usually read
// these numbers, not the raw per-year/per-unit-vol math.
type Greeks struct {
	Delta float64
	Gamma float64
	Theta float64
	Vega  float64
}

// Greeks prices a European option via Black-Scholes: s = spot, k = strike,
// t = years to expiry, r = risk-free rate (RiskFreeRate unless a caller has
// something better), sigma = implied volatility, q = dividend yield
// (data.FundamentalsProvider.DividendYieldPct, 0 if unavailable).
func BlackScholesGreeks(s, k, t, r, sigma, q float64, right Right) Greeks {
	if s <= 0 || k <= 0 || t <= 0 || sigma <= 0 {
		return Greeks{}
	}

	sqrtT := math.Sqrt(t)
	d1 := (math.Log(s/k) + (r-q+sigma*sigma/2)*t) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT

	discQ := math.Exp(-q * t)
	discR := math.Exp(-r * t)
	nd1 := normPDF(d1)

	gamma := discQ * nd1 / (s * sigma * sqrtT)
	vega := s * discQ * nd1 * sqrtT / 100

	var delta, theta float64
	if right == Put {
		delta = -discQ * normCDF(-d1)
		theta = (-s*discQ*nd1*sigma/(2*sqrtT) + r*k*discR*normCDF(-d2) - q*s*discQ*normCDF(-d1)) / 365
	} else {
		delta = discQ * normCDF(d1)
		theta = (-s*discQ*nd1*sigma/(2*sqrtT) - r*k*discR*normCDF(d2) + q*s*discQ*normCDF(d1)) / 365
	}

	return Greeks{Delta: delta, Gamma: gamma, Theta: theta, Vega: vega}
}

// normCDF is the standard normal CDF N(x), via math.Erfc per the design doc
// (avoids pulling in a stats package for one function).
func normCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

func normPDF(x float64) float64 {
	return math.Exp(-x*x/2) / math.Sqrt(2*math.Pi)
}

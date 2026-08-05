package option

import "testing"

// Reference values cross-checked against an independent Black-Scholes
// implementation (Python's math.erfc), same s=k=310, t=30/365, r=0.04,
// sigma=0.34, q=0.
func TestBlackScholesGreeks(t *testing.T) {
	const s, k, tt, r, sigma, q = 310, 310, 30.0 / 365, 0.04, 0.34, 0.0

	call := BlackScholesGreeks(s, k, tt, r, sigma, q, Call)
	wantCall := Greeks{Delta: 0.5328618627010296, Gamma: 0.013157655907876554, Theta: -0.21696137083617856, Vega: 0.35335335545256863}
	assertGreeksClose(t, "call", call, wantCall)

	put := BlackScholesGreeks(s, k, tt, r, sigma, q, Put)
	wantPut := Greeks{Delta: -0.46713813729897036, Gamma: 0.013157655907876554, Theta: -0.18310027544499666, Vega: 0.35335335545256863}
	assertGreeksClose(t, "put", put, wantPut)
}

func TestBlackScholesGreeksInvalidInputs(t *testing.T) {
	for _, g := range [][]float64{
		{0, 310, 0.1, 0.04, 0.3, 0},
		{310, 0, 0.1, 0.04, 0.3, 0},
		{310, 310, 0, 0.04, 0.3, 0},
		{310, 310, 0.1, 0.04, 0, 0},
	} {
		got := BlackScholesGreeks(g[0], g[1], g[2], g[3], g[4], g[5], Call)
		if got != (Greeks{}) {
			t.Errorf("BlackScholesGreeks(%v) = %+v, want zero value", g, got)
		}
	}
}

func assertGreeksClose(t *testing.T, name string, got, want Greeks) {
	t.Helper()
	const eps = 1e-9
	if abs(got.Delta-want.Delta) > eps || abs(got.Gamma-want.Gamma) > eps ||
		abs(got.Theta-want.Theta) > eps || abs(got.Vega-want.Vega) > eps {
		t.Errorf("%s greeks = %+v, want %+v", name, got, want)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

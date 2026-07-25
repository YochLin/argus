package receval

import "sort"

// Stats aggregates the matured windows in one group (e.g. one action, one
// source, or one market, at one horizon) into hit-rate and return
// statistics — the shared shape behind every breakdown table
// docs/offline-rec-eval.md §6 calls for.
type Stats struct {
	N          int
	Hits       int
	HaveBenchN int
	returns    []float64
	excessSum  float64
}

func (s Stats) HitRate() float64 {
	if s.N == 0 {
		return 0
	}
	return float64(s.Hits) / float64(s.N) * 100
}

func (s Stats) AvgReturn() float64 {
	if s.N == 0 {
		return 0
	}
	var sum float64
	for _, r := range s.returns {
		sum += r
	}
	return sum / float64(s.N)
}

func (s Stats) MedianReturn() float64 {
	if len(s.returns) == 0 {
		return 0
	}
	sorted := append([]float64(nil), s.returns...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func (s Stats) AvgExcess() float64 {
	if s.N == 0 {
		return 0
	}
	return s.excessSum / float64(s.N)
}

// Aggregate groups recs' matured windows by keyFunc(rec) x horizon into
// Stats — reused for the overview table (keyFunc returns rec.Action), the
// source breakdown (keyFunc returns DisplaySource(rec.Source)), and the
// market breakdown (keyFunc returns rec.Market), since all three need
// identical n/hit-rate/return math over a different grouping key.
// Unscorable recs are skipped entirely, and an immature window contributes
// to no group.
func Aggregate(recs []ScoredRec, keyFunc func(Recommendation) string) map[string]map[int]Stats {
	out := make(map[string]map[int]Stats)
	for _, sr := range recs {
		if sr.Unscorable {
			continue
		}
		key := keyFunc(sr.Rec)
		if out[key] == nil {
			out[key] = make(map[int]Stats)
		}
		for _, w := range sr.Windows {
			if !w.Matured {
				continue
			}
			s := out[key][w.Horizon]
			s.N++
			if w.Hit {
				s.Hits++
			}
			if w.HaveBench {
				s.HaveBenchN++
			}
			s.returns = append(s.returns, w.TickerReturnPct)
			s.excessSum += w.ExcessReturnPct
			out[key][w.Horizon] = s
		}
	}
	return out
}

// DisplaySource normalizes a stored recommendation source for display: rows
// saved before the source column existed have "". A small, deliberate
// duplicate of bot.displaySource — this package can't import internal/bot
// (nor should it), same can't-share-an-import shape as formatFundamentals.
func DisplaySource(source string) string {
	if source == "" {
		return "watchlist"
	}
	return source
}

// Extremes returns the n best and n worst scored recs, ranked by excess
// return at horizon (best[0] is the single best, worst[0] the single
// worst) — only among recs whose horizon window actually matured.
func Extremes(recs []ScoredRec, horizon, n int) (best, worst []ScoredRec) {
	var matured []ScoredRec
	for _, sr := range recs {
		if sr.Unscorable {
			continue
		}
		for _, w := range sr.Windows {
			if w.Horizon == horizon && w.Matured {
				matured = append(matured, sr)
				break
			}
		}
	}
	sort.Slice(matured, func(i, j int) bool {
		return excessAt(matured[i], horizon) > excessAt(matured[j], horizon)
	})
	best = firstN(matured, n)
	worst = firstN(reverseCopy(matured), n)
	return best, worst
}

func excessAt(sr ScoredRec, horizon int) float64 {
	for _, w := range sr.Windows {
		if w.Horizon == horizon {
			return w.ExcessReturnPct
		}
	}
	return 0
}

func firstN(recs []ScoredRec, n int) []ScoredRec {
	if len(recs) > n {
		return recs[:n]
	}
	return recs
}

func reverseCopy(recs []ScoredRec) []ScoredRec {
	out := make([]ScoredRec, len(recs))
	for i, r := range recs {
		out[len(recs)-1-i] = r
	}
	return out
}

// Counts tallies the honesty-disclosure numbers docs/offline-rec-eval.md §5/
// §6 call for: how many scored recs turned out unscorable (and why), and
// how many requested-horizon windows are matured vs. still pending. HOLD/no-
// action recommendations aren't included — this package never sees them,
// since callers filter to BUY/SELL before calling Score.
type Counts struct {
	Unscorable         int
	UnscorableByReason map[string]int
	MaturedByHorizon   map[int]int
	ImmatureByHorizon  map[int]int
}

func CountOutcomes(recs []ScoredRec) Counts {
	c := Counts{
		UnscorableByReason: map[string]int{},
		MaturedByHorizon:   map[int]int{},
		ImmatureByHorizon:  map[int]int{},
	}
	for _, sr := range recs {
		if sr.Unscorable {
			c.Unscorable++
			c.UnscorableByReason[sr.Reason]++
			continue
		}
		for _, w := range sr.Windows {
			if w.Matured {
				c.MaturedByHorizon[w.Horizon]++
			} else {
				c.ImmatureByHorizon[w.Horizon]++
			}
		}
	}
	return c
}

package web

import (
	"log"
	"sort"
	"sync"
	"time"

	"argus/internal/data"
	"argus/internal/market"
	"argus/internal/receval"
)

// recPerfHorizons widens argus eval's own default -horizons ("5,20,60") to
// also include a 1-day window, matching the web dashboard's day-by-day
// excess-return bar chart (docs/phase-8-trader-analytics.md §5.2 — same
// receval functions, same windows, not a second scoring implementation). The
// 1-day horizon also doubles as the "still open" test for the active-signals
// list below: a rec's shortest window is the first to mature.
var recPerfHorizons = []int{1, 5, 10, 20}

// recPerfExtremeCount mirrors argus eval's own hardcoded best/worst-5.
const recPerfExtremeCount = 5

// recPerfCacheTTL is deliberately an hour, the mcptools longCacheTTL order
// of magnitude (see CLAUDE.md's internal/mcptools entry) — the first
// request pays for a Yahoo GetHistory call per distinct scored ticker
// (could be dozens, several seconds), every request inside the TTL after
// that is free. Not a reuse of mcptools' own cache: that type is
// unexported and tied to *mcp.CallToolResult, the same can't-share-an-import
// constraint CLAUDE.md already documents for that package.
const recPerfCacheTTL = time.Hour

type recPerfCacheEntry struct {
	resp      recPerformanceResponse
	expiresAt time.Time
}

// recPerfStore is /recs' package-local result cache, shaped like quotes.go's
// quoteCache (a provider-wrapping struct owned by *Server, not a package
// global) but caching the whole assembled response per market rather than
// one value per ticker — the expensive part here is the fan-out of history
// calls across every distinct scored ticker, not any single one of them.
type recPerfStore struct {
	database dbReader
	history  data.HistoryProvider

	mu    sync.Mutex
	cache map[market.MarketID]recPerfCacheEntry
}

func newRecPerfStore(database dbReader, history data.HistoryProvider) *recPerfStore {
	return &recPerfStore{database: database, history: history, cache: make(map[market.MarketID]recPerfCacheEntry)}
}

func (s *recPerfStore) Get(m market.MarketID) (recPerformanceResponse, error) {
	s.mu.Lock()
	if e, ok := s.cache[m]; ok && time.Now().Before(e.expiresAt) {
		s.mu.Unlock()
		return e.resp, nil
	}
	s.mu.Unlock()

	resp, err := buildRecPerformance(s.database, s.history, m)
	if err != nil {
		return recPerformanceResponse{}, err
	}

	s.mu.Lock()
	s.cache[m] = recPerfCacheEntry{resp: resp, expiresAt: time.Now().Add(recPerfCacheTTL)}
	s.mu.Unlock()
	return resp, nil
}

// recPerfStatsCell mirrors receval.Stats for one (group, horizon) cell.
// LowSample is n < minSampleSize (reports.go's own threshold — the "低樣本
// 灰顯" convention this page reuses per docs/phase-8-trader-analytics.md
// §5.2), rendered even when n is 0 — a cell simply absent from the response
// means the group/horizon combination has no matured data at all, which the
// frontend treats identically to n=0.
type recPerfStatsCell struct {
	Horizon      int     `json:"horizon"`
	N            int     `json:"n"`
	HitRatePct   float64 `json:"hitRatePct"`
	AvgReturnPct float64 `json:"avgReturnPct"`
	AvgExcessPct float64 `json:"avgExcessPct"`
	LowSample    bool    `json:"lowSample"`
}

type recPerfGroup struct {
	Key   string             `json:"key"`
	Cells []recPerfStatsCell `json:"cells"`
}

// recPerfExtreme is one best/worst-list row — the widest configured horizon
// only (recPerfHorizons' last entry), same as argus eval's own extremes
// table.
type recPerfExtreme struct {
	Ticker          string  `json:"ticker"`
	Date            string  `json:"date"`
	Action          string  `json:"action"`
	Source          string  `json:"source"`
	EntryPrice      float64 `json:"entryPrice"`
	ExcessReturnPct float64 `json:"excessReturnPct"`
}

// recPerfCounts is the survivorship-bias honesty disclosure
// (docs/offline-rec-eval.md §5, referenced by docs/phase-8-trader-
// analytics.md §5.2) — Unscorable must always be shown alongside Scorable,
// never silently dropped from the page.
type recPerfCounts struct {
	Total      int `json:"total"`
	Hold       int `json:"hold"`
	Scorable   int `json:"scorable"`
	Unscorable int `json:"unscorable"`
}

// recPerfActiveSignal is one still-open (unmatured shortest-horizon window)
// BUY/SELL recommendation, with its excess return recomputed entry-to-today
// rather than entry-to-a-fixed-horizon — the only field that needs fresh
// math here, since EntryDate/EntryPrice/Action already come straight off the
// ScoredRec receval.Score already produced.
type recPerfActiveSignal struct {
	Ticker          string  `json:"ticker"`
	Action          string  `json:"action"`
	Source          string  `json:"source"`
	EntryDate       string  `json:"entryDate"`
	EntryPrice      float64 `json:"entryPrice"`
	DaysHeld        int     `json:"daysHeld"`
	ExcessReturnPct float64 `json:"excessReturnPct"`
}

type recPerformanceResponse struct {
	Counts   recPerfCounts    `json:"counts"`
	Horizons []int            `json:"horizons"`
	BySource []recPerfGroup   `json:"bySource"`
	ByAction []recPerfGroup   `json:"byAction"`
	Best     []recPerfExtreme `json:"best"`
	Worst    []recPerfExtreme `json:"worst"`

	// Overall is the ungrouped hit-rate/return breakdown across every scored
	// BUY/SELL rec, one cell per horizon — the hero row's top-line stat.
	Overall []recPerfStatsCell `json:"overall"`
	// BestHorizon is the horizon (from recPerfHorizons) with the highest
	// HitRatePct in Overall — ties resolve to the shortest horizon, since
	// recPerfHorizons is scanned in ascending order.
	BestHorizon int `json:"bestHorizon"`

	// ActedVsSkipped compares acted-on recs (BUY+SELL, key "acted") against
	// HOLD calls (key "skipped") — did following the recommendation actually
	// do better than sitting on it?
	ActedVsSkipped []recPerfGroup `json:"actedVsSkipped"`

	ActiveSignals []recPerfActiveSignal `json:"activeSignals"`
}

// buildRecPerformance assembles /api/rec-performance: the recommendations
// table's full history (db.GetRecommendationsSince, whole-table, same as
// argus eval's own read), scored against actual subsequent price action via
// internal/receval — reused wholesale rather than reimplemented, so this
// page's numbers are guaranteed to match a CLI `argus eval` report run
// against the same database (docs/phase-8-trader-analytics.md §5.2). One
// ticker's history fetch failing degrades that ticker's recommendations to
// "no history data" (receval.Score's own unscorable path), not a whole-
// response failure — same attach-what's-available convention as
// buildMAEMFESummary.
func buildRecPerformance(database dbReader, history data.HistoryProvider, m market.MarketID) (recPerformanceResponse, error) {
	all, err := database.GetRecommendationsSince("2000-01-01")
	if err != nil {
		return recPerformanceResponse{}, err
	}

	var total, holdCount int
	var scorable, holds []receval.Recommendation
	for _, r := range all {
		if market.Of(r.Ticker) != m {
			continue
		}
		total++
		if r.Action != "BUY" && r.Action != "SELL" {
			holdCount++
			if r.Action == "HOLD" {
				holds = append(holds, receval.Recommendation{
					Date: r.Date, Ticker: r.Ticker, Action: r.Action,
					Price: r.Price, Source: r.Source, Market: r.Market,
				})
			}
			continue
		}
		scorable = append(scorable, receval.Recommendation{
			Date: r.Date, Ticker: r.Ticker, Action: r.Action,
			Price: r.Price, Source: r.Source, Market: r.Market,
		})
	}

	resp := recPerformanceResponse{
		Horizons:       recPerfHorizons,
		Counts:         recPerfCounts{Total: total, Hold: holdCount, Scorable: len(scorable)},
		BySource:       []recPerfGroup{},
		ByAction:       []recPerfGroup{},
		Best:           []recPerfExtreme{},
		Worst:          []recPerfExtreme{},
		Overall:        []recPerfStatsCell{},
		ActedVsSkipped: []recPerfGroup{},
		ActiveSignals:  []recPerfActiveSignal{},
	}
	if len(scorable) == 0 {
		return resp, nil
	}

	benchTicker := benchmarkFor(m)
	fetchSet := map[string]bool{benchTicker: true}
	for _, r := range scorable {
		fetchSet[r.Ticker] = true
	}
	for _, r := range holds {
		fetchSet[r.Ticker] = true
	}
	candles := make(map[string][]data.Candle, len(fetchSet))
	for ticker := range fetchSet {
		c, err := history.GetHistory(ticker, "max")
		if err != nil {
			log.Printf("web: rec-performance: get history for %s: %v", ticker, err)
			continue // left out of candles -> Score reports "no history data"
		}
		candles[ticker] = c
	}

	scored := make([]receval.ScoredRec, 0, len(scorable))
	for _, r := range scorable {
		scored = append(scored, receval.Score(r, candles[r.Ticker], candles[benchTicker], recPerfHorizons))
	}

	counts := receval.CountOutcomes(scored)
	resp.Counts.Unscorable = counts.Unscorable

	resp.BySource = recPerfGroups(receval.Aggregate(scored, func(r receval.Recommendation) string {
		return receval.DisplaySource(r.Source)
	}))
	resp.ByAction = recPerfGroups(receval.Aggregate(scored, func(r receval.Recommendation) string {
		return r.Action
	}))

	maxHorizon := recPerfHorizons[len(recPerfHorizons)-1]
	best, worst := receval.Extremes(scored, maxHorizon, recPerfExtremeCount)
	resp.Best = recPerfExtremes(best, maxHorizon)
	resp.Worst = recPerfExtremes(worst, maxHorizon)

	overallGroups := recPerfGroups(receval.Aggregate(scored, func(receval.Recommendation) string { return "all" }))
	if len(overallGroups) == 1 {
		resp.Overall = overallGroups[0].Cells
		resp.BestHorizon = bestHorizon(resp.Overall)
	}

	holdScored := make([]receval.ScoredRec, 0, len(holds))
	for _, r := range holds {
		holdScored = append(holdScored, receval.Score(r, candles[r.Ticker], candles[benchTicker], recPerfHorizons))
	}
	actedVsSkipped := receval.Aggregate(scored, func(receval.Recommendation) string { return "acted" })
	for k, v := range receval.Aggregate(holdScored, func(receval.Recommendation) string { return "skipped" }) {
		actedVsSkipped[k] = v
	}
	resp.ActedVsSkipped = recPerfGroups(actedVsSkipped)

	resp.ActiveSignals = recPerfActiveSignals(scored, candles, benchTicker)

	return resp, nil
}

// bestHorizon scans an Overall breakdown for the horizon with the highest
// hit rate, preferring the shortest horizon on a tie (cells is already
// ordered by ascending recPerfHorizons).
func bestHorizon(cells []recPerfStatsCell) int {
	var best recPerfStatsCell
	for _, c := range cells {
		if c.N > 0 && c.HitRatePct > best.HitRatePct {
			best = c
		}
	}
	return best.Horizon
}

// recPerfActiveSignals lists still-open BUY/SELL recs — those whose
// shortest-horizon window (recPerfHorizons[0]) hasn't matured yet — with an
// excess return recomputed entry-to-today instead of entry-to-horizon, using
// only the candles already fetched for scoring (no extra data call).
func recPerfActiveSignals(scored []receval.ScoredRec, candles map[string][]data.Candle, benchTicker string) []recPerfActiveSignal {
	out := []recPerfActiveSignal{}
	bench := candles[benchTicker]
	for _, sr := range scored {
		if sr.Unscorable || len(sr.Windows) == 0 || sr.Windows[0].Matured {
			continue
		}
		ticker := candles[sr.Rec.Ticker]
		entryIdx := candleIndexForDate(ticker, sr.EntryDate)
		if entryIdx < 0 {
			continue
		}
		lastIdx := len(ticker) - 1
		tickerReturn := pctChangeLocal(sr.EntryPrice, ticker[lastIdx].Close)
		excess := tickerReturn
		if benchEntryIdx := candleIndexForDate(bench, sr.EntryDate); benchEntryIdx >= 0 {
			if benchEntry := bench[benchEntryIdx].Close; benchEntry > 0 {
				excess = tickerReturn - pctChangeLocal(benchEntry, bench[len(bench)-1].Close)
			}
		}
		out = append(out, recPerfActiveSignal{
			Ticker: sr.Rec.Ticker, Action: sr.Rec.Action, Source: receval.DisplaySource(sr.Rec.Source),
			EntryDate: sr.EntryDate, EntryPrice: sr.EntryPrice,
			DaysHeld:        lastIdx - entryIdx,
			ExcessReturnPct: excess,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryDate < out[j].EntryDate })
	return out
}

// candleIndexForDate finds the candle matching date (YYYY-MM-DD) in an
// oldest-first series, or -1 if absent.
func candleIndexForDate(candles []data.Candle, date string) int {
	for i, c := range candles {
		if c.Date.Format("2006-01-02") == date {
			return i
		}
	}
	return -1
}

func pctChangeLocal(from, to float64) float64 {
	if from == 0 {
		return 0
	}
	return (to - from) / from * 100
}

// recPerfGroups converts receval.Aggregate's map[string]map[int]Stats into a
// stable-ordered, JSON-friendly slice — sorted by key so the response is
// deterministic across calls (Go map iteration order isn't).
func recPerfGroups(groups map[string]map[int]receval.Stats) []recPerfGroup {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]recPerfGroup, 0, len(keys))
	for _, k := range keys {
		cells := make([]recPerfStatsCell, 0, len(recPerfHorizons))
		for _, h := range recPerfHorizons {
			s := groups[k][h]
			cells = append(cells, recPerfStatsCell{
				Horizon:      h,
				N:            s.N,
				HitRatePct:   s.HitRate(),
				AvgReturnPct: s.AvgReturn(),
				AvgExcessPct: s.AvgExcess(),
				LowSample:    s.N < minSampleSize,
			})
		}
		out = append(out, recPerfGroup{Key: k, Cells: cells})
	}
	return out
}

func recPerfExtremes(recs []receval.ScoredRec, horizon int) []recPerfExtreme {
	out := make([]recPerfExtreme, 0, len(recs))
	for _, sr := range recs {
		var excess float64
		for _, w := range sr.Windows {
			if w.Horizon == horizon {
				excess = w.ExcessReturnPct
				break
			}
		}
		out = append(out, recPerfExtreme{
			Ticker: sr.Rec.Ticker, Date: sr.Rec.Date, Action: sr.Rec.Action,
			Source:     receval.DisplaySource(sr.Rec.Source),
			EntryPrice: sr.EntryPrice, ExcessReturnPct: excess,
		})
	}
	return out
}

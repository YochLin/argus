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

// recPerfHorizons mirrors argus eval's own default -horizons ("5,20,60") so
// the /recs page's numbers are directly comparable to a CLI report run
// against the same database (docs/phase-8-trader-analytics.md §5.2 — same
// receval functions, same windows, not a second scoring implementation).
var recPerfHorizons = []int{5, 20, 60}

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

type recPerformanceResponse struct {
	Counts   recPerfCounts    `json:"counts"`
	Horizons []int            `json:"horizons"`
	BySource []recPerfGroup   `json:"bySource"`
	ByAction []recPerfGroup   `json:"byAction"`
	Best     []recPerfExtreme `json:"best"`
	Worst    []recPerfExtreme `json:"worst"`
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
	var scorable []receval.Recommendation
	for _, r := range all {
		if market.Of(r.Ticker) != m {
			continue
		}
		total++
		if r.Action != "BUY" && r.Action != "SELL" {
			holdCount++
			continue
		}
		scorable = append(scorable, receval.Recommendation{
			Date: r.Date, Ticker: r.Ticker, Action: r.Action,
			Price: r.Price, Source: r.Source, Market: r.Market,
		})
	}

	resp := recPerformanceResponse{
		Horizons: recPerfHorizons,
		Counts:   recPerfCounts{Total: total, Hold: holdCount, Scorable: len(scorable)},
		BySource: []recPerfGroup{},
		ByAction: []recPerfGroup{},
		Best:     []recPerfExtreme{},
		Worst:    []recPerfExtreme{},
	}
	if len(scorable) == 0 {
		return resp, nil
	}

	benchTicker := benchmarkFor(m)
	fetchSet := map[string]bool{benchTicker: true}
	for _, r := range scorable {
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

	return resp, nil
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
			EntryPrice: sr.EntryPrice, ExcessReturnPct: excess,
		})
	}
	return out
}

package web

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/signals"
)

// sectorFlowPeriods maps the "tf" query param to signals.TickerMoneyFlow's
// trading-day period (docs/phase-18-sector-money-flow.md §4.2: 1日≈1、
// 1週≈5、1月≈20 trading days, not calendar days). All three windows are
// computed from the same one candle fetch per ticker in a single scan, not
// three separate provider round trips.
var sectorFlowPeriods = map[string]int{"1d": 1, "1w": 5, "1m": 20}

// sectorFlowHistoryRange comfortably covers the longest period (20 trading
// days) plus TickerMoneyFlow's one leading baseline day, tolerant of
// holidays thinning a calendar month down to fewer trading days.
const sectorFlowHistoryRange = "3mo"

// sectorFlowRequestDelay paces per-ticker external calls across a ~150-500
// ticker scan, the same "wall-clock is free, request rate isn't" tradeoff
// bot.universeScanRequestDelay documents (that constant is an unexported
// internal/bot value, so it's duplicated here rather than imported). A var,
// not a const, so tests can zero it out rather than taking several real
// seconds per test case.
var sectorFlowRequestDelay = 1 * time.Second

// SectorFlowItem is one constituent ticker within a SectorFlowSector. Cap is
// real market cap for US, the dollar-volume proxy for TW (see
// SectorFlowSector.MarketCap's doc comment) — same field regardless of
// source so the frontend never has to special-case market.
type SectorFlowItem struct {
	Ticker    string  `json:"ticker"`
	Cap       float64 `json:"cap"`
	Flow      float64 `json:"flow"`
	ChangePct float64 `json:"changePct"`
	Held      bool    `json:"held"`
}

// SectorFlowSector aggregates its constituents' money flow. NetFlow/
// DollarVolume/MarketCap are summed across constituents before any ratio is
// taken (see TickerMoneyFlow's doc comment on why summing first, not
// averaging per-ticker results, matters for cap-weighting). MarketCap holds
// real market cap for US (Finnhub); TW has no free market-cap source (see
// docs/phase-18-sector-money-flow.md §3), so it holds the same dollar-volume
// proxy as DollarVolume there — the field name doesn't change, only what
// backs it, so the frontend renders one code path and the TW page adds its
// own "proxy, not real cap" note.
type SectorFlowSector struct {
	Name         string            `json:"name"`
	NetFlow      float64           `json:"netFlow"`
	DollarVolume float64           `json:"dollarVolume"`
	MarketCap    float64           `json:"marketCap"`
	ChangePct    float64           `json:"changePct"`
	TickerCount  int               `json:"tickerCount"`
	HeldCount    int               `json:"heldCount"`
	Items        []SectorFlowItem  `json:"items"`
}

// SectorFlowResponse is /api/sectorflow's body. Ready false means no scan
// has completed yet for this market (process just restarted) — the
// frontend shows a "not ready" state rather than treating an empty Sectors
// slice as "market has no sectors."
type SectorFlowResponse struct {
	Ready      bool               `json:"ready"`
	ComputedAt time.Time          `json:"computedAt"`
	Sectors    []SectorFlowSector `json:"sectors"`
}

// sectorFlowCache holds the latest scan result per market×timeframe —
// process-only (Phase 18 §4.3), never persisted to DB. A stale-until-next-
// scan in-memory value is an acceptable tradeoff for skipping a per-request,
// several-hundred-ticker recompute that would otherwise make a dashboard
// page load take minutes.
type sectorFlowCache struct {
	mu   sync.RWMutex
	data map[market.MarketID]map[string]SectorFlowResponse // market -> tf -> response
}

func newSectorFlowCache() *sectorFlowCache {
	return &sectorFlowCache{data: make(map[market.MarketID]map[string]SectorFlowResponse)}
}

func (c *sectorFlowCache) get(m market.MarketID, tf string) SectorFlowResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if byTF, ok := c.data[m]; ok {
		if resp, ok := byTF[tf]; ok {
			return resp
		}
	}
	return SectorFlowResponse{Ready: false}
}

func (c *sectorFlowCache) set(m market.MarketID, byTF map[string]SectorFlowResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[m] = byTF
}

// periodChangePct is the ticker's own price change over the trailing period
// trading days — TickerMoneyFlow's flow figure has no direct "up or down
// overall" reading of its own (a mixed week of big-down/small-up days can
// net either sign), so this is computed separately off the same candles for
// the treemap's per-tf color scale (docs/phase-18-sector-money-flow.md §4.5).
func periodChangePct(candles []data.Candle, period int) float64 {
	if period <= 0 || len(candles) < period+1 {
		return 0
	}
	start := candles[len(candles)-period-1].Close
	end := candles[len(candles)-1].Close
	if start == 0 {
		return 0
	}
	return (end - start) / start * 100
}

// sectorFlowBuilder accumulates one sector's aggregate across constituents
// before ChangePct's cap-weighted average can be finalized (needs the total
// cap first) — capWeightedChangeSum stays unexported so it never leaks into
// the JSON response.
type sectorFlowBuilder struct {
	sector               SectorFlowSector
	capWeightedChangeSum float64
}

// RunSectorFlowScan computes market m's sector money-flow snapshot (all
// three tf windows from one candle fetch per ticker) and stores it in
// s.sectorFlow — Phase 18's nightly scheduler job, registered in main.go
// right after that market's closing snapshot/universe scan. A silent
// background job like bot.RunClosingSnapshot: a single ticker's fetch
// failure is logged and skipped (attach-what's-available), not surfaced
// anywhere but the log. The whole scan is skipped outright — leaving
// whatever the cache already held untouched, not overwriting it with an
// empty result — when market m has no classification source configured at
// all (s.sector nil for US, s.industryMap nil for TW), matching every other
// optional-provider nil-check's "feature quietly absent" convention.
func (s *Server) RunSectorFlowScan(ctx context.Context, m market.MarketID) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in sector flow scan (%s): %v", m, p)
		}
	}()

	var industryMap map[string]string
	if m == market.TW {
		if s.industryMap == nil {
			logger.Infof("web: sector flow scan (%s): no industry classification source configured, skipping", m)
			return
		}
		var err error
		industryMap, err = s.industryMap.GetIndustryMap()
		if err != nil {
			logger.Errorf("web: sector flow scan (%s): get industry map: %v", m, err)
			return
		}
	} else if s.sector == nil {
		logger.Infof("web: sector flow scan (%s): no sector provider configured, skipping", m)
		return
	}

	entries, err := s.db.GetUniverse()
	if err != nil {
		logger.Errorf("web: sector flow scan (%s): get universe: %v", m, err)
		return
	}
	positions, err := s.db.GetPositions()
	if err != nil {
		logger.Errorf("web: sector flow scan (%s): get positions: %v", m, err)
		positions = nil // degrade to "nothing held known" rather than aborting the scan
	}
	held := make(map[string]bool, len(positions))
	for _, p := range positions {
		if market.Of(p.Ticker) == m {
			held[p.Ticker] = true
		}
	}

	perTF := make(map[string]map[string]*sectorFlowBuilder, len(sectorFlowPeriods))
	for tf := range sectorFlowPeriods {
		perTF[tf] = make(map[string]*sectorFlowBuilder)
	}

	scanned := 0
	first := true
	for _, entry := range entries {
		ticker := entry.Ticker
		if market.Of(ticker) != m {
			continue
		}
		if !first {
			select {
			case <-ctx.Done():
				logger.Infof("web: sector flow scan (%s): cancelled after %d tickers", m, scanned)
				return
			case <-time.After(sectorFlowRequestDelay):
			}
		}
		first = false

		var sectorName string
		var usCap float64
		if m == market.TW {
			name := industryMap[ticker]
			if name == "" {
				continue
			}
			sectorName = name
		} else {
			info, err := s.sector.GetSector(ticker)
			if err != nil {
				logger.Errorf("web: sector flow scan (%s): get sector for %s: %v", m, ticker, err)
				continue
			}
			sectorName = info.Industry
			usCap = info.MarketCapMillion
		}

		candles, err := s.history.GetHistory(ticker, sectorFlowHistoryRange)
		if err != nil {
			logger.Errorf("web: sector flow scan (%s): get history for %s: %v", m, ticker, err)
			continue
		}
		scanned++
		isHeld := held[ticker]

		for tf, period := range sectorFlowPeriods {
			posMF, negMF, dollarVol := signals.TickerMoneyFlow(candles, period)
			changePct := periodChangePct(candles, period)
			cap := usCap
			if m == market.TW {
				cap = dollarVol
			}

			b, ok := perTF[tf][sectorName]
			if !ok {
				b = &sectorFlowBuilder{sector: SectorFlowSector{Name: sectorName}}
				perTF[tf][sectorName] = b
			}
			b.sector.NetFlow += posMF - negMF
			b.sector.DollarVolume += dollarVol
			b.sector.MarketCap += cap
			b.sector.TickerCount++
			if isHeld {
				b.sector.HeldCount++
			}
			b.capWeightedChangeSum += changePct * cap
			b.sector.Items = append(b.sector.Items, SectorFlowItem{
				Ticker:    ticker,
				Cap:       cap,
				Flow:      posMF - negMF,
				ChangePct: changePct,
				Held:      isHeld,
			})
		}
	}

	now := time.Now()
	byTF := make(map[string]SectorFlowResponse, len(perTF))
	for tf, sectors := range perTF {
		list := make([]SectorFlowSector, 0, len(sectors))
		for _, b := range sectors {
			if b.sector.MarketCap > 0 {
				b.sector.ChangePct = b.capWeightedChangeSum / b.sector.MarketCap
			}
			sort.Slice(b.sector.Items, func(i, j int) bool { return b.sector.Items[i].Cap > b.sector.Items[j].Cap })
			list = append(list, b.sector)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].MarketCap > list[j].MarketCap })
		byTF[tf] = SectorFlowResponse{Ready: true, ComputedAt: now, Sectors: list}
	}
	s.sectorFlow.set(m, byTF)
	logger.Infof("web: sector flow scan (%s): computed %d sectors across %d tickers", m, len(byTF["1d"].Sectors), scanned)
}

// handleSectorFlow serves GET /api/sectorflow?market=us|tw&tf=1d|1w|1m —
// an invalid/absent tf defaults to "1d", matching marketParam's own
// default-on-anything-unrecognized convention.
func (s *Server) handleSectorFlow(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleSectorFlow: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()
	tf := r.URL.Query().Get("tf")
	if _, ok := sectorFlowPeriods[tf]; !ok {
		tf = "1d"
	}
	resp := s.sectorFlow.get(marketParam(r), tf)
	writeJSON(w, http.StatusOK, resp)
}

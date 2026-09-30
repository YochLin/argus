package web

import (
	"net/http"
	"slices"
	"sort"
	"sync"
	"time"

	"argus/internal/data"
	"argus/internal/logger"
	"argus/internal/market"
)

// Phase 27 P3 — the chart page's 同業比較 card: the viewed ticker next to the
// watchlist/held tickers of the same market that share its sector, with 20/60/
// 120-trading-day changes and each peer's 60-day change relative to the viewed
// ticker. Peers are limited to names the user already tracks: their history is
// what the list page fetches anyway, and "who else do I hold in this sector"
// is the question the card answers.

// peerHistoryRange gives >= 121 candles (the 120-day change needs one more
// close than its window) for both markets.
const peerHistoryRange = "1y"

const (
	// peerSectorTTL: a company's industry classification is effectively static;
	// the TTL only bounds how stale a reclassification can get.
	peerSectorTTL = 24 * time.Hour
	// peerSectorFailTTL: a failed US lookup (no industry — ETFs — or a transient
	// error) is remembered briefly so the same unclassifiable names don't cost
	// a Finnhub call on every page view.
	peerSectorFailTTL = time.Hour
)

type peerRow struct {
	Ticker string   `json:"ticker"`
	Self   bool     `json:"self"`
	Price  float64  `json:"price"` // last close, not a live quote
	Chg20  *float64 `json:"chg20"` // nil when the history is shorter than the window
	Chg60  *float64 `json:"chg60"`
	Chg120 *float64 `json:"chg120"`
	Rel    *float64 `json:"rel"` // Chg60 minus the viewed ticker's; nil for that ticker itself
}

// peersResponse is /api/chart/peers' body. Peers is empty ("[]", not null)
// when there is nothing to compare — fewer than two tickers in the sector, or
// no sector source for this market — and the frontend hides the card then.
type peersResponse struct {
	Sector string    `json:"sector"`
	Peers  []peerRow `json:"peers"`
}

type peerSectorEntry struct {
	name    string
	expires time.Time
}

// peerSectorCache is usable as a zero value (tests build Server literals).
type peerSectorCache struct {
	mu    sync.Mutex
	us    map[string]peerSectorEntry
	tw    map[string]string
	twExp time.Time
}

// sectorsFor returns ticker -> sector for the tickers of market m whose sector
// is known; unknown ones are simply absent.
func (s *Server) sectorsFor(m market.MarketID, tickers []string) map[string]string {
	c := &s.peerSectors
	out := make(map[string]string, len(tickers))

	if m == market.TW {
		if s.industryMap == nil {
			return out
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.tw == nil || time.Now().After(c.twExp) {
			if mp, err := s.industryMap.GetIndustryMap(); err != nil {
				logger.Errorf("web: peers: get industry map: %v", err)
			} else {
				c.tw, c.twExp = mp, time.Now().Add(peerSectorTTL)
			}
		}
		for _, t := range tickers {
			if name := c.tw[t]; name != "" {
				out[t] = name
			}
		}
		return out
	}

	if s.sector == nil {
		return out
	}
	now := time.Now()
	var stale []string
	c.mu.Lock()
	for _, t := range tickers {
		if e, ok := c.us[t]; ok && now.Before(e.expires) {
			if e.name != "" {
				out[t] = e.name
			}
		} else {
			stale = append(stale, t)
		}
	}
	c.mu.Unlock()

	var wg sync.WaitGroup
	for _, t := range stale {
		wg.Add(1)
		go func(ticker string) {
			defer wg.Done()
			info, err := s.sector.GetSector(ticker)
			e := peerSectorEntry{name: info.Industry, expires: time.Now().Add(peerSectorTTL)}
			if err != nil {
				logger.Errorf("web: peers: get sector for %s: %v", ticker, err)
				e = peerSectorEntry{expires: time.Now().Add(peerSectorFailTTL)}
			}
			c.mu.Lock()
			if c.us == nil {
				c.us = make(map[string]peerSectorEntry)
			}
			c.us[ticker] = e
			if e.name != "" {
				out[ticker] = e.name
			}
			c.mu.Unlock()
		}(t)
	}
	wg.Wait()
	return out
}

func (s *Server) buildPeers(ticker string) (peersResponse, error) {
	resp := peersResponse{Peers: []peerRow{}}
	m := market.Of(ticker)

	tracked, err := buildTickers(s.db, m)
	if err != nil {
		return resp, err
	}
	pool := tracked.Tickers
	if !slices.Contains(pool, ticker) { // reached by URL, neither watched nor held
		pool = append(pool, ticker)
	}

	sectors := s.sectorsFor(m, pool)
	sector := sectors[ticker]
	if sector == "" {
		return resp, nil
	}
	group := []string{ticker}
	for _, t := range pool {
		if t != ticker && sectors[t] == sector {
			group = append(group, t)
		}
	}
	if len(group) < 2 {
		return resp, nil
	}

	histories := fetchHistories(s.history, group, peerHistoryRange)
	if len(histories[ticker]) == 0 {
		return resp, nil // nothing to be relative to
	}
	selfChg60, selfOK := trailingChangePct(histories[ticker], 60)

	rows := make([]peerRow, 0, len(group))
	for _, t := range group {
		cs := histories[t]
		if len(cs) == 0 {
			continue // history unavailable — drop the row rather than show a blank price
		}
		row := peerRow{Ticker: t, Self: t == ticker, Price: cs[len(cs)-1].Close}
		row.Chg20 = changePtr(cs, 20)
		row.Chg60 = changePtr(cs, 60)
		row.Chg120 = changePtr(cs, 120)
		if !row.Self && selfOK && row.Chg60 != nil {
			rel := *row.Chg60 - selfChg60
			row.Rel = &rel
		}
		rows = append(rows, row)
	}
	if len(rows) < 2 {
		return resp, nil
	}

	// Strongest 60-day first, rows without a 60-day figure last.
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].Chg60, rows[j].Chg60
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		return *a > *b
	})
	resp.Sector, resp.Peers = sector, rows
	return resp, nil
}

func changePtr(candles []data.Candle, period int) *float64 {
	if v, ok := trailingChangePct(candles, period); ok {
		return &v
	}
	return nil
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handlePeers: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	ticker := r.URL.Query().Get("ticker")
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "ticker is required")
		return
	}
	resp, err := s.buildPeers(ticker)
	if err != nil {
		logger.Errorf("web: build peers for %s: %v", ticker, err)
		writeError(w, http.StatusInternalServerError, "failed to build peers")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

package web

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"argus/internal/market"
)

// marketParam parses r's "market" query parameter into a market.MarketID —
// "tw" for Taiwan, anything else (including absent, the common case)
// defaults to US, preserving pre-Phase-6 behavior for every existing
// client. See docs/phase-6-tw-market.md §4.4.
func marketParam(r *http.Request) market.MarketID {
	if r.URL.Query().Get("market") == "tw" {
		return market.TW
	}
	return market.US
}

// dashboardResponse is /api/dashboard's body. Only raw numbers/dates/
// tickers — no display strings — per docs/phase-5-web-dashboard.md's UI
// language decision: the frontend picks zh/en display text itself from
// /api/config's lang, so internal/i18n never needs touching for this API.
type dashboardResponse struct {
	KPIs      kpisResponse       `json:"kpis"`
	Curve     []DateValue        `json:"curve"`
	Positions []positionResponse `json:"positions"`
}

type kpisResponse struct {
	NetPnL       float64 `json:"netPnL"`
	WinRate      float64 `json:"winRate"`
	ProfitFactor float64 `json:"profitFactor"`
	Expectancy   float64 `json:"expectancy"`
	MaxDrawdown  float64 `json:"maxDrawdown"`
	// YTDReturnPct/QTDReturnPct/HTDReturnPct are *float64 (not omitempty) so
	// "no usable baseline yet" (see pnl.go's PeriodReturnPct) serializes as
	// an explicit JSON null, never an omitted key or a misleading 0 —
	// docs/phase-5-web-dashboard.md's "don't pad missing data with zero"
	// principle.
	YTDReturnPct *float64 `json:"ytdReturnPct"`
	QTDReturnPct *float64 `json:"qtdReturnPct"`
	HTDReturnPct *float64 `json:"htdReturnPct"`
}

type positionResponse struct {
	Ticker           string  `json:"ticker"`
	Shares           float64 `json:"shares"`
	AvgCost          float64 `json:"avgCost"`
	Price            float64 `json:"price"`
	MarketValue      float64 `json:"marketValue"`
	UnrealizedPnL    float64 `json:"unrealizedPnL"`
	UnrealizedPnLPct float64 `json:"unrealizedPnLPct"`
}

type statusResponse struct {
	WatchingCount int     `json:"watchingCount"`
	SPYChangePct  float64 `json:"spyChangePct"`
	LastCloseDate string  `json:"lastCloseDate"`
}

type configResponse struct {
	Lang string `json:"lang"`
}

// calendarResponse is /api/calendar's body — same "raw data only" rule as
// dashboardResponse. Days is the DailyPnL series (pnl.go) restricted to the
// requested month; a calendar day absent from Days simply has no data
// (weekend, holiday, or before daily_snapshots started) rather than a
// misleading 0 — the frontend must render "no data" distinctly from a
// $0 day. Transactions is the whole month's raw transaction rows, letting
// the frontend's click-a-day panel and week/month summary rows (design
// doc's A3) both work off one response with no further API calls.
type calendarResponse struct {
	Month        string                `json:"month"`
	Days         []DateValue           `json:"days"`
	Transactions []transactionResponse `json:"transactions"`
}

type transactionResponse struct {
	Date        string  `json:"date"`
	Ticker      string  `json:"ticker"`
	Side        string  `json:"side"`
	Shares      float64 `json:"shares"`
	Price       float64 `json:"price"`
	Fee         float64 `json:"fee"`
	RealizedPnL float64 `json:"realizedPnL"`
}

// roundsResponse is /api/rounds' body — every position round trip across
// every ticker ever transacted (rounds.go's segmentRounds), most-recently-
// started first. The frontend's round picker groups these by ticker.
type roundsResponse struct {
	Rounds []roundSummary `json:"rounds"`
}

type roundSummary struct {
	Ticker      string  `json:"ticker"`
	Start       string  `json:"start"`
	End         string  `json:"end"` // "" while still open
	Open        bool    `json:"open"`
	Shares      float64 `json:"shares"`
	RealizedPnL float64 `json:"realizedPnL"`
}

// roundDetailResponse is /api/round-detail's body: one round's daily
// candles (padded before/after — see buildRoundDetail) plus its own legs as
// trades, for the round detail page's lightweight-charts K-line and buy/
// sell markers. MAEPct/MFEPct (Phase 5 PR4, design doc §A2) are the round's
// own approximate max adverse/favorable excursion, computed from the same
// candles already fetched here — HasMAEMFE is false only when the round's
// window has no matching candle data at all (shouldn't normally happen for
// a real round, but a delisted/renamed ticker could hit it).
type roundDetailResponse struct {
	Ticker    string                `json:"ticker"`
	Start     string                `json:"start"`
	End       string                `json:"end"`
	Candles   []candleResponse      `json:"candles"`
	Trades    []transactionResponse `json:"trades"`
	MAEPct    float64               `json:"maePct"`
	MFEPct    float64               `json:"mfePct"`
	HasMAEMFE bool                  `json:"hasMaeMfe"`
}

type candleResponse struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

// chartResponse is /api/chart's body: ticker's ~1y of daily candles plus the
// support/resistance levels computed from that same window (chart.go's
// buildChart). Candles/Levels are "[]" not "null" when empty, same
// dashboard.go convention.
type chartResponse struct {
	Ticker  string           `json:"ticker"`
	Candles []candleResponse `json:"candles"`
	Levels  []levelResponse  `json:"levels"`
}

type levelResponse struct {
	Price     float64 `json:"price"`
	Touches   int     `json:"touches"`
	FirstDate string  `json:"firstDate"`
	LastDate  string  `json:"lastDate"`
}

// tickersResponse is /api/tickers' body - the /chart list page's ticker
// picker source (chart.go's buildTickers).
type tickersResponse struct {
	Tickers []string `json:"tickers"`
}

// companyNamesResponse is /api/company-names' body — TW ticker → Chinese
// short name (e.g. "2330" → "台積電"), see companynames.go's
// buildCompanyNames. Always a map (possibly empty), never null, so the
// frontend can index it unconditionally.
type companyNamesResponse struct {
	Names map[string]string `json:"names"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, configResponse{Lang: string(s.lang)})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleStatus: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	writeJSON(w, http.StatusOK, buildStatus(s.db, marketParam(r)))
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleDashboard: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	resp, err := buildDashboard(s.db, s.quotes, marketParam(r))
	if err != nil {
		log.Printf("web: build dashboard: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build dashboard")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleCalendar: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	resp, err := buildCalendar(s.db, r.URL.Query().Get("month"), marketParam(r))
	if err != nil {
		log.Printf("web: build calendar: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build calendar")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRounds(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleRounds: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	resp, err := buildRounds(s.db, marketParam(r))
	if err != nil {
		log.Printf("web: build rounds: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build rounds")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRoundDetail(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleRoundDetail: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	ticker := r.URL.Query().Get("ticker")
	start := r.URL.Query().Get("start")
	if ticker == "" || start == "" {
		writeError(w, http.StatusBadRequest, "ticker and start are required")
		return
	}

	resp, err := buildRoundDetail(s.db, s.history, ticker, start)
	if errors.Is(err, errRoundNotFound) {
		writeError(w, http.StatusNotFound, "round not found")
		return
	}
	if err != nil {
		log.Printf("web: build round detail: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build round detail")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleReports: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	resp, err := buildReports(s.db, s.history, marketParam(r))
	if err != nil {
		log.Printf("web: build reports: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build reports")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChart(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleChart: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	ticker := r.URL.Query().Get("ticker")
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "ticker is required")
		return
	}

	resp, err := buildChart(s.history, ticker)
	if err != nil {
		log.Printf("web: build chart for %s: %v", ticker, err)
		writeError(w, http.StatusInternalServerError, "failed to build chart")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleTickers(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleTickers: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	resp, err := buildTickers(s.db, marketParam(r))
	if err != nil {
		log.Printf("web: build tickers: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build tickers")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCompanyNames(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleCompanyNames: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	writeJSON(w, http.StatusOK, buildCompanyNames(s.db, s.companyNames))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("web: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

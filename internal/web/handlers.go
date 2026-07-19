package web

import (
	"encoding/json"
	"log"
	"net/http"
)

// dashboardResponse is /api/dashboard's body. Only raw numbers/dates/
// tickers — no display strings — per docs/phase-5-web-dashboard.md's UI
// language decision: the frontend picks zh/en display text itself from
// /api/config's lang, so internal/i18n never needs touching for this API.
type dashboardResponse struct {
	KPIs      kpisResponse       `json:"kpis"`
	Curve     []DateValue        `json:"curve"`
	Positions []positionResponse `json:"positions"`
	Status    statusResponse     `json:"status"`
}

type kpisResponse struct {
	NetPnL       float64 `json:"netPnL"`
	WinRate      float64 `json:"winRate"`
	ProfitFactor float64 `json:"profitFactor"`
	Expectancy   float64 `json:"expectancy"`
	MaxDrawdown  float64 `json:"maxDrawdown"`
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

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, configResponse{Lang: string(s.lang)})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("web: panic in handleDashboard: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	resp, err := buildDashboard(s.db, s.quotes)
	if err != nil {
		log.Printf("web: build dashboard: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to build dashboard")
		return
	}
	writeJSON(w, http.StatusOK, resp)
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

package web

import (
	"net/http"
	"strings"

	"argus/internal/db"
	"argus/internal/logger"
)

// eventsListLimit is the fixed page size for GET /api/events — same "no
// pagination until someone actually needs it" call as llmRunsListLimit, and
// for a stronger reason: price events fire a handful of times a month, not
// once per report.
const eventsListLimit = 60

// priceEventResponse is one row of GET /api/events — db.PriceEvent verbatim.
// Summary is "" for an event recorded past a run's LLM-writeup cap (see
// db.SavePriceEvent), which the client must render as "no summary" rather
// than as a failure.
type priceEventResponse struct {
	ID            int64   `json:"id"`
	Ticker        string  `json:"ticker"`
	Market        string  `json:"market"`
	Date          string  `json:"date"`
	GapPct        float64 `json:"gapPct"`
	ChangePct     float64 `json:"changePct"`
	CumulativePct float64 `json:"cumulativePct"`
	Summary       string  `json:"summary"`
	CreatedAt     string  `json:"createdAt"`
}

type eventsResponse struct {
	Events []priceEventResponse `json:"events"`
}

// handleEvents is Phase 20 後續 PR1's web read of the price-event log — the
// web counterpart of the /events Telegram command (bot.handleEvents), which
// until now was the only place a recorded event summary could be read at
// all. ?ticker=X switches from the cross-ticker recent list to that ticker's
// own history, same two-form split as the command.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleEvents: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	ticker := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("ticker")))
	var events []db.PriceEvent
	var err error
	if ticker == "" {
		events, err = s.db.GetRecentPriceEvents(eventsListLimit)
	} else {
		events, err = s.db.GetPriceEventsForTicker(ticker, eventsListLimit)
	}
	if err != nil {
		logger.Errorf("web: list price events: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list price events")
		return
	}

	resp := eventsResponse{Events: make([]priceEventResponse, len(events))}
	for i, ev := range events {
		resp.Events[i] = priceEventResponse{
			ID:            ev.ID,
			Ticker:        ev.Ticker,
			Market:        ev.Market,
			Date:          ev.Date,
			GapPct:        ev.GapPct,
			ChangePct:     ev.ChangePct,
			CumulativePct: ev.CumulativePct,
			Summary:       ev.Summary,
			CreatedAt:     ev.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

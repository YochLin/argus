package web

import (
	"net/http"
	"time"

	"argus/internal/service"
)

// displayCurrencies are the top bar's 顯示幣別 choices besides TWD (the
// design's list). Values across the wealth pages are all TWD server-side;
// the client multiplies by 1/rate for display only.
var displayCurrencies = []string{"USD", "JPY", "EUR", "CNY"}

// handleWealthFX serves GET /api/wealth/fx: TWD per one unit of each display
// currency. A currency whose rate can't be priced is omitted rather than
// guessed (same "don't fabricate" rule as service.RateToTWD), and the client
// then greys it out.
func (s *Server) handleWealthFX(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("2006-01-02")
	rates := map[string]float64{}
	for _, c := range displayCurrencies {
		if rate, ok := service.RateToTWD(c, today, true, s.quotes, s.fxDB); ok {
			rates[c] = rate
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rates": rates})
}

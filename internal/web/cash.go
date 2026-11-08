package web

import (
	"math"
	"net/http"
	"strconv"

	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// settingWriter is the one store method POST /api/cash needs.
type settingWriter interface {
	SetSetting(key, value string) error
}

type cashRequest struct {
	Market string  `json:"market"`
	Amount float64 `json:"amount"`
}

// handleCashSet is POST /api/cash: the risk page's cash card, the web
// counterpart of Telegram's /cash. It writes the same settings key
// (service.CashSettingKey) the bot's adjustCash and the Sinopac sync keep
// nudging, so a value set here is the baseline those build on — and, like
// /cash, it's a correction of a reference value, not a ledger entry.
func (s *Server) handleCashSet(w http.ResponseWriter, r *http.Request) {
	var req cashRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var m market.MarketID
	switch req.Market {
	case "us":
		m = market.US
	case "tw":
		m = market.TW
	default:
		writeError(w, http.StatusBadRequest, "market must be us or tw")
		return
	}
	if req.Amount < 0 || math.IsInf(req.Amount, 0) || math.IsNaN(req.Amount) {
		writeError(w, http.StatusBadRequest, "amount must be a number of at least 0")
		return
	}
	if err := s.cashDB.SetSetting(service.CashSettingKey(m), strconv.FormatFloat(req.Amount, 'f', 2, 64)); err != nil {
		logger.Errorf("web: set cash %s: %v", m, err)
		writeError(w, http.StatusInternalServerError, "failed to save cash")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
}

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// TradeExecutor is web's narrow view of *bot.Bot for the write endpoints
// that need bot-layer behavior beyond the raw DB write — Telegram
// confirmation push, buy's auto-watchlist-add, and a closing sell's
// sell-review trigger (see docs/phase-10-web-trade-input.md §4.2's "one
// write path" rule: these three must never be duplicated by calling
// db.RecordBuy/RecordSell directly). internal/web doesn't import
// internal/bot and *bot.Bot doesn't import internal/web — main.go wires the
// concrete type in, the same seam convention as bot.Channel/
// llm.Provider/NewClientWithProvider.
type TradeExecutor interface {
	ExecuteBuy(ticker string, shares, price float64, fee *float64, date string) (string, error)
	ExecuteSell(ctx context.Context, ticker string, shares, price float64, fee *float64, date string) (string, error)
	ExecuteSetStop(ticker string, price float64) (string, error)
	// ExecuteAddBuyAlert backs POST /api/buy-alerts/add — a quote fetch (to
	// infer the alert's direction, see bot.buyAlertDirection) is bot-layer
	// behavior beyond the raw DB write, same rationale as ExecuteSetStop.
	ExecuteAddBuyAlert(ticker string, price float64) (string, error)
}

// watchlistWriter is web's narrow view of *db.DB for watchlist add/remove —
// unlike buy/sell/stop, these have no bot-layer behavior beyond the DB
// write itself (see handleAdd/handleRemove in internal/bot), so they skip
// the TradeExecutor seam and hit the DB directly.
type watchlistWriter interface {
	AddTicker(ticker string) error
	RemoveTicker(ticker string) error
}

// buyAlertWriter is watchlistWriter's counterpart for buy-alert removal —
// deleting a row by id has no bot-layer behavior beyond the DB write, unlike
// adding one (which needs a quote fetch, hence ExecuteAddBuyAlert above).
type buyAlertWriter interface {
	RemoveBuyAlert(id int64) error
}

// thesisWriter backs POST /api/thesis (Phase 21) — like watchlistWriter, a
// direct DB write with no bot-layer behavior beyond db.SetThesis itself.
type thesisWriter interface {
	SetThesis(ticker, text string) error
}

// Fee is a *float64 so an omitted JSON field (nil) can be told apart from an
// explicit 0 — nil triggers ExecuteBuy/ExecuteSell's TW fee auto-calc
// (Phase 13 §3.2), matching parseTradeArgs' feeSet distinction.
type tradeRequest struct {
	Ticker string   `json:"ticker"`
	Shares float64  `json:"shares"`
	Price  float64  `json:"price"`
	Fee    *float64 `json:"fee"`
	Date   string   `json:"date"`
}

type stopRequest struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"`
}

type tickerRequest struct {
	Ticker string `json:"ticker"`
}

type buyAlertRequest struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"`
}

type buyAlertRemoveRequest struct {
	ID int64 `json:"id"`
}

type thesisRequest struct {
	Ticker string `json:"ticker"`
	Text   string `json:"text"`
}

// tradeResponse's Message is the exact same i18n-rendered confirmation (or
// failure) text the corresponding Telegram command would have sent — see
// docs/phase-10-web-trade-input.md §4.2's "Telegram 照常同步" decision.
type tradeResponse struct {
	Message string `json:"message"`
}

// resolveTradeDate defaults an empty date to today and validates a
// caller-supplied one against the same YYYY-MM-DD shape /buy and /sell
// accept — mirroring parseTradeArgs' semantics without exporting it (date
// defaults in the handler, not in the shared bot-layer function, per
// docs/phase-10-web-trade-input.md §4.2).
func resolveTradeDate(date string) (string, bool) {
	if date == "" {
		return time.Now().Format("2006-01-02"), true
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", false
	}
	return date, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func (s *Server) handleTradeBuy(w http.ResponseWriter, r *http.Request) {
	var req tradeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	date, ok := resolveTradeDate(req.Date)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}
	msg, err := s.trade.ExecuteBuy(req.Ticker, req.Shares, req.Price, req.Fee, date)
	if err != nil {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: msg})
}

func (s *Server) handleTradeSell(w http.ResponseWriter, r *http.Request) {
	var req tradeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	date, ok := resolveTradeDate(req.Date)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}
	msg, err := s.trade.ExecuteSell(r.Context(), req.Ticker, req.Shares, req.Price, req.Fee, date)
	if err != nil {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: msg})
}

func (s *Server) handleSetStop(w http.ResponseWriter, r *http.Request) {
	var req stopRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	msg, err := s.trade.ExecuteSetStop(req.Ticker, req.Price)
	if err != nil {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: msg})
}

func (s *Server) handleBuyAlertAdd(w http.ResponseWriter, r *http.Request) {
	var req buyAlertRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	msg, err := s.trade.ExecuteAddBuyAlert(req.Ticker, req.Price)
	if err != nil {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: msg})
}

func (s *Server) handleBuyAlertRemove(w http.ResponseWriter, r *http.Request) {
	var req buyAlertRemoveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.buyAlertDB.RemoveBuyAlert(req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove buy alert")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "removed"})
}

// handleThesisSet backs POST /api/thesis (Phase 21) — the write endpoint
// shared by both frontend entry points (ChartView's round-detail edit and
// TradeModal's buy-form textarea). Same requireWritable/requireAuth gate as
// every other write route (see server.go); ticker/text are both required
// server-side even though the buy form only ever sends a non-empty text
// (skip-if-blank is a frontend convenience, not something to trust).
func (s *Server) handleThesisSet(w http.ResponseWriter, r *http.Request) {
	var req thesisRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	text := strings.TrimSpace(req.Text)
	if ticker == "" || text == "" {
		writeError(w, http.StatusBadRequest, "ticker and text are required")
		return
	}
	if err := s.thesisDB.SetThesis(ticker, text); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save thesis")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
}

func (s *Server) handleWatchlistAdd(w http.ResponseWriter, r *http.Request) {
	var req tickerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "ticker is required")
		return
	}
	if err := s.watchlistDB.AddTicker(ticker); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add ticker")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "added " + ticker})
}

func (s *Server) handleWatchlistRemove(w http.ResponseWriter, r *http.Request) {
	var req tickerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "ticker is required")
		return
	}
	if err := s.watchlistDB.RemoveTicker(ticker); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove ticker")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "removed " + ticker})
}

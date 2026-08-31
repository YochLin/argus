package web

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/service"
)

// Phase 24 Stage 4 Step 4.2: the resource half of /api/v1. Every handler
// here is a translation layer and nothing else — parse the request, call an
// internal/service method (or, for the three trade routes, the same
// TradeExecutor the dashboard's write endpoints use), wrap the result in the
// envelope. No business logic lives in this file; that's the whole point of
// the surface existing on top of the service layer rather than beside it.
//
// POST /api/v1/recommendations/trigger is the one route here that calls no
// service: generating a recommendation is still bot.RunRecommend, since
// Stage 1.3's full pipeline extraction never landed (see PLAN.md's Step 3.2
// note on gatherRecommendationInputs). It reaches it through the Recommender
// interface below — injected by internal/app, the same seam TradeExecutor
// has used since Stage 1.1 — so this package still doesn't know
// internal/bot exists, and swapping in a real RecommendationService later
// changes one line in internal/app and nothing here.

// cst pins the deployment's dating zone the same way internal/scheduler,
// internal/bot, and internal/service do (see those packages' own `cst` —
// Alpine's Docker image has no tzdata, so time.Local is UTC in prod, not
// CST). handleAPIScanHits' default date must use this, not time.Now()'s
// process-local zone, to land on the same day scan.go dates scan_hits rows.
var cst = time.FixedZone("CST", 8*3600)

// apiV1Store is the narrow slice of *db.DB the v1 read endpoints need beyond
// dbReader's existing methods.
type apiV1Store interface {
	GetScanHits(date string) (map[string]string, error)
	GetRecentNotifications(limit int) ([]db.Notification, error)
	MarkNotificationRead(id int64) error
}

// apiLimit reads a ?limit= bounded to [1, max], defaulting to def. For
// handleAPINotifications this bounds a real DB LIMIT; handleAPIRecommendationsLatest
// only applies it to GetRecommendationsSince's already-in-memory 90-day
// result, not the query itself — small at this table's current size, but not
// the allocation guard the name might suggest there.
func apiLimit(r *http.Request, def, max int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return def
	}
	return min(n, max)
}

type apiPosition struct {
	Ticker           string  `json:"ticker"`
	Shares           float64 `json:"shares"`
	AvgPrice         float64 `json:"avgPrice"`
	StopPrice        float64 `json:"stopPrice"`
	Price            float64 `json:"price"`
	MarketValue      float64 `json:"marketValue"`
	UnrealizedPnL    float64 `json:"unrealizedPnl"`
	UnrealizedPnLPct float64 `json:"unrealizedPnlPct"`
	// Stale is true when this position's quote couldn't be fetched, so the
	// client can grey the row out instead of showing a confident 0.
	Stale bool `json:"stale"`
}

type apiPortfolio struct {
	Market           string        `json:"market"`
	Positions        []apiPosition `json:"positions"`
	TotalMarketValue float64       `json:"totalMarketValue"`
	RealizedPnL      float64       `json:"realizedPnl"`
	Cash             float64       `json:"cash"`
	HasCash          bool          `json:"hasCash"`
	AccountValue     float64       `json:"accountValue"`
}

func (s *Server) handleAPIPortfolio(w http.ResponseWriter, r *http.Request) {
	snap, err := s.portfolios().Snapshot(marketParam(r))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load portfolio")
		return
	}
	out := apiPortfolio{
		Market:           string(snap.Market),
		Positions:        make([]apiPosition, 0, len(snap.Positions)),
		TotalMarketValue: snap.TotalMarketValue,
		RealizedPnL:      snap.RealizedPnL,
		Cash:             snap.Cash,
		HasCash:          snap.HasCash,
		AccountValue:     snap.AccountValue,
	}
	for _, p := range snap.Positions {
		item := apiPosition{
			Ticker:           p.Position.Ticker,
			Shares:           p.Position.Shares,
			AvgPrice:         p.Position.AvgCost,
			StopPrice:        p.Position.StopPrice,
			MarketValue:      p.MarketValue,
			UnrealizedPnL:    p.UnrealizedPnL,
			UnrealizedPnLPct: p.UnrealizedPnLPct,
			Stale:            p.Quote == nil,
		}
		if p.Quote != nil {
			item.Price = p.Quote.Price
		}
		out.Positions = append(out.Positions, item)
	}
	writeAPIOK(w, out)
}

func (s *Server) handleAPIWatchlistGet(w http.ResponseWriter, r *http.Request) {
	tickers, err := s.db.GetWatchlistByMarket(marketParam(r))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load watchlist")
		return
	}
	if tickers == nil {
		tickers = []string{}
	}
	writeAPIOK(w, map[string]any{"tickers": tickers})
}

// handleAPIWatchlistAdd/Remove normalize through service.NormalizeTicker —
// the dashboard's own handleWatchlistAdd/Remove (trade.go) call the same
// function, so there's exactly one definition of what a ticker looks like,
// shared with the bot and MCP too. apiTickerBody's own empty check below
// means NormalizeTicker's ErrInvalidTicker branch is unreachable from here,
// but it's kept as the single source of truth rather than duplicated.
func (s *Server) handleAPIWatchlistAdd(w http.ResponseWriter, r *http.Request) {
	ticker, ok := s.apiTickerBody(w, r)
	if !ok {
		return
	}
	normalized, err := service.NormalizeTicker(ticker)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.watchlistDB.AddTicker(normalized); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to add ticker")
		return
	}
	writeAPIOK(w, map[string]string{"ticker": normalized})
}

func (s *Server) handleAPIWatchlistRemove(w http.ResponseWriter, r *http.Request) {
	ticker, ok := s.apiTickerBody(w, r)
	if !ok {
		return
	}
	normalized, err := service.NormalizeTicker(ticker)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.watchlistDB.RemoveTicker(normalized); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to remove ticker")
		return
	}
	writeAPIOK(w, map[string]string{"ticker": normalized})
}

func (s *Server) apiTickerBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Ticker string `json:"ticker"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return "", false
	}
	if strings.TrimSpace(body.Ticker) == "" {
		writeAPIError(w, http.StatusBadRequest, "ticker is required")
		return "", false
	}
	return body.Ticker, true
}

// handleAPITradeBuy/Sell/SetStop are the /api/v1 translation of trade.go's
// execBuy/execSell/execSetStop — same business logic (including the
// explicit post-call Notify, Phase 24 tech debt 3: a trade booked from an
// app should still show up in Telegram), different decode/envelope only.
func (s *Server) handleAPITradeBuy(w http.ResponseWriter, r *http.Request) {
	var req tradeRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	msg, err := s.execBuy(req)
	if errors.Is(err, errInvalidTradeDate) {
		writeAPIError(w, http.StatusBadRequest, "invalid date")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, msg)
		return
	}
	writeAPIOK(w, map[string]string{"message": msg})
}

func (s *Server) handleAPITradeSell(w http.ResponseWriter, r *http.Request) {
	var req tradeRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	msg, err := s.execSell(r.Context(), req)
	if errors.Is(err, errInvalidTradeDate) {
		writeAPIError(w, http.StatusBadRequest, "invalid date")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, msg)
		return
	}
	writeAPIOK(w, map[string]string{"message": msg})
}

func (s *Server) handleAPISetStop(w http.ResponseWriter, r *http.Request) {
	var req stopRequest
	if !decodeAPIJSON(w, r, &req) {
		return
	}
	msg, err := s.execSetStop(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, msg)
		return
	}
	writeAPIOK(w, map[string]string{"message": msg})
}

type apiRecommendation struct {
	Date   string  `json:"date"`
	Ticker string  `json:"ticker"`
	Action string  `json:"action"`
	Reason string  `json:"reason"`
	Price  float64 `json:"price"`
	Source string  `json:"source"`
	Market string  `json:"market"`
}

// handleAPIRecommendationsLatest is GET /api/v1/recommendations/latest —
// the stored history, newest first, optionally filtered to one market. The
// 90-day floor keeps this off a full-table scan as the history grows; an app
// showing "latest" has no use for a two-year-old row.
func (s *Server) handleAPIRecommendationsLatest(w http.ResponseWriter, r *http.Request) {
	since := time.Now().AddDate(0, 0, -90).Format("2006-01-02")
	rows, err := s.db.GetRecommendationsSince(since)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load recommendations")
		return
	}
	wantMarket := string(marketParam(r))
	limit := apiLimit(r, 20, 200)
	out := make([]apiRecommendation, 0, limit)
	// GetRecommendationsSince returns oldest-first; walk backwards so the
	// limit keeps the newest rows rather than the oldest.
	for i := len(rows) - 1; i >= 0 && len(out) < limit; i-- {
		if rows[i].Market != wantMarket {
			continue
		}
		out = append(out, apiRecommendation{
			Date:   rows[i].Date,
			Ticker: rows[i].Ticker,
			Action: rows[i].Action,
			Reason: rows[i].Reason,
			Price:  rows[i].Price,
			Source: rows[i].Source,
			Market: rows[i].Market,
		})
	}
	writeAPIOK(w, map[string]any{"recommendations": out})
}

type apiScanHit struct {
	Ticker string `json:"ticker"`
	Reason string `json:"reason"`
}

// handleAPIScanHits is GET /api/v1/scan/hits?date=YYYY-MM-DD, defaulting to
// today in CST — matching scan.go's own s.now().In(cst) dating, not the
// process's zone (see the cst var above).
func (s *Server) handleAPIScanHits(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().In(cst).Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid date")
		return
	}
	hits, err := s.apiDB.GetScanHits(date)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load scan hits")
		return
	}
	out := make([]apiScanHit, 0, len(hits))
	for ticker, reason := range hits {
		out = append(out, apiScanHit{Ticker: ticker, Reason: reason})
	}
	// Map iteration order is random; a client rendering a list needs a
	// stable one.
	slices.SortFunc(out, func(a, b apiScanHit) int { return strings.Compare(a.Ticker, b.Ticker) })
	writeAPIOK(w, map[string]any{"date": date, "hits": out})
}

type apiNotification struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	Level     string `json:"level"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"createdAt"`
}

func (s *Server) handleAPINotifications(w http.ResponseWriter, r *http.Request) {
	rows, err := s.apiDB.GetRecentNotifications(apiLimit(r, 50, 500))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load notifications")
		return
	}
	out := make([]apiNotification, 0, len(rows))
	for _, n := range rows {
		out = append(out, apiNotification{ID: n.ID, Type: n.Type, Text: n.Text, Level: n.Level, Read: n.Read, CreatedAt: n.CreatedAt})
	}
	writeAPIOK(w, map[string]any{"notifications": out})
}

// handleAPINotificationRead is POST /api/v1/notifications/{id}/read. A path
// parameter rather than a body: marking one row read is addressing a
// resource, and net/http's pattern matcher already parses it.
//
// db.MarkNotificationRead is an UPDATE with no RowsAffected check, so an id
// that doesn't exist (already deleted, or never existed) is a silent no-op
// that still answers {"read": true} — deliberate, not overlooked: this is a
// single-user system and changing the db method's signature to distinguish
// "updated" from "nothing to update" isn't worth it for a client that would
// only ever hit this by racing itself.
func (s *Server) handleAPINotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid notification id")
		return
	}
	if err := s.apiDB.MarkNotificationRead(id); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to mark read")
		return
	}
	writeAPIOK(w, map[string]any{"id": id, "read": true})
}

// requireAPITrade is requireTrade with the v1 envelope — same 409 when no
// TradeExecutor was injected. Since Phase 24 Stage 3 that no longer means
// "Telegram is unconfigured": internal/app injects a headless bot in that
// case, so a nil executor now only happens on a Server built without the
// seam at all, i.e. a test.
func (s *Server) requireAPITrade(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.trade == nil {
			writeAPIError(w, http.StatusConflict, "trade execution is not available")
			return
		}
		next(w, r)
	}
}

// handleAPIRecommendationsTrigger is POST /api/v1/recommendations/trigger
// (?market=us|tw): start a fresh recommendation run and answer 202
// immediately. A run is a full data gather plus an LLM call — minutes, well
// past any mobile client's request timeout — so the result is collected from
// GET /api/v1/recommendations/latest afterwards, not from this response.
//
// One run at a time, process-wide: a second concurrent run would fetch the
// same quotes, burn a second LLM call and race the first one's writes into
// the recommendations table, all for a result the user asked for once and
// double-tapped. 409 rather than a queue — the caller retrying in a minute
// is the whole of the recovery, and a queue would need a lifecycle nothing
// here has.
func (s *Server) handleAPIRecommendationsTrigger(w http.ResponseWriter, r *http.Request) {
	if s.recommender == nil {
		writeAPIError(w, http.StatusConflict, "recommendation runs are not available")
		return
	}
	if !s.recRunning.CompareAndSwap(false, true) {
		writeAPIError(w, http.StatusConflict, "a recommendation run is already in progress")
		return
	}
	m := marketParam(r)
	// WithoutCancel, not r.Context() directly: net/http cancels a request's
	// context the moment its handler returns, which here is immediately —
	// passing it through would abort the run at its first ctx check.
	ctx := context.WithoutCancel(r.Context())
	go func() {
		defer s.recRunning.Store(false)
		s.recommender.RunRecommend(ctx, m)
	}()
	writeJSON(w, http.StatusAccepted, apiResponse{
		Success:   true,
		Data:      map[string]string{"status": "started", "market": string(m)},
		Timestamp: time.Now().Unix(),
	})
}

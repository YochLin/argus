package web

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// cashflowItem mirrors db.RecurringCashflow for JSON, with the linked
// asset's venue resolved (Venue empty for an unlinked flow or one whose
// asset carries none).
type cashflowItem struct {
	ID         int64   `json:"id"`
	Direction  string  `json:"direction"` // "in" | "out"
	Name       string  `json:"name"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	DayOfMonth *int64  `json:"dayOfMonth,omitempty"`
	Category   string  `json:"category,omitempty"`
	AssetID    *int64  `json:"assetId,omitempty"`
	Venue      string  `json:"venue,omitempty"`
	Active     bool    `json:"active"`
}

// cashEvent is one row of the 90-day cash event table (§8.4) — derived, not
// stored: a recurring flow's day_of_month projected forward, or an open
// option contract's expiry. Amount is nil for an "event" (an expiry isn't a
// known cash amount until assignment/exercise is decided).
type cashEvent struct {
	Date      string   `json:"date"`
	Item      string   `json:"item"`
	Venue     string   `json:"venue,omitempty"`
	Amount    *float64 `json:"amount"`
	Currency  string   `json:"currency,omitempty"`
	Direction string   `json:"direction"` // "in" | "out" | "event"
}

// cashResponse backs GET /api/wealth/cash (`/w/cash`, §9.4 PR5).
// MonthlyIn/Out/Net are nil when any active flow's currency couldn't be
// priced to TWD today — same whole-metric-degrades rule as /w/alloc's
// totals, rather than silently summing a partial total.
type cashResponse struct {
	AsOf       string         `json:"asOf"`
	Items      []cashflowItem `json:"items"`
	MonthlyIn  *float64       `json:"monthlyIn"`
	MonthlyOut *float64       `json:"monthlyOut"`
	MonthlyNet *float64       `json:"monthlyNet"`
	Events     []cashEvent    `json:"events"`
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// recurringCashflowEvents projects each active flow's day_of_month onto the
// next 90 days (up to 4 candidate months covers any 90-day window regardless
// of today's date) — the event table's recurring-flow half; a flow with no
// day_of_month only ever shows up in the monthly total, never here.
func recurringCashflowEvents(list []db.RecurringCashflow, venueByAsset map[int64]string, today time.Time) []cashEvent {
	end := today.AddDate(0, 0, 90)
	var out []cashEvent
	for _, c := range list {
		if c.DayOfMonth == nil || *c.DayOfMonth < 1 {
			continue
		}
		day := int(*c.DayOfMonth)
		for offset := 0; offset < 4; offset++ {
			monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location()).AddDate(0, offset, 0)
			occDay := min(day, daysInMonth(monthStart.Year(), monthStart.Month()))
			occ := time.Date(monthStart.Year(), monthStart.Month(), occDay, 0, 0, 0, 0, today.Location())
			if occ.Before(today) || occ.After(end) {
				continue
			}
			amount := c.Amount
			venue := ""
			if c.AssetID != nil {
				venue = venueByAsset[*c.AssetID]
			}
			out = append(out, cashEvent{
				Date: occ.Format("2006-01-02"), Item: c.Name, Venue: venue,
				Amount: &amount, Currency: c.Currency, Direction: c.Direction,
			})
		}
	}
	return out
}

// optionExpiryEvents adds each open contract expiring within the window
// (§8.4) — no amount, since assignment/exercise isn't decided at listing
// time (options.go's own expiry-scan job resolves that, never
// automatically). US-only, same as the rest of internal/option.
func optionExpiryEvents(positions []db.OptionPosition, today, end time.Time) []cashEvent {
	var out []cashEvent
	for _, o := range positions {
		expiry, err := time.Parse("2006-01-02", o.Expiry)
		if err != nil || expiry.Before(today) || expiry.After(end) {
			continue
		}
		out = append(out, cashEvent{Date: o.Expiry, Item: o.ContractSymbol, Direction: "event"})
	}
	return out
}

// handleWealthCashList backs GET /api/wealth/cash — ungated read, same
// convention as every other wealth GET route.
func (s *Server) handleWealthCashList(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthCashList: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	resp := cashResponse{AsOf: today.Format("2006-01-02"), Items: []cashflowItem{}, Events: []cashEvent{}}

	all, err := s.db.ListRecurringCashflows(false)
	if err != nil {
		logger.Errorf("web: wealth cash: list: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load cash flows")
		return
	}

	assetList, err := s.db.ListAssetsWithValue(true)
	if err != nil {
		logger.Errorf("web: wealth cash: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load assets")
		return
	}
	venueByAsset := make(map[int64]string, len(assetList))
	for _, a := range assetList {
		venueByAsset[a.ID] = a.Venue
	}

	var active []db.RecurringCashflow
	for _, c := range all {
		venue := ""
		if c.AssetID != nil {
			venue = venueByAsset[*c.AssetID]
		}
		resp.Items = append(resp.Items, cashflowItem{
			ID: c.ID, Direction: c.Direction, Name: c.Name, Amount: c.Amount, Currency: c.Currency,
			DayOfMonth: c.DayOfMonth, Category: c.Category, AssetID: c.AssetID, Venue: venue, Active: c.Active,
		})
		if c.Active {
			active = append(active, c)
		}
	}

	var monthlyIn, monthlyOut float64
	fxOK := true
	for _, c := range active {
		rate, rok := service.RateToTWD(c.Currency, resp.AsOf, true, s.quotes, s.fxDB)
		if !rok {
			fxOK = false
			continue
		}
		amt := c.Amount * rate
		if c.Direction == "in" {
			monthlyIn += amt
		} else {
			monthlyOut += amt
		}
	}
	if fxOK {
		net := monthlyIn - monthlyOut
		resp.MonthlyIn, resp.MonthlyOut, resp.MonthlyNet = &monthlyIn, &monthlyOut, &net
	}

	events := recurringCashflowEvents(active, venueByAsset, today)
	optionPositions, err := s.db.GetOptionPositions()
	if err != nil {
		logger.Errorf("web: wealth cash: option positions: %v", err)
	} else {
		events = append(events, optionExpiryEvents(optionPositions, today, today.AddDate(0, 0, 90))...)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Date < events[j].Date })
	if events == nil {
		events = []cashEvent{}
	}
	resp.Events = events

	writeJSON(w, http.StatusOK, resp)
}

type wealthCashCreateRequest struct {
	Direction  string  `json:"direction"`
	Name       string  `json:"name"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	DayOfMonth *int64  `json:"dayOfMonth"`
	Category   string  `json:"category"`
	AssetID    *int64  `json:"assetId"`
}

// handleWealthCashCreate backs POST /api/wealth/cash — the add-flow form's
// single write path, gated like every other wealth write route.
func (s *Server) handleWealthCashCreate(w http.ResponseWriter, r *http.Request) {
	var req wealthCashCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Direction = strings.TrimSpace(req.Direction)
	req.Name = strings.TrimSpace(req.Name)
	if req.Direction != "in" && req.Direction != "out" {
		writeError(w, http.StatusBadRequest, "direction must be \"in\" or \"out\"")
		return
	}
	if req.Name == "" || req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "name and a positive amount are required")
		return
	}
	if req.DayOfMonth != nil && (*req.DayOfMonth < 1 || *req.DayOfMonth > 31) {
		writeError(w, http.StatusBadRequest, "dayOfMonth must be between 1 and 31")
		return
	}
	id, err := s.wealthDB.CreateRecurringCashflow(db.NewRecurringCashflow{
		Direction: req.Direction, Name: req.Name, Amount: req.Amount, Currency: strings.TrimSpace(req.Currency),
		DayOfMonth: req.DayOfMonth, AssetID: req.AssetID, Category: strings.TrimSpace(req.Category),
	})
	if err != nil {
		logger.Errorf("web: create recurring cashflow: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create cash flow")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

type wealthCashDeactivateRequest struct {
	ID int64 `json:"id"`
}

// handleWealthCashDeactivate backs POST /api/wealth/cash/deactivate — pauses
// a flow (soft delete, see db.DeactivateRecurringCashflow's doc comment).
func (s *Server) handleWealthCashDeactivate(w http.ResponseWriter, r *http.Request) {
	var req wealthCashDeactivateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID <= 0 {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.wealthDB.DeactivateRecurringCashflow(req.ID); err != nil {
		logger.Errorf("web: deactivate recurring cashflow: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to pause cash flow")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "paused"})
}

package web

import (
	"net/http"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
)

// fxRateStore is web's narrow view of *db.DB for currency-conversion
// caching (§8.2) — a separate field from wealthWriter/dbReader since it's
// read-and-write together (a cache), unlike either of those interfaces'
// pure read or pure write shape.
type fxRateStore interface {
	GetFXRate(date, pair string) (float64, bool, error)
	SaveFXRate(date, pair string, rate float64) error
}

// wealthEntry is one signed, native-currency amount going into a net-worth
// total — an asset's latest value, a liability's latest value negated, or
// the equity virtual row's market value (§9.1 rule 3: never a real assets
// row, always derived at read time from positions/net_worth_snapshots).
type wealthEntry struct {
	Currency string
	Group    string
	Value    float64
}

func assetEntries(list []db.AssetWithValue) []wealthEntry {
	out := make([]wealthEntry, 0, len(list))
	for _, a := range list {
		if a.Value == nil {
			continue
		}
		v := *a.Value
		if a.Side == "liability" {
			v = -v
		}
		out = append(out, wealthEntry{Currency: a.Currency, Group: a.AssetGroup, Value: v})
	}
	return out
}

// equityEntries wraps the trading account's own net worth (already
// cash+positions, from net_worth_snapshots) as the "equity" virtual row,
// one per market that has any history — always asset_group "growth". A
// market with no net_worth_snapshots row on/before the date (e.g. an
// account that has never held anything in that market) simply contributes
// nothing, distinct from a currency-conversion failure below.
func equityEntries(usTotal float64, usOK bool, twTotal float64, twOK bool) []wealthEntry {
	var out []wealthEntry
	if usOK && usTotal != 0 {
		out = append(out, wealthEntry{Currency: "USD", Group: "growth", Value: usTotal})
	}
	if twOK && twTotal != 0 {
		out = append(out, wealthEntry{Currency: "TWD", Group: "growth", Value: twTotal})
	}
	return out
}

// rateToTWD resolves currency's rate to TWD on date. TWD itself is always
// 1. live=true additionally fetches+caches today's rate from the quote
// provider when fx_rates doesn't have it yet — only ever passed true for
// today's date, since a historical rate can't be fetched live. ok=false is
// the signal the caller must degrade the *whole* metric to "—" rather than
// silently sum only the currencies it could price (§8.17.1's rule against
// fabricating a number from an incomplete period applies equally to an
// incomplete currency set).
func rateToTWD(currency, date string, live bool, quotes quoteGetter, fx fxRateStore) (float64, bool) {
	if currency == "" || currency == "TWD" {
		return 1, true
	}
	pair := currency + "TWD"
	if rate, ok, err := fx.GetFXRate(date, pair); err == nil && ok {
		return rate, true
	}
	if !live {
		return 0, false
	}
	q, err := quotes.GetQuote(pair + "=X")
	if err != nil || q.Price <= 0 {
		return 0, false
	}
	// Best-effort cache: a save failure doesn't invalidate the rate we just
	// fetched, it only means tomorrow's historical lookup for today will
	// have to re-fetch (or degrade) — not worth failing the whole request
	// over.
	if err := fx.SaveFXRate(date, pair, q.Price); err != nil {
		logger.Errorf("web: cache fx rate %s@%s: %v", pair, date, err)
	}
	return q.Price, true
}

// sumEntriesTWD converts every entry to TWD and totals it, plus per-group
// subtotals. ok=false means at least one entry's currency couldn't be
// priced for date — the caller must not render a partial sum.
func sumEntriesTWD(entries []wealthEntry, date string, live bool, quotes quoteGetter, fx fxRateStore) (total float64, byGroup map[string]float64, ok bool) {
	byGroup = make(map[string]float64, len(assets.AssetGroups))
	for _, e := range entries {
		rate, rok := rateToTWD(e.Currency, date, live, quotes, fx)
		if !rok {
			return 0, nil, false
		}
		converted := e.Value * rate
		total += converted
		byGroup[e.Group] += converted
	}
	return total, byGroup, true
}

// wealthTotals is the one place that assembles "everything the user owns,
// in TWD, as of date." live must only be true for today (see rateToTWD).
func (s *Server) wealthTotals(date string, live bool) (float64, map[string]float64, bool) {
	var list []db.AssetWithValue
	var err error
	if live {
		list, err = s.db.ListAssetsWithValue(false)
	} else {
		list, err = s.db.ListAssetsValueAsOf(date, false)
	}
	if err != nil {
		logger.Errorf("web: wealth totals: list assets as of %s: %v", date, err)
		return 0, nil, false
	}
	entries := assetEntries(list)

	usTotal, usOK, err := s.db.GetNetWorthOnOrBefore(date, market.US)
	if err != nil {
		logger.Errorf("web: wealth totals: US net worth as of %s: %v", date, err)
		return 0, nil, false
	}
	twTotal, twOK, err := s.db.GetNetWorthOnOrBefore(date, market.TW)
	if err != nil {
		logger.Errorf("web: wealth totals: TW net worth as of %s: %v", date, err)
		return 0, nil, false
	}
	entries = append(entries, equityEntries(usTotal, usOK, twTotal, twOK)...)

	return sumEntriesTWD(entries, date, live, s.quotes, s.fxDB)
}

// periodReturnPct is (todayTotal - baseline)/baseline, or ok=false when
// either side is unavailable — todayTotal/todayOK are passed in rather than
// recomputed so YTD and MoM don't each re-run wealthTotals(today) (and its
// live FX fetch) a second and third time.
func (s *Server) periodReturnPct(fromDate string, todayTotal float64, todayOK bool) (float64, bool) {
	if !todayOK {
		return 0, false
	}
	baseline, _, ok := s.wealthTotals(fromDate, false)
	if !ok || baseline == 0 {
		return 0, false
	}
	return (todayTotal - baseline) / baseline * 100, true
}

type driftRowResponse struct {
	Group       string  `json:"group"`
	MarketValue float64 `json:"marketValue"`
	CurrentPct  float64 `json:"currentPct"`
	TargetPct   float64 `json:"targetPct"`
	DeviationPt float64 `json:"deviationPt"`
}

// wealthHomeResponse is /w's variant-B hero (§8.17.1: net worth big number
// + YTD% + MoM%) plus the five-column allocation table (§8.17.2). Any
// pointer field left nil means "not enough history yet" — the frontend
// renders "—", it must never treat nil as 0.
type wealthHomeResponse struct {
	AsOf       string             `json:"asOf"`
	NetWorth   *float64           `json:"netWorth"`
	YTDPct     *float64           `json:"ytdPct"`
	MoMPct     *float64           `json:"momPct"`
	Model      string             `json:"model"`
	Allocation []driftRowResponse `json:"allocation"`
}

// handleWealthHome backs GET /api/wealth/networth?model=conserv|balanced|
// growth — ungated like every other read route. model defaults to
// "balanced" and an unrecognized value falls back to it rather than
// erroring, since switching models is just a display toggle (§8.17.3: all
// three presets are always computable from the same data, the query param
// only picks which target column to show).
func (s *Server) handleWealthHome(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthHome: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	model := assets.AllocationModel(r.URL.Query().Get("model"))
	if _, ok := assets.ModelPresets[model]; !ok {
		model = assets.ModelBalanced
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	todayTotal, byGroup, todayOK := s.wealthTotals(today, true)

	resp := wealthHomeResponse{AsOf: today, Model: string(model)}
	if todayOK {
		nw := todayTotal
		resp.NetWorth = &nw
		rows := assets.ComputeDrift(byGroup, todayTotal, model)
		resp.Allocation = make([]driftRowResponse, 0, len(rows))
		for _, row := range rows {
			resp.Allocation = append(resp.Allocation, driftRowResponse{
				Group: row.Group, MarketValue: row.MarketValue, CurrentPct: row.CurrentPct,
				TargetPct: row.TargetPct, DeviationPt: row.DeviationPt,
			})
		}
	}

	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	if pct, ok := s.periodReturnPct(yearStart, todayTotal, todayOK); ok {
		resp.YTDPct = &pct
	}
	monthAgo := now.AddDate(0, -1, 0).Format("2006-01-02")
	if pct, ok := s.periodReturnPct(monthAgo, todayTotal, todayOK); ok {
		resp.MoMPct = &pct
	}

	writeJSON(w, http.StatusOK, resp)
}

package web

import (
	"net/http"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// fxRateStore is web's narrow view of *db.DB for currency-conversion
// caching (§8.2), matching service.WealthFXStore's method set.
type fxRateStore interface {
	GetFXRate(date, pair string) (float64, bool, error)
	SaveFXRate(date, pair string, rate float64) error
}

type driftRowResponse struct {
	Group       string  `json:"group"`
	MarketValue float64 `json:"marketValue"`
	CurrentPct  float64 `json:"currentPct"`
	TargetPct   float64 `json:"targetPct"`
	DeviationPt float64 `json:"deviationPt"`
}

// wealthHomeResponse is /w's variant-B hero (§8.17.1: net worth big number
// + YTD% + MoM%) plus the five-column allocation table (§8.17.2).
// TotalAssets/TotalLiabilities back the hero's inline stat row (design-mock
// parity — the hero shows net worth alongside the two totals it nets out
// of, not just the net figure alone). Any pointer field left nil means "not
// enough history yet" — the frontend renders "—", it must never treat nil
// as 0.
type wealthHomeResponse struct {
	AsOf             string             `json:"asOf"`
	NetWorth         *float64           `json:"netWorth"`
	YTDPct           *float64           `json:"ytdPct"`
	MoMPct           *float64           `json:"momPct"`
	TotalAssets      *float64           `json:"totalAssets"`
	TotalLiabilities *float64           `json:"totalLiabilities"`
	Model            string             `json:"model"`
	Allocation       []driftRowResponse `json:"allocation"`
	// StaleCount is how many hand-maintained records (manual/import, assets
	// and liabilities alike) last got a value more than staleDays ago — the
	// template's amber "N 筆資料超過 90 天未更新" banner. Synced sources
	// refresh themselves, so they never count.
	StaleCount int `json:"staleCount"`
}

const staleDays = 90

// countStale counts manual/import records whose latest snapshot is older
// than staleDays as of now. A record with no snapshot yet isn't stale, it's
// empty — the page already renders that as "—".
func countStale(list []db.AssetWithValue, now time.Time) int {
	cutoff := now.AddDate(0, 0, -staleDays).Format("2006-01-02")
	n := 0
	for _, a := range list {
		if a.AsOf == "" || (a.Source != "manual" && a.Source != "import") {
			continue
		}
		if a.AsOf < cutoff {
			n++
		}
	}
	return n
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
	todayTotal, byGroup, todayOK := service.WealthTotals(s.db, s.fxDB, s.quotes, today, true)

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
		if ta, tl, ok := service.AssetLiabilityTotals(s.db, s.fxDB, s.quotes, today, true); ok {
			resp.TotalAssets = &ta
			resp.TotalLiabilities = &tl
		}
	}

	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	if pct, ok := service.PeriodReturnPct(s.db, s.fxDB, s.quotes, yearStart, todayTotal, todayOK); ok {
		resp.YTDPct = &pct
	}
	monthAgo := now.AddDate(0, -1, 0).Format("2006-01-02")
	if pct, ok := service.PeriodReturnPct(s.db, s.fxDB, s.quotes, monthAgo, todayTotal, todayOK); ok {
		resp.MoMPct = &pct
	}

	if list, err := s.db.ListAssetsWithValue(false); err != nil {
		logger.Errorf("web: wealth home: list assets for stale count: %v", err)
	} else {
		resp.StaleCount = countStale(list, now)
	}

	writeJSON(w, http.StatusOK, resp)
}

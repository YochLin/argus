package web

import (
	"net/http"
	"sort"
	"time"

	"argus/internal/assets"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// allocRow is /w/alloc's allocation-table line — ComputeDrift's DriftRow
// plus the venue of the group's largest single holding (§8.3-2's table has
// a venue column; a group can hold several assets across different venues,
// so this is a "where most of it lives" hint, not a claim every asset in
// the group shares one venue).
type allocRow struct {
	Group       string  `json:"group"`
	MarketValue float64 `json:"marketValue"`
	CurrentPct  float64 `json:"currentPct"`
	TargetPct   float64 `json:"targetPct"`
	DeviationPt float64 `json:"deviationPt"`
	Venue       string  `json:"venue,omitempty"`
}

type allocOrder struct {
	Group       string  `json:"group"`
	Side        string  `json:"side"` // "buy" or "sell"
	Amount      float64 `json:"amount"`
	DeviationPt float64 `json:"deviationPt"`
	AssetName   string  `json:"assetName,omitempty"` // the group's largest holding, if any yet
	Venue       string  `json:"venue,omitempty"`
}

type allocLockedRow struct {
	Group       string  `json:"group"`
	MarketValue float64 `json:"marketValue"`
	DeviationPt float64 `json:"deviationPt"`
}

type currencyExposureRow struct {
	Currency string  `json:"currency"`
	Pct      float64 `json:"pct"`
}

// concentrationWarning is §10.2③'s single-position warning — PctOfEquity is
// the share of the combined US+TW open-position book, PctOfAssets the share
// of total net worth. Only ever a warning; /w/alloc never suggests which
// position to trim (same line §8.10 draws for tax-loss harvesting).
type concentrationWarning struct {
	Ticker      string  `json:"ticker"`
	Market      string  `json:"market"`
	PctOfEquity float64 `json:"pctOfEquity"`
	PctOfAssets float64 `json:"pctOfAssets"`
}

// allocResponse backs GET /api/wealth/alloc (`/w/alloc`, §9.4 PR4) —
// ungated like every other wealth read route. Every totals/percentage field
// is nil/empty until there's at least one priced asset (§8.17.1's "don't
// fabricate a number" rule).
type allocResponse struct {
	AsOf             string                 `json:"asOf"`
	Model            string                 `json:"model"`
	TotalAssets      *float64               `json:"totalAssets"`
	Allocation       []allocRow             `json:"allocation"`
	Orders           []allocOrder           `json:"orders"`
	RebalTotal       float64                `json:"rebalTotal"`
	Locked           []allocLockedRow       `json:"locked"`
	RiskPct          *float64               `json:"riskPct"`
	RiskTargetLow    *float64               `json:"riskTargetLow"`
	RiskTargetHigh   *float64               `json:"riskTargetHigh"`
	CurrencyExposure []currencyExposureRow  `json:"currencyExposure"`
	Concentration    []concentrationWarning `json:"concentration"`
}

// largestAsset returns the name/venue of items' highest-ValueTWD entry, or
// ("", "") for an empty group (a target group with no asset in it yet — the
// order still makes sense, "buy into <group>", just with no specific
// account to point at).
func largestAsset(items []balanceSheetItem) (name, venue string) {
	var best *balanceSheetItem
	for i := range items {
		if best == nil || items[i].ValueTWD > best.ValueTWD {
			best = &items[i]
		}
	}
	if best == nil {
		return "", ""
	}
	return best.Name, best.Venue
}

func (s *Server) handleWealthAlloc(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthAlloc: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	model := assets.AllocationModel(r.URL.Query().Get("model"))
	if _, ok := assets.ModelPresets[model]; !ok {
		model = assets.ModelBalanced
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	resp := allocResponse{
		AsOf: today, Model: string(model),
		Allocation: []allocRow{}, Orders: []allocOrder{}, Locked: []allocLockedRow{},
		CurrencyExposure: []currencyExposureRow{}, Concentration: []concentrationWarning{},
	}

	list, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: wealth alloc: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load assets")
		return
	}
	usTotal, usOK, err := s.db.GetNetWorthOnOrBefore(today, market.US)
	if err != nil {
		logger.Errorf("web: wealth alloc: US net worth: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load net worth")
		return
	}
	twTotal, twOK, err := s.db.GetNetWorthOnOrBefore(today, market.TW)
	if err != nil {
		logger.Errorf("web: wealth alloc: TW net worth: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load net worth")
		return
	}

	groups, byCurrency, totalAssets, fxOK := s.buildAssetGroups(list, today, usTotal, usOK, twTotal, twOK)
	if !fxOK || totalAssets <= 0 {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	ta := totalAssets
	resp.TotalAssets = &ta

	byGroup := make(map[string]float64, len(assets.AssetGroups))
	for _, g := range assets.AssetGroups {
		byGroup[g] = groups[g].MarketValue
	}
	driftRows := assets.ComputeDrift(byGroup, totalAssets, model)
	orders := assets.ComputeRebalanceOrders(driftRows, totalAssets)

	orderByGroup := make(map[string]assets.RebalanceOrder, len(orders))
	for _, o := range orders {
		orderByGroup[o.Group] = o
		resp.RebalTotal += o.Amount
	}

	for _, row := range driftRows {
		name, venue := largestAsset(groups[row.Group].Assets)
		resp.Allocation = append(resp.Allocation, allocRow{
			Group: row.Group, MarketValue: row.MarketValue, CurrentPct: row.CurrentPct,
			TargetPct: row.TargetPct, DeviationPt: row.DeviationPt, Venue: venue,
		})
		if assets.LockedGroups[row.Group] {
			resp.Locked = append(resp.Locked, allocLockedRow{Group: row.Group, MarketValue: row.MarketValue, DeviationPt: row.DeviationPt})
			continue
		}
		if o, ok := orderByGroup[row.Group]; ok {
			resp.Orders = append(resp.Orders, allocOrder{
				Group: o.Group, Side: o.Side, Amount: o.Amount, DeviationPt: o.DeviationPt,
				AssetName: name, Venue: venue,
			})
		}
		if row.Group == "growth" {
			cur := row.CurrentPct
			resp.RiskPct = &cur
			lo, hi := row.TargetPct-assets.RebalanceThresholdPt, row.TargetPct+assets.RebalanceThresholdPt
			resp.RiskTargetLow, resp.RiskTargetHigh = &lo, &hi
		}
	}

	for cur, v := range byCurrency {
		resp.CurrencyExposure = append(resp.CurrencyExposure, currencyExposureRow{Currency: cur, Pct: v / totalAssets * 100})
	}
	sort.Slice(resp.CurrencyExposure, func(i, j int) bool { return resp.CurrencyExposure[i].Pct > resp.CurrencyExposure[j].Pct })

	resp.Concentration = s.computeConcentration(today, totalAssets)

	writeJSON(w, http.StatusOK, resp)
}

// computeConcentration flags any open equity position over
// assets.ConcentrationThresholdPct of the combined US+TW position book
// (§10.2③). Reuses s.portfolios() rather than re-querying positions/quotes
// — the same valuation /api/portfolio already shows.
func (s *Server) computeConcentration(today string, totalAssets float64) []concentrationWarning {
	type posVal struct {
		Ticker, Market string
		ValueTWD       float64
	}
	var vals []posVal
	var equityTWD float64

	for _, m := range []market.MarketID{market.US, market.TW} {
		snap, err := s.portfolios().Snapshot(m)
		if err != nil {
			logger.Errorf("web: wealth alloc: portfolio snapshot %s: %v", m, err)
			continue
		}
		if len(snap.Positions) == 0 {
			continue
		}
		currency := "TWD"
		if m == market.US {
			currency = "USD"
		}
		rate, rok := service.RateToTWD(currency, today, true, s.quotes, s.fxDB)
		if !rok {
			continue
		}
		for _, pv := range snap.Positions {
			v := pv.MarketValue * rate
			equityTWD += v
			vals = append(vals, posVal{Ticker: pv.Position.Ticker, Market: string(m), ValueTWD: v})
		}
	}
	if equityTWD <= 0 {
		return []concentrationWarning{}
	}

	out := []concentrationWarning{}
	for _, pv := range vals {
		pctEquity := pv.ValueTWD / equityTWD * 100
		if pctEquity < assets.ConcentrationThresholdPct {
			continue
		}
		cw := concentrationWarning{Ticker: pv.Ticker, Market: pv.Market, PctOfEquity: pctEquity}
		if totalAssets > 0 {
			cw.PctOfAssets = pv.ValueTWD / totalAssets * 100
		}
		out = append(out, cw)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PctOfEquity > out[j].PctOfEquity })
	return out
}

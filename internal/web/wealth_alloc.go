package web

import (
	"net/http"
	"sort"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/service"
)

// allocRow is /w/alloc's allocation-table line — ComputeCategoryDrift's
// DriftRow plus the venue of the category's largest single holding
// (§8.3-2's table has a venue column; a category can hold several assets
// across different venues, so this is a "where most of it lives" hint, not
// a claim every asset in the category shares one venue). Keyed by
// assets.AllocCategories (§8.5's nine-type split), not the four-bucket
// asset_group the rest of the wealth pages use.
type allocRow struct {
	Category    string  `json:"category"`
	MarketValue float64 `json:"marketValue"`
	CurrentPct  float64 `json:"currentPct"`
	TargetPct   float64 `json:"targetPct"`
	DeviationPt float64 `json:"deviationPt"`
	Venue       string  `json:"venue,omitempty"`
}

type allocOrder struct {
	Category    string  `json:"category"`
	Side        string  `json:"side"` // "buy" or "sell"
	Amount      float64 `json:"amount"`
	DeviationPt float64 `json:"deviationPt"`
	AssetName   string  `json:"assetName,omitempty"` // the category's largest holding, if any yet
	Venue       string  `json:"venue,omitempty"`
}

type allocLockedRow struct {
	Category    string  `json:"category"`
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

// assetCategoryRow is buildAssetCategories' per-category accumulator —
// assetGroupRow's shape (see wealth_balance.go), keyed by
// assets.AllocCategories instead of assets.AssetGroups.
type assetCategoryRow struct {
	Category    string
	MarketValue float64
	Assets      []balanceSheetItem
}

// buildAssetCategories is buildAssetGroups' /w/alloc-only counterpart —
// same FX-conversion/assets-only-no-liabilities shape, keyed by
// assets.CategoryOf(a.Type) instead of a.AssetGroup. Not shared with
// /w/balance: that page's "資產分組" section is asset_group-based by
// design (docs/phase-9-asset-platform.md §8.5), so it keeps calling
// buildAssetGroups untouched.
func (s *Server) buildAssetCategories(list []db.AssetWithValue, today string, usTotal float64, usOK bool, twTotal float64, twOK bool) (categories map[string]*assetCategoryRow, byCurrency map[string]float64, totalAssets float64, fxOK bool) {
	categories = make(map[string]*assetCategoryRow, len(assets.AllocCategories))
	for _, c := range assets.AllocCategories {
		categories[c] = &assetCategoryRow{Category: c, Assets: []balanceSheetItem{}}
	}
	byCurrency = make(map[string]float64)
	fxOK = true

	for _, a := range list {
		if a.Value == nil || a.Side == "liability" {
			continue
		}
		rate, rok := service.RateToTWD(a.Currency, today, true, s.quotes, s.fxDB)
		if !rok {
			fxOK = false
			continue
		}
		valueTWD := *a.Value * rate
		totalAssets += valueTWD
		byCurrency[a.Currency] += valueTWD
		c := categories[assets.CategoryOf(a.Type)]
		if c == nil {
			continue
		}
		c.MarketValue += valueTWD
		c.Assets = append(c.Assets, balanceSheetItem{AssetID: a.ID, Name: a.Name, Venue: a.Venue, Currency: a.Currency, ValueTWD: valueTWD, Type: a.Type, Source: a.Source})
	}
	for _, e := range service.EquityEntries(usTotal, usOK, twTotal, twOK) {
		rate, rok := service.RateToTWD(e.Currency, today, true, s.quotes, s.fxDB)
		if !rok {
			fxOK = false
			continue
		}
		valueTWD := e.Value * rate
		totalAssets += valueTWD
		byCurrency[e.Currency] += valueTWD
		c := categories["equity"]
		typ := "equity_tw"
		if e.Currency != "TWD" {
			typ = "equity_us"
		}
		c.MarketValue += valueTWD
		c.Assets = append(c.Assets, balanceSheetItem{Name: typ, Currency: e.Currency, ValueTWD: valueTWD, Type: typ, Source: "sync"})
	}
	return categories, byCurrency, totalAssets, fxOK
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
	if _, ok := assets.CategoryModelPresets[model]; !ok {
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

	categories, byCurrency, totalAssets, fxOK := s.buildAssetCategories(list, today, usTotal, usOK, twTotal, twOK)
	if !fxOK || totalAssets <= 0 {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	ta := totalAssets
	resp.TotalAssets = &ta

	byCategory := make(map[string]float64, len(assets.AllocCategories))
	for _, c := range assets.AllocCategories {
		byCategory[c] = categories[c].MarketValue
	}
	driftRows := assets.ComputeCategoryDrift(byCategory, totalAssets, model)
	orders := assets.ComputeCategoryRebalanceOrders(driftRows, totalAssets)

	orderByCategory := make(map[string]assets.RebalanceOrder, len(orders))
	for _, o := range orders {
		orderByCategory[o.Group] = o
		resp.RebalTotal += o.Amount
	}

	// riskCategories are the market-risk/volatile slice of the nine
	// categories (the old four-group system's single "growth" bucket split
	// three ways) — riskPct/riskTarget sum their current/target% rather
	// than picking one row, same reasoning as summing MarketValue instead
	// of averaging percentages.
	riskCategories := map[string]bool{"equity": true, "fund": true, "crypto": true}
	var riskCur, riskTarget float64

	for _, row := range driftRows {
		name, venue := largestAsset(categories[row.Group].Assets)
		resp.Allocation = append(resp.Allocation, allocRow{
			Category: row.Group, MarketValue: row.MarketValue, CurrentPct: row.CurrentPct,
			TargetPct: row.TargetPct, DeviationPt: row.DeviationPt, Venue: venue,
		})
		if riskCategories[row.Group] {
			riskCur += row.CurrentPct
			riskTarget += row.TargetPct
		}
		if assets.LockedCategories[row.Group] {
			resp.Locked = append(resp.Locked, allocLockedRow{Category: row.Group, MarketValue: row.MarketValue, DeviationPt: row.DeviationPt})
			continue
		}
		if o, ok := orderByCategory[row.Group]; ok {
			resp.Orders = append(resp.Orders, allocOrder{
				Category: o.Group, Side: o.Side, Amount: o.Amount, DeviationPt: o.DeviationPt,
				AssetName: name, Venue: venue,
			})
		}
	}
	cur := riskCur
	resp.RiskPct = &cur
	lo, hi := riskTarget-assets.RebalanceThresholdPt, riskTarget+assets.RebalanceThresholdPt
	resp.RiskTargetLow, resp.RiskTargetHigh = &lo, &hi

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

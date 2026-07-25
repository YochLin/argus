package web

import (
	"log"
	"math"
	"strconv"

	"argus/internal/market"
)

// cashSettingKey/cashSettingKeyTWD mirror internal/bot/handlers.go's own
// constants of the same name — the settings table key /cash writes to.
// internal/web can't import internal/bot's unexported constants, so this is
// duplicated the same way spyTicker/twBenchmarkTicker are in dashboard.go
// rather than restructuring package boundaries for two string literals.
const (
	cashSettingKey    = "cash_balance"
	cashSettingKeyTWD = "cash_balance_twd"
)

func cashSettingKeyFor(m market.MarketID) string {
	if m == market.TW {
		return cashSettingKeyTWD
	}
	return cashSettingKey
}

// loadCash returns m's declared cash balance (0 if /cash has never set it) —
// mirrors internal/bot's Bot.loadCash, duplicated for the same
// can't-share-an-import reason as cashSettingKeyFor above.
func loadCash(database dbReader, m market.MarketID) (float64, error) {
	raw, ok, err := database.GetSetting(cashSettingKeyFor(m))
	if err != nil || !ok {
		return 0, err
	}
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return amount, nil
}

// buildRisk assembles the /api/risk response (Phase 8 PR1, see
// docs/phase-8-trader-analytics.md §3.2): current open-risk exposure across
// every position in market m, priced with live quotes (this is a "right
// now" view, not a replay off daily_snapshots, so it deliberately doesn't
// reuse pnl.go's DailyPnL engine). accountValue is the sum of each
// position's live market value plus m's declared cash balance — a position
// whose quote fetch fails contributes 0 to that sum (logged, not fatal),
// same "attach what's available" degrade convention buildDashboard uses for
// its own positions list.
//
// A position with no stop price set is excluded from the heat sum entirely
// (openRisk/openRiskPct come back nil, not 0 — 0 would read as "no risk,"
// which is the opposite of the truth) rather than assuming some fallback
// risk like a full loss of avgCost; see the design doc for why that
// alternative was rejected. A position that has already broken through its
// stop (price < stopPrice) reports openRisk 0 — that risk is no longer
// "open," it's already a realized/unrealized loss on the books — the
// frontend flags this case separately via price vs. stopPrice, not via a
// zero openRisk (which is ambiguous with "stop is exactly at price").
func buildRisk(database dbReader, quotes quoteGetter, m market.MarketID, heatThresholdPct float64) (riskResponse, error) {
	allPositions, err := database.GetPositions()
	if err != nil {
		return riskResponse{}, err
	}
	positions := filterPositionsByMarket(allPositions, m)

	cash, err := loadCash(database, m)
	if err != nil {
		log.Printf("web: risk: load cash for %s: %v", m, err)
	}

	type priced struct {
		pos   pricedPosition
		value float64
		ok    bool
	}
	priceds := make([]priced, 0, len(positions))
	var totalValue float64
	for _, p := range positions {
		q, err := quotes.GetQuote(p.Ticker)
		if err != nil {
			log.Printf("web: risk: get quote for %s: %v", p.Ticker, err)
			priceds = append(priceds, priced{pos: pricedPosition{Ticker: p.Ticker, Shares: p.Shares, AvgCost: p.AvgCost, StopPrice: p.StopPrice}})
			continue
		}
		value := q.Price * p.Shares
		totalValue += value
		priceds = append(priceds, priced{
			pos:   pricedPosition{Ticker: p.Ticker, Shares: p.Shares, AvgCost: p.AvgCost, StopPrice: p.StopPrice, Price: q.Price},
			value: value,
			ok:    true,
		})
	}

	accountValue := totalValue + cash

	resp := riskResponse{
		AccountValue:     accountValue,
		Cash:             cash,
		HeatThresholdPct: heatThresholdPct,
		Positions:        make([]riskPositionResponse, 0, len(priceds)),
	}

	var heatSum float64
	for _, pr := range priceds {
		rp := riskPositionResponse{
			Ticker:    pr.pos.Ticker,
			Shares:    pr.pos.Shares,
			AvgCost:   pr.pos.AvgCost,
			Price:     pr.pos.Price,
			Value:     pr.value,
			StopPrice: pr.pos.StopPrice,
		}
		if accountValue != 0 {
			rp.WeightPct = pr.value / accountValue * 100
		}
		if pr.ok && pr.pos.AvgCost != 0 {
			rp.UnrealizedPnLPct = (pr.pos.Price - pr.pos.AvgCost) / pr.pos.AvgCost * 100
		}
		if pr.ok && pr.pos.StopPrice > 0 {
			openRisk := math.Max(0, (pr.pos.Price-pr.pos.StopPrice)*pr.pos.Shares)
			var openRiskPct float64
			if accountValue != 0 {
				openRiskPct = openRisk / accountValue * 100
			}
			rp.OpenRisk = &openRisk
			rp.OpenRiskPct = &openRiskPct
			heatSum += openRisk
		}
		resp.Positions = append(resp.Positions, rp)
	}
	if accountValue != 0 {
		resp.HeatPct = heatSum / accountValue * 100
	}
	return resp, nil
}

// pricedPosition is buildRisk's internal working value — a db.Position plus
// whatever live price was resolved for it (0 if the quote fetch failed).
type pricedPosition struct {
	Ticker    string
	Shares    float64
	AvgCost   float64
	StopPrice float64
	Price     float64
}

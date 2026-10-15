package bot

import (
	"context"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/sinopac"
)

// syncDCAFundValues is RunClosingSnapshot(TW)'s tail step: a 定期定額 ETF
// never enters the trading account (SINOPAC_SKIP_TICKERS keeps the sync from
// proposing it), so it is recorded as a wealth fund asset whose fund code is
// the ticker, and this writes that asset's value each TW close from the
// broker's own holding — otherwise the fund's value would sit at whatever the
// user last typed. The trading account is only read, never written.
func (b *Bot) syncDCAFundValues(ctx context.Context, date string) {
	if b.sinopac == nil {
		return
	}
	list, err := b.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("dca funds: list assets: %v", err)
		return
	}
	codes := map[int64]string{}
	for _, a := range list {
		if a.Type != "fund" || a.Currency != "TWD" {
			continue
		}
		det, err := b.db.GetFundDetails(a.ID)
		if err != nil {
			logger.Errorf("dca funds: details %d: %v", a.ID, err)
			continue
		}
		if det != nil && det.Code != "" {
			codes[a.ID] = det.Code
		}
	}
	if len(codes) == 0 {
		return
	}

	held, err := b.sinopac.PositionUnit(ctx)
	if err != nil {
		logger.Errorf("dca funds: position_unit: %v", err)
		return
	}
	broker := map[string]sinopac.Position{}
	for _, p := range held {
		broker[p.Code] = p
	}
	traded, err := b.db.GetPositions()
	if err != nil {
		logger.Errorf("dca funds: positions: %v", err)
		return
	}
	tradedShares := map[string]float64{}
	for _, p := range traded {
		tradedShares[p.Ticker] = p.Shares
	}

	for id, code := range codes {
		p, ok := broker[code]
		if !ok {
			// ponytail: a fully sold DCA holding keeps its last value; archive the asset by hand.
			continue
		}
		q, err := b.provider.GetQuote(code)
		if err != nil || q.Price <= 0 {
			logger.Errorf("dca funds: quote %s: %v", code, err)
			continue
		}
		s, ok := dcaSnapshot(id, date, p, tradedShares[code], q.Price)
		if !ok {
			continue
		}
		if err := b.db.UpsertAssetSnapshot(s); err != nil {
			logger.Errorf("dca funds: save %s: %v", code, err)
		}
	}
}

// dcaSnapshot values the part of the broker's holding that isn't already in
// the trading account (shares bought by hand are counted there), at the
// broker's average cost. ok=false when nothing is left over.
func dcaSnapshot(assetID int64, date string, p sinopac.Position, tradedShares, price float64) (db.AssetSnapshot, bool) {
	shares := float64(p.Quantity) - tradedShares
	if shares <= 0 {
		return db.AssetSnapshot{}, false
	}
	cost := shares * p.Price
	return db.AssetSnapshot{AssetID: assetID, Date: date, Value: shares * price, Cost: &cost, Source: "sync"}, true
}

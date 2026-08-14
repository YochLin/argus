package service

import (
	"fmt"
	"strconv"
	"sync"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

const (
	CashSettingKeyUSD = "cash_balance"
	CashSettingKeyTWD = "cash_balance_twd"
)

// CashSettingKey returns the persisted cash-balance key for a market.
func CashSettingKey(m market.MarketID) string {
	if m == market.TW {
		return CashSettingKeyTWD
	}
	return CashSettingKeyUSD
}

// PortfolioStore is the read boundary needed to value an open portfolio.
type PortfolioStore interface {
	GetPositions() ([]db.Position, error)
	GetRealizedPnL(market.MarketID) (float64, error)
	GetSetting(key string) (string, bool, error)
}

type QuoteReader interface {
	GetQuote(ticker string) (*data.Quote, error)
}

// PositionValuation contains one open position and its best-effort live
// quote valuation. QuoteErr is retained so adapters can render/log a
// per-position failure without failing the whole portfolio response.
type PositionValuation struct {
	Position         db.Position
	Quote            *data.Quote
	QuoteErr         error
	MarketValue      float64
	UnrealizedPnL    float64
	UnrealizedPnLPct float64
}

type PortfolioSnapshot struct {
	Market           market.MarketID
	Positions        []PositionValuation
	TotalMarketValue float64
	RealizedPnL      float64
	RealizedPnLErr   error
	Cash             float64
	HasCash          bool
	CashErr          error
	AccountValue     float64
}

type PortfolioService struct {
	store  PortfolioStore
	quotes QuoteReader
}

func NewPortfolioService(store PortfolioStore, quotes QuoteReader) *PortfolioService {
	return &PortfolioService{store: store, quotes: quotes}
}

// Snapshot returns the open positions for one market with best-effort live
// valuation. Database failure while loading positions is fatal; individual
// quote, realized-P&L, or cash failures are retained on the result so a
// caller can degrade only the affected field.
func (s *PortfolioService) Snapshot(m market.MarketID) (PortfolioSnapshot, error) {
	positions, err := s.store.GetPositions()
	if err != nil {
		return PortfolioSnapshot{}, err
	}

	snapshot := PortfolioSnapshot{Market: m, Positions: make([]PositionValuation, 0)}
	for _, p := range positions {
		if market.Of(p.Ticker) != m {
			continue
		}
		snapshot.Positions = append(snapshot.Positions, PositionValuation{Position: p})
	}

	// Quote providers and the web quote cache are safe for concurrent reads.
	// Keep the result slice in DB order while matching the web dashboard's
	// existing parallel quote-fetch behavior.
	var wg sync.WaitGroup
	for i := range snapshot.Positions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pv := &snapshot.Positions[i]
			if s.quotes == nil {
				pv.QuoteErr = fmt.Errorf("quote provider unavailable")
				return
			}
			q, quoteErr := s.quotes.GetQuote(pv.Position.Ticker)
			if quoteErr != nil {
				pv.QuoteErr = quoteErr
				return
			}
			pv.Quote = q
		}(i)
	}
	wg.Wait()

	for i := range snapshot.Positions {
		pv := &snapshot.Positions[i]
		if pv.Quote == nil {
			continue
		}
		pv.MarketValue = pv.Quote.Price * pv.Position.Shares
		pv.UnrealizedPnL = (pv.Quote.Price - pv.Position.AvgCost) * pv.Position.Shares
		if pv.Position.AvgCost != 0 {
			pv.UnrealizedPnLPct = (pv.Quote.Price - pv.Position.AvgCost) / pv.Position.AvgCost * 100
		}
		snapshot.TotalMarketValue += pv.MarketValue
	}

	snapshot.RealizedPnL, snapshot.RealizedPnLErr = s.store.GetRealizedPnL(m)
	rawCash, ok, cashErr := s.store.GetSetting(CashSettingKey(m))
	snapshot.CashErr = cashErr
	if cashErr == nil && ok {
		snapshot.Cash, snapshot.CashErr = strconv.ParseFloat(rawCash, 64)
		if snapshot.CashErr == nil {
			snapshot.HasCash = true
		}
	}
	if snapshot.HasCash {
		snapshot.AccountValue = snapshot.TotalMarketValue + snapshot.Cash
	} else {
		snapshot.AccountValue = snapshot.TotalMarketValue
	}
	return snapshot, nil
}

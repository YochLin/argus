package service

import (
	"argus/internal/db"
	"argus/internal/market"
)

// AnalyticsStore is the persistence boundary needed by AnalyticsService.
type AnalyticsStore interface {
	GetAllTransactions() ([]db.Transaction, error)
	GetTransactions(ticker string) ([]db.Transaction, error)
	GetDailySnapshotsForTickers(tickers []string, from, to string) ([]db.DailySnapshot, error)
	GetSnapshotCloseRange(ticker, from, to string) (first, last float64, ok bool, err error)
}

// AnalyticsService provides centralized portfolio performance, round trip, and PnL analytics.
type AnalyticsService struct {
	store AnalyticsStore
}

// NewAnalyticsService constructs an AnalyticsService.
func NewAnalyticsService(store AnalyticsStore) *AnalyticsService {
	return &AnalyticsService{store: store}
}

// TickerRounds returns all round trips (both closed and open) for ticker.
func (s *AnalyticsService) TickerRounds(ticker string) ([]Round, error) {
	normTicker, err := NormalizeTicker(ticker)
	if err != nil {
		return nil, err
	}
	txs, err := s.store.GetTransactions(normTicker)
	if err != nil {
		return nil, err
	}
	return SegmentRounds(txs), nil
}

// LastClosedRound returns the latest completed round trip for ticker.
func (s *AnalyticsService) LastClosedRound(ticker string) (Round, bool, error) {
	normTicker, err := NormalizeTicker(ticker)
	if err != nil {
		return Round{}, false, err
	}
	txs, err := s.store.GetTransactions(normTicker)
	if err != nil {
		return Round{}, false, err
	}
	r, ok := LastClosedRound(txs)
	return r, ok, nil
}

// MarketRounds returns all round trips across tickers trading in market m.
func (s *AnalyticsService) MarketRounds(m market.MarketID) (map[string][]Round, error) {
	allTxs, err := s.store.GetAllTransactions()
	if err != nil {
		return nil, err
	}
	byTicker := make(map[string][]db.Transaction)
	for _, tx := range allTxs {
		if market.Of(tx.Ticker) == m {
			byTicker[tx.Ticker] = append(byTicker[tx.Ticker], tx)
		}
	}
	result := make(map[string][]Round, len(byTicker))
	for t, txs := range byTicker {
		result[t] = SegmentRounds(txs)
	}
	return result, nil
}

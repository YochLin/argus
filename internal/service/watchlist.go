package service

import (
	"errors"
	"strings"

	"argus/internal/market"
)

// ErrInvalidTicker is returned when an adapter passes an empty ticker to a
// watchlist use case. Adapters can map it to their own usage response.
var ErrInvalidTicker = errors.New("invalid ticker")

// WatchlistStore is the persistence boundary for watchlist operations.
type WatchlistStore interface {
	AddTicker(ticker string) error
	RemoveTicker(ticker string) error
	GetWatchlist() ([]string, error)
	GetWatchlistByMarket(market.MarketID) ([]string, error)
}

// WatchlistService owns ticker normalization and watchlist persistence so
// Telegram, Web, and MCP do not each implement slightly different versions.
type WatchlistService struct {
	store WatchlistStore
}

func NewWatchlistService(store WatchlistStore) *WatchlistService {
	return &WatchlistService{store: store}
}

func (s *WatchlistService) AddTicker(rawTicker string) error {
	ticker, err := NormalizeTicker(rawTicker)
	if err != nil {
		return err
	}
	if err := s.store.AddTicker(ticker); err != nil {
		return err
	}
	return nil
}

func (s *WatchlistService) RemoveTicker(rawTicker string) error {
	ticker, err := NormalizeTicker(rawTicker)
	if err != nil {
		return err
	}
	if err := s.store.RemoveTicker(ticker); err != nil {
		return err
	}
	return nil
}

// Add normalizes and records a ticker, returning the normalized value for
// adapters that include it in a confirmation message.
func (s *WatchlistService) Add(rawTicker string) (string, error) {
	ticker, err := NormalizeTicker(rawTicker)
	if err != nil {
		return "", err
	}
	if err := s.AddTicker(ticker); err != nil {
		return "", err
	}
	return ticker, nil
}

// Remove normalizes and removes a ticker, returning the normalized value for
// adapters that include it in a confirmation message.
func (s *WatchlistService) Remove(rawTicker string) (string, error) {
	ticker, err := NormalizeTicker(rawTicker)
	if err != nil {
		return "", err
	}
	if err := s.RemoveTicker(ticker); err != nil {
		return "", err
	}
	return ticker, nil
}

func (s *WatchlistService) GetWatchlist() ([]string, error) {
	return s.store.GetWatchlist()
}

func (s *WatchlistService) GetWatchlistByMarket(m market.MarketID) ([]string, error) {
	return s.store.GetWatchlistByMarket(m)
}

// NormalizeTicker is shared by write adapters. Market-specific validity is
// intentionally still determined by market.Of, matching the existing DB
// behavior; this function only owns the transport-independent normalization.
func NormalizeTicker(rawTicker string) (string, error) {
	ticker := strings.ToUpper(strings.TrimSpace(rawTicker))
	if ticker == "" {
		return "", ErrInvalidTicker
	}
	return ticker, nil
}

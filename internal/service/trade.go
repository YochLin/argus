// Package service contains application use cases shared by delivery
// adapters such as Telegram, Web, and MCP.
package service

import (
	"errors"
	"fmt"
	"strings"

	"argus/internal/db"
	"argus/internal/market"
	"argus/internal/paper"
)

// ErrInvalidTrade means that a trade request cannot be executed because its
// input values are invalid. Adapters can map this to their own validation
// response without knowing the persistence details.
var ErrInvalidTrade = errors.New("invalid trade")

// TradeStore is the persistence boundary needed by the trade use cases. The
// concrete *db.DB implements this today; keeping the service on this narrow
// interface makes it usable from non-Telegram adapters and straightforward to
// unit test without opening SQLite.
type TradeStore interface {
	GetPosition(ticker string) (db.Position, bool, error)
	RecordBuyExt(ticker string, shares, price, fee float64, date, extID string) (db.Position, error)
	RecordSellExt(ticker string, shares, price, fee float64, date, extID string) (db.Position, float64, error)
	AddTicker(ticker string) error
}

// BuyInput describes a buy request after the transport has parsed its input.
// A nil Fee means that the service should calculate the market-specific fee.
// ExtID is "" for every caller except Phase 16's Shioaji-synced pending
// actions — see db.RecordBuyExt.
type BuyInput struct {
	Ticker string
	Shares float64
	Price  float64
	Fee    *float64
	Date   string
	ExtID  string
}

// SellInput describes a sell request after the transport has parsed its input.
// A nil Fee means that the service should calculate the market-specific fee.
// ExtID is "" for every caller except Phase 16's Shioaji-synced pending
// actions — see db.RecordSellExt.
type SellInput struct {
	Ticker string
	Shares float64
	Price  float64
	Fee    *float64
	Date   string
	ExtID  string
}

// TradeResult is the transport-neutral result of a completed trade.
type TradeResult struct {
	Ticker      string
	Position    db.Position
	RealizedPnL float64
	StopPrice   float64
	Closed      bool
	Fee         float64
	FeeAuto     bool
}

// TradeService owns the application-level trade workflow. It deliberately
// does not format messages or trigger channel-specific side effects such as
// Telegram notifications or LLM reviews.
type TradeService struct {
	store         TradeStore
	twFeeDiscount float64
}

func NewTradeService(store TradeStore, twFeeDiscount float64) *TradeService {
	return &TradeService{store: store, twFeeDiscount: twFeeDiscount}
}

// Buy records a buy and automatically puts the ticker on the watchlist. A
// watchlist failure is intentionally non-fatal: the trade has already been
// recorded and callers should not report it as failed.
func (s *TradeService) Buy(in BuyInput) (TradeResult, error) {
	ticker, fee, feeAuto, err := validateTrade(in.Ticker, in.Shares, in.Price, in.Fee, "BUY", s.twFeeDiscount)
	if err != nil {
		return TradeResult{}, err
	}

	pos, err := s.store.RecordBuyExt(ticker, in.Shares, in.Price, fee, in.Date, in.ExtID)
	if err != nil {
		return TradeResult{}, err
	}
	_ = s.store.AddTicker(ticker)

	return TradeResult{
		Ticker:   ticker,
		Position: pos,
		Fee:      fee,
		FeeAuto:  feeAuto,
	}, nil
}

// Sell records a sell and returns whether it fully closed the position. The
// caller decides what to do with that state (for example, starting a review
// in the background for Telegram).
func (s *TradeService) Sell(in SellInput) (TradeResult, error) {
	ticker, fee, feeAuto, err := validateTrade(in.Ticker, in.Shares, in.Price, in.Fee, "SELL", s.twFeeDiscount)
	if err != nil {
		return TradeResult{}, err
	}

	var stopPrice float64
	if pos, ok, getErr := s.store.GetPosition(ticker); getErr == nil && ok {
		stopPrice = pos.StopPrice
	}

	pos, realizedPnL, err := s.store.RecordSellExt(ticker, in.Shares, in.Price, fee, in.Date, in.ExtID)
	if err != nil {
		return TradeResult{}, err
	}

	return TradeResult{
		Ticker:      ticker,
		Position:    pos,
		RealizedPnL: realizedPnL,
		StopPrice:   stopPrice,
		Closed:      pos.Shares == 0,
		Fee:         fee,
		FeeAuto:     feeAuto,
	}, nil
}

func validateTrade(rawTicker string, shares, price float64, fee *float64, side string, twFeeDiscount float64) (string, float64, bool, error) {
	ticker := strings.ToUpper(strings.TrimSpace(rawTicker))
	if ticker == "" || shares <= 0 || price <= 0 || (fee != nil && *fee < 0) {
		return "", 0, false, fmt.Errorf("%w: %s requires ticker, positive shares and price, and non-negative fee", ErrInvalidTrade, side)
	}
	if fee != nil {
		return ticker, *fee, false, nil
	}
	return ticker, paper.FeeFor(market.Of(ticker), side, shares*price, twFeeDiscount), true, nil
}

package service

import (
	"errors"
	"fmt"
	"math"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/market"
	"argus/internal/paper"
	"argus/internal/signals"
)

const (
	// StopCandidateATRMult is the default ATR multiplier for the
	// volatility-adaptive stop candidate.
	StopCandidateATRMult = 2.0

	// TargetRMultiple is the textbook 2R target multiple.
	TargetRMultiple = 2.0

	// TrailProfitPct is the profit gate (10%) for MA5 trend defense.
	TrailProfitPct = 10.0

	// Signal state constants.
	StopLossSignalFamily     = "stop_loss"
	TrailingStopSignalFamily = "trailing_stop"
	TargetSignalFamily       = "target"
	MA5TrailSignalFamily     = "ma5_trail"
	BreachedState            = "breached"
	HitState                 = "hit"
)

var (
	ErrNoReferencePrice   = errors.New("no reference price available")
	ErrNonPositivePrice   = errors.New("price must be positive")
	ErrInvalidStopPrice   = errors.New("stop price must be below latest close")
	ErrQuoteUnavailable   = errors.New("quote unavailable")
	ErrHistoryUnavailable = errors.New("history unavailable")
)

// InvalidStopPriceError is returned when a stop price is at or above the latest close.
// It carries the reference close price used during validation so callers can format
// error messages without redundant market data queries.
type InvalidStopPriceError struct {
	Price       float64
	LatestClose float64
}

func (e *InvalidStopPriceError) Error() string {
	return fmt.Sprintf("stop price %.2f must be below latest close %.2f", e.Price, e.LatestClose)
}

func (e *InvalidStopPriceError) Is(target error) bool {
	return target == ErrInvalidStopPrice
}

// RiskStore is the persistence boundary needed for risk and alert operations.
type RiskStore interface {
	GetPosition(ticker string) (db.Position, bool, error)
	SetStopPrice(ticker string, price float64) error
	GetSignalState(ticker, family string) (string, error)
	SetSignalState(ticker, family, state string) error
	GetEarliestBuyDate(ticker string) (string, bool, error)
	GetPeakClose(ticker, sinceDate string) (float64, bool, error)
	AddBuyAlert(ticker string, price float64, direction string) (int64, error)
	GetBuyAlertsByTicker(ticker string) ([]db.BuyAlert, error)
	RemoveBuyAlert(id int64) error
}

type RiskHistoryReader interface {
	GetHistory(ticker, rangeParam string) ([]data.Candle, error)
}

type RiskQuoteReader interface {
	GetQuote(ticker string) (*data.Quote, error)
}

// StopSuggestion holds reference prices computed from recent OHLCV history.
type StopSuggestion struct {
	LatestClose float64
	Low10       float64
	Low20       float64
	ATRBased    float64
}

type SetStopInput struct {
	Ticker string
	Price  float64
}

type SetStopResult struct {
	Ticker       string
	StopPrice    float64
	LatestClose  float64
	DistPct      float64
	RiskPerShare float64
	AvgCost      float64
}

type BuyAlertInput struct {
	Ticker string
	Price  float64
}

type BuyAlertResult struct {
	ID           int64
	Ticker       string
	Price        float64
	CurrentPrice float64
	Direction    string
}

type StopLossAlert struct {
	Ticker       string
	AvgCost      float64
	CurrentPrice float64
	StopPrice    float64 // 0 if percentage-based fallback
	LossPct      float64
	IsCustomStop bool
}

type TrailingStopAlert struct {
	Ticker          string
	PeakPrice       float64
	CurrentPrice    float64
	DrawdownPct     float64
	ThresholdPct    float64
	ATRBased        bool
	TrailingATRMult float64
}

type TargetAlert struct {
	Ticker       string
	AvgCost      float64
	StopPrice    float64
	TargetPrice  float64
	CurrentPrice float64
	RMultiple    float64
}

type MA5BreakAlert struct {
	Ticker       string
	AvgCost      float64
	CurrentPrice float64
	MA5          float64
	ProfitPct    float64
}

type BuyAlertTrigger struct {
	Alert        db.BuyAlert
	CurrentPrice float64
}

type RiskService struct {
	store                RiskStore
	history              RiskHistoryReader
	quotes               RiskQuoteReader
	stopCandidateATRMult float64
}

func NewRiskService(store RiskStore, history RiskHistoryReader, quotes RiskQuoteReader, stopCandidateATRMult float64) *RiskService {
	if stopCandidateATRMult <= 0 {
		stopCandidateATRMult = StopCandidateATRMult
	}
	return &RiskService{
		store:                store,
		history:              history,
		quotes:               quotes,
		stopCandidateATRMult: stopCandidateATRMult,
	}
}

// ComputeStopSuggestion fetches ticker's OHLCV history and derives candidate stop levels.
// Falls back to a live quote for LatestClose when history is unavailable.
func (s *RiskService) ComputeStopSuggestion(ticker string) (StopSuggestion, bool) {
	var res StopSuggestion

	if s.history != nil {
		candles, err := s.history.GetHistory(ticker, "1y")
		if err == nil && len(candles) > 0 {
			closes := data.Closes(candles)
			highs := data.Highs(candles)
			lows := data.Lows(candles)
			res.LatestClose = closes[len(closes)-1]
			res.Low10 = signals.LowestClose(closes, 10)
			res.Low20 = signals.LowestClose(closes, 20)
			if atr := signals.ATR(highs, lows, closes, 14); atr > 0 {
				res.ATRBased = res.LatestClose - s.stopCandidateATRMult*atr
			}
			return res, true
		}
	}

	if s.quotes != nil {
		q, err := s.quotes.GetQuote(ticker)
		if err == nil && q != nil && q.Price > 0 {
			res.LatestClose = q.Price
			return res, true
		}
	}

	return StopSuggestion{}, false
}

// SetStop validates and records a custom stop-loss price for an open position.
func (s *RiskService) SetStop(in SetStopInput) (SetStopResult, error) {
	ticker, err := NormalizeTicker(in.Ticker)
	if err != nil {
		return SetStopResult{}, err
	}
	if in.Price <= 0 {
		return SetStopResult{}, ErrNonPositivePrice
	}

	pos, ok, err := s.store.GetPosition(ticker)
	if err != nil {
		return SetStopResult{}, err
	}
	if !ok {
		return SetStopResult{}, db.ErrNoPosition
	}

	suggestion, refOK := s.ComputeStopSuggestion(ticker)
	if !refOK {
		return SetStopResult{}, ErrNoReferencePrice
	}
	if in.Price >= suggestion.LatestClose {
		return SetStopResult{}, &InvalidStopPriceError{
			Price:       in.Price,
			LatestClose: suggestion.LatestClose,
		}
	}

	if err := s.store.SetStopPrice(ticker, in.Price); err != nil {
		return SetStopResult{}, err
	}

	distPct := (suggestion.LatestClose - in.Price) / suggestion.LatestClose * 100
	riskPerShare := pos.AvgCost - in.Price

	return SetStopResult{
		Ticker:       ticker,
		StopPrice:    in.Price,
		LatestClose:  suggestion.LatestClose,
		DistPct:      distPct,
		RiskPerShare: riskPerShare,
		AvgCost:      pos.AvgCost,
	}, nil
}

// AddBuyAlert adds a target price alert for a ticker.
func (s *RiskService) AddBuyAlert(in BuyAlertInput) (BuyAlertResult, error) {
	ticker, err := NormalizeTicker(in.Ticker)
	if err != nil {
		return BuyAlertResult{}, err
	}
	if in.Price <= 0 {
		return BuyAlertResult{}, ErrNonPositivePrice
	}
	if s.quotes == nil {
		return BuyAlertResult{}, ErrQuoteUnavailable
	}

	q, err := s.quotes.GetQuote(ticker)
	if err != nil || q == nil || q.Price <= 0 {
		return BuyAlertResult{}, ErrQuoteUnavailable
	}

	direction := BuyAlertDirection(in.Price, q.Price)
	id, err := s.store.AddBuyAlert(ticker, in.Price, direction)
	if err != nil {
		return BuyAlertResult{}, err
	}

	return BuyAlertResult{
		ID:           id,
		Ticker:       ticker,
		Price:        in.Price,
		CurrentPrice: q.Price,
		Direction:    direction,
	}, nil
}

// RemoveBuyAlert removes a buy alert by its unique ID.
func (s *RiskService) RemoveBuyAlert(id int64) error {
	return s.store.RemoveBuyAlert(id)
}

// RemoveBuyAlertByPrice removes buy alerts matching ticker and target price.
func (s *RiskService) RemoveBuyAlertByPrice(ticker string, price float64) (bool, error) {
	normTicker, err := NormalizeTicker(ticker)
	if err != nil {
		return false, err
	}
	alerts, err := s.store.GetBuyAlertsByTicker(normTicker)
	if err != nil {
		return false, err
	}
	removed := false
	for _, a := range alerts {
		if math.Abs(a.Price-price) < 1e-4 {
			if err := s.store.RemoveBuyAlert(a.ID); err != nil {
				return false, err
			}
			removed = true
		}
	}
	return removed, nil
}

// GetBuyAlerts retrieves all buy alerts for a ticker.
func (s *RiskService) GetBuyAlerts(ticker string) ([]db.BuyAlert, error) {
	normTicker, err := NormalizeTicker(ticker)
	if err != nil {
		return nil, err
	}
	return s.store.GetBuyAlertsByTicker(normTicker)
}

// EvaluateStopLoss checks open positions for stop-loss breaches (custom stop price or percentage fallback).
func (s *RiskService) EvaluateStopLoss(positions []db.Position, prices map[string]float64, stopLossPct, stopLossPctTW float64) ([]StopLossAlert, error) {
	var alerts []StopLossAlert

	for _, p := range positions {
		price, ok := s.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}

		effectivePct := stopLossPct
		if market.Of(p.Ticker) == market.TW {
			effectivePct = stopLossPctTW
		}

		prev, err := s.store.GetSignalState(p.Ticker, StopLossSignalFamily)
		if err != nil {
			logger.Errorf("stop loss state %s: %v", p.Ticker, err)
			prev = ""
		}

		if p.StopPrice > 0 {
			_, shouldAlert, newState := StopBreachDecision(price, p.StopPrice, prev)
			if newState != prev {
				if err := s.store.SetSignalState(p.Ticker, StopLossSignalFamily, newState); err != nil {
					logger.Errorf("stop loss state %s: %v", p.Ticker, err)
				}
			}
			if shouldAlert {
				alerts = append(alerts, StopLossAlert{
					Ticker:       p.Ticker,
					AvgCost:      p.AvgCost,
					CurrentPrice: price,
					StopPrice:    p.StopPrice,
					LossPct:      (p.AvgCost - price) / p.AvgCost * 100,
					IsCustomStop: true,
				})
			}
			continue
		}

		if effectivePct <= 0 {
			continue
		}
		lossPct := (p.AvgCost - price) / p.AvgCost * 100
		_, shouldAlert, newState := BreachAlertDecision(lossPct, effectivePct, prev)
		if newState != prev {
			if err := s.store.SetSignalState(p.Ticker, StopLossSignalFamily, newState); err != nil {
				logger.Errorf("stop loss state %s: %v", p.Ticker, err)
			}
		}
		if shouldAlert {
			alerts = append(alerts, StopLossAlert{
				Ticker:       p.Ticker,
				AvgCost:      p.AvgCost,
				CurrentPrice: price,
				LossPct:      lossPct,
				IsCustomStop: false,
			})
		}
	}

	return alerts, nil
}

// EvaluateTrailingStop checks open positions for trailing stop drawdowns from their peak close.
func (s *RiskService) EvaluateTrailingStop(
	positions []db.Position,
	prices map[string]float64,
	atrs map[string]float64,
	trailingStopPct, trailingStopPctTW, trailingStopATRMult float64,
) ([]TrailingStopAlert, error) {
	if trailingStopPct <= 0 && trailingStopPctTW <= 0 && trailingStopATRMult <= 0 {
		return nil, nil
	}

	var alerts []TrailingStopAlert

	for _, p := range positions {
		effectivePct := trailingStopPct
		if market.Of(p.Ticker) == market.TW {
			effectivePct = trailingStopPctTW
		}
		if effectivePct <= 0 && trailingStopATRMult <= 0 {
			continue
		}

		buyDate, ok, err := s.store.GetEarliestBuyDate(p.Ticker)
		if err != nil {
			logger.Errorf("trailing stop: earliest buy %s: %v", p.Ticker, err)
			continue
		}
		if !ok {
			continue
		}

		peak, ok, err := s.store.GetPeakClose(p.Ticker, buyDate)
		if err != nil {
			logger.Errorf("trailing stop: peak close %s: %v", p.Ticker, err)
			continue
		}
		if !ok || peak <= 0 {
			continue
		}

		price, ok := s.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}

		drawdownPct := (peak - price) / peak * 100
		atr := atrs[p.Ticker]
		if atr <= 0 && trailingStopATRMult > 0 && s.history != nil {
			if candles, err := s.history.GetHistory(p.Ticker, "1y"); err == nil && len(candles) >= 14 {
				atr = signals.ATR(data.Highs(candles), data.Lows(candles), data.Closes(candles), 14)
			}
		}

		thresholdPct, atrBased, ok := paper.TrailingStopThreshold(effectivePct, trailingStopATRMult, atr, peak)
		if !ok {
			logger.Warnf("trailing stop: no usable threshold for %s (fixed=%.2f atrMult=%.2f atr=%.2f)", p.Ticker, effectivePct, trailingStopATRMult, atr)
			continue
		}

		prev, err := s.store.GetSignalState(p.Ticker, TrailingStopSignalFamily)
		if err != nil {
			logger.Errorf("trailing stop state %s: %v", p.Ticker, err)
			prev = ""
		}

		_, shouldAlert, newState := BreachAlertDecision(drawdownPct, thresholdPct, prev)
		if newState != prev {
			if err := s.store.SetSignalState(p.Ticker, TrailingStopSignalFamily, newState); err != nil {
				logger.Errorf("trailing stop state %s: %v", p.Ticker, err)
			}
		}
		if shouldAlert {
			alerts = append(alerts, TrailingStopAlert{
				Ticker:          p.Ticker,
				PeakPrice:       peak,
				CurrentPrice:    price,
				DrawdownPct:     drawdownPct,
				ThresholdPct:    thresholdPct,
				ATRBased:        atrBased,
				TrailingATRMult: trailingStopATRMult,
			})
		}
	}

	return alerts, nil
}

// EvaluateTargetAlerts checks open positions for reaching 2R target prices.
func (s *RiskService) EvaluateTargetAlerts(positions []db.Position, prices map[string]float64) ([]TargetAlert, error) {
	var alerts []TargetAlert

	for _, p := range positions {
		if p.StopPrice <= 0 || p.StopPrice >= p.AvgCost {
			continue
		}
		price, ok := s.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		target := p.AvgCost + TargetRMultiple*(p.AvgCost-p.StopPrice)

		prev, err := s.store.GetSignalState(p.Ticker, TargetSignalFamily)
		if err != nil {
			logger.Errorf("target state %s: %v", p.Ticker, err)
			prev = ""
		}
		_, shouldAlert, newState := TargetReachedDecision(price, target, prev)
		if newState != prev {
			if err := s.store.SetSignalState(p.Ticker, TargetSignalFamily, newState); err != nil {
				logger.Errorf("target state %s: %v", p.Ticker, err)
			}
		}
		if shouldAlert {
			alerts = append(alerts, TargetAlert{
				Ticker:       p.Ticker,
				AvgCost:      p.AvgCost,
				StopPrice:    p.StopPrice,
				TargetPrice:  target,
				CurrentPrice: price,
				RMultiple:    TargetRMultiple,
			})
		}
	}

	return alerts, nil
}

// EvaluateMA5BreakAlerts checks positions with >= 10% unrealized profit for MA5 breaks.
func (s *RiskService) EvaluateMA5BreakAlerts(positions []db.Position, prices map[string]float64, ma5s map[string]float64) ([]MA5BreakAlert, error) {
	var alerts []MA5BreakAlert

	for _, p := range positions {
		if p.AvgCost <= 0 {
			continue
		}
		price, ok := s.priceFor(p.Ticker, prices)
		if !ok {
			continue
		}
		profitPct := (price - p.AvgCost) / p.AvgCost * 100
		if profitPct < TrailProfitPct {
			continue
		}

		ma5, ok := ma5s[p.Ticker]
		if !ok || ma5 <= 0 {
			if s.history != nil {
				if candles, err := s.history.GetHistory(p.Ticker, "1y"); err == nil && len(candles) >= 5 {
					ma5 = signals.MA(data.Closes(candles), 5)
				}
			}
		}
		if ma5 <= 0 {
			continue
		}

		prev, err := s.store.GetSignalState(p.Ticker, MA5TrailSignalFamily)
		if err != nil {
			logger.Errorf("ma5 trail state %s: %v", p.Ticker, err)
			prev = ""
		}
		_, shouldAlert, newState := StopBreachDecision(price, ma5, prev)
		if newState != prev {
			if err := s.store.SetSignalState(p.Ticker, MA5TrailSignalFamily, newState); err != nil {
				logger.Errorf("ma5 trail state %s: %v", p.Ticker, err)
			}
		}
		if shouldAlert {
			alerts = append(alerts, MA5BreakAlert{
				Ticker:       p.Ticker,
				AvgCost:      p.AvgCost,
				CurrentPrice: price,
				MA5:          ma5,
				ProfitPct:    profitPct,
			})
		}
	}

	return alerts, nil
}

// EvaluateBuyAlerts checks and triggers buy alerts. Triggered alerts are removed from persistence.
func (s *RiskService) EvaluateBuyAlerts(alerts []db.BuyAlert, prices map[string]float64) ([]BuyAlertTrigger, error) {
	var triggered []BuyAlertTrigger

	for _, a := range alerts {
		price, ok := s.priceFor(a.Ticker, prices)
		if !ok {
			continue
		}
		if !BuyAlertTriggered(price, a) {
			continue
		}
		if err := s.store.RemoveBuyAlert(a.ID); err != nil {
			logger.Errorf("buy alert remove %s (id=%d): %v", a.Ticker, a.ID, err)
		}
		triggered = append(triggered, BuyAlertTrigger{
			Alert:        a,
			CurrentPrice: price,
		})
	}

	return triggered, nil
}

func (s *RiskService) priceFor(ticker string, prices map[string]float64) (float64, bool) {
	if p, ok := prices[ticker]; ok {
		return p, true
	}
	if s.quotes != nil {
		q, err := s.quotes.GetQuote(ticker)
		if err == nil && q != nil && q.Price > 0 {
			return q.Price, true
		}
	}
	return 0, false
}

// BreachAlertDecision dedupes threshold-based adverse moves (percentage).
func BreachAlertDecision(adverseMovePct, thresholdPct float64, prevState string) (breached, shouldAlert bool, newState string) {
	if adverseMovePct < thresholdPct {
		return false, false, ""
	}
	if prevState == BreachedState {
		return true, false, BreachedState
	}
	return true, true, BreachedState
}

// StopBreachDecision dedupes absolute price drops below a threshold.
func StopBreachDecision(closePrice, stopPrice float64, prevState string) (breached, shouldAlert bool, newState string) {
	if closePrice >= stopPrice {
		return false, false, ""
	}
	if prevState == BreachedState {
		return true, false, BreachedState
	}
	return true, true, BreachedState
}

// TargetReachedDecision dedupes absolute price reaching or exceeding a target.
func TargetReachedDecision(closePrice, targetPrice float64, prevState string) (reached, shouldAlert bool, newState string) {
	if closePrice < targetPrice {
		return false, false, ""
	}
	if prevState == HitState {
		return true, false, HitState
	}
	return true, true, HitState
}

// BuyAlertDirection infers whether a buy alert is watching above or below the current price.
func BuyAlertDirection(price, currentPrice float64) string {
	if price <= currentPrice {
		return db.BuyAlertBelow
	}
	return db.BuyAlertAbove
}

// BuyAlertTriggered checks if current price satisfies the buy alert condition.
func BuyAlertTriggered(price float64, alert db.BuyAlert) bool {
	if alert.Direction == db.BuyAlertAbove {
		return price >= alert.Price
	}
	return price <= alert.Price
}

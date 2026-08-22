package service

import (
	"math"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
)

type mockRiskStore struct {
	positions    map[string]db.Position
	signalStates map[string]string // key: ticker:family
	buyAlerts    map[int64]db.BuyAlert
	nextAlertID  int64
	earliestBuy  map[string]string
	peakClose    map[string]float64
}

func newMockRiskStore() *mockRiskStore {
	return &mockRiskStore{
		positions:    make(map[string]db.Position),
		signalStates: make(map[string]string),
		buyAlerts:    make(map[int64]db.BuyAlert),
		nextAlertID:  1,
		earliestBuy:  make(map[string]string),
		peakClose:    make(map[string]float64),
	}
}

func (m *mockRiskStore) GetPosition(ticker string) (db.Position, bool, error) {
	p, ok := m.positions[ticker]
	return p, ok, nil
}

func (m *mockRiskStore) GetPositions() ([]db.Position, error) {
	out := make([]db.Position, 0, len(m.positions))
	for _, p := range m.positions {
		out = append(out, p)
	}
	return out, nil
}

func (m *mockRiskStore) SetStopPrice(ticker string, price float64) error {
	p, ok := m.positions[ticker]
	if !ok {
		return db.ErrNoPosition
	}
	p.StopPrice = price
	m.positions[ticker] = p
	return nil
}

func (m *mockRiskStore) GetSignalState(ticker, family string) (string, error) {
	return m.signalStates[ticker+":"+family], nil
}

func (m *mockRiskStore) SetSignalState(ticker, family, state string) error {
	if state == "" {
		delete(m.signalStates, ticker+":"+family)
	} else {
		m.signalStates[ticker+":"+family] = state
	}
	return nil
}

func (m *mockRiskStore) GetEarliestBuyDate(ticker string) (string, bool, error) {
	d, ok := m.earliestBuy[ticker]
	return d, ok, nil
}

func (m *mockRiskStore) GetPeakClose(ticker, sinceDate string) (float64, bool, error) {
	p, ok := m.peakClose[ticker]
	return p, ok, nil
}

func (m *mockRiskStore) AddBuyAlert(ticker string, price float64, direction string) (int64, error) {
	id := m.nextAlertID
	m.nextAlertID++
	m.buyAlerts[id] = db.BuyAlert{
		ID:        id,
		Ticker:    ticker,
		Price:     price,
		Direction: direction,
	}
	return id, nil
}

func (m *mockRiskStore) GetBuyAlertsByTicker(ticker string) ([]db.BuyAlert, error) {
	var out []db.BuyAlert
	for _, a := range m.buyAlerts {
		if a.Ticker == ticker {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *mockRiskStore) RemoveBuyAlert(id int64) error {
	delete(m.buyAlerts, id)
	return nil
}

type mockRiskHistory struct {
	history map[string][]data.Candle
}

func (m *mockRiskHistory) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	if c, ok := m.history[ticker]; ok {
		return c, nil
	}
	return nil, ErrHistoryUnavailable
}

type mockRiskQuotes struct {
	quotes map[string]*data.Quote
}

func (m *mockRiskQuotes) GetQuote(ticker string) (*data.Quote, error) {
	if q, ok := m.quotes[ticker]; ok {
		return q, nil
	}
	return nil, ErrQuoteUnavailable
}

func TestComputeStopSuggestion(t *testing.T) {
	store := newMockRiskStore()
	t0 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	aaplCandles := make([]data.Candle, 25)
	for i := 0; i < 25; i++ {
		aaplCandles[i] = data.Candle{
			Date:  t0.AddDate(0, 0, i),
			Open:  100 + float64(i),
			High:  105 + float64(i),
			Low:   95 + float64(i),
			Close: 100 + float64(i),
		}
	}
	history := &mockRiskHistory{
		history: map[string][]data.Candle{
			"AAPL": aaplCandles,
		},
	}
	quotes := &mockRiskQuotes{
		quotes: map[string]*data.Quote{
			"MSFT": {Ticker: "MSFT", Price: 300},
		},
	}

	svc := NewRiskService(store, history, quotes, 2.0)

	// AAPL from history
	sugg, ok := svc.ComputeStopSuggestion("AAPL")
	if !ok {
		t.Fatalf("expected suggestion for AAPL")
	}
	if sugg.LatestClose != 124 {
		t.Errorf("expected LatestClose=124, got %v", sugg.LatestClose)
	}
	if sugg.Low10 != 115 {
		t.Errorf("expected Low10=115, got %v", sugg.Low10)
	}
	if sugg.Low20 != 105 {
		t.Errorf("expected Low20=105, got %v", sugg.Low20)
	}
	if sugg.ATRBased <= 0 {
		t.Errorf("expected positive ATRBased, got %v", sugg.ATRBased)
	}

	// MSFT from quotes fallback
	suggMSFT, ok := svc.ComputeStopSuggestion("MSFT")
	if !ok {
		t.Fatalf("expected suggestion for MSFT")
	}
	if suggMSFT.LatestClose != 300 {
		t.Errorf("expected LatestClose=300, got %v", suggMSFT.LatestClose)
	}
	if suggMSFT.Low10 != 0 {
		t.Errorf("expected Low10=0 on fallback, got %v", suggMSFT.Low10)
	}

	// Unknown
	_, ok = svc.ComputeStopSuggestion("UNKNOWN")
	if ok {
		t.Fatalf("expected false for UNKNOWN")
	}
}

func TestSetStop(t *testing.T) {
	store := newMockRiskStore()
	store.positions["AAPL"] = db.Position{Ticker: "AAPL", Shares: 10, AvgCost: 150}
	quotes := &mockRiskQuotes{
		quotes: map[string]*data.Quote{
			"AAPL": {Ticker: "AAPL", Price: 160},
		},
	}

	svc := NewRiskService(store, nil, quotes, 2.0)

	// Valid stop
	res, err := svc.SetStop(SetStopInput{Ticker: "AAPL", Price: 140})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StopPrice != 140 {
		t.Errorf("expected StopPrice=140, got %v", res.StopPrice)
	}
	if res.LatestClose != 160 {
		t.Errorf("expected LatestClose=160, got %v", res.LatestClose)
	}
	if math.Abs(res.DistPct-12.5) > 1e-4 {
		t.Errorf("expected DistPct=12.5, got %v", res.DistPct)
	}
	if res.RiskPerShare != 10 { // 150 - 140
		t.Errorf("expected RiskPerShare=10, got %v", res.RiskPerShare)
	}

	// Invalid stop >= close
	_, err = svc.SetStop(SetStopInput{Ticker: "AAPL", Price: 165})
	if err == nil {
		t.Fatalf("expected error for stop price >= latest close")
	}

	// No position
	_, err = svc.SetStop(SetStopInput{Ticker: "GOOG", Price: 100})
	if err != db.ErrNoPosition {
		t.Fatalf("expected ErrNoPosition, got %v", err)
	}
}

func TestBuyAlerts(t *testing.T) {
	store := newMockRiskStore()
	quotes := &mockRiskQuotes{
		quotes: map[string]*data.Quote{
			"NVDA": {Ticker: "NVDA", Price: 100},
		},
	}
	svc := NewRiskService(store, nil, quotes, 2.0)

	// Add Dip Alert (<= current)
	res1, err := svc.AddBuyAlert(BuyAlertInput{Ticker: "NVDA", Price: 90})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1.Direction != db.BuyAlertBelow {
		t.Errorf("expected BuyAlertBelow, got %v", res1.Direction)
	}

	// Add Breakout Alert (> current)
	res2, err := svc.AddBuyAlert(BuyAlertInput{Ticker: "NVDA", Price: 110})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.Direction != db.BuyAlertAbove {
		t.Errorf("expected BuyAlertAbove, got %v", res2.Direction)
	}

	// List
	alerts, err := svc.GetBuyAlerts("NVDA")
	if err != nil || len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %v, err=%v", len(alerts), err)
	}

	// Evaluate buy alert triggers
	// NVDA drops to 89 -> triggers dip alert (90)
	triggers, err := svc.EvaluateBuyAlerts(alerts, map[string]float64{"NVDA": 89})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(triggers) != 1 || triggers[0].Alert.ID != res1.ID {
		t.Fatalf("expected trigger for alert 1, got %v", triggers)
	}

	// Verify alert 1 was deleted from store
	remaining, _ := svc.GetBuyAlerts("NVDA")
	if len(remaining) != 1 || remaining[0].ID != res2.ID {
		t.Fatalf("expected only alert 2 remaining, got %v", remaining)
	}
}

func TestEvaluateStopLoss(t *testing.T) {
	store := newMockRiskStore()
	svc := NewRiskService(store, nil, nil, 2.0)

	positions := []db.Position{
		{Ticker: "AAPL", AvgCost: 100, StopPrice: 90}, // custom stop at 90
		{Ticker: "MSFT", AvgCost: 200, StopPrice: 0},  // 10% global stop -> 180
	}

	// Price drops: AAPL=88 (breach), MSFT=175 (breach)
	prices := map[string]float64{
		"AAPL": 88,
		"MSFT": 175,
	}

	alerts, err := svc.EvaluateStopLoss(positions, prices, 10.0, 10.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %v", len(alerts))
	}

	// Check deduplication (call again with same prices -> 0 alerts)
	alerts2, err := svc.EvaluateStopLoss(positions, prices, 10.0, 10.0)
	if err != nil || len(alerts2) != 0 {
		t.Fatalf("expected 0 alerts on second call, got %v", len(alerts2))
	}

	// Price recovers: AAPL=95, MSFT=190 -> state resets
	pricesRecover := map[string]float64{
		"AAPL": 95,
		"MSFT": 190,
	}
	_, _ = svc.EvaluateStopLoss(positions, pricesRecover, 10.0, 10.0)

	// Price drops again -> alerts again
	alerts3, _ := svc.EvaluateStopLoss(positions, prices, 10.0, 10.0)
	if len(alerts3) != 2 {
		t.Fatalf("expected 2 alerts after recovery, got %v", len(alerts3))
	}
}

func TestEvaluateTrailingStop(t *testing.T) {
	store := newMockRiskStore()
	store.earliestBuy["AAPL"] = "2026-08-01"
	store.peakClose["AAPL"] = 200 // peak = 200

	svc := NewRiskService(store, nil, nil, 2.0)

	positions := []db.Position{
		{Ticker: "AAPL", AvgCost: 150},
	}

	// 10% trailing stop -> threshold is 180. Current price = 175 (drawdown = 12.5%)
	prices := map[string]float64{"AAPL": 175}

	alerts, err := svc.EvaluateTrailingStop(positions, prices, nil, 10.0, 10.0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 trailing stop alert, got %v", len(alerts))
	}
	if alerts[0].PeakPrice != 200 || alerts[0].CurrentPrice != 175 {
		t.Errorf("unexpected alert values: %+v", alerts[0])
	}
}

func TestEvaluateTargetAndMA5Alerts(t *testing.T) {
	store := newMockRiskStore()
	svc := NewRiskService(store, nil, nil, 2.0)

	// Target 2R: cost=100, stop=90 -> 1R=10 -> target=120
	positions := []db.Position{
		{Ticker: "AAPL", AvgCost: 100, StopPrice: 90},
	}

	// Target hit at 122
	targetAlerts, err := svc.EvaluateTargetAlerts(positions, map[string]float64{"AAPL": 122})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targetAlerts) != 1 || targetAlerts[0].TargetPrice != 120 {
		t.Fatalf("expected target alert at 120, got %+v", targetAlerts)
	}

	// MA5 break: profit >= 10% (cost=100, price=115, profit=15%). MA5=118 -> breaks MA5
	ma5Alerts, err := svc.EvaluateMA5BreakAlerts(positions, map[string]float64{"AAPL": 115}, map[string]float64{"AAPL": 118})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ma5Alerts) != 1 {
		t.Fatalf("expected 1 MA5 break alert, got %+v", ma5Alerts)
	}
}

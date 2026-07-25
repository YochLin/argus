package web

import (
	"errors"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

func txWithStop(ticker, side string, shares, price float64, date string, stopPrice float64) db.Transaction {
	t := tx(ticker, side, shares, price, date)
	t.StopPrice = stopPrice
	return t
}

func TestBuildDistributions_RMultiple(t *testing.T) {
	sell := txWithStop("AAPL", "SELL", 10, 130, "2026-01-20", 90) // stop set below entry
	sell.RealizedPnL = 300                                        // (130-100)*10

	sellNoStop := txWithStop("MSFT", "SELL", 5, 320, "2026-02-01", 0) // no stop on record
	sellNoStop.RealizedPnL = 100

	fdb := &fakeDB{
		txs: []db.Transaction{
			txWithStop("AAPL", "BUY", 10, 100, "2026-01-05", 0),
			sell,
			txWithStop("MSFT", "BUY", 5, 300, "2026-01-10", 0),
			sellNoStop,
		},
	}
	fh := &fakeHistory{}

	got, err := buildDistributions(fdb, fh, market.US)
	if err != nil {
		t.Fatalf("buildDistributions() error = %v", err)
	}

	// AAPL: R = 300 / ((100-90)*10) = 3. MSFT has no stop -> NoStopCount.
	if len(got.RMultiples) != 1 || got.RMultiples[0].Ticker != "AAPL" {
		t.Fatalf("RMultiples = %+v, want one AAPL sample", got.RMultiples)
	}
	if r := got.RMultiples[0].R; r != 3 {
		t.Errorf("RMultiples[0].R = %v, want 3", r)
	}
	if got.NoStopCount != 1 {
		t.Errorf("NoStopCount = %d, want 1 (MSFT)", got.NoStopCount)
	}
	if got.EarliestRDate != "2026-01-05" {
		t.Errorf("EarliestRDate = %q, want 2026-01-05", got.EarliestRDate)
	}

	// Both rounds are closed, so both contribute a holding-return sample
	// regardless of whether R was computable.
	if len(got.HoldingReturns) != 2 {
		t.Errorf("HoldingReturns = %+v, want 2 samples", got.HoldingReturns)
	}
}

func TestBuildDistributions_StopAtOrAboveEntryExcluded(t *testing.T) {
	sell := txWithStop("AAPL", "SELL", 10, 130, "2026-01-20", 100) // stop == entry, invalid risk
	sell.RealizedPnL = 300

	fdb := &fakeDB{
		txs: []db.Transaction{
			txWithStop("AAPL", "BUY", 10, 100, "2026-01-05", 0),
			sell,
		},
	}
	got, err := buildDistributions(fdb, &fakeHistory{}, market.US)
	if err != nil {
		t.Fatalf("buildDistributions() error = %v", err)
	}
	if len(got.RMultiples) != 0 || got.NoStopCount != 1 {
		t.Errorf("got RMultiples=%+v NoStopCount=%d, want 0 samples / NoStopCount 1", got.RMultiples, got.NoStopCount)
	}
}

func TestBuildDistributions_MAEReturns(t *testing.T) {
	sell := txWithStop("AAPL", "SELL", 10, 130, "2026-01-20", 90)
	sell.RealizedPnL = 300

	fdb := &fakeDB{
		txs: []db.Transaction{
			txWithStop("AAPL", "BUY", 10, 100, "2026-01-05", 0),
			sell,
		},
	}
	fh := &fakeHistory{candles: map[string][]data.Candle{
		"AAPL": {
			lowHighCandle("2026-01-05", 95, 105),
			lowHighCandle("2026-01-10", 85, 110), // deepest adverse: (85-100)/100 = -15%
			lowHighCandle("2026-01-20", 125, 135),
		},
	}}

	got, err := buildDistributions(fdb, fh, market.US)
	if err != nil {
		t.Fatalf("buildDistributions() error = %v", err)
	}
	if len(got.MAEReturns) != 1 {
		t.Fatalf("MAEReturns = %+v, want 1 sample", got.MAEReturns)
	}
	if s := got.MAEReturns[0]; s.MAEPct != -15 {
		t.Errorf("MAEReturns[0].MAEPct = %v, want -15", s.MAEPct)
	}
}

func TestBuildDistributions_MissingHistorySkipsMAESample(t *testing.T) {
	sell := txWithStop("AAPL", "SELL", 10, 130, "2026-01-20", 90)
	sell.RealizedPnL = 300
	fdb := &fakeDB{
		txs: []db.Transaction{
			txWithStop("AAPL", "BUY", 10, 100, "2026-01-05", 0),
			sell,
		},
	}
	got, err := buildDistributions(fdb, &fakeHistory{err: errors.New("yahoo down")}, market.US)
	if err != nil {
		t.Fatalf("buildDistributions() error = %v", err)
	}
	if len(got.MAEReturns) != 0 || got.SkippedMAECount != 1 {
		t.Errorf("got MAEReturns=%+v SkippedMAECount=%d, want 0 samples / SkippedMAECount 1", got.MAEReturns, got.SkippedMAECount)
	}
}

func lowHighCandle(date string, low, high float64) data.Candle {
	c := candle(date, (low+high)/2)
	c.Low = low
	c.High = high
	return c
}

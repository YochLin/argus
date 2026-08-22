package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/signals"
)

// TestUniverseScanChunkFullCoverage verifies UniverseScanChunk rotates
// through every ticker exactly once over a full chunkCount-day cycle, with
// no gaps or duplicates — the property the daily scan job actually depends
// on for eventual full universe coverage.
func TestUniverseScanChunkFullCoverage(t *testing.T) {
	var tickers []string
	for i := 0; i < 503; i++ {
		tickers = append(tickers, fmt.Sprintf("T%03d", i))
	}

	const chunkCount = 1 // matches bot.scanChunkCount in production
	seen := make(map[string]int)
	for day := 0; day < chunkCount; day++ {
		for _, ticker := range UniverseScanChunk(tickers, chunkCount, day) {
			seen[ticker]++
		}
	}

	if len(seen) != len(tickers) {
		t.Fatalf("covered %d/%d tickers over a full cycle, want all of them", len(seen), len(tickers))
	}
	for ticker, n := range seen {
		if n != 1 {
			t.Errorf("ticker %s scanned %d times over a full cycle, want exactly 1", ticker, n)
		}
	}
}

func TestUniverseScanChunkEmptyAndNegativeDay(t *testing.T) {
	if got := UniverseScanChunk(nil, 1, 0); got != nil {
		t.Errorf("UniverseScanChunk(nil, ...) = %v, want nil", got)
	}
	tickers := []string{"A", "B", "C", "D", "E"}
	// A negative dayIndex must still resolve to a valid, in-range chunk
	// rather than panicking on a negative slice index.
	got := UniverseScanChunk(tickers, 1, -1)
	if len(got) == 0 {
		t.Errorf("UniverseScanChunk(..., -1) = %v, want a non-empty chunk", got)
	}
}

func TestScanServiceRevenueGrowthOK(t *testing.T) {
	tests := []struct {
		name         string
		fundamentals func(string) (*data.Fundamentals, error)
		ticker       string
		minPct       float64
		want         bool
	}{
		{"no fundamentals reader configured", nil, "AAPL", 5, false},
		{"fetch error fails closed", func(string) (*data.Fundamentals, error) { return nil, errors.New("boom") }, "AAPL", 5, false},
		{"US ticker uses RevenueGrowthYoY, above threshold", func(string) (*data.Fundamentals, error) {
			return &data.Fundamentals{RevenueGrowthYoY: 10}, nil
		}, "AAPL", 5, true},
		{"US ticker below threshold", func(string) (*data.Fundamentals, error) {
			return &data.Fundamentals{RevenueGrowthYoY: 3}, nil
		}, "AAPL", 5, false},
		{"TW ticker uses MonthRevenueYoYPct, above threshold", func(string) (*data.Fundamentals, error) {
			return &data.Fundamentals{MonthRevenueYoYPct: 10}, nil
		}, "2330", 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ScanService{fundamentals: tt.fundamentals}
			if got := s.revenueGrowthOK(tt.ticker, tt.minPct); got != tt.want {
				t.Errorf("revenueGrowthOK() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestScanServiceCheckStatefulSignalsRSIDedup exercises the signal_states
// persistence loop CheckStatefulSignals moved from bot.checkStatefulSignals
// (Phase 24 Stage 1 Scan & Strategy Service extraction): the same
// overbought RSI reading fires once and then goes silent until state
// changes, proving the store round-trip survived the move.
func TestScanServiceCheckStatefulSignalsRSIDedup(t *testing.T) {
	store := newMockRiskStore()
	s := NewScanService(store, signals.NewDetector(i18n.EN), nil, nil)

	candles := make([]data.Candle, 15)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range candles {
		candles[i] = data.Candle{Date: base.AddDate(0, 0, i), Close: 100 + float64(i)}
	}

	first := s.CheckStatefulSignals("AAPL", candles)
	if !hasSignalType(first, "rsi_overbought") {
		t.Fatalf("CheckStatefulSignals() first call = %v, want an rsi_overbought signal", first)
	}
	if got := store.signalStates["AAPL:"+signals.FamilyRSI]; got != signals.StateOverbought {
		t.Errorf("persisted RSI state = %q, want %q", got, signals.StateOverbought)
	}

	second := s.CheckStatefulSignals("AAPL", candles)
	if hasSignalType(second, "rsi_overbought") {
		t.Errorf("CheckStatefulSignals() second call = %v, want no repeat rsi_overbought signal", second)
	}
}

func hasSignalType(sigs []signals.Signal, typ string) bool {
	for _, s := range sigs {
		if s.Type == typ {
			return true
		}
	}
	return false
}

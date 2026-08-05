package web

import (
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

func TestBuildPaper_Empty(t *testing.T) {
	got, err := buildPaper(&fakeDB{}, &fakeQuotes{}, market.US, 100000, 1000000)
	if err != nil {
		t.Fatalf("buildPaper() error = %v", err)
	}
	if got.KPIs.InitialCash != 100000 {
		t.Errorf("InitialCash = %v, want 100000", got.KPIs.InitialCash)
	}
	if got.Positions == nil || len(got.Positions) != 0 {
		t.Errorf("Positions = %v, want empty non-nil slice", got.Positions)
	}
	if got.Closed == nil || len(got.Closed) != 0 {
		t.Errorf("Closed = %v, want empty non-nil slice", got.Closed)
	}
	if got.Curve == nil || len(got.Curve) != 0 {
		t.Errorf("Curve = %v, want empty non-nil slice", got.Curve)
	}
}

func TestBuildPaper_OpenPosition(t *testing.T) {
	f := &fakeDB{
		positions: []db.Position{{Ticker: "AAPL", Shares: 10, AvgCost: 150, StopPrice: 140}},
		txs: []db.Transaction{
			{Ticker: "AAPL", Side: "BUY", Shares: 10, Price: 150, Date: "2026-01-02", Market: "us"},
		},
	}
	q := &fakeQuotes{quotes: map[string]*data.Quote{"AAPL": {Ticker: "AAPL", Price: 160}}}

	got, err := buildPaper(f, q, market.US, 100000, 1000000)
	if err != nil {
		t.Fatalf("buildPaper() error = %v", err)
	}
	if len(got.Positions) != 1 {
		t.Fatalf("Positions = %v, want 1 entry", got.Positions)
	}
	p := got.Positions[0]
	if p.EntryDate != "2026-01-02" {
		t.Errorf("EntryDate = %q, want 2026-01-02", p.EntryDate)
	}
	if p.Price != 160 {
		t.Errorf("Price = %v, want 160", p.Price)
	}
	wantUnrealized := (160.0 - 150.0) * 10
	if p.UnrealizedPnL != wantUnrealized {
		t.Errorf("UnrealizedPnL = %v, want %v", p.UnrealizedPnL, wantUnrealized)
	}
	wantDistToStop := (160.0 - 140.0) / 160.0 * 100
	if p.DistToStopPct != wantDistToStop {
		t.Errorf("DistToStopPct = %v, want %v", p.DistToStopPct, wantDistToStop)
	}
	if got.KPIs.PositionsValue != 1600 {
		t.Errorf("PositionsValue = %v, want 1600", got.KPIs.PositionsValue)
	}
	if got.KPIs.Equity != 1600 { // fakeDB has no cash setting -> 0
		t.Errorf("Equity = %v, want 1600", got.KPIs.Equity)
	}
}

func TestBuildPaper_ClosedRounds_ExitReason(t *testing.T) {
	f := &fakeDB{
		txs: []db.Transaction{
			// Round 1: stop-loss exit — sell price at or below the stop snapshot.
			{Ticker: "AAA", Side: "BUY", Shares: 10, Price: 100, Date: "2026-01-02", Market: "us"},
			{Ticker: "AAA", Side: "SELL", Shares: 10, Price: 90, Date: "2026-01-05", RealizedPnL: -100, StopPrice: 90, Market: "us"},
			// Round 2: plain LLM sell — exit price above the stop snapshot.
			{Ticker: "BBB", Side: "BUY", Shares: 5, Price: 50, Date: "2026-01-03", Market: "us"},
			{Ticker: "BBB", Side: "SELL", Shares: 5, Price: 60, Date: "2026-01-10", RealizedPnL: 50, StopPrice: 45, Market: "us"},
		},
	}

	got, err := buildPaper(f, &fakeQuotes{}, market.US, 100000, 1000000)
	if err != nil {
		t.Fatalf("buildPaper() error = %v", err)
	}
	if len(got.Closed) != 2 {
		t.Fatalf("Closed = %v, want 2 entries", got.Closed)
	}
	// Sorted most-recent-exit first.
	bbb, aaa := got.Closed[0], got.Closed[1]
	if bbb.Ticker != "BBB" || bbb.ExitReason != "llm_sell" {
		t.Errorf("BBB round = %+v, want ticker BBB reason llm_sell", bbb)
	}
	if bbb.RealizedPnL != 50 || bbb.AvgCost != 50 || bbb.ExitPrice != 60 {
		t.Errorf("BBB round = %+v, want realizedPnL 50 avgCost 50 exitPrice 60", bbb)
	}
	if aaa.Ticker != "AAA" || aaa.ExitReason != "stop" {
		t.Errorf("AAA round = %+v, want ticker AAA reason stop", aaa)
	}
	if aaa.RealizedPnL != -100 {
		t.Errorf("AAA round realizedPnL = %v, want -100", aaa.RealizedPnL)
	}
}

func TestBuildPaper_MarketFilter(t *testing.T) {
	f := &fakeDB{
		positions: []db.Position{
			{Ticker: "AAPL", Shares: 10, AvgCost: 150},
			{Ticker: "2330", Shares: 100, AvgCost: 500},
		},
	}
	q := &fakeQuotes{quotes: map[string]*data.Quote{"2330": {Ticker: "2330", Price: 550}}}
	got, err := buildPaper(f, q, market.TW, 100000, 1000000)
	if err != nil {
		t.Fatalf("buildPaper() error = %v", err)
	}
	if got.KPIs.InitialCash != 1000000 {
		t.Errorf("InitialCash = %v, want 1000000 for TW", got.KPIs.InitialCash)
	}
	if len(got.Positions) != 1 || got.Positions[0].Ticker != "2330" {
		t.Errorf("Positions = %v, want only 2330", got.Positions)
	}
}

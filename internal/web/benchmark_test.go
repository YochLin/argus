package web

import (
	"testing"

	"argus/internal/db"
)

// TestBenchmarkReplay pins all four documented behaviors
// (docs/phase-8-trader-analytics.md §4.2) in one fixture: a trade dated
// before the benchmark's own history begins is excluded from investment but
// still counted toward the real-share balance a later partial sell's
// proportion depends on; a BUY invests shares*price+fee at that day's
// benchmark close; a SELL liquidates the matching proportion of the
// ticker's benchmark holding; and a transaction dated a day with no
// benchmark close (2024-01-03) carries forward the most recent prior close
// for both its own pricing and the output date axis (which skips straight
// from 01-02 to 01-04).
func TestBenchmarkReplay(t *testing.T) {
	txs := []db.Transaction{
		tx("AAPL", "BUY", 10, 50, "2024-01-01"),  // before benchmark history — excluded from investment
		tx("AAPL", "BUY", 10, 60, "2024-01-02"),  // fee added below
		tx("AAPL", "SELL", 10, 65, "2024-01-03"), // gap day — no benchmark close
		tx("AAPL", "BUY", 5, 70, "2024-01-05"),
	}
	txs[1].Fee = 1

	benchCloses := map[string]float64{
		"2024-01-02": 100,
		"2024-01-04": 120,
		"2024-01-05": 120,
	}

	got := BenchmarkReplay(txs, benchCloses)

	want := []DateValue{
		{Date: "2024-01-02", Value: 0},
		{Date: "2024-01-04", Value: 60.1},
		{Date: "2024-01-05", Value: 60.1},
	}
	if len(got) != len(want) {
		t.Fatalf("BenchmarkReplay() = %+v, want len %d", got, len(want))
	}
	for i, w := range want {
		if got[i].Date != w.Date || !approxEqual(got[i].Value, w.Value) {
			t.Errorf("BenchmarkReplay()[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestBenchmarkReplay_NoBenchmarkData(t *testing.T) {
	txs := []db.Transaction{tx("AAPL", "BUY", 10, 50, "2024-01-01")}
	if got := BenchmarkReplay(txs, nil); got != nil {
		t.Errorf("BenchmarkReplay() with no benchmark closes = %v, want nil", got)
	}
}

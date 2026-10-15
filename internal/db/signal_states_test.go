package db

import "testing"

func TestSignalStateRoundTrip(t *testing.T) {
	d := newTestDB(t)

	state, err := d.GetSignalState("AAPL", "macd")
	if err != nil || state != "" {
		t.Fatalf("GetSignalState() before any set = %q, %v; want \"\", nil", state, err)
	}

	if err := d.SetSignalState("AAPL", "macd", "bullish"); err != nil {
		t.Fatalf("SetSignalState() error = %v", err)
	}
	if state, _ := d.GetSignalState("AAPL", "macd"); state != "bullish" {
		t.Errorf("GetSignalState() = %q, want bullish", state)
	}

	// Upsert replaces the previous state for the same (ticker, signal).
	if err := d.SetSignalState("AAPL", "macd", "bearish"); err != nil {
		t.Fatalf("SetSignalState() (upsert) error = %v", err)
	}
	if state, _ := d.GetSignalState("AAPL", "macd"); state != "bearish" {
		t.Errorf("GetSignalState() after upsert = %q, want bearish", state)
	}

	// Different signal family on the same ticker is independent.
	if state, _ := d.GetSignalState("AAPL", "rsi"); state != "" {
		t.Errorf("GetSignalState(rsi) = %q, want \"\" (unset)", state)
	}
}

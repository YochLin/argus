package data

import "testing"

func TestTickerLabel(t *testing.T) {
	if got := TickerLabel("2330", "台積電"); got != "台積電(2330)" {
		t.Errorf("TickerLabel(2330, 台積電) = %q, want 台積電(2330)", got)
	}
	if got := TickerLabel("AAPL", ""); got != "AAPL" {
		t.Errorf("TickerLabel(AAPL, \"\") = %q, want AAPL unchanged", got)
	}
}

package bot

import (
	"testing"

	"argus/internal/db"
)

func TestPositionsSlice(t *testing.T) {
	positions := map[string]db.Position{
		"MSFT": {Ticker: "MSFT", Shares: 1, AvgCost: 400},
		"AAPL": {Ticker: "AAPL", Shares: 2, AvgCost: 200},
	}
	got := positionsSlice(positions)
	if len(got) != 2 || got[0].Ticker != "AAPL" || got[1].Ticker != "MSFT" {
		t.Errorf("positionsSlice() = %+v, want [AAPL, MSFT] order", got)
	}

	if got := positionsSlice(nil); len(got) != 0 {
		t.Errorf("positionsSlice(nil) = %+v, want empty", got)
	}
}

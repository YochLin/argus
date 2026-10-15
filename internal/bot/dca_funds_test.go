package bot

import (
	"testing"

	"argus/internal/sinopac"
)

func TestDCASnapshot(t *testing.T) {
	p := sinopac.Position{Code: "0050", Quantity: 1000, Price: 150}

	// 200 of the broker's 1000 shares were bought by hand and already count
	// in the trading account.
	s, ok := dcaSnapshot(7, "2026-10-15", p, 200, 190)
	if !ok || s.AssetID != 7 || s.Value != 800*190 || s.Cost == nil || *s.Cost != 800*150 || s.Source != "sync" {
		t.Fatalf("got %+v ok=%v", s, ok)
	}

	if _, ok := dcaSnapshot(7, "2026-10-15", p, 1000, 190); ok {
		t.Fatal("nothing left over after the traded shares should write nothing")
	}
}

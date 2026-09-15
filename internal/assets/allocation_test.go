package assets

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestComputeDriftPercentagesAndDeviation(t *testing.T) {
	byGroup := map[string]float64{"liquid": 200, "growth": 500, "income": 200, "hard": 100}
	rows := ComputeDrift(byGroup, 1000, ModelBalanced)
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}

	byG := map[string]DriftRow{}
	for _, r := range rows {
		byG[r.Group] = r
	}

	liquid := byG["liquid"]
	if !approxEqual(liquid.CurrentPct, 20) {
		t.Errorf("liquid.CurrentPct = %v, want 20", liquid.CurrentPct)
	}
	if liquid.TargetPct != ModelPresets[ModelBalanced]["liquid"] {
		t.Errorf("liquid.TargetPct = %v, want %v", liquid.TargetPct, ModelPresets[ModelBalanced]["liquid"])
	}
	wantDeviation := liquid.CurrentPct - liquid.TargetPct
	if !approxEqual(liquid.DeviationPt, wantDeviation) {
		t.Errorf("liquid.DeviationPt = %v, want %v", liquid.DeviationPt, wantDeviation)
	}

	growth := byG["growth"]
	if !approxEqual(growth.CurrentPct, 50) || growth.MarketValue != 500 {
		t.Errorf("growth = %+v, want CurrentPct 50, MarketValue 500", growth)
	}
}

func TestComputeDriftZeroTotalReturnsNil(t *testing.T) {
	if rows := ComputeDrift(map[string]float64{"liquid": 100}, 0, ModelBalanced); rows != nil {
		t.Errorf("ComputeDrift(total=0) = %v, want nil", rows)
	}
}

// TestComputeRebalanceOrdersSkipsLockedAndSmallDrift exercises the three
// rules that matter: locked ("hard") never orders no matter how far off,
// drift under RebalanceThresholdPt never orders, and everything else nets
// out to a buy/sell amount that would close the gap to target.
func TestComputeRebalanceOrdersSkipsLockedAndSmallDrift(t *testing.T) {
	byGroup := map[string]float64{"liquid": 200, "growth": 500, "income": 200, "hard": 100}
	rows := ComputeDrift(byGroup, 1000, ModelBalanced)
	orders := ComputeRebalanceOrders(rows, 1000)
	if len(orders) != 1 {
		t.Fatalf("len(orders) = %d, want 1 (only growth clears the threshold and isn't locked): %+v", len(orders), orders)
	}
	o := orders[0]
	if o.Group != "growth" || o.Side != "sell" || !approxEqual(o.Amount, 150) {
		t.Errorf("orders[0] = %+v, want {growth sell 150 ...}", o)
	}
}

func TestComputeRebalanceOrdersZeroTotalReturnsNil(t *testing.T) {
	rows := ComputeDrift(map[string]float64{"liquid": 100}, 100, ModelBalanced)
	if orders := ComputeRebalanceOrders(rows, 0); orders != nil {
		t.Errorf("ComputeRebalanceOrders(total=0) = %v, want nil", orders)
	}
}

// TestModelPresetsSumToRoughly100 pins the honesty disclaimer in
// ModelPresets' doc comment: each preset's four buckets should still add up
// to (approximately) the whole portfolio, or the allocation table's
// percentages silently stop meaning anything.
func TestModelPresetsSumToRoughly100(t *testing.T) {
	for model, preset := range ModelPresets {
		var sum float64
		for _, g := range AssetGroups {
			sum += preset[g]
		}
		if sum < 99 || sum > 101 {
			t.Errorf("ModelPresets[%q] sums to %v, want ~100", model, sum)
		}
	}
}

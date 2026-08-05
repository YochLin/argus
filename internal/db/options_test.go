package db

import (
	"errors"
	"math"
	"testing"
)

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

const testCall = "AAPL260918C00320000"

func TestRecordOption_LongRoundTrip(t *testing.T) {
	d := newTestDB(t)

	pos, realized, err := d.RecordOption(testCall, "BUY", 2, 5.40, 0, "2026-08-01")
	if err != nil {
		t.Fatalf("open BUY: %v", err)
	}
	if pos.Contracts != 2 || pos.AvgPremium != 5.40 || realized != 0 {
		t.Fatalf("after open: %+v realized=%v", pos, realized)
	}
	if pos.Underlying != "AAPL" || pos.Right != "C" || pos.Strike != 320 || pos.Expiry != "2026-09-18" {
		t.Fatalf("derived fields wrong: %+v", pos)
	}

	// Partial close: sell 1 of 2 @ 7.20 -> realized (7.20-5.40)*1*100 = 180.
	pos, realized, err = d.RecordOption(testCall, "SELL", 1, 7.20, 0, "2026-08-05")
	if err != nil {
		t.Fatalf("partial close SELL: %v", err)
	}
	if pos.Contracts != 1 {
		t.Fatalf("after partial close: contracts = %v, want 1", pos.Contracts)
	}
	if !approxEqual(realized, 180) {
		t.Fatalf("realized = %v, want 180", realized)
	}

	// Full close of the remaining 1.
	pos, realized, err = d.RecordOption(testCall, "SELL", 1, 6.00, 1, "2026-08-06")
	if err != nil {
		t.Fatalf("full close SELL: %v", err)
	}
	if pos.Contracts != 0 {
		t.Fatalf("after full close: contracts = %v, want 0", pos.Contracts)
	}
	wantRealized := (6.00-5.40)*1*100 - 1
	if !approxEqual(realized, wantRealized) {
		t.Fatalf("realized = %v, want %v", realized, wantRealized)
	}
	if _, ok, err := d.GetOptionPosition(testCall); err != nil || ok {
		t.Fatalf("GetOptionPosition after full close: ok=%v err=%v, want ok=false", ok, err)
	}
}

func TestRecordOption_ShortRoundTrip(t *testing.T) {
	d := newTestDB(t)

	// Sell to open (e.g. a covered call) at 5.40, buy to close at 3.00 ->
	// profit (entry-exit) since it's a credit position.
	pos, realized, err := d.RecordOption(testCall, "SELL", 1, 5.40, 0, "2026-08-01")
	if err != nil {
		t.Fatalf("open SELL: %v", err)
	}
	if pos.Contracts != -1 {
		t.Fatalf("after open SELL: contracts = %v, want -1", pos.Contracts)
	}

	pos, realized, err = d.RecordOption(testCall, "BUY", 1, 3.00, 0, "2026-08-10")
	if err != nil {
		t.Fatalf("close BUY: %v", err)
	}
	if pos.Contracts != 0 {
		t.Fatalf("after close: contracts = %v, want 0", pos.Contracts)
	}
	wantRealized := (5.40 - 3.00) * 1 * 100
	if !approxEqual(realized, wantRealized) {
		t.Fatalf("realized = %v, want %v (short profits when bought back cheaper)", realized, wantRealized)
	}
}

func TestRecordOption_CrossesZeroRejected(t *testing.T) {
	d := newTestDB(t)

	if _, _, err := d.RecordOption(testCall, "BUY", 2, 5.40, 0, "2026-08-01"); err != nil {
		t.Fatalf("open: %v", err)
	}
	_, _, err := d.RecordOption(testCall, "SELL", 3, 5.00, 0, "2026-08-02")
	if !errors.Is(err, ErrCrossesZero) {
		t.Fatalf("selling 3 against a 2-long position: err = %v, want ErrCrossesZero", err)
	}
	// Position must be untouched by the rejected attempt.
	pos, ok, err := d.GetOptionPosition(testCall)
	if err != nil || !ok || pos.Contracts != 2 {
		t.Fatalf("position after rejected cross: %+v ok=%v err=%v, want untouched 2 contracts", pos, ok, err)
	}
}

func TestRecordOption_InvalidSymbol(t *testing.T) {
	d := newTestDB(t)
	if _, _, err := d.RecordOption("NOTANOPTION", "BUY", 1, 1, 0, "2026-08-01"); err == nil {
		t.Fatal("expected error for invalid OCC symbol")
	}
}

func TestSetOptionStop_NoPosition(t *testing.T) {
	d := newTestDB(t)
	if err := d.SetOptionStop(testCall, 1.0); !errors.Is(err, ErrNoPosition) {
		t.Fatalf("SetOptionStop with no position: err = %v, want ErrNoPosition", err)
	}
}

func TestGetOptionPositionsByUnderlying(t *testing.T) {
	d := newTestDB(t)
	if _, _, err := d.RecordOption(testCall, "BUY", 1, 5.40, 0, "2026-08-01"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, _, err := d.RecordOption("MSFT260918C00400000", "BUY", 1, 3.0, 0, "2026-08-01"); err != nil {
		t.Fatalf("open second: %v", err)
	}

	positions, err := d.GetOptionPositionsByUnderlying("AAPL")
	if err != nil {
		t.Fatalf("GetOptionPositionsByUnderlying: %v", err)
	}
	if len(positions) != 1 || positions[0].Underlying != "AAPL" {
		t.Fatalf("positions = %+v, want exactly one AAPL contract", positions)
	}
}

func TestSaveATMIV(t *testing.T) {
	d := newTestDB(t)
	if err := d.SaveATMIV("AAPL", "2026-08-05", 0.34, 44); err != nil {
		t.Fatalf("SaveATMIV: %v", err)
	}
	// Overwrite same day should not error (INSERT OR REPLACE).
	if err := d.SaveATMIV("AAPL", "2026-08-05", 0.35, 43); err != nil {
		t.Fatalf("SaveATMIV overwrite: %v", err)
	}
}

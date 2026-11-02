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

func TestEditOptionPosition_ChangesContractAndKeepsStop(t *testing.T) {
	d := newTestDB(t)
	if _, _, err := d.RecordOption(testCall, "BUY", 1, 5, 0, "2026-08-01"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetOptionStop(testCall, 2); err != nil {
		t.Fatal(err)
	}
	const fixed = "AAPL260918P00300000"
	if _, err := d.EditOptionPosition(testCall, fixed, "SELL", 3, 4.5); err != nil {
		t.Fatalf("EditOptionPosition: %v", err)
	}
	if _, ok, _ := d.GetOptionPosition(testCall); ok {
		t.Error("old symbol still held")
	}
	p, ok, err := d.GetOptionPosition(fixed)
	if err != nil || !ok {
		t.Fatalf("GetOptionPosition(fixed): ok=%v err=%v", ok, err)
	}
	if p.Contracts != -3 || p.AvgPremium != 4.5 || p.Right != "P" || p.Strike != 300 || p.StopPremium != 2 || !p.Edited {
		t.Errorf("position = %+v", p)
	}
	txs, err := d.GetAllOptionTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 1 || txs[0].ContractSymbol != fixed || txs[0].Action != OptionActionSellToOpen {
		t.Errorf("ledger = %+v, want the open moved and flipped to STO", txs)
	}
}

func TestEditOptionPosition_RejectsHeldContract(t *testing.T) {
	d := newTestDB(t)
	const other = "AAPL260918C00330000"
	d.RecordOption(testCall, "BUY", 1, 5, 0, "2026-08-01")
	d.RecordOption(other, "BUY", 1, 5, 0, "2026-08-01")
	if _, err := d.EditOptionPosition(testCall, other, "BUY", 1, 5); !errors.Is(err, ErrOptionSymbolTaken) {
		t.Fatalf("err = %v, want ErrOptionSymbolTaken", err)
	}
}

func TestDeleteOptionPosition_KeepsEarlierCloses(t *testing.T) {
	d := newTestDB(t)
	d.RecordOption(testCall, "BUY", 1, 5, 0, "2026-08-01")
	d.RecordOption(testCall, "SELL", 1, 6, 0, "2026-08-02")
	d.RecordOption(testCall, "BUY", 1, 7, 0, "2026-08-03")
	if err := d.DeleteOptionPosition(testCall); err != nil {
		t.Fatalf("DeleteOptionPosition: %v", err)
	}
	if _, ok, _ := d.GetOptionPosition(testCall); ok {
		t.Error("position still held")
	}
	txs, _ := d.GetAllOptionTransactions()
	if len(txs) != 2 || txs[0].Action != OptionActionSellToClose {
		t.Errorf("ledger = %+v, want the first round only", txs)
	}
}

func TestEditOptionClosed(t *testing.T) {
	d := newTestDB(t)
	d.RecordOption(testCall, "SELL", 2, 5, 0, "2026-08-01")
	d.ResolveOption(testCall, OptionActionExpired, "2026-09-18")
	id := func() int64 { txs, _ := d.GetAllOptionTransactions(); return txs[0].ID }()

	if err := d.EditOptionClosed(id, OptionActionClosed, 1, "2026-09-01", -320); err != nil {
		t.Fatalf("EditOptionClosed: %v", err)
	}
	txs, _ := d.GetAllOptionTransactions()
	got := txs[0]
	if got.Action != OptionActionBuyToClose || got.Contracts != 1 || got.Date != "2026-09-01" || got.RealizedPnL != -320 || !got.Edited {
		t.Errorf("row = %+v, want BTC (the open was short) with the typed values", got)
	}
	if err := d.EditOptionClosed(id, OptionActionAssigned, 1, "2026-09-01", 0); !errors.Is(err, ErrOptionStockLinked) {
		t.Errorf("into ASSIGNED: err = %v, want ErrOptionStockLinked", err)
	}
	if err := d.EditOptionClosed(txs[1].ID, OptionActionExpired, 1, "2026-09-01", 0); !errors.Is(err, ErrTransactionNotFound) {
		t.Errorf("an open row: err = %v, want ErrTransactionNotFound", err)
	}
}

func TestUndoOptionClose(t *testing.T) {
	cases := []struct {
		name        string
		close       func(d *DB)
		wantLeft    float64
		wantContr   float64
		wantPremium float64
	}{
		{"full close of a long", func(d *DB) { d.RecordOption(testCall, "SELL", 2, 7, 1, "2026-08-02") }, 0, 2, 5.5},
		{"partial close", func(d *DB) { d.RecordOption(testCall, "SELL", 1, 7, 0, "2026-08-02") }, 1, 2, 5.5},
		{"expired", func(d *DB) { d.ResolveOption(testCall, OptionActionExpired, "2026-09-18") }, 0, 2, 5.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newTestDB(t)
			d.RecordOption(testCall, "BUY", 1, 5, 0, "2026-08-01")
			d.RecordOption(testCall, "BUY", 1, 6, 0, "2026-08-01")
			c.close(d)
			txs, _ := d.GetAllOptionTransactions()
			if _, err := d.UndoOptionClose(txs[0].ID); err != nil {
				t.Fatalf("UndoOptionClose: %v", err)
			}
			p, ok, err := d.GetOptionPosition(testCall)
			if err != nil || !ok {
				t.Fatalf("GetOptionPosition: ok=%v err=%v", ok, err)
			}
			if p.Contracts != c.wantContr || !approxEqual(p.AvgPremium, c.wantPremium) {
				t.Errorf("position = %v @ %v, want %v @ %v", p.Contracts, p.AvgPremium, c.wantContr, c.wantPremium)
			}
			if after, _ := d.GetAllOptionTransactions(); len(after) != len(txs)-1 {
				t.Errorf("ledger has %d rows, want the close gone", len(after))
			}
		})
	}
}

func TestUndoOptionClose_ShortExpired(t *testing.T) {
	d := newTestDB(t)
	d.RecordOption(testCall, "SELL", 1, 5, 0, "2026-08-01")
	d.ResolveOption(testCall, OptionActionExpired, "2026-09-18")
	txs, _ := d.GetAllOptionTransactions()
	p, err := d.UndoOptionClose(txs[0].ID)
	if err != nil || p.Contracts != -1 || !approxEqual(p.AvgPremium, 5) {
		t.Fatalf("UndoOptionClose = %+v, %v; want -1 @ 5", p, err)
	}
}

func TestUndoOptionClose_Refusals(t *testing.T) {
	d := newTestDB(t)
	d.RecordOption(testCall, "SELL", 1, 5, 0, "2026-08-01")
	d.ResolveOption(testCall, OptionActionAssigned, "2026-09-18")
	txs, _ := d.GetAllOptionTransactions()
	if _, err := d.UndoOptionClose(txs[0].ID); !errors.Is(err, ErrOptionStockLinked) {
		t.Errorf("assigned: err = %v, want ErrOptionStockLinked", err)
	}

	d = newTestDB(t)
	d.RecordOption(testCall, "BUY", 1, 5, 0, "2026-08-01")
	d.RecordOption(testCall, "SELL", 1, 6, 0, "2026-08-02")
	d.RecordOption(testCall, "SELL", 1, 6, 0, "2026-08-03") // reopened short
	txs, _ = d.GetAllOptionTransactions()
	if _, err := d.UndoOptionClose(txs[1].ID); !errors.Is(err, ErrCrossesZero) {
		t.Errorf("reopened other side: err = %v, want ErrCrossesZero", err)
	}
}

// Migration 42's backfill, run on rows as they looked before it.
func TestMigrationBackfillsEntryPremium(t *testing.T) {
	d := newTestDB(t)
	d.RecordOption(testCall, "BUY", 2, 5, 0, "2026-08-01")
	d.RecordOption(testCall, "SELL", 1, 7, 1, "2026-08-02")
	d.ResolveOption(testCall, OptionActionExpired, "2026-09-18")
	if _, err := d.conn.Exec(`UPDATE option_transactions SET entry_premium = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`UPDATE option_transactions SET entry_premium = CASE action
		WHEN 'STC' THEN premium - (realized_pnl + fee) / (contracts * multiplier)
		WHEN 'BTC' THEN premium + (realized_pnl + fee) / (contracts * multiplier)
		ELSE ABS(realized_pnl) / (contracts * multiplier)
	END WHERE action NOT IN ('BTO', 'STO') AND contracts > 0`); err != nil {
		t.Fatal(err)
	}
	rows, err := d.conn.Query(`SELECT entry_premium FROM option_transactions WHERE action NOT IN ('BTO','STO')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var got float64
		if err := rows.Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !approxEqual(got, 5) {
			t.Errorf("entry_premium = %v, want 5", got)
		}
	}
}

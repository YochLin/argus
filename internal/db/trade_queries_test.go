package db

import "testing"

func TestGetPositionsOrdering(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("MSFT", 1, 400, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 1, 200, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	got, err := d.GetPositions()
	if err != nil {
		t.Fatalf("GetPositions() error = %v", err)
	}
	if len(got) != 2 || got[0].Ticker != "AAPL" || got[1].Ticker != "MSFT" {
		t.Errorf("GetPositions() = %+v, want [AAPL, MSFT]", got)
	}
}

func TestGetTransactionStats(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-06-15"); err != nil { // outside range
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 5, 120, 0, "2026-07-10"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	t.Run("month with a buy and a sell", func(t *testing.T) {
		count, sellCount, realized, err := d.GetTransactionStats("2026-07-01", "2026-07-31")
		if err != nil {
			t.Fatalf("GetTransactionStats() error = %v", err)
		}
		wantRealized := (120.0 - 100.0) * 5
		if count != 2 || sellCount != 1 || realized != wantRealized {
			t.Errorf("GetTransactionStats() = %d, %d, %v; want 2, 1, %v", count, sellCount, realized, wantRealized)
		}
	})

	t.Run("month with no transactions", func(t *testing.T) {
		count, sellCount, realized, err := d.GetTransactionStats("2026-09-01", "2026-09-30")
		if err != nil {
			t.Fatalf("GetTransactionStats() error = %v", err)
		}
		if count != 0 || sellCount != 0 || realized != 0 {
			t.Errorf("GetTransactionStats() = %d, %d, %v; want 0, 0, 0", count, sellCount, realized)
		}
	})
}

func TestGetEarliestBuyDate(t *testing.T) {
	d := newTestDB(t)

	if _, ok, err := d.GetEarliestBuyDate("AAPL"); err != nil || ok {
		t.Fatalf("GetEarliestBuyDate() before any buy: ok = %v, err = %v; want false, nil", ok, err)
	}

	if _, err := d.RecordBuy("AAPL", 5, 200, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 5, 210, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	date, ok, err := d.GetEarliestBuyDate("AAPL")
	if err != nil || !ok || date != "2026-07-01" {
		t.Errorf("GetEarliestBuyDate() = %q, %v, %v; want 2026-07-01, true, nil", date, ok, err)
	}
}

func TestGetTransactions(t *testing.T) {
	d := newTestDB(t)

	if txs, err := d.GetTransactions("AAPL"); err != nil || len(txs) != 0 {
		t.Fatalf("GetTransactions() before any trade: txs = %v, err = %v; want empty, nil", txs, err)
	}

	if _, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 4, 220, 0.5, "2026-07-10"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	// A different ticker's trades must not leak in.
	if _, err := d.RecordBuy("MSFT", 5, 300, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	txs, err := d.GetTransactions("AAPL")
	if err != nil {
		t.Fatalf("GetTransactions() error = %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("GetTransactions() len = %d, want 2", len(txs))
	}
	if txs[0].Side != "BUY" || txs[0].Shares != 10 || txs[0].Price != 200 {
		t.Errorf("GetTransactions()[0] = %+v, want the BUY leg first (oldest date)", txs[0])
	}
	if txs[1].Side != "SELL" || txs[1].Shares != 4 || txs[1].Price != 220 {
		t.Errorf("GetTransactions()[1] = %+v, want the SELL leg second", txs[1])
	}
}

func TestGetAllTransactions(t *testing.T) {
	d := newTestDB(t)

	if txs, err := d.GetAllTransactions(); err != nil || len(txs) != 0 {
		t.Fatalf("GetAllTransactions() before any trade: txs = %v, err = %v; want empty, nil", txs, err)
	}

	if _, err := d.RecordBuy("MSFT", 5, 300, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 4, 220, 0.5, "2026-07-10"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	txs, err := d.GetAllTransactions()
	if err != nil {
		t.Fatalf("GetAllTransactions() error = %v", err)
	}
	if len(txs) != 3 {
		t.Fatalf("GetAllTransactions() len = %d, want 3 (across every ticker)", len(txs))
	}
	// Ordered by date across tickers, not grouped by ticker: AAPL BUY
	// (07-01) before MSFT BUY (07-05) before AAPL SELL (07-10).
	if txs[0].Ticker != "AAPL" || txs[0].Side != "BUY" || txs[0].Date != "2026-07-01" {
		t.Errorf("GetAllTransactions()[0] = %+v, want AAPL BUY 2026-07-01 (oldest date first)", txs[0])
	}
	if txs[1].Ticker != "MSFT" || txs[1].Date != "2026-07-05" {
		t.Errorf("GetAllTransactions()[1] = %+v, want MSFT 2026-07-05", txs[1])
	}
	if txs[2].Ticker != "AAPL" || txs[2].Side != "SELL" || txs[2].Date != "2026-07-10" {
		t.Errorf("GetAllTransactions()[2] = %+v, want AAPL SELL 2026-07-10", txs[2])
	}
}

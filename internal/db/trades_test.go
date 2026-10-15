package db

import (
	"errors"
	"math"
	"testing"

	"argus/internal/market"
)

func TestRecordBuyWeightedAverageCost(t *testing.T) {
	d := newTestDB(t)

	pos, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-07-01")
	if err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	// (10*200 + 1) / 10 = 200.1
	if pos.Shares != 10 || pos.AvgCost != 200.1 {
		t.Errorf("RecordBuy() = %+v, want Shares=10 AvgCost=200.1", pos)
	}

	// A second buy folds into a weighted average: (10*200.1 + 10*220) / 20 = 210.05
	pos, err = d.RecordBuy("AAPL", 10, 220, 0, "2026-07-02")
	if err != nil {
		t.Fatalf("RecordBuy() (second) error = %v", err)
	}
	if pos.Shares != 20 || pos.AvgCost != 210.05 {
		t.Errorf("RecordBuy() (second) = %+v, want Shares=20 AvgCost=210.05", pos)
	}

	got, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok {
		t.Fatalf("GetPosition() = %+v, %v, %v", got, ok, err)
	}
	if got.Shares != 20 || got.AvgCost != 210.05 {
		t.Errorf("GetPosition() = %+v, want Shares=20 AvgCost=210.05", got)
	}
}

func TestRecordSellRealizedPnLAndPartialClose(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	// Sell 4 @ 220 with $1 fee: realized = (220-200)*4 - 1 = 79
	pos, pnl, err := d.RecordSell("AAPL", 4, 220, 1, "2026-07-05")
	if err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	if pnl != 79 {
		t.Errorf("RecordSell() realizedPnL = %v, want 79", pnl)
	}
	if pos.Shares != 6 || pos.AvgCost != 200 {
		t.Errorf("RecordSell() remaining position = %+v, want Shares=6 AvgCost=200", pos)
	}

	total, err := d.GetRealizedPnL(market.US)
	if err != nil || total != 79 {
		t.Errorf("GetRealizedPnL() = %v, %v; want 79, nil", total, err)
	}

	// Selling the rest closes the position out entirely.
	pos, _, err = d.RecordSell("AAPL", 6, 210, 0, "2026-07-06")
	if err != nil {
		t.Fatalf("RecordSell() (final) error = %v", err)
	}
	if pos.Shares != 0 {
		t.Errorf("RecordSell() (final) remaining shares = %v, want 0", pos.Shares)
	}
	if _, ok, err := d.GetPosition("AAPL"); err != nil || ok {
		t.Errorf("GetPosition() after full close: ok = %v, err = %v; want false, nil", ok, err)
	}
}

func TestRecordSellErrors(t *testing.T) {
	d := newTestDB(t)

	if _, _, err := d.RecordSell("AAPL", 1, 100, 0, "2026-07-01"); !errors.Is(err, ErrNoPosition) {
		t.Errorf("RecordSell() with no position error = %v, want ErrNoPosition", err)
	}

	if _, err := d.RecordBuy("AAPL", 5, 100, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 10, 100, 0, "2026-07-02"); !errors.Is(err, ErrInsufficientShares) {
		t.Errorf("RecordSell() oversized error = %v, want ErrInsufficientShares", err)
	}
}

// TestRecordSellFIFO pins the FIFO-vs-weighted-average divergence this was
// added to fix: a partial sell must consume the *oldest* open lot first
// (here, lot1 @ 100), not the blended average (125) across both lots. Before
// this change realizedPnL would have been (200-125)*5=375 and the remaining
// position's AvgCost would have stayed 125 — both wrong against a real
// broker's FIFO accounting.
func TestRecordSellFIFO(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() (lot1) error = %v", err)
	}
	if _, err := d.RecordBuy("AAPL", 10, 150, 0, "2026-07-02"); err != nil {
		t.Fatalf("RecordBuy() (lot2) error = %v", err)
	}

	// Sell 5 @ 200: FIFO takes all 5 from lot1 (cost 100/share).
	// realized = 5*200 - 5*100 - 0 = 500.
	pos, pnl, err := d.RecordSell("AAPL", 5, 200, 0, "2026-07-03")
	if err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	if pnl != 500 {
		t.Errorf("RecordSell() realizedPnL = %v, want 500 (FIFO off lot1 @ 100, not the 125 blended average)", pnl)
	}
	// Remaining: 5 shares left of lot1 @ 100, 10 shares of lot2 @ 150.
	// avg_cost = (5*100 + 10*150) / 15 = 133.333...
	wantAvgCost := (5*100.0 + 10*150.0) / 15
	if pos.Shares != 15 || math.Abs(pos.AvgCost-wantAvgCost) > 1e-9 {
		t.Errorf("RecordSell() remaining position = %+v, want Shares=15 AvgCost=%v", pos, wantAvgCost)
	}

	got, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok {
		t.Fatalf("GetPosition() = %+v, %v, %v", got, ok, err)
	}
	if got.Shares != 15 || math.Abs(got.AvgCost-wantAvgCost) > 1e-9 {
		t.Errorf("GetPosition() = %+v, want Shares=15 AvgCost=%v", got, wantAvgCost)
	}
}

// TestDeleteTransaction covers the typo-recovery path a Telegram /undo
// command and the dashboard's per-row delete button both rely on: only the
// most recent transaction for a ticker may be deleted, a deleted SELL must
// restore exactly the lots it FIFO-consumed, and a deleted BUY that was the
// ticker's only transaction must remove the position entirely.
func TestDeleteTransaction(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("AAPL", 10, 100, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() (lot1) error = %v", err)
	}
	buy2, err := d.RecordBuy("AAPL", 10, 150, 0, "2026-07-02")
	if err != nil {
		t.Fatalf("RecordBuy() (lot2) error = %v", err)
	}
	if buy2.Shares != 20 {
		t.Fatalf("position after two buys = %+v, want Shares=20", buy2)
	}

	// Deleting an older transaction (lot1, no longer the latest) is refused.
	txs, err := d.GetTransactions("AAPL")
	if err != nil || len(txs) != 2 {
		t.Fatalf("GetTransactions() = %v, %v; want 2 rows", txs, err)
	}
	if _, err := d.DeleteTransaction(txs[0].ID); !errors.Is(err, ErrNotLatestTransaction) {
		t.Errorf("DeleteTransaction(lot1) error = %v, want ErrNotLatestTransaction", err)
	}

	// Deleting a nonexistent id is refused.
	if _, err := d.DeleteTransaction(999999); !errors.Is(err, ErrTransactionNotFound) {
		t.Errorf("DeleteTransaction(missing) error = %v, want ErrTransactionNotFound", err)
	}

	// Sell 5 (FIFO from lot1, leaving lot1 @ 5 remaining, lot2 untouched @ 10).
	if _, _, err := d.RecordSell("AAPL", 5, 200, 0, "2026-07-03"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	sellTxs, err := d.GetTransactions("AAPL")
	if err != nil {
		t.Fatalf("GetTransactions() error = %v", err)
	}
	sellID := sellTxs[len(sellTxs)-1].ID

	// Undoing the SELL restores lot1 to fully open again.
	deletedSell, err := d.DeleteTransaction(sellID)
	if err != nil {
		t.Fatalf("DeleteTransaction(sell) error = %v", err)
	}
	if deletedSell.Side != "SELL" || deletedSell.Shares != 5 {
		t.Errorf("DeleteTransaction(sell) returned = %+v, want Side=SELL Shares=5", deletedSell)
	}
	pos, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok {
		t.Fatalf("GetPosition() after undoing sell = %+v, %v, %v", pos, ok, err)
	}
	wantAvgCost := (10*100.0 + 10*150.0) / 20
	if pos.Shares != 20 || math.Abs(pos.AvgCost-wantAvgCost) > 1e-9 {
		t.Errorf("GetPosition() after undoing sell = %+v, want Shares=20 AvgCost=%v (restored to pre-sell state)", pos, wantAvgCost)
	}

	// Undo the latest BUY (lot2) — position falls back to just lot1.
	txs, err = d.GetTransactions("AAPL")
	if err != nil {
		t.Fatalf("GetTransactions() error = %v", err)
	}
	lot2Latest := txs[len(txs)-1].ID
	deletedBuy, err := d.DeleteTransaction(lot2Latest)
	if err != nil {
		t.Fatalf("DeleteTransaction(buy2) error = %v", err)
	}
	if deletedBuy.Side != "BUY" || deletedBuy.Price != 150 {
		t.Errorf("DeleteTransaction(buy2) returned = %+v, want Side=BUY Price=150", deletedBuy)
	}
	pos, ok, err = d.GetPosition("AAPL")
	if err != nil || !ok {
		t.Fatalf("GetPosition() after undoing buy2 = %+v, %v, %v", pos, ok, err)
	}
	if pos.Shares != 10 || pos.AvgCost != 100 {
		t.Errorf("GetPosition() after undoing buy2 = %+v, want Shares=10 AvgCost=100", pos)
	}

	// Undo the last remaining transaction: the position disappears entirely.
	txs, err = d.GetTransactions("AAPL")
	if err != nil || len(txs) != 1 {
		t.Fatalf("GetTransactions() = %v, %v; want 1 row", txs, err)
	}
	if _, err := d.DeleteTransaction(txs[0].ID); err != nil {
		t.Fatalf("DeleteTransaction(lot1) error = %v", err)
	}
	if _, ok, err := d.GetPosition("AAPL"); err != nil || ok {
		t.Errorf("GetPosition() after undoing the last transaction: ok = %v, err = %v; want false, nil", ok, err)
	}
}

func TestSetStopPrice(t *testing.T) {
	d := newTestDB(t)

	if err := d.SetStopPrice("AAPL", 190); !errors.Is(err, ErrNoPosition) {
		t.Errorf("SetStopPrice() with no position error = %v, want ErrNoPosition", err)
	}

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if err := d.SetStopPrice("AAPL", 190); err != nil {
		t.Fatalf("SetStopPrice() error = %v", err)
	}

	pos, ok, err := d.GetPosition("AAPL")
	if err != nil {
		t.Fatalf("GetPosition() error = %v", err)
	}
	if !ok || pos.StopPrice != 190 {
		t.Errorf("GetPosition() = %+v, want StopPrice=190", pos)
	}

	// A full close deletes the positions row, so the stop price goes with
	// it rather than lingering for a later, unrelated round in the ticker.
	if _, _, err := d.RecordSell("AAPL", 10, 210, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	if err := d.SetStopPrice("AAPL", 190); !errors.Is(err, ErrNoPosition) {
		t.Errorf("SetStopPrice() after full close error = %v, want ErrNoPosition", err)
	}
}

// TestRecordSellStopPrice pins Phase 8 PR1's migration 13 behavior (see
// docs/phase-8-trader-analytics.md §3.1): a SELL transaction records the
// position's stop_price at the moment of sale, BUY rows and sells with no
// stop set stay at the 0 sentinel.
func TestRecordSellStopPrice(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if err := d.SetStopPrice("AAPL", 190); err != nil {
		t.Fatalf("SetStopPrice() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 10, 210, 0, "2026-07-05"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	if _, err := d.RecordBuy("MSFT", 5, 300, 0, "2026-07-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("MSFT", 5, 310, 0, "2026-07-06"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	aaplTxs, err := d.GetTransactions("AAPL")
	if err != nil {
		t.Fatalf("GetTransactions(AAPL) error = %v", err)
	}
	if len(aaplTxs) != 2 {
		t.Fatalf("GetTransactions(AAPL) = %+v, want 2 rows", aaplTxs)
	}
	if aaplTxs[0].Side != "BUY" || aaplTxs[0].StopPrice != 0 {
		t.Errorf("AAPL BUY row StopPrice = %v, want 0", aaplTxs[0].StopPrice)
	}
	if aaplTxs[1].Side != "SELL" || aaplTxs[1].StopPrice != 190 {
		t.Errorf("AAPL SELL row StopPrice = %v, want 190", aaplTxs[1].StopPrice)
	}

	msftTxs, err := d.GetTransactions("MSFT")
	if err != nil {
		t.Fatalf("GetTransactions(MSFT) error = %v", err)
	}
	if len(msftTxs) != 2 || msftTxs[1].StopPrice != 0 {
		t.Errorf("GetTransactions(MSFT) = %+v, want SELL row StopPrice=0 (no stop was set)", msftTxs)
	}
}

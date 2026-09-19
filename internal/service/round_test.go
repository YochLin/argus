package service

import (
	"testing"

	"argus/internal/db"
)

func makeTx(ticker, side string, shares, price, fee, realizedPnL float64, date string) db.Transaction {
	return db.Transaction{
		Ticker:      ticker,
		Side:        side,
		Shares:      shares,
		Price:       price,
		Fee:         fee,
		RealizedPnL: realizedPnL,
		Date:        date,
	}
}

func TestSegmentRounds(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := SegmentRounds(nil); len(got) != 0 {
			t.Errorf("SegmentRounds(nil) = %v, want empty", got)
		}
	})

	t.Run("one closed round", func(t *testing.T) {
		txs := []db.Transaction{
			makeTx("AAPL", "BUY", 10, 100, 1, 0, "2026-06-01"),
			makeTx("AAPL", "SELL", 10, 120, 1, 198, "2026-06-10"),
		}
		got := SegmentRounds(txs)
		if len(got) != 1 {
			t.Fatalf("SegmentRounds() = %+v, want 1 round", got)
		}
		r := got[0]
		if r.StartDate != "2026-06-01" || r.EndDate != "2026-06-10" || len(r.Legs) != 2 {
			t.Errorf("unexpected round: %+v", r)
		}
		if shares := RoundBuyShares(r.Legs); shares != 10 {
			t.Errorf("RoundBuyShares = %v, want 10", shares)
		}
		if pnl := RoundRealizedPnL(r.Legs); pnl != 198 {
			t.Errorf("RoundRealizedPnL = %v, want 198", pnl)
		}
		days, ok := RoundHoldingDays(r)
		if !ok || days != 9 {
			t.Errorf("RoundHoldingDays = %d (ok=%v), want 9", days, ok)
		}
	})

	t.Run("trailing open round", func(t *testing.T) {
		txs := []db.Transaction{
			makeTx("AAPL", "BUY", 10, 100, 1, 0, "2026-06-01"),
			makeTx("AAPL", "SELL", 10, 120, 1, 198, "2026-06-10"),
			makeTx("AAPL", "BUY", 5, 130, 1, 0, "2026-06-15"),
		}
		got := SegmentRounds(txs)
		if len(got) != 2 {
			t.Fatalf("SegmentRounds() len = %d, want 2", len(got))
		}
		if got[1].EndDate != "" {
			t.Errorf("trailing round EndDate = %q, want empty", got[1].EndDate)
		}

		lastClosed, ok := LastClosedRound(txs)
		if !ok || lastClosed.StartDate != "2026-06-01" || lastClosed.EndDate != "2026-06-10" {
			t.Errorf("LastClosedRound = %+v, ok=%v, want first round", lastClosed, ok)
		}
	})
}

func TestWeightedAvgPriceAndRoundEntryPrice(t *testing.T) {
	legs := []db.Transaction{
		makeTx("AAPL", "BUY", 10, 100, 10, 0, "2026-06-01"),
		makeTx("AAPL", "BUY", 10, 110, 10, 0, "2026-06-02"),
		makeTx("AAPL", "SELL", 20, 120, 10, 0, "2026-06-10"),
	}

	rawBuy := WeightedAvgPrice(legs, "BUY")
	if rawBuy != 105 {
		t.Errorf("WeightedAvgPrice(BUY) = %v, want 105", rawBuy)
	}

	rawSell := WeightedAvgPrice(legs, "SELL")
	if rawSell != 120 {
		t.Errorf("WeightedAvgPrice(SELL) = %v, want 120", rawSell)
	}

	entryWithFee, ok := RoundEntryPrice(legs)
	// cost = (10*100 + 10 + 10*110 + 10) / 20 = (1010 + 1110) / 20 = 2120 / 20 = 106
	if !ok || entryWithFee != 106 {
		t.Errorf("RoundEntryPrice = %v (ok=%v), want 106", entryWithFee, ok)
	}
}

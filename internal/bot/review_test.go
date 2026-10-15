package bot

import (
	"testing"

	"argus/internal/db"
)

func TestLastClosedRound(t *testing.T) {
	t.Run("no transactions at all", func(t *testing.T) {
		_, ok := lastClosedRound(nil)
		if ok {
			t.Errorf("lastClosedRound(nil) ok = true, want false")
		}
	})

	t.Run("still-open position is not a closed round", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-07-01"},
		}
		_, ok := lastClosedRound(txs)
		if ok {
			t.Errorf("lastClosedRound() ok = true for a still-open position, want false")
		}
	})

	t.Run("single buy and sell", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-07-01"},
			{Side: "SELL", Shares: 10, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-07-01" || round.EndDate != "2026-07-10" || len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() = %+v, want start 2026-07-01, end 2026-07-10, 2 legs", round)
		}
	})

	t.Run("multiple buys and partial sells closing out", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 5, Date: "2026-07-01"},
			{Side: "BUY", Shares: 5, Date: "2026-07-02"},
			{Side: "SELL", Shares: 3, Date: "2026-07-05"},
			{Side: "SELL", Shares: 7, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-07-01" || round.EndDate != "2026-07-10" || len(round.Legs) != 4 {
			t.Errorf("lastClosedRound() = %+v, want start 2026-07-01, end 2026-07-10, 4 legs", round)
		}
	})

	t.Run("closed then re-entered picks the latest round", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-01-01"},
			{Side: "SELL", Shares: 10, Date: "2026-01-15"},
			{Side: "BUY", Shares: 5, Date: "2026-07-01"},
			{Side: "SELL", Shares: 5, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-07-01" || round.EndDate != "2026-07-10" || len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() = %+v, want the second (latest) round only", round)
		}
	})

	t.Run("closed then re-entered and still open returns the prior closed round", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-01-01"},
			{Side: "SELL", Shares: 10, Date: "2026-01-15"},
			{Side: "BUY", Shares: 5, Date: "2026-07-01"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true")
		}
		if round.StartDate != "2026-01-01" || round.EndDate != "2026-01-15" || len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() = %+v, want the first (only closed) round", round)
		}
	})

	t.Run("float dust residue counts as closed", func(t *testing.T) {
		txs := []db.Transaction{
			{Side: "BUY", Shares: 10, Date: "2026-07-01"},
			{Side: "SELL", Shares: 9.9999999995, Date: "2026-07-10"},
		}
		round, ok := lastClosedRound(txs)
		if !ok {
			t.Fatalf("lastClosedRound() ok = false, want true (residue within float tolerance)")
		}
		if len(round.Legs) != 2 {
			t.Errorf("lastClosedRound() legs = %d, want 2", len(round.Legs))
		}
	})
}

func TestBuildClosedTradeReviewNoTransactions(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)

	_, ok, err := b.buildClosedTradeReview("AAPL", 0)
	if err != nil || ok {
		t.Fatalf("buildClosedTradeReview() = _, %v, %v; want ok=false, err=nil for a never-traded ticker", ok, err)
	}
}

func TestBuildClosedTradeReviewFull(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	if _, err := d.RecordBuy("AAPL", 10, 200, 1, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("AAPL", 10, 220, 1, "2026-06-20"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}

	for _, s := range []db.DailySnapshot{
		{Ticker: "AAPL", Date: "2026-06-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-06-10", Close: 235}, // period high
		{Ticker: "AAPL", Date: "2026-06-15", Close: 190}, // period low
		{Ticker: "AAPL", Date: "2026-06-20", Close: 220},
		{Ticker: "SPY", Date: "2026-06-01", Close: 500},
		{Ticker: "SPY", Date: "2026-06-20", Close: 525},
	} {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot() error = %v", err)
		}
	}

	if err := d.SetRoundThesis("AAPL", "2026-06-01", "long-term compounder"); err != nil {
		t.Fatalf("SetRoundThesis() error = %v", err)
	}
	// A later round's thesis must not leak into this round's review.
	if err := d.SetRoundThesis("AAPL", "2026-08-03", "a different, later round"); err != nil {
		t.Fatalf("SetRoundThesis() error = %v", err)
	}

	if err := d.SaveRecommendations("2026-06-10", []db.Recommendation{
		{Ticker: "AAPL", Action: "HOLD", Reason: "still in range"},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}
	// Outside the holding window — must not leak into the review.
	if err := d.SaveRecommendations("2026-07-01", []db.Recommendation{
		{Ticker: "AAPL", Action: "BUY", Reason: "after the round closed"},
	}); err != nil {
		t.Fatalf("SaveRecommendations() error = %v", err)
	}

	trade, ok, err := b.buildClosedTradeReview("AAPL", 0)
	if err != nil || !ok {
		t.Fatalf("buildClosedTradeReview() = _, %v, %v; want ok=true, err=nil", ok, err)
	}

	if len(trade.Legs) != 2 {
		t.Fatalf("buildClosedTradeReview() legs = %d, want 2", len(trade.Legs))
	}
	// realized P&L: (220-200)*10 - 1(buy fee, folded into avg cost) - 1(sell fee) = 198.
	if trade.RealizedPnL < 197.9 || trade.RealizedPnL > 198.1 {
		t.Errorf("buildClosedTradeReview() RealizedPnL = %v, want ~198", trade.RealizedPnL)
	}
	if trade.HoldingDays != 19 {
		t.Errorf("buildClosedTradeReview() HoldingDays = %d, want 19", trade.HoldingDays)
	}
	if trade.PeriodHigh != 235 || trade.PeriodLow != 190 {
		t.Errorf("buildClosedTradeReview() period high/low = %v/%v, want 235/190", trade.PeriodHigh, trade.PeriodLow)
	}
	if trade.VsSPY == nil {
		t.Fatal("buildClosedTradeReview() VsSPY = nil, want a comparison (both endpoints have SPY snapshots)")
	}
	// Ticker: (220-200)/200*100 = 10%; SPY: (525-500)/500*100 = 5%.
	if trade.VsSPY.TickerPct != 10 || trade.VsSPY.SPYPct != 5 {
		t.Errorf("buildClosedTradeReview() VsSPY = %+v, want {10 5}", trade.VsSPY)
	}
	if trade.Thesis == nil || *trade.Thesis != "long-term compounder" {
		t.Errorf("buildClosedTradeReview() Thesis = %v, want \"long-term compounder\"", trade.Thesis)
	}
	if len(trade.Recommendations) != 1 || trade.Recommendations[0].Action != "HOLD" {
		t.Errorf("buildClosedTradeReview() Recommendations = %+v, want exactly the in-window HOLD", trade.Recommendations)
	}
}

// A TW round must be set against 0050 (service.BenchmarkFor), not SPY: the
// review used to hardcode SPY, comparing a TWD stock's return with a USD index.
func TestBuildClosedTradeReviewTWUsesTWBenchmark(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	if _, err := d.RecordBuy("2330", 1000, 600, 0, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, _, err := d.RecordSell("2330", 1000, 660, 0, "2026-06-20"); err != nil {
		t.Fatalf("RecordSell() error = %v", err)
	}
	for _, s := range []db.DailySnapshot{
		{Ticker: "0050", Date: "2026-06-01", Close: 100},
		{Ticker: "0050", Date: "2026-06-20", Close: 104},
		{Ticker: "SPY", Date: "2026-06-01", Close: 500},
		{Ticker: "SPY", Date: "2026-06-20", Close: 525},
	} {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot() error = %v", err)
		}
	}

	trade, ok, err := b.buildClosedTradeReview("2330", 0)
	if err != nil || !ok {
		t.Fatalf("buildClosedTradeReview() = _, %v, %v; want ok=true, err=nil", ok, err)
	}
	if trade.VsSPY == nil {
		t.Fatal("VsSPY = nil, want a comparison against 0050")
	}
	if trade.VsSPY.Bench != "0050" || trade.VsSPY.SPYPct != 4 {
		t.Errorf("VsSPY = %+v, want Bench 0050 and benchmark return 4%%", trade.VsSPY)
	}
}

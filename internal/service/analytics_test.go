package service

import (
	"testing"

	"argus/internal/db"
	"argus/internal/market"
)

type fakeAnalyticsStore struct {
	txs map[string][]db.Transaction
}

func (f *fakeAnalyticsStore) GetAllTransactions() ([]db.Transaction, error) {
	var all []db.Transaction
	for _, l := range f.txs {
		all = append(all, l...)
	}
	return all, nil
}

func (f *fakeAnalyticsStore) GetTransactions(ticker string) ([]db.Transaction, error) {
	return f.txs[ticker], nil
}

func (f *fakeAnalyticsStore) GetDailySnapshotsForTickers([]string, string, string) ([]db.DailySnapshot, error) {
	return nil, nil
}

func (f *fakeAnalyticsStore) GetSnapshotCloseRange(string, string, string) (float64, float64, bool, error) {
	return 0, 0, false, nil
}

func TestAnalyticsService(t *testing.T) {
	store := &fakeAnalyticsStore{
		txs: map[string][]db.Transaction{
			"AAPL": {
				makeTx("AAPL", "BUY", 10, 100, 0, 0, "2026-06-01"),
				makeTx("AAPL", "SELL", 10, 120, 0, 200, "2026-06-10"),
				makeTx("AAPL", "BUY", 5, 130, 0, 0, "2026-06-15"),
			},
			"2330": {
				makeTx("2330", "BUY", 1000, 500, 0, 0, "2026-06-01"),
				makeTx("2330", "SELL", 1000, 600, 0, 100000, "2026-06-05"),
			},
		},
	}

	svc := NewAnalyticsService(store)

	t.Run("TickerRounds", func(t *testing.T) {
		rounds, err := svc.TickerRounds("aapl")
		if err != nil {
			t.Fatalf("TickerRounds error = %v", err)
		}
		if len(rounds) != 2 {
			t.Errorf("len(rounds) = %d, want 2", len(rounds))
		}
	})

	t.Run("LastClosedRound", func(t *testing.T) {
		r, ok, err := svc.LastClosedRound("AAPL")
		if err != nil || !ok {
			t.Fatalf("LastClosedRound error = %v, ok = %v", err, ok)
		}
		if r.StartDate != "2026-06-01" || r.EndDate != "2026-06-10" {
			t.Errorf("unexpected round = %+v", r)
		}
	})

	t.Run("MarketRounds", func(t *testing.T) {
		usRounds, err := svc.MarketRounds(market.US)
		if err != nil {
			t.Fatalf("MarketRounds error = %v", err)
		}
		if len(usRounds["AAPL"]) != 2 || len(usRounds["2330"]) != 0 {
			t.Errorf("MarketRounds(US) = %+v, want only AAPL", usRounds)
		}

		twRounds, err := svc.MarketRounds(market.TW)
		if err != nil {
			t.Fatalf("MarketRounds error = %v", err)
		}
		if len(twRounds["2330"]) != 1 || len(twRounds["AAPL"]) != 0 {
			t.Errorf("MarketRounds(TW) = %+v, want only 2330", twRounds)
		}
	})
}

package web

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// fakeHistory implements data.HistoryProvider for tests.
type fakeHistory struct {
	candles map[string][]data.Candle // ticker -> candles, same set regardless of rangeParam
	err     error

	lastTicker string
	lastRange  string
}

func (f *fakeHistory) GetHistory(ticker, rangeParam string) ([]data.Candle, error) {
	f.lastTicker = ticker
	f.lastRange = rangeParam
	if f.err != nil {
		return nil, f.err
	}
	return f.candles[ticker], nil
}

func candle(date string, close float64) data.Candle {
	t, _ := time.Parse("2006-01-02", date)
	return data.Candle{Date: t, Open: close, High: close, Low: close, Close: close, Volume: 1000}
}

func TestSegmentRounds(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := segmentRounds(nil); len(got) != 0 {
			t.Errorf("segmentRounds(nil) = %v, want empty", got)
		}
	})

	t.Run("one closed round", func(t *testing.T) {
		txs := []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
			tx("AAPL", "SELL", 10, 120, "2026-06-10"),
		}
		got := segmentRounds(txs)
		if len(got) != 1 {
			t.Fatalf("segmentRounds() = %+v, want 1 round", got)
		}
		if got[0].StartDate != "2026-06-01" || got[0].EndDate != "2026-06-10" {
			t.Errorf("round = %+v, want start 2026-06-01 end 2026-06-10", got[0])
		}
	})

	t.Run("closed then reopened, both returned", func(t *testing.T) {
		txs := []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-05-01"),
			tx("AAPL", "SELL", 10, 110, "2026-05-10"),
			tx("AAPL", "BUY", 5, 130, "2026-06-01"),
		}
		got := segmentRounds(txs)
		if len(got) != 2 {
			t.Fatalf("segmentRounds() = %+v, want 2 rounds", got)
		}
		if got[0].EndDate != "2026-05-10" {
			t.Errorf("round[0].EndDate = %q, want 2026-05-10 (closed)", got[0].EndDate)
		}
		if got[1].EndDate != "" {
			t.Errorf("round[1].EndDate = %q, want empty (still open)", got[1].EndDate)
		}
	})

	t.Run("still open trailing round is not dropped", func(t *testing.T) {
		txs := []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		}
		got := segmentRounds(txs)
		if len(got) != 1 || got[0].EndDate != "" {
			t.Fatalf("segmentRounds() = %+v, want 1 open round", got)
		}
	})
}

func TestRoundHistoryRange(t *testing.T) {
	now := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)

	// Exact boundary checks (not eyeballed "N months ago" dates) since
	// calendar month lengths make it hard to hit a days<=X edge precisely.
	mk := func(daysAgo int) string {
		return now.AddDate(0, 0, -daysAgo).Format("2006-01-02")
	}
	boundary := []struct {
		daysAgo int
		want    string
	}{
		{0, "1y"},
		{300, "1y"},
		{301, "2y"},
		{650, "2y"},
		{651, "5y"},
		{1700, "5y"},
		{1701, "max"},
	}
	for _, tt := range boundary {
		got := roundHistoryRange(mk(tt.daysAgo), now)
		if got != tt.want {
			t.Errorf("roundHistoryRange(%d days ago) = %q, want %q", tt.daysAgo, got, tt.want)
		}
	}

	if got := roundHistoryRange("garbage", now); got != "1y" {
		t.Errorf("roundHistoryRange(garbage) = %q, want 1y fallback", got)
	}
}

func TestBuildRounds(t *testing.T) {
	aaplSell := tx("AAPL", "SELL", 10, 120, "2026-06-10")
	aaplSell.RealizedPnL = 200 // (120-100)*10, as db.RecordSell would have computed and stored

	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
			aaplSell,
			tx("MSFT", "BUY", 5, 400, "2026-07-01"),
		},
	}
	got, err := buildRounds(fdb, market.US)
	if err != nil {
		t.Fatalf("buildRounds() error = %v", err)
	}
	if len(got.Rounds) != 2 {
		t.Fatalf("Rounds = %+v, want 2", got.Rounds)
	}
	// Most-recently-started first.
	if got.Rounds[0].Ticker != "MSFT" || !got.Rounds[0].Open {
		t.Errorf("Rounds[0] = %+v, want open MSFT round first (started later)", got.Rounds[0])
	}
	if got.Rounds[1].Ticker != "AAPL" || got.Rounds[1].Open {
		t.Errorf("Rounds[1] = %+v, want closed AAPL round", got.Rounds[1])
	}
	if got.Rounds[1].RealizedPnL != 200 { // (120-100)*10
		t.Errorf("Rounds[1].RealizedPnL = %v, want 200", got.Rounds[1].RealizedPnL)
	}
}

func TestBuildRounds_Empty(t *testing.T) {
	got, err := buildRounds(&fakeDB{}, market.US)
	if err != nil {
		t.Fatalf("buildRounds() error = %v", err)
	}
	if got.Rounds == nil || len(got.Rounds) != 0 {
		t.Errorf("Rounds = %v, want empty non-nil slice", got.Rounds)
	}
}

func TestBuildRoundDetail(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
			tx("AAPL", "SELL", 10, 120, "2026-06-10"),
		},
	}
	hist := &fakeHistory{candles: map[string][]data.Candle{
		"AAPL": {
			candle("2026-05-01", 90), // well before the padded window, should be excluded
			candle("2026-05-20", 95),
			candle("2026-06-01", 100),
			candle("2026-06-10", 120),
			candle("2026-06-20", 125),
			candle("2027-01-01", 150), // well after, should be excluded
		},
	}}

	got, err := buildRoundDetail(fdb, hist, "AAPL", "2026-06-01")
	if err != nil {
		t.Fatalf("buildRoundDetail() error = %v", err)
	}
	if got.Start != "2026-06-01" || got.End != "2026-06-10" {
		t.Errorf("round = %+v, want start 2026-06-01 end 2026-06-10", got)
	}
	if len(got.Trades) != 2 {
		t.Fatalf("Trades = %+v, want the round's 2 legs", got.Trades)
	}

	for _, c := range got.Candles {
		if c.Date == "2026-05-01" || c.Date == "2027-01-01" {
			t.Errorf("Candles contains %s, want it excluded by the padding window", c.Date)
		}
	}
	if len(got.Candles) == 0 {
		t.Error("Candles is empty, want at least the in-window ones")
	}

	if hist.lastTicker != "AAPL" || hist.lastRange != "1y" {
		t.Errorf("GetHistory called with (%q, %q), want (AAPL, 1y) for a round this recent",
			hist.lastTicker, hist.lastRange)
	}

	// MAE/MFE window is the round's own [start, end] (06-01..06-10), not the
	// padded chart window — the 06-20 candle (+25% vs cost 100) must not
	// leak into MFE, which should come out as +20% (06-10's close, the
	// round's actual exit day).
	if !got.HasMAEMFE {
		t.Fatal("HasMAEMFE = false, want true")
	}
	if got.MAEPct != 0 {
		t.Errorf("MAEPct = %v, want 0 (entry day's own close never dips below cost)", got.MAEPct)
	}
	if got.MFEPct != 20 {
		t.Errorf("MFEPct = %v, want 20 (bounded by the round's own end date, not the padded window)", got.MFEPct)
	}
}

func TestBuildRoundDetail_RoundNotFound(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
			tx("AAPL", "SELL", 10, 120, "2026-06-10"),
		},
	}
	_, err := buildRoundDetail(fdb, &fakeHistory{}, "AAPL", "1999-01-01")
	if !errors.Is(err, errRoundNotFound) {
		t.Errorf("buildRoundDetail() error = %v, want errRoundNotFound", err)
	}
}

func TestBuildRoundDetail_HistoryErrorPropagates(t *testing.T) {
	fdb := &fakeDB{
		txs: []db.Transaction{
			tx("AAPL", "BUY", 10, 100, "2026-06-01"),
		},
	}
	_, err := buildRoundDetail(fdb, &fakeHistory{err: errors.New("yahoo down")}, "AAPL", "2026-06-01")
	if err == nil || errors.Is(err, errRoundNotFound) {
		t.Errorf("buildRoundDetail() error = %v, want the history provider's error to propagate", err)
	}
}

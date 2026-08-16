package signals

import (
	"testing"

	"argus/internal/data"
	"argus/internal/market"
)

func TestCheckPriceEvent(t *testing.T) {
	us := DefaultEventThresholds(market.US) // gap 5 / move 7
	tw := DefaultEventThresholds(market.TW) // gap 7 / move 10

	t.Run("gap up triggers, US", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 106, Price: 106, ChangePercent: 6}
		ev := CheckPriceEvent(q, us)
		if ev == nil || ev.GapPct != 6 || ev.ChangePct != 0 {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=6, ChangePct=0", ev)
		}
	})

	t.Run("gap down triggers, US", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 94, Price: 94, ChangePercent: -6}
		ev := CheckPriceEvent(q, us)
		if ev == nil || ev.GapPct != -6 {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=-6", ev)
		}
	})

	t.Run("big move up triggers without a gap, US", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 101, Price: 108, ChangePercent: 8}
		ev := CheckPriceEvent(q, us)
		if ev == nil || ev.GapPct != 0 || ev.ChangePct != 8 {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=0, ChangePct=8", ev)
		}
	})

	t.Run("both gap and big move trigger together", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 106, Price: 109, ChangePercent: 9}
		ev := CheckPriceEvent(q, us)
		if ev == nil || ev.GapPct != 6 || ev.ChangePct != 9 {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=6, ChangePct=9", ev)
		}
	})

	t.Run("neither threshold crossed returns nil", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 101, Price: 102, ChangePercent: 2}
		if ev := CheckPriceEvent(q, us); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil", ev)
		}
	})

	t.Run("PrevClose == 0 returns nil even with a huge move", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 0, Open: 50, Price: 100, ChangePercent: 100}
		if ev := CheckPriceEvent(q, us); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil", ev)
		}
	})

	t.Run("Open == 0 skips the gap check but big move still triggers", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 0, Price: 108, ChangePercent: 8}
		ev := CheckPriceEvent(q, us)
		if ev == nil || ev.GapPct != 0 || ev.ChangePct != 8 {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=0, ChangePct=8", ev)
		}
	})

	t.Run("TW thresholds are wider, a US-triggering move doesn't fire", func(t *testing.T) {
		q := &data.Quote{Ticker: "2330", PrevClose: 100, Open: 106, Price: 106, ChangePercent: 6}
		if ev := CheckPriceEvent(q, tw); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil (6%% is under both TW thresholds)", ev)
		}
	})

	t.Run("TW gap at its own threshold triggers", func(t *testing.T) {
		q := &data.Quote{Ticker: "2330", PrevClose: 100, Open: 107, Price: 107, ChangePercent: 7}
		ev := CheckPriceEvent(q, tw)
		if ev == nil || !almostEqual(ev.GapPct, 7) {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=7", ev)
		}
	})
}

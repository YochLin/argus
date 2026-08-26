package signals

import (
	"testing"

	"argus/internal/data"
	"argus/internal/market"
)

func TestCheckPriceEvent(t *testing.T) {
	us := DefaultEventThresholds(market.US) // gap 5 / move 7 / cumulative 8
	tw := DefaultEventThresholds(market.TW) // gap 7 / move 10 / cumulative 10

	t.Run("gap up triggers, US", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 106, Price: 106, ChangePercent: 6}
		ev := CheckPriceEvent(q, 0, us)
		if ev == nil || ev.GapPct != 6 || !ev.GapTriggered || ev.ChangeTriggered {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=6 triggered, change not triggered", ev)
		}
	})

	t.Run("gap down triggers, US", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 94, Price: 94, ChangePercent: -6}
		ev := CheckPriceEvent(q, 0, us)
		if ev == nil || ev.GapPct != -6 || !ev.GapTriggered {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=-6 triggered", ev)
		}
	})

	t.Run("big move up triggers without a gap, US", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 101, Price: 108, ChangePercent: 8}
		ev := CheckPriceEvent(q, 0, us)
		if ev == nil || ev.GapTriggered || !ev.ChangeTriggered || ev.ChangePct != 8 {
			t.Fatalf("CheckPriceEvent() = %+v, want ChangePct=8 triggered, gap not triggered", ev)
		}
		// Phase 20 後續 PR3: the untriggered gap is still reported, not zeroed.
		if !almostEqual(ev.GapPct, 1) {
			t.Errorf("GapPct = %v, want the real +1%% gap alongside the triggering move", ev.GapPct)
		}
	})

	t.Run("a gap that closes flat still reports the flat close", func(t *testing.T) {
		// The case the pre-PR3 shape couldn't express: opened -6%, bought back
		// all day, closed -0.4%. Both numbers must reach the summary.
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 94, Price: 99.6, ChangePercent: -0.4}
		ev := CheckPriceEvent(q, 0, us)
		if ev == nil || !ev.GapTriggered || ev.ChangeTriggered {
			t.Fatalf("CheckPriceEvent() = %+v, want gap triggered only", ev)
		}
		if !almostEqual(ev.ChangePct, -0.4) {
			t.Errorf("ChangePct = %v, want -0.4", ev.ChangePct)
		}
	})

	t.Run("both gap and big move trigger together", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 106, Price: 109, ChangePercent: 9}
		ev := CheckPriceEvent(q, 0, us)
		if ev == nil || !ev.GapTriggered || !ev.ChangeTriggered || ev.GapPct != 6 || ev.ChangePct != 9 {
			t.Fatalf("CheckPriceEvent() = %+v, want both triggered with 6/9", ev)
		}
	})

	t.Run("neither threshold crossed returns nil", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 101, Price: 102, ChangePercent: 2}
		if ev := CheckPriceEvent(q, 0, us); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil", ev)
		}
	})

	t.Run("PrevClose == 0 returns nil even with a huge move", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 0, Open: 50, Price: 100, ChangePercent: 100}
		if ev := CheckPriceEvent(q, 0, us); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil", ev)
		}
	})

	t.Run("Open == 0 skips the gap check but big move still triggers", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 0, Price: 108, ChangePercent: 8}
		ev := CheckPriceEvent(q, 0, us)
		if ev == nil || ev.GapPct != 0 || ev.GapTriggered || !ev.ChangeTriggered {
			t.Fatalf("CheckPriceEvent() = %+v, want no gap, change triggered", ev)
		}
	})

	t.Run("TW thresholds are wider, a US-triggering move doesn't fire", func(t *testing.T) {
		q := &data.Quote{Ticker: "2330", PrevClose: 100, Open: 106, Price: 106, ChangePercent: 6}
		if ev := CheckPriceEvent(q, 0, tw); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil (6%% is under both TW thresholds)", ev)
		}
	})

	t.Run("TW gap at its own threshold triggers", func(t *testing.T) {
		q := &data.Quote{Ticker: "2330", PrevClose: 100, Open: 107, Price: 107, ChangePercent: 7}
		ev := CheckPriceEvent(q, 0, tw)
		if ev == nil || !ev.GapTriggered || !almostEqual(ev.GapPct, 7) {
			t.Fatalf("CheckPriceEvent() = %+v, want GapPct=7 triggered", ev)
		}
	})
}

func TestCheckPriceEventCumulative(t *testing.T) {
	us := DefaultEventThresholds(market.US) // cumulative decline 8% over 5 sessions
	quiet := &data.Quote{Ticker: "AAPL", PrevClose: 92, Open: 92, Price: 91, ChangePercent: -1.1}

	t.Run("decline past threshold triggers", func(t *testing.T) {
		ev := CheckPriceEvent(quiet, 100, us)
		if ev == nil || !ev.CumulativeTriggered || !almostEqual(ev.CumulativePct, -9) {
			t.Fatalf("CheckPriceEvent() = %+v, want CumulativePct=-9 triggered", ev)
		}
	})

	t.Run("decline under threshold returns nil", func(t *testing.T) {
		if ev := CheckPriceEvent(quiet, 95, us); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil", ev)
		}
	})

	t.Run("rally does not trigger, only decline", func(t *testing.T) {
		if ev := CheckPriceEvent(quiet, 80, us); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil (rally, not decline)", ev)
		}
	})

	t.Run("windowAgoClose <= 0 skips the cumulative check", func(t *testing.T) {
		if ev := CheckPriceEvent(quiet, 0, us); ev != nil {
			t.Errorf("CheckPriceEvent() = %+v, want nil", ev)
		}
	})

	t.Run("an untriggered cumulative move still rides along with a gap", func(t *testing.T) {
		q := &data.Quote{Ticker: "AAPL", PrevClose: 100, Open: 106, Price: 106, ChangePercent: 6}
		ev := CheckPriceEvent(q, 100, us)
		if ev == nil || !ev.GapTriggered || ev.CumulativeTriggered {
			t.Fatalf("CheckPriceEvent() = %+v, want gap triggered, cumulative not", ev)
		}
		if !almostEqual(ev.CumulativePct, 6) {
			t.Errorf("CumulativePct = %v, want the real +6%% five-day change", ev.CumulativePct)
		}
	})
}

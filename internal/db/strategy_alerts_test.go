package db

import "testing"

func TestStrategyAlerts_SaveAndDedupe(t *testing.T) {
	d := newTestDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.SaveStrategyAlert("NVDA", "strategy_squeeze_breakout", "2026-09-30", AlertChannelWatchlist, "msg A"))
	must(d.SaveStrategyAlert("NVDA", "strategy_box_bottom", "2026-09-10", AlertChannelScan, "msg B"))
	// same strategy on the same bar again: kept once, first message wins
	must(d.SaveStrategyAlert("NVDA", "strategy_squeeze_breakout", "2026-09-30", AlertChannelScan, "msg A again"))
	must(d.SaveStrategyAlert("AAPL", "strategy_squeeze_breakout", "2026-09-30", AlertChannelWatchlist, "other ticker"))

	got, err := d.StrategyAlertsFor("NVDA")
	must(err)
	if len(got) != 2 || got[0].Strategy != "strategy_box_bottom" || got[1].Message != "msg A" || got[1].Channel != AlertChannelWatchlist {
		t.Fatalf("want box(9/10) then squeeze(9/30, first message kept), got %+v", got)
	}
	if none, _ := d.StrategyAlertsFor("TSLA"); len(none) != 0 {
		t.Fatalf("unknown ticker returned %+v", none)
	}
}

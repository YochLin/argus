package web

import (
	"testing"

	"argus/internal/data"
	"argus/internal/db"
)

func TestBuildStrategyAlerts(t *testing.T) {
	fdb := &fakeDB{
		strategyAlerts: []db.StrategyAlert{
			{Ticker: "AAPL", Strategy: "strategy_squeeze_breakout", SignalDate: "2026-09-04", Channel: db.AlertChannelWatchlist, Message: "網 1 …"},
			// A Friday scan hit: the report that promotes it runs the next Monday.
			{Ticker: "AAPL", Strategy: "strategy_box_bottom", SignalDate: "2026-09-11", Channel: db.AlertChannelScan, Message: "網 2 …"},
			// Nothing from the LLM within the window.
			{Ticker: "AAPL", Strategy: "strategy_trend_pullback", SignalDate: "2026-09-25", Channel: db.AlertChannelScan},
			{Ticker: "MSFT", Strategy: "strategy_mtf_cross", SignalDate: "2026-09-04", Channel: db.AlertChannelWatchlist},
		},
		recs: []db.Recommendation{
			{Date: "2026-09-04", Ticker: "AAPL", Action: "HOLD", Reason: "same day"},
			{Date: "2026-09-04", Ticker: "MSFT", Action: "SELL", Reason: "someone else's"},
			{Date: "2026-09-07", Ticker: "AAPL", Action: "BUY", Reason: "monday"},
			{Date: "2026-09-12", Ticker: "AAPL", Action: "", Reason: "row from before the action column"},
			{Date: "2026-09-30", Ticker: "AAPL", Action: "SELL", Reason: "too late for the 09-25 signal"},
		},
	}

	got := buildStrategyAlerts(fdb, "AAPL")
	if len(got) != 3 {
		t.Fatalf("got %d alerts, want AAPL's three: %+v", len(got), got)
	}
	if got[0].Type != "strategy_squeeze_breakout" || got[0].Channel != "watchlist" || got[0].Message != "網 1 …" {
		t.Errorf("first alert passes its fields through: %+v", got[0])
	}
	if v := got[0].Verdict; v == nil || v.Action != "HOLD" || v.Date != "2026-09-04" {
		t.Errorf("a same-day recommendation is the verdict: %+v", v)
	}
	// 09-11 → the only candidate inside 09-11..09-15 has no action, so none.
	if got[1].Verdict != nil {
		t.Errorf("a recommendation row without an action is not a verdict: %+v", got[1].Verdict)
	}
	if got[2].Verdict != nil {
		t.Errorf("a recommendation past the window must not attach: %+v", got[2].Verdict)
	}
}

func TestBuildStrategyAlerts_FirstVerdictAfterAWeekend(t *testing.T) {
	fdb := &fakeDB{
		strategyAlerts: []db.StrategyAlert{{Ticker: "AAPL", Strategy: "strategy_box_bottom", SignalDate: "2026-09-11", Channel: db.AlertChannelScan}},
		recs:           []db.Recommendation{{Date: "2026-09-14", Ticker: "AAPL", Action: "BUY", Reason: "monday"}},
	}
	got := buildStrategyAlerts(fdb, "AAPL")
	if len(got) != 1 || got[0].Verdict == nil || got[0].Verdict.Date != "2026-09-14" || got[0].Verdict.Action != "BUY" {
		t.Fatalf("a Friday signal should pick up Monday's call: %+v", got)
	}
}

func TestBuildStrategyAlerts_NoneIsEmptyNotNull(t *testing.T) {
	if got := buildStrategyAlerts(&fakeDB{}, "AAPL"); got == nil || len(got) != 0 {
		t.Errorf("want an empty slice (it is marshalled as []), got %#v", got)
	}
	if got := buildStrategyAlerts(nil, "AAPL"); got == nil || len(got) != 0 {
		t.Errorf("a nil database degrades to empty: %#v", got)
	}
}

func TestBuildChart_Strategies(t *testing.T) {
	fdb := &fakeDB{strategyAlerts: []db.StrategyAlert{{Ticker: "AAPL", Strategy: "strategy_mtf_cross", SignalDate: "2026-02-02", Channel: db.AlertChannelWatchlist}}}
	hist := &fakeHistory{candles: map[string][]data.Candle{"AAPL": levelBaseCandlesForTest(40)}}
	got, err := buildChart(fdb, &fakeQuotes{}, hist, "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Strategies) != 1 || got.Strategies[0].Type != "strategy_mtf_cross" {
		t.Errorf("chart strategies = %+v", got.Strategies)
	}
}

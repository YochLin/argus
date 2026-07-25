package receval

import "testing"

func scoredRec(action, source, mkt string, horizon int, excess float64, hit, matured bool) ScoredRec {
	return ScoredRec{
		Rec: Recommendation{Action: action, Source: source, Market: mkt},
		Windows: []WindowScore{
			{Horizon: horizon, Matured: matured, TickerReturnPct: excess, ExcessReturnPct: excess, Hit: hit, HaveBench: matured},
		},
	}
}

func TestAggregateByAction(t *testing.T) {
	recs := []ScoredRec{
		scoredRec("BUY", "watchlist", "us", 5, 10, true, true),
		scoredRec("BUY", "movers", "us", 5, -4, false, true),
		scoredRec("SELL", "scan", "us", 5, 2, false, true),
		scoredRec("BUY", "watchlist", "us", 5, 0, false, false), // immature, must not count
		{Unscorable: true, Rec: Recommendation{Action: "BUY"}},  // must not count
	}

	byAction := Aggregate(recs, func(r Recommendation) string { return r.Action })
	buy := byAction["BUY"][5]
	if buy.N != 2 || buy.Hits != 1 {
		t.Fatalf("BUY stats = %+v, want N=2 Hits=1", buy)
	}
	if got := buy.AvgReturn(); got != 3 {
		t.Errorf("BUY AvgReturn = %v, want 3 ((10 + -4)/2)", got)
	}

	sell := byAction["SELL"][5]
	if sell.N != 1 || sell.Hits != 0 {
		t.Fatalf("SELL stats = %+v, want N=1 Hits=0", sell)
	}
}

func TestAggregateBySourceUsesDisplaySource(t *testing.T) {
	recs := []ScoredRec{
		scoredRec("BUY", "", "us", 5, 5, true, true), // "" -> "watchlist"
		scoredRec("BUY", "scan", "us", 5, -5, false, true),
	}
	bySource := Aggregate(recs, func(r Recommendation) string { return DisplaySource(r.Source) })
	if _, ok := bySource["watchlist"]; !ok {
		t.Fatalf("bySource keys = %v, want \"watchlist\" present for \"\" source", bySource)
	}
	if _, ok := bySource[""]; ok {
		t.Errorf("bySource should not have a raw \"\" key")
	}
}

func TestExtremesOrdering(t *testing.T) {
	recs := []ScoredRec{
		scoredRec("BUY", "watchlist", "us", 60, 5, true, true),
		scoredRec("BUY", "watchlist", "us", 60, 30, true, true),
		scoredRec("BUY", "watchlist", "us", 60, -20, false, true),
		scoredRec("BUY", "watchlist", "us", 60, 0, false, false), // immature, excluded
	}
	best, worst := Extremes(recs, 60, 2)
	if len(best) != 2 || excessAt(best[0], 60) != 30 || excessAt(best[1], 60) != 5 {
		t.Fatalf("best = %+v, want [30, 5]", best)
	}
	if len(worst) != 2 || excessAt(worst[0], 60) != -20 || excessAt(worst[1], 60) != 5 {
		t.Fatalf("worst = %+v, want [-20, 5]", worst)
	}
}

func TestCountOutcomes(t *testing.T) {
	recs := []ScoredRec{
		{Unscorable: true, Reason: "no history data"},
		{Unscorable: true, Reason: "no history data"},
		{Unscorable: true, Reason: "recommendation predates fetched history range"},
		scoredRec("BUY", "watchlist", "us", 5, 1, true, true),
		scoredRec("BUY", "watchlist", "us", 20, 0, false, false),
	}
	c := CountOutcomes(recs)
	if c.Unscorable != 3 {
		t.Fatalf("Unscorable = %d, want 3", c.Unscorable)
	}
	if c.UnscorableByReason["no history data"] != 2 {
		t.Errorf("UnscorableByReason[no history data] = %d, want 2", c.UnscorableByReason["no history data"])
	}
	if c.MaturedByHorizon[5] != 1 {
		t.Errorf("MaturedByHorizon[5] = %d, want 1", c.MaturedByHorizon[5])
	}
	if c.ImmatureByHorizon[20] != 1 {
		t.Errorf("ImmatureByHorizon[20] = %d, want 1", c.ImmatureByHorizon[20])
	}
}

package bot

import (
	"testing"

	"argus/internal/market"
)

func TestSummarizeTrack(t *testing.T) {
	rows := []trackRow{
		{Action: "BUY", Source: "watchlist", ChangePct: 10, Hit: true},
		{Action: "BUY", Source: "watchlist", ChangePct: -2, Hit: false},
		{Action: "SELL", Source: "scan", ChangePct: -5, Hit: true},
		{Action: "BUY", Source: "scan", ChangePct: 4, Hit: true},
	}

	overall, bySource, _ := summarizeTrack(rows)

	if overall.Evaluated != 4 || overall.Hits != 3 {
		t.Fatalf("summarizeTrack() overall = %+v, want Evaluated=4 Hits=3", overall)
	}
	if got, want := overall.HitRate(), 75.0; got != want {
		t.Errorf("overall.HitRate() = %v, want %v", got, want)
	}
	// BUY avg: (10 + -2 + 4) / 3 = 4; SELL avg: -5 / 1 = -5
	if got, want := overall.AvgBuyPct(), 4.0; got != want {
		t.Errorf("overall.AvgBuyPct() = %v, want %v", got, want)
	}
	if got, want := overall.AvgSellPct(), -5.0; got != want {
		t.Errorf("overall.AvgSellPct() = %v, want %v", got, want)
	}

	if len(bySource) != 2 {
		t.Fatalf("summarizeTrack() bySource = %+v, want exactly 2 groups", bySource)
	}
	watchlistStats := bySource["watchlist"]
	if watchlistStats.Evaluated != 2 || watchlistStats.Hits != 1 {
		t.Errorf("bySource[watchlist] = %+v, want Evaluated=2 Hits=1", watchlistStats)
	}
	scanStats := bySource["scan"]
	if scanStats.Evaluated != 2 || scanStats.Hits != 2 {
		t.Errorf("bySource[scan] = %+v, want Evaluated=2 Hits=2", scanStats)
	}
}

func TestTrackSourceStatsZeroDivision(t *testing.T) {
	var s trackSourceStats
	if s.HitRate() != 0 || s.AvgBuyPct() != 0 || s.AvgSellPct() != 0 {
		t.Errorf("zero-value trackSourceStats methods = %v, %v, %v; want all 0", s.HitRate(), s.AvgBuyPct(), s.AvgSellPct())
	}
}

func TestParseRecommendMarketArg(t *testing.T) {
	cases := []struct {
		args string
		want []market.MarketID
		ok   bool
	}{
		{"", []market.MarketID{market.US, market.TW}, true},
		{"  ", []market.MarketID{market.US, market.TW}, true},
		{"us", []market.MarketID{market.US}, true},
		{"US", []market.MarketID{market.US}, true},
		{"tw", []market.MarketID{market.TW}, true},
		{"TW", []market.MarketID{market.TW}, true},
		{"jp", nil, false},
	}
	for _, c := range cases {
		got, ok := parseRecommendMarketArg(c.args)
		if ok != c.ok {
			t.Errorf("parseRecommendMarketArg(%q) ok = %v, want %v", c.args, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseRecommendMarketArg(%q) = %v, want %v", c.args, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseRecommendMarketArg(%q) = %v, want %v", c.args, got, c.want)
				break
			}
		}
	}
}

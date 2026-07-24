package web

import (
	"errors"
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/market"
)

// levelBaseCandlesForTest builds n days of flat OHLC (High=101, Low=99,
// Close=100) so signals.PriceLevels' ATR-derived clustering tolerance comes
// out to a fixed, predictable value - same fixture shape as
// internal/signals' own levelBaseCandles, duplicated here since it's
// unexported to that package.
func levelBaseCandlesForTest(n int) []data.Candle {
	candles := make([]data.Candle, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		candles[i] = data.Candle{
			Date:   now.AddDate(0, 0, -n+i),
			Open:   100,
			High:   101,
			Low:    99,
			Close:  100,
			Volume: 1_000_000,
		}
	}
	return candles
}

func TestBuildChart(t *testing.T) {
	// A double top far enough apart to each land in its own strict-pivot
	// window but close enough in price to cluster into one level - mirrors
	// internal/signals' TestPriceLevelsDoubleTopMerges, routed through
	// buildChart to confirm candles pass through and levels get computed
	// from that same slice.
	candles := levelBaseCandlesForTest(40)
	candles[10].High = 110.3
	candles[20].High = 110.5

	hist := &fakeHistory{candles: map[string][]data.Candle{"AAPL": candles}}

	got, err := buildChart(hist, "AAPL")
	if err != nil {
		t.Fatalf("buildChart() error = %v", err)
	}
	if got.Ticker != "AAPL" {
		t.Errorf("Ticker = %q, want AAPL", got.Ticker)
	}
	if len(got.Candles) != len(candles) {
		t.Errorf("Candles len = %d, want %d", len(got.Candles), len(candles))
	}
	if len(got.Levels) != 1 {
		t.Fatalf("Levels = %+v, want exactly 1 merged level", got.Levels)
	}
	want := (110.3 + 110.5) / 2
	if got.Levels[0].Price != want {
		t.Errorf("Levels[0].Price = %v, want %v", got.Levels[0].Price, want)
	}
	if got.Levels[0].Touches != 2 {
		t.Errorf("Levels[0].Touches = %d, want 2", got.Levels[0].Touches)
	}

	if hist.lastTicker != "AAPL" || hist.lastRange != "1y" {
		t.Errorf("GetHistory called with (%q, %q), want (AAPL, 1y)", hist.lastTicker, hist.lastRange)
	}
}

func TestBuildChart_NoLevels(t *testing.T) {
	hist := &fakeHistory{candles: map[string][]data.Candle{"AAPL": levelBaseCandlesForTest(40)}}

	got, err := buildChart(hist, "AAPL")
	if err != nil {
		t.Fatalf("buildChart() error = %v", err)
	}
	if got.Levels == nil || len(got.Levels) != 0 {
		t.Errorf("Levels = %v, want empty non-nil slice", got.Levels)
	}
}

func TestBuildChart_HistoryErrorPropagates(t *testing.T) {
	hist := &fakeHistory{err: errors.New("yahoo down")}
	if _, err := buildChart(hist, "AAPL"); err == nil {
		t.Fatal("buildChart() error = nil, want the history provider's error to propagate")
	}
}

func TestBuildTickers(t *testing.T) {
	fdb := &fakeDB{
		watchlist: []string{"AAPL", "2330", "MSFT"},
		positions: []db.Position{
			{Ticker: "NVDA"},
			{Ticker: "AAPL"}, // also on watchlist - must dedupe
			{Ticker: "2454"},
		},
	}

	us, err := buildTickers(fdb, market.US)
	if err != nil {
		t.Fatalf("buildTickers(US) error = %v", err)
	}
	wantUS := []string{"AAPL", "MSFT", "NVDA"}
	if !equalStrings(us.Tickers, wantUS) {
		t.Errorf("buildTickers(US) = %v, want %v", us.Tickers, wantUS)
	}

	tw, err := buildTickers(fdb, market.TW)
	if err != nil {
		t.Fatalf("buildTickers(TW) error = %v", err)
	}
	wantTW := []string{"2330", "2454"}
	if !equalStrings(tw.Tickers, wantTW) {
		t.Errorf("buildTickers(TW) = %v, want %v", tw.Tickers, wantTW)
	}
}

func TestBuildTickers_Empty(t *testing.T) {
	got, err := buildTickers(&fakeDB{}, market.US)
	if err != nil {
		t.Fatalf("buildTickers() error = %v", err)
	}
	if got.Tickers == nil || len(got.Tickers) != 0 {
		t.Errorf("Tickers = %v, want empty non-nil slice", got.Tickers)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

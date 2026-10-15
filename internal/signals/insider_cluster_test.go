package signals

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/market"
)

func insiderClusterBuyCandles() []data.Candle {
	return buildTrendBreakoutCandles()
}

func insiderTx(name, code string, change int64, price float64, daysAgo int) data.InsiderTransaction {
	date := time.Now().AddDate(0, 0, -daysAgo)
	return data.InsiderTransaction{
		Name: name, TransactionCode: code, Change: change, TransactionPrice: price,
		TransactionDate: date.Format("2006-01-02"),
	}
}

func TestCheckInsiderClusterBuyExact(t *testing.T) {
	p := DefaultScreenParams(market.US)
	candles := insiderClusterBuyCandles()

	twoFilersBuying := []data.InsiderTransaction{
		insiderTx("A B", "P", 5000, 100, 10), // $500,000
		insiderTx("C D", "P", 3000, 100, 20), // $300,000
	}
	if !CheckInsiderClusterBuyExact(candles, twoFilersBuying, p) {
		t.Error("2 filers, $800k combined P notional, no offsetting sells: want a trigger")
	}

	oneFiler := []data.InsiderTransaction{
		insiderTx("A B", "P", 5000, 100, 10),
		insiderTx("A B", "P", 3000, 100, 20), // same filer twice, not a cluster
	}
	if CheckInsiderClusterBuyExact(candles, oneFiler, p) {
		t.Error("only 1 distinct filer: want no trigger")
	}

	tooSmall := []data.InsiderTransaction{
		insiderTx("A B", "P", 100, 100, 10), // $10,000
		insiderTx("C D", "P", 100, 100, 20), // $10,000
	}
	if CheckInsiderClusterBuyExact(candles, tooSmall, p) {
		t.Error("2 filers but combined notional under InsiderMinNotional: want no trigger")
	}

	sellsOutweighBuys := []data.InsiderTransaction{
		insiderTx("A B", "P", 5000, 100, 10),   // $500,000 buy
		insiderTx("C D", "P", 3000, 100, 20),   // $300,000 buy
		insiderTx("E F", "S", -20000, 100, 15), // $2,000,000 sell, same window
	}
	if CheckInsiderClusterBuyExact(candles, sellsOutweighBuys, p) {
		t.Error("sell notional >= buy notional in the same window: want no trigger")
	}

	outsideWindow := []data.InsiderTransaction{
		insiderTx("A B", "P", 5000, 100, 10),
		insiderTx("C D", "P", 3000, 100, 200), // 200 days ago, outside insiderLookbackDays
	}
	if CheckInsiderClusterBuyExact(candles, outsideWindow, p) {
		t.Error("second filer's buy is outside the 90-day window: want no trigger (only 1 filer counts)")
	}

	// Trend gate still applies regardless of a qualifying cluster.
	flat := generateBaseCandles(80)
	if CheckInsiderClusterBuyExact(flat, twoFilersBuying, p) {
		t.Error("flat/no-uptrend candles: want no trigger even with a qualifying cluster")
	}
}

package bot

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/llm"
	"argus/internal/market"
)

func candle(date string) data.Candle {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return data.Candle{Date: d}
}

func TestCountCandleGaps(t *testing.T) {
	cases := []struct {
		name   string
		stocks []llm.StockData
		market market.MarketID
		want   int
	}{
		{
			name:   "consecutive trading days, no gap",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-17"), candle("2026-08-18")}}}, // Mon -> Tue
			market: market.US,
			want:   0,
		},
		{
			name:   "weekend between Friday and Monday is not a gap",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-14"), candle("2026-08-17")}}}, // Fri -> Mon
			market: market.US,
			want:   0,
		},
		{
			name:   "missing bar mid-week is a gap",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-17"), candle("2026-08-19")}}}, // Mon -> Wed, Tue missing
			market: market.US,
			want:   1,
		},
		{
			name: "US holiday (Labor Day) does not count as a gap",
			// 2026-09-04 Fri -> 2026-09-08 Tue, skipping weekend + Labor Day (Mon 2026-09-07).
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-09-04"), candle("2026-09-08")}}},
			market: market.US,
			want:   0,
		},
		{
			name:   "TW has no holiday calendar, so it falls back to weekday-only",
			stocks: []llm.StockData{{Candles: []data.Candle{candle("2026-08-17"), candle("2026-08-19")}}},
			market: market.TW,
			want:   1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := countCandleGaps(c.stocks, c.market); got != c.want {
				t.Errorf("countCandleGaps() = %d, want %d", got, c.want)
			}
		})
	}
}

package bot

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/llm"
)

func candle(date string) data.Candle {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return data.Candle{Date: d}
}

func stockWithCandles(dates ...string) llm.StockData {
	candles := make([]data.Candle, len(dates))
	for i, d := range dates {
		candles[i] = candle(d)
	}
	return llm.StockData{Candles: candles}
}

func TestCountCandleGaps(t *testing.T) {
	cases := []struct {
		name  string
		lists [][]llm.StockData
		want  int
	}{
		{
			name:  "single ticker has no peers to attest a gap against",
			lists: [][]llm.StockData{{stockWithCandles("2026-08-17", "2026-08-19")}}, // Mon -> Wed, Tue missing
			want:  0,
		},
		{
			name: "peer traded on the missing day: real gap",
			lists: [][]llm.StockData{{
				stockWithCandles("2026-08-17", "2026-08-19"),               // Mon -> Wed, Tue missing
				stockWithCandles("2026-08-17", "2026-08-18", "2026-08-19"), // peer has the Tue bar
			}},
			want: 1,
		},
		{
			name: "weekend between Friday and Monday, no peer bars either: not a gap",
			lists: [][]llm.StockData{{
				stockWithCandles("2026-08-14", "2026-08-17"), // Fri -> Mon
				stockWithCandles("2026-08-14", "2026-08-17"),
			}},
			want: 0,
		},
		{
			name: "TW holiday nobody in the batch traded through: not a gap",
			// Dragon Boat Festival 2026-06-19 — every ticker in the batch
			// skips it, so it never enters the calendar union.
			lists: [][]llm.StockData{{
				stockWithCandles("2026-06-18", "2026-06-22"),
				stockWithCandles("2026-06-18", "2026-06-22"),
				stockWithCandles("2026-06-18", "2026-06-22"),
			}},
			want: 0,
		},
		{
			name: "candle gap spans watchlist and candidates lists together",
			lists: [][]llm.StockData{
				{stockWithCandles("2026-08-17", "2026-08-19")},               // watchlist: Tue missing
				{stockWithCandles("2026-08-17", "2026-08-18", "2026-08-19")}, // candidates: has Tue
			},
			want: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := countCandleGaps(c.lists...); got != c.want {
				t.Errorf("countCandleGaps() = %d, want %d", got, c.want)
			}
		})
	}
}

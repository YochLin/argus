package signals

import (
	"time"

	"argus/internal/data"
)

func generateBaseCandles(count int) []data.Candle {
	candles := make([]data.Candle, count)
	now := time.Now()
	for i := 0; i < count; i++ {
		t := now.AddDate(0, 0, -count+i)
		price := 100.0 + float64(i%3)
		candles[i] = data.Candle{
			Date:   t,
			Open:   price,
			High:   price + 1.0,
			Low:    price - 1.0,
			Close:  price,
			Volume: 1_000_000,
		}
	}
	return candles
}

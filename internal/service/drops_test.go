package service

import (
	"testing"
	"time"

	"argus/internal/data"
)

func TestDropOn(t *testing.T) {
	day := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	var bars []data.Candle
	for i, c := range []float64{100, 100, 98, 96, 93} {
		bars = append(bars, data.Candle{Date: day.AddDate(0, 0, i), Open: c, High: c + 1, Low: c - 1, Close: c})
	}
	date := bars[4].Date.Format("2006-01-02")

	got := DropOn(bars, date)
	if got == nil || got.Start != 2 || got.End != 4 {
		t.Fatalf("DropOn = %+v", got)
	}
	if got := DropOn(bars, "2026-03-07"); got != nil {
		t.Errorf("no candle for the date: got %+v", got)
	}
	bars = append(bars, data.Candle{Date: day.AddDate(0, 0, 5), Open: 94, High: 95, Low: 93, Close: 94})
	if got := DropOn(bars, bars[5].Date.Format("2006-01-02")); got != nil {
		t.Errorf("the drop ended the bar before: got %+v", got)
	}
}

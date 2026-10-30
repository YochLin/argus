package service

import (
	"testing"
	"time"

	"argus/internal/data"
	"argus/internal/signals"
)

func TestDropTechBreaks(t *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var bars []data.Candle
	for i := 0; i < 70; i++ {
		bars = append(bars, data.Candle{Date: day.AddDate(0, 0, i), Open: 100, High: 101, Low: 99, Close: 100})
	}
	bars[67].Close, bars[68].Close, bars[69].Close = 99.5, 99.2, 90
	drop := signals.DropEvent{Start: 67, End: 69, PreClose: 100}
	if ma, low := DropTechBreaks(bars, drop); !ma || !low {
		t.Errorf("ma60, low20 = %v, %v, want both", ma, low)
	}
	bars[69].Close = 99.1
	if ma, low := DropTechBreaks(bars, drop); !ma || low {
		t.Errorf("ma60, low20 = %v, %v, want only the average broken", ma, low)
	}
	if ma, low := DropTechBreaks(bars[:5], signals.DropEvent{Start: 2, End: 4, PreClose: 100}); ma || low {
		t.Errorf("too little history: %v, %v", ma, low)
	}
}

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

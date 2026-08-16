package db

import "testing"

func TestHasPriceEventAndSavePriceEvent(t *testing.T) {
	d := newTestDB(t)

	has, err := d.HasPriceEvent("AAPL", "2026-08-14")
	if err != nil {
		t.Fatalf("HasPriceEvent() error = %v", err)
	}
	if has {
		t.Errorf("HasPriceEvent() = true before any write, want false")
	}

	if err := d.SavePriceEvent(PriceEvent{Ticker: "AAPL", Market: "us", Date: "2026-08-14", GapPct: 6.2, Summary: "gapped up on earnings beat"}); err != nil {
		t.Fatalf("SavePriceEvent() error = %v", err)
	}

	has, err = d.HasPriceEvent("AAPL", "2026-08-14")
	if err != nil {
		t.Fatalf("HasPriceEvent() error = %v", err)
	}
	if !has {
		t.Errorf("HasPriceEvent() = false after write, want true")
	}

	// Same (ticker, date) again must violate the unique index.
	if err := d.SavePriceEvent(PriceEvent{Ticker: "AAPL", Market: "us", Date: "2026-08-14", GapPct: 6.2}); err == nil {
		t.Error("SavePriceEvent() duplicate (ticker, date) = nil error, want unique-index violation")
	}
}

func TestGetRecentPriceEvents(t *testing.T) {
	d := newTestDB(t)

	if err := d.SavePriceEvent(PriceEvent{Ticker: "AAPL", Market: "us", Date: "2026-08-14", ChangePct: 8}); err != nil {
		t.Fatalf("SavePriceEvent() error = %v", err)
	}
	if err := d.SavePriceEvent(PriceEvent{Ticker: "MSFT", Market: "us", Date: "2026-08-14", GapPct: -5.5}); err != nil {
		t.Fatalf("SavePriceEvent() error = %v", err)
	}
	if err := d.SavePriceEvent(PriceEvent{Ticker: "2330", Market: "tw", Date: "2026-08-14", ChangePct: 10}); err != nil {
		t.Fatalf("SavePriceEvent() error = %v", err)
	}

	got, err := d.GetRecentPriceEvents(2)
	if err != nil {
		t.Fatalf("GetRecentPriceEvents() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetRecentPriceEvents(2) = %+v, want exactly 2", got)
	}
	// Newest first.
	if got[0].Ticker != "2330" || got[1].Ticker != "MSFT" {
		t.Errorf("GetRecentPriceEvents(2) = %+v, want [2330, MSFT] newest-first", got)
	}
}

func TestGetPriceEventsForTicker(t *testing.T) {
	d := newTestDB(t)

	if err := d.SavePriceEvent(PriceEvent{Ticker: "AAPL", Market: "us", Date: "2026-08-10", ChangePct: 7.5}); err != nil {
		t.Fatalf("SavePriceEvent() error = %v", err)
	}
	if err := d.SavePriceEvent(PriceEvent{Ticker: "AAPL", Market: "us", Date: "2026-08-14", GapPct: 5.1, Summary: "gapped on guidance cut"}); err != nil {
		t.Fatalf("SavePriceEvent() error = %v", err)
	}
	if err := d.SavePriceEvent(PriceEvent{Ticker: "MSFT", Market: "us", Date: "2026-08-14", ChangePct: 8}); err != nil {
		t.Fatalf("SavePriceEvent() error = %v", err)
	}

	got, err := d.GetPriceEventsForTicker("AAPL", 20)
	if err != nil {
		t.Fatalf("GetPriceEventsForTicker() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetPriceEventsForTicker(AAPL) = %+v, want exactly 2", got)
	}
	if got[0].Date != "2026-08-14" || got[0].Summary != "gapped on guidance cut" {
		t.Errorf("GetPriceEventsForTicker(AAPL)[0] = %+v, want the 2026-08-14 row newest-first", got[0])
	}
}

func TestGetPriceEventsEmpty(t *testing.T) {
	d := newTestDB(t)

	got, err := d.GetRecentPriceEvents(20)
	if err != nil || len(got) != 0 {
		t.Errorf("GetRecentPriceEvents() on empty table = %+v, %v; want empty, nil", got, err)
	}

	got, err = d.GetPriceEventsForTicker("AAPL", 20)
	if err != nil || len(got) != 0 {
		t.Errorf("GetPriceEventsForTicker() on empty table = %+v, %v; want empty, nil", got, err)
	}
}

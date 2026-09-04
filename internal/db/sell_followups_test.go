package db

import "testing"

func TestHasSellFollowupAndSaveSellFollowup(t *testing.T) {
	d := newTestDB(t)

	has, err := d.HasSellFollowup("AAPL", "2026-08-14")
	if err != nil {
		t.Fatalf("HasSellFollowup() error = %v", err)
	}
	if has {
		t.Errorf("HasSellFollowup() = true before any write, want false")
	}

	f := SellFollowup{
		Ticker: "AAPL", Market: "us", ExitDate: "2026-08-14", ReviewDate: "2026-08-21",
		ExitPrice: 200, PriceAtReview: 220, PctSinceExit: 10, Verdict: "sold_early",
	}
	if err := d.SaveSellFollowup(f); err != nil {
		t.Fatalf("SaveSellFollowup() error = %v", err)
	}

	has, err = d.HasSellFollowup("AAPL", "2026-08-14")
	if err != nil {
		t.Fatalf("HasSellFollowup() error = %v", err)
	}
	if !has {
		t.Errorf("HasSellFollowup() = false after write, want true")
	}

	// Same (ticker, exit_date) again must violate the unique index.
	if err := d.SaveSellFollowup(f); err == nil {
		t.Error("SaveSellFollowup() duplicate (ticker, exit_date) = nil error, want unique-index violation")
	}
}

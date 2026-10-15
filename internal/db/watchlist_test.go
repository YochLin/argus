package db

import "testing"

func TestWatchlistRoundTrip(t *testing.T) {
	d := newTestDB(t)

	if got, err := d.GetWatchlist(); err != nil || len(got) != 0 {
		t.Fatalf("GetWatchlist() = %v, %v; want empty, nil", got, err)
	}

	if err := d.AddTicker("MSFT"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	got, err := d.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	want := []string{"AAPL", "MSFT"} // ORDER BY ticker
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("GetWatchlist() = %v, want %v", got, want)
	}
}

func TestAddTickerIsIdempotent(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() (duplicate) error = %v, want nil (INSERT OR IGNORE)", err)
	}

	got, err := d.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("GetWatchlist() = %v, want exactly one entry", got)
	}
}

func TestRemoveTicker(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	if err := d.RemoveTicker("AAPL"); err != nil {
		t.Fatalf("RemoveTicker() error = %v", err)
	}

	got, err := d.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetWatchlist() = %v, want empty after removal", got)
	}

	// Removing a ticker that was never added should be a no-op, not an error.
	if err := d.RemoveTicker("NOPE"); err != nil {
		t.Errorf("RemoveTicker() on absent ticker error = %v, want nil", err)
	}
}

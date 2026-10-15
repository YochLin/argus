package db

import "testing"

func TestSaveSnapshotUpsert(t *testing.T) {
	d := newTestDB(t)

	s := DailySnapshot{
		Ticker: "AAPL", Date: "2026-07-05",
		Open: 200, Close: 205, High: 206, Low: 199, Volume: 1000, ChangePercent: 2.5,
	}
	if err := d.SaveSnapshot(s); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	// Same ticker+date should replace, not conflict (INSERT OR REPLACE with a
	// UNIQUE(ticker, date) constraint).
	s.Close = 210
	if err := d.SaveSnapshot(s); err != nil {
		t.Fatalf("SaveSnapshot() (upsert) error = %v", err)
	}
}

func TestGetSnapshotClose(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveSnapshot(DailySnapshot{
		Ticker: "AAPL", Date: "2026-07-05",
		Open: 200, Close: 205, High: 206, Low: 199, Volume: 1000, ChangePercent: 2.5,
	}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	close, ok, err := d.GetSnapshotClose("AAPL", "2026-07-05")
	if err != nil || !ok || close != 205 {
		t.Errorf("GetSnapshotClose() = %v, %v, %v; want 205, true, nil", close, ok, err)
	}

	_, ok, err = d.GetSnapshotClose("AAPL", "2026-07-06")
	if err != nil || ok {
		t.Errorf("GetSnapshotClose() for absent date: ok = %v, err = %v; want false, nil", ok, err)
	}
}

func TestGetSnapshotCloseRange(t *testing.T) {
	d := newTestDB(t)

	snap := func(date string, close float64) {
		t.Helper()
		if err := d.SaveSnapshot(DailySnapshot{Ticker: "SPY", Date: date, Close: close}); err != nil {
			t.Fatalf("SaveSnapshot(%s) error = %v", date, err)
		}
	}
	snap("2026-06-30", 500) // outside range
	snap("2026-07-01", 510)
	snap("2026-07-15", 520)
	snap("2026-07-31", 530)

	t.Run("multiple snapshots in range", func(t *testing.T) {
		first, last, ok, err := d.GetSnapshotCloseRange("SPY", "2026-07-01", "2026-07-31")
		if err != nil || !ok || first != 510 || last != 530 {
			t.Errorf("GetSnapshotCloseRange() = %v, %v, %v, %v; want 510, 530, true, nil", first, last, ok, err)
		}
	})

	t.Run("fewer than two snapshots in range", func(t *testing.T) {
		_, _, ok, err := d.GetSnapshotCloseRange("SPY", "2026-08-01", "2026-08-31")
		if err != nil {
			t.Fatalf("GetSnapshotCloseRange() error = %v", err)
		}
		if ok {
			t.Error("GetSnapshotCloseRange() with < 2 rows in range should return ok=false")
		}
	})
}

func TestGetRecentCloses(t *testing.T) {
	d := newTestDB(t)

	snap := func(date string, close float64) {
		t.Helper()
		if err := d.SaveSnapshot(DailySnapshot{Ticker: "AAPL", Date: date, Close: close}); err != nil {
			t.Fatalf("SaveSnapshot(%s) error = %v", date, err)
		}
	}
	snap("2026-08-10", 100)
	snap("2026-08-11", 102)
	snap("2026-08-12", 98)
	snap("2026-08-13", 95)
	snap("2026-08-14", 93)

	t.Run("returns the n most recent closes, oldest first", func(t *testing.T) {
		got, err := d.GetRecentCloses("AAPL", 3)
		if err != nil {
			t.Fatalf("GetRecentCloses() error = %v", err)
		}
		want := []float64{98, 95, 93}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("GetRecentCloses(3) = %v, want %v", got, want)
		}
	})

	t.Run("fewer rows than n returns what's available", func(t *testing.T) {
		got, err := d.GetRecentCloses("AAPL", 10)
		if err != nil || len(got) != 5 {
			t.Errorf("GetRecentCloses(10) = %v, %v; want 5 rows, nil error", got, err)
		}
	})

	t.Run("unknown ticker returns empty", func(t *testing.T) {
		got, err := d.GetRecentCloses("MSFT", 5)
		if err != nil || len(got) != 0 {
			t.Errorf("GetRecentCloses(MSFT) = %v, %v; want empty, nil", got, err)
		}
	})
}

func TestGetLatestSnapshot(t *testing.T) {
	d := newTestDB(t)

	if _, ok, err := d.GetLatestSnapshot("AAPL"); err != nil || ok {
		t.Fatalf("GetLatestSnapshot() before any snapshot: ok = %v, err = %v; want false, nil", ok, err)
	}

	if err := d.SaveSnapshot(DailySnapshot{Ticker: "AAPL", Date: "2026-07-01", Close: 200}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}
	if err := d.SaveSnapshot(DailySnapshot{Ticker: "AAPL", Date: "2026-07-05", Close: 210}); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	got, ok, err := d.GetLatestSnapshot("AAPL")
	if err != nil || !ok || got.Date != "2026-07-05" || got.Close != 210 {
		t.Errorf("GetLatestSnapshot() = %+v, %v, %v; want Date=2026-07-05 Close=210, true, nil", got, ok, err)
	}
}

func TestGetPeakClose(t *testing.T) {
	d := newTestDB(t)

	if _, ok, err := d.GetPeakClose("AAPL", "2026-07-01"); err != nil || ok {
		t.Fatalf("GetPeakClose() before any snapshot: ok = %v, err = %v; want false, nil", ok, err)
	}

	for _, s := range []DailySnapshot{
		{Ticker: "AAPL", Date: "2026-06-30", Close: 300}, // before sinceDate, must be excluded
		{Ticker: "AAPL", Date: "2026-07-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-07-03", Close: 220},
		{Ticker: "AAPL", Date: "2026-07-05", Close: 210},
	} {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot() error = %v", err)
		}
	}

	peak, ok, err := d.GetPeakClose("AAPL", "2026-07-01")
	if err != nil || !ok || peak != 220 {
		t.Errorf("GetPeakClose() = %v, %v, %v; want 220, true, nil", peak, ok, err)
	}
}

func TestGetCloseExtremes(t *testing.T) {
	d := newTestDB(t)

	if _, _, ok, err := d.GetCloseExtremes("AAPL", "2026-07-01", "2026-07-10"); err != nil || ok {
		t.Fatalf("GetCloseExtremes() before any snapshot: ok = %v, err = %v; want false, nil", ok, err)
	}

	for _, s := range []DailySnapshot{
		{Ticker: "AAPL", Date: "2026-06-25", Close: 999}, // before the window, must be excluded
		{Ticker: "AAPL", Date: "2026-07-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-07-03", Close: 230},
		{Ticker: "AAPL", Date: "2026-07-05", Close: 190},
		{Ticker: "AAPL", Date: "2026-07-15", Close: 999}, // after the window, must be excluded
	} {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot() error = %v", err)
		}
	}

	high, low, ok, err := d.GetCloseExtremes("AAPL", "2026-07-01", "2026-07-10")
	if err != nil || !ok || high != 230 || low != 190 {
		t.Errorf("GetCloseExtremes() = %v, %v, %v, %v; want 230, 190, true, nil", high, low, ok, err)
	}
}

func TestGetDailySnapshotsForTickers(t *testing.T) {
	d := newTestDB(t)

	if got, err := d.GetDailySnapshotsForTickers(nil, "2026-07-01", "2026-07-31"); err != nil || got != nil {
		t.Fatalf("GetDailySnapshotsForTickers(nil tickers) = %v, %v; want nil, nil", got, err)
	}

	snaps := []DailySnapshot{
		{Ticker: "AAPL", Date: "2026-07-01", Close: 200},
		{Ticker: "AAPL", Date: "2026-07-02", Close: 205},
		{Ticker: "MSFT", Date: "2026-07-01", Close: 300},
		{Ticker: "MSFT", Date: "2026-07-15", Close: 310}, // outside the queried window
		{Ticker: "NVDA", Date: "2026-07-01", Close: 900}, // not in the tickers filter
	}
	for _, s := range snaps {
		if err := d.SaveSnapshot(s); err != nil {
			t.Fatalf("SaveSnapshot(%+v) error = %v", s, err)
		}
	}

	got, err := d.GetDailySnapshotsForTickers([]string{"AAPL", "MSFT"}, "2026-07-01", "2026-07-05")
	if err != nil {
		t.Fatalf("GetDailySnapshotsForTickers() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("GetDailySnapshotsForTickers() len = %d, want 3 (AAPL x2, MSFT x1 in-window; NVDA excluded, MSFT 07-15 out of range)", len(got))
	}
	if got[0].Ticker != "AAPL" || got[0].Date != "2026-07-01" || got[0].Close != 200 {
		t.Errorf("GetDailySnapshotsForTickers()[0] = %+v, want AAPL 2026-07-01 close 200 (ordered by ticker, date)", got[0])
	}
	if got[1].Ticker != "AAPL" || got[1].Date != "2026-07-02" {
		t.Errorf("GetDailySnapshotsForTickers()[1] = %+v, want AAPL 2026-07-02", got[1])
	}
	if got[2].Ticker != "MSFT" || got[2].Date != "2026-07-01" {
		t.Errorf("GetDailySnapshotsForTickers()[2] = %+v, want MSFT 2026-07-01", got[2])
	}
}

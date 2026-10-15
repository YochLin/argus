package db

import "testing"

// TestSeedSP500 verifies New() seeds the universe table from the embedded
// ticker list on first creation, and that reopening the same database
// (simulating a restart) doesn't duplicate or re-seed rows the user may have
// since removed.
// sp500Entries filters entries down to the 'sp500'-sourced ones — New() also
// seeds a 'tw' tier (see TestSeedTW150 below), so tests scoped to the S&P
// 500 seed specifically must not assume every universe row is 'sp500'.
func sp500Entries(entries []UniverseEntry) []UniverseEntry {
	var out []UniverseEntry
	for _, e := range entries {
		if e.Source == "sp500" {
			out = append(out, e)
		}
	}
	return out
}

func TestSeedSP500(t *testing.T) {
	d := newTestDB(t)

	all, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	entries := sp500Entries(all)
	if len(entries) < 400 {
		t.Fatalf("sp500 entries = %d, want >= 400 (S&P 500 seed)", len(entries))
	}

	// A user removes a seeded ticker...
	if err := d.RemoveUniverseTicker(entries[0].Ticker); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}
	afterRemoveAll, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	afterRemove := sp500Entries(afterRemoveAll)
	if len(afterRemove) != len(entries)-1 {
		t.Fatalf("sp500 entries after remove = %d, want %d", len(afterRemove), len(entries)-1)
	}

	// ...and it must not come back on a later seedSP500() call (only re-seeds
	// when the sp500 source is entirely empty).
	if err := d.seedSP500(); err != nil {
		t.Fatalf("seedSP500() error = %v", err)
	}
	afterReseedAll, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	afterReseed := sp500Entries(afterReseedAll)
	if len(afterReseed) != len(afterRemove) {
		t.Errorf("sp500 entries after reseed = %d, want %d (removed ticker should stay gone)", len(afterReseed), len(afterRemove))
	}
}

// TestSeedTW150 is TestSeedSP500's Phase 6 PR2 mirror for the 'tw' seed
// tier — same fresh-install-only, sticks-after-remove behavior, scoped to
// tw150Tickers instead of sp500Tickers.
func TestSeedTW150(t *testing.T) {
	d := newTestDB(t)

	all, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	var entries []UniverseEntry
	for _, e := range all {
		if e.Source == "tw" {
			entries = append(entries, e)
		}
	}
	if len(entries) < 50 {
		t.Fatalf("tw entries = %d, want >= 50 (tw150 seed)", len(entries))
	}

	if err := d.RemoveUniverseTicker(entries[0].Ticker); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}
	if err := d.seedTW150(); err != nil {
		t.Fatalf("seedTW150() error = %v", err)
	}
	afterAll, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	var afterCount int
	for _, e := range afterAll {
		if e.Source == "tw" {
			afterCount++
		}
	}
	if afterCount != len(entries)-1 {
		t.Errorf("tw entries after remove+reseed = %d, want %d (removed ticker should stay gone)", afterCount, len(entries)-1)
	}
}

func TestUniverseAddRemove(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddUniverseTicker("ZZZZ", "manual"); err != nil {
		t.Fatalf("AddUniverseTicker() error = %v", err)
	}
	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Ticker == "ZZZZ" {
			found = true
			if e.Source != "manual" {
				t.Errorf("ZZZZ source = %q, want manual", e.Source)
			}
		}
	}
	if !found {
		t.Fatalf("ZZZZ not found in universe after AddUniverseTicker()")
	}

	if err := d.RemoveUniverseTicker("ZZZZ"); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}
	entries, err = d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	for _, e := range entries {
		if e.Ticker == "ZZZZ" {
			t.Fatalf("ZZZZ still present after RemoveUniverseTicker()")
		}
	}
}

// TestUniverseRemoveIsTombstoneNotDelete verifies migration 9's soft-delete:
// RemoveUniverseTicker must not erase the row (SyncSP500's stick guarantee
// depends on the tombstone still being there to check against).
func TestUniverseRemoveIsTombstoneNotDelete(t *testing.T) {
	d := newTestDB(t)

	if err := d.AddUniverseTicker("ZZZZ", "manual"); err != nil {
		t.Fatalf("AddUniverseTicker() error = %v", err)
	}
	if err := d.RemoveUniverseTicker("ZZZZ"); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}

	var removed bool
	if err := d.conn.QueryRow(`SELECT removed FROM universe WHERE ticker = 'ZZZZ'`).Scan(&removed); err != nil {
		t.Fatalf("row for ZZZZ should still exist after RemoveUniverseTicker(): %v", err)
	}
	if !removed {
		t.Error("ZZZZ's removed flag should be 1 after RemoveUniverseTicker()")
	}

	// Adding it back must clear the tombstone, not silently no-op.
	if err := d.AddUniverseTicker("ZZZZ", "manual"); err != nil {
		t.Fatalf("AddUniverseTicker() (re-add) error = %v", err)
	}
	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Ticker == "ZZZZ" {
			found = true
		}
	}
	if !found {
		t.Error("ZZZZ should be active again in GetUniverse() after re-adding a tombstoned ticker")
	}
}

func TestSyncSP500NewTickerAutoAdded(t *testing.T) {
	d := newTestDB(t)

	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	missingTicker := sp500Entries(entries)[0].Ticker
	if _, err := d.conn.Exec(`DELETE FROM universe WHERE ticker = ?`, missingTicker); err != nil {
		t.Fatalf("simulating a never-seeded ticker: %v", err)
	}

	added, delisted, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() error = %v", err)
	}
	if len(delisted) != 0 {
		t.Errorf("SyncSP500() delisted = %v, want empty", delisted)
	}
	found := false
	for _, ticker := range added {
		if ticker == missingTicker {
			found = true
		}
	}
	if !found {
		t.Fatalf("SyncSP500() added = %v, want it to contain %q", added, missingTicker)
	}

	after, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	if len(after) != len(entries) {
		t.Errorf("GetUniverse() len after sync = %d, want %d (missing ticker restored)", len(after), len(entries))
	}
}

func TestSyncSP500TombstonedTickerSticks(t *testing.T) {
	d := newTestDB(t)

	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	removedTicker := sp500Entries(entries)[0].Ticker
	if err := d.RemoveUniverseTicker(removedTicker); err != nil {
		t.Fatalf("RemoveUniverseTicker() error = %v", err)
	}

	added, delisted, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() error = %v", err)
	}
	for _, ticker := range added {
		if ticker == removedTicker {
			t.Fatalf("SyncSP500() added = %v, must not re-add a tombstoned ticker (%q)", added, removedTicker)
		}
	}
	for _, ticker := range delisted {
		if ticker == removedTicker {
			t.Fatalf("SyncSP500() delisted = %v, a tombstoned ticker must be skipped by the diff entirely (%q)", delisted, removedTicker)
		}
	}

	after, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse() error = %v", err)
	}
	for _, e := range after {
		if e.Ticker == removedTicker {
			t.Errorf("GetUniverse() after SyncSP500() should still exclude the tombstoned ticker %q", removedTicker)
		}
	}
}

func TestSyncSP500DelistedReportedButRowNotDeleted(t *testing.T) {
	d := newTestDB(t)

	if _, err := d.conn.Exec(`INSERT INTO universe (ticker, source) VALUES ('FAKEDELISTED', 'sp500')`); err != nil {
		t.Fatalf("inserting a fake sp500-sourced ticker: %v", err)
	}

	added, delisted, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() error = %v", err)
	}
	for _, ticker := range added {
		if ticker == "FAKEDELISTED" {
			t.Fatalf("SyncSP500() added = %v, should not include an already-present ticker", added)
		}
	}
	found := false
	for _, ticker := range delisted {
		if ticker == "FAKEDELISTED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SyncSP500() delisted = %v, want it to contain FAKEDELISTED", delisted)
	}

	var removed bool
	if err := d.conn.QueryRow(`SELECT removed FROM universe WHERE ticker = 'FAKEDELISTED'`).Scan(&removed); err != nil {
		t.Fatalf("FAKEDELISTED row should still exist after SyncSP500(): %v", err)
	}
	if removed {
		t.Error("SyncSP500() must never set removed on a delisted ticker — only the user's /universe remove does that")
	}
}

func TestSyncSP500Idempotent(t *testing.T) {
	d := newTestDB(t)

	// Fresh DB is seeded to exactly match the embedded list — no drift at
	// all — so the first call should already be a no-op, and a second call
	// must agree.
	added1, delisted1, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() (first run) error = %v", err)
	}
	if len(added1) != 0 || len(delisted1) != 0 {
		t.Fatalf("SyncSP500() (first run) = %v, %v, want both empty on a freshly seeded DB", added1, delisted1)
	}

	added2, delisted2, err := d.SyncSP500()
	if err != nil {
		t.Fatalf("SyncSP500() (second run) error = %v", err)
	}
	if len(added2) != 0 || len(delisted2) != 0 {
		t.Fatalf("SyncSP500() (second run) = %v, %v, want both empty (idempotent)", added2, delisted2)
	}
}

func TestScanHitsGroupedByTicker(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveScanHit("ZZZZ", "2026-07-08", "RSI oversold (28.4)"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}
	if err := d.SaveScanHit("ZZZZ", "2026-07-08", "MACD golden cross"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}
	if err := d.SaveScanHit("YYYY", "2026-07-08", "RSI oversold (25.1)"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}
	// A different date must not leak into the same-day query.
	if err := d.SaveScanHit("ZZZZ", "2026-07-07", "RSI oversold (29.0)"); err != nil {
		t.Fatalf("SaveScanHit() error = %v", err)
	}

	got, err := d.GetScanHits("2026-07-08")
	if err != nil {
		t.Fatalf("GetScanHits() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetScanHits() len = %d, want 2", len(got))
	}
	want := "RSI oversold (28.4); MACD golden cross"
	if got["ZZZZ"] != want {
		t.Errorf("GetScanHits()[ZZZZ] = %q, want %q", got["ZZZZ"], want)
	}
	if got["YYYY"] != "RSI oversold (25.1)" {
		t.Errorf("GetScanHits()[YYYY] = %q, want %q", got["YYYY"], "RSI oversold (25.1)")
	}
}

package db

import (
	"path/filepath"
	"testing"
)

// newTestDB opens a fresh sqlite file in a t.TempDir() so each test gets an
// isolated, real database (modernc.org/sqlite is pure Go — no cgo needed,
// nothing to mock).
func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestBackup(t *testing.T) {
	d := newTestDB(t)
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := d.Backup(dest); err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	backup, err := New(dest)
	if err != nil {
		t.Fatalf("open backup file: %v", err)
	}
	defer backup.Close()
	got, err := backup.GetWatchlist()
	if err != nil || len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("backup GetWatchlist() = %v, %v; want [AAPL], nil", got, err)
	}
}

// TestOpenReadOnly verifies both halves of the read-only connection's
// contract: it can see data committed by a separate read-write connection
// to the same file (proving it isn't just an empty in-memory stand-in), and
// SQLite's query_only pragma actually rejects a write attempted through it
// (proving "read-only" is enforced, not just a naming convention) — see
// OpenReadOnly's doc comment for why this can't rely on the DSN's
// conventional "mode=ro" instead.
func TestOpenReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	rw, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer rw.Close()
	if err := rw.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}

	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer ro.Close()

	got, err := ro.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("GetWatchlist() = %v, want [AAPL]", got)
	}

	if err := ro.AddTicker("MSFT"); err == nil {
		t.Error("AddTicker() through a read-only connection should have failed, got nil error")
	}
}

// TestOpenForWrites verifies the write-pilot connection is genuinely
// writable (unlike OpenReadOnly) and that its writes are visible through a
// separate connection to the same file — the two-writer-process scenario
// add_to_watchlist/remove_from_watchlist actually run under.
func TestOpenForWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	rw, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer rw.Close()

	writer, err := OpenForWrites(path)
	if err != nil {
		t.Fatalf("OpenForWrites() error = %v", err)
	}
	defer writer.Close()

	if err := writer.AddTicker("TSLA"); err != nil {
		t.Fatalf("AddTicker() through the write connection error = %v", err)
	}

	got, err := rw.GetWatchlist()
	if err != nil {
		t.Fatalf("GetWatchlist() error = %v", err)
	}
	if len(got) != 1 || got[0] != "TSLA" {
		t.Errorf("GetWatchlist() via the main connection = %v, want [TSLA] (write from the second connection should be visible)", got)
	}

	if err := writer.RemoveTicker("TSLA"); err != nil {
		t.Fatalf("RemoveTicker() through the write connection error = %v", err)
	}
	if got, err := rw.GetWatchlist(); err != nil || len(got) != 0 {
		t.Errorf("GetWatchlist() after remove = %v, %v; want empty, nil", got, err)
	}
}

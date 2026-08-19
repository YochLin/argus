package db

import "testing"

func TestBlockUnblockListNewsSources(t *testing.T) {
	d := newTestDB(t)

	got, err := d.ListBlockedNewsSources()
	if err != nil || len(got) != 0 {
		t.Fatalf("ListBlockedNewsSources() on empty table = %+v, %v; want empty, nil", got, err)
	}

	if err := d.BlockNewsSource("Content Farm Daily"); err != nil {
		t.Fatalf("BlockNewsSource() error = %v", err)
	}
	// Re-blocking is a no-op, not an error.
	if err := d.BlockNewsSource("Content Farm Daily"); err != nil {
		t.Fatalf("BlockNewsSource() re-block error = %v", err)
	}

	if blocked, err := d.IsNewsSourceBlocked("  content farm daily  "); err != nil || !blocked {
		t.Errorf("IsNewsSourceBlocked() = %v, %v; want true (case/whitespace-insensitive)", blocked, err)
	}
	if blocked, err := d.IsNewsSourceBlocked("Some Other Source"); err != nil || blocked {
		t.Errorf("IsNewsSourceBlocked() = %v, %v; want false", blocked, err)
	}

	got, err = d.ListBlockedNewsSources()
	if err != nil {
		t.Fatalf("ListBlockedNewsSources() error = %v", err)
	}
	if len(got) != 1 || got[0].Source != "Content Farm Daily" {
		t.Fatalf("ListBlockedNewsSources() = %+v, want 1 row for Content Farm Daily", got)
	}

	if err := d.UnblockNewsSource("Content Farm Daily"); err != nil {
		t.Fatalf("UnblockNewsSource() error = %v", err)
	}
	got, err = d.ListBlockedNewsSources()
	if err != nil || len(got) != 0 {
		t.Fatalf("ListBlockedNewsSources() after unblock = %+v, %v; want empty, nil", got, err)
	}
}

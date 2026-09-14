package db

import (
	"testing"
)

// TestRefreshTWLiquidUniverse_RotatesOnlyItsOwnTier pins the three boundaries
// the rotation has to respect at once: it drops its own stale rows, it leaves
// every other source alone even when absent from the new ranking, and a
// ticker the user removed by hand does not come back just because it still
// ranks. The last one is the reason this hard-DELETEs instead of tombstoning
// — see RefreshTWLiquidUniverse's doc comment.
func TestRefreshTWLiquidUniverse_RotatesOnlyItsOwnTier(t *testing.T) {
	d := newTestDB(t)

	// 2330 is in the embedded tw150 list newTestDB already seeded; the rest
	// are deliberately absent from it, so they exercise the rotation's own
	// insert/delete rather than colliding with the 'tw' tier.
	if _, _, err := d.RefreshTWLiquidUniverse([]string{"2330", "1215", "4142"}); err != nil {
		t.Fatalf("seed refresh: %v", err)
	}
	if err := d.AddUniverseTicker("6547", "manual"); err != nil {
		t.Fatalf("AddUniverseTicker: %v", err)
	}
	// The user does not want 1215 scanned, whatever it ranks.
	if err := d.RemoveUniverseTicker("1215"); err != nil {
		t.Fatalf("RemoveUniverseTicker: %v", err)
	}

	// Next month: 4142 fell out, 2762 came in, 1215 still ranks.
	added, dropped, err := d.RefreshTWLiquidUniverse([]string{"2330", "1215", "2762"})
	if err != nil {
		t.Fatalf("RefreshTWLiquidUniverse: %v", err)
	}
	if len(added) != 1 || added[0] != "2762" {
		t.Errorf("added = %v, want [2762]", added)
	}
	if len(dropped) != 1 || dropped[0] != "4142" {
		t.Errorf("dropped = %v, want [4142]", dropped)
	}

	active := make(map[string]string)
	entries, err := d.GetUniverse()
	if err != nil {
		t.Fatalf("GetUniverse: %v", err)
	}
	for _, e := range entries {
		active[e.Ticker] = e.Source
	}
	for ticker, want := range map[string]string{
		"2762": twLiquidSource, // rotated in
		"2330": "tw",           // already seeded: rotation must not re-tier it
		"1101": "tw",           // seeded tier untouched by the rotation
		"6547": "manual",       // manual entry untouched by the rotation
	} {
		if got, ok := active[ticker]; !ok || got != want {
			t.Errorf("universe[%s] = %q (present=%v), want %q", ticker, got, ok, want)
		}
	}
	if _, ok := active["4142"]; ok {
		t.Error("4142 still active after falling out of the ranking")
	}
	if _, ok := active["1215"]; ok {
		t.Error("1215 came back despite a manual /universe remove")
	}
}

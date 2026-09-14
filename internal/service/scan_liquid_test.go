package service

import (
	"context"
	"testing"
	"time"

	"argus/internal/sinopac"
)

// fakeRanker serves canned whole-market quotes per date; a date with no
// entry comes back empty, which is how the real daemon reports a non-trading
// day.
type fakeRanker struct {
	byDate map[string][]sinopac.DailyQuote
	calls  []string
}

func (f *fakeRanker) DailyQuotes(_ context.Context, date string) ([]sinopac.DailyQuote, error) {
	f.calls = append(f.calls, date)
	return f.byDate[date], nil
}

// TestRefreshTWUniverse_SumsSessionsAndFiltersNonStocks pins what the
// multi-session sum is for: 8299 out-turns every name on one session and
// nothing on the others, so a single-day rank would hand the scan pool a 當沖
// flash while the five-day sum does not. It also pins the walk past
// non-trading dates, the ETF/warrant filter, and the penny-stock floor.
func TestRefreshTWUniverse_SumsSessionsAndFiltersNonStocks(t *testing.T) {
	steady := func(amount float64) []sinopac.DailyQuote {
		return []sinopac.DailyQuote{
			{Code: "2330", Close: 2400, Amount: amount},
			{Code: "2317", Close: 200, Amount: amount * 0.9},
			{Code: "0050", Close: 100, Amount: amount * 5},   // ETF, dropped
			{Code: "031234", Close: 5, Amount: amount * 5},   // warrant, dropped
			{Code: "9999", Close: 4.2, Amount: amount * 1.5}, // under the price floor
		}
	}
	// now is a Monday, so 09-13/09-12 are the weekend gap the walk has to
	// step over before it reaches five real sessions.
	now := time.Date(2026, 9, 14, 20, 0, 0, 0, cst)
	byDate := map[string][]sinopac.DailyQuote{
		"2026-09-14": append(steady(1e9), sinopac.DailyQuote{Code: "8299", Close: 90, Amount: 3e9}),
		// 2026-09-13 (Sun) / 2026-09-12 (Sat): absent, so empty.
		"2026-09-11": steady(1e9),
		"2026-09-10": steady(1e9),
		"2026-09-09": steady(1e9),
		"2026-09-08": steady(1e9),
	}

	store := &fakeScanStore{mockRiskStore: newMockRiskStore()}
	ranker := &fakeRanker{byDate: byDate}
	s := NewScanService(ScanConfig{
		Store:  store,
		Ranker: ranker,
		Now:    func() time.Time { return now },
	})

	if _, _, err := s.RefreshTWUniverse(context.Background()); err != nil {
		t.Fatalf("RefreshTWUniverse: %v", err)
	}

	got := store.refreshed
	want := []string{"2330", "2317", "8299"} // 5e9 > 4.5e9 > 3e9
	if len(got) != len(want) {
		t.Fatalf("refreshed = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("refreshed = %v, want %v", got, want)
		}
	}
	if len(ranker.calls) != 7 {
		t.Errorf("scanner called on %d dates (%v), want 7 (5 sessions + the weekend gap)", len(ranker.calls), ranker.calls)
	}
}

// TestRefreshTWUniverse_NoRankerIsNoOp: without a Shioaji daemon there is no
// ranking source, and the pool must be left exactly as seedTW150 and
// /universe left it rather than rotated to empty.
func TestRefreshTWUniverse_NoRankerIsNoOp(t *testing.T) {
	store := &fakeScanStore{mockRiskStore: newMockRiskStore()}
	s := NewScanService(ScanConfig{Store: store})

	added, dropped, err := s.RefreshTWUniverse(context.Background())
	if err != nil || added != nil || dropped != nil {
		t.Errorf("RefreshTWUniverse() = %v, %v, %v; want nil, nil, nil", added, dropped, err)
	}
	if store.refreshed != nil {
		t.Errorf("store was rotated to %v with no ranker configured", store.refreshed)
	}
}

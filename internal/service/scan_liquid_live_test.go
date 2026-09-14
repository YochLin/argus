package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"argus/internal/sinopac"
)

// TestRefreshTWUniverse_Live runs the rotation against a real Shioaji daemon,
// skipped unless SHIOAJI_ADDR points at one — the same env-gated live-check
// convention cmd/strategyscan's FIB_CACHE tests use.
//
// It exists because the unit tests above cannot see the daemon's response
// shape, and that shape is where both of this feature's real bugs were:
// /data/scanner caps count at 200 (a 422, not a silent clamp) and returns
// every row twice, which together made the original scanner-based ranking
// unable to produce more than 100 distinct tickers. A daemon upgrade that
// changes daily_quotes the same way would otherwise pass every test in this
// package and quietly rotate the scan pool to something wrong.
//
// Assertions are deliberately shape-only — counts, code format, ordering —
// never a specific ticker or turnover figure, which are live market data and
// would make this a test that fails on a quiet day rather than on a bug.
func TestRefreshTWUniverse_Live(t *testing.T) {
	addr := os.Getenv("SHIOAJI_ADDR")
	if addr == "" {
		t.Skip("SHIOAJI_ADDR not set; skipping live Shioaji daemon check")
	}

	store := &fakeScanStore{mockRiskStore: newMockRiskStore()}
	s := NewScanService(ScanConfig{Store: store, Ranker: sinopac.New(addr)})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, _, err := s.RefreshTWUniverse(ctx); err != nil {
		t.Fatalf("RefreshTWUniverse: %v", err)
	}

	got := store.refreshed
	if len(got) != twLiquidTopN {
		t.Errorf("ranked %d tickers, want %d — the whole market should comfortably fill the cut", len(got), twLiquidTopN)
	}
	seen := make(map[string]bool, len(got))
	for _, code := range got {
		if !isTWCommonStock(code) {
			t.Errorf("ranked %q, which is not a TW common stock", code)
		}
		if seen[code] {
			t.Errorf("ranked %q twice — the feed is duplicating rows again", code)
		}
		seen[code] = true
	}
	// 2330 is the one name whose presence is safe to assert: it has been the
	// most-traded stock on the exchange every session for years, and if it is
	// missing from a 300-name turnover ranking, the ranking is broken, not
	// the market.
	if !seen["2330"] {
		t.Errorf("2330 absent from the top %d by turnover; got %s...", twLiquidTopN, strings.Join(got[:min(10, len(got))], ", "))
	}
}

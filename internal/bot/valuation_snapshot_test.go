package bot

import (
	"path/filepath"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
)

// fakeSECProvider is a data.FundamentalHistoryProvider stub recording call
// count, so a test can assert a cache hit skipped the network entirely.
type fakeSECProvider struct {
	calls int
	snap  *data.FundamentalSnapshot
	err   error
}

func (f *fakeSECProvider) GetFundamentalSnapshot(ticker string, candles []data.Candle) (*data.FundamentalSnapshot, error) {
	f.calls++
	return f.snap, f.err
}

func newSECTestBot(t *testing.T) (*Bot, *db.DB) {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return &Bot{db: d, history: noDataProvider{}}, d
}

func TestCachedSECSnapshot_NilWhenNotConfigured(t *testing.T) {
	b, _ := newSECTestBot(t)
	if got := b.cachedValuationSnapshot("AAPL"); got != nil {
		t.Errorf("cachedValuationSnapshot() = %+v, want nil (secFundamentals not configured)", got)
	}
}

func TestCachedSECSnapshot_NilForTWTicker(t *testing.T) {
	b, _ := newSECTestBot(t)
	b.secFundamentals = &fakeSECProvider{snap: &data.FundamentalSnapshot{Ticker: "2330"}}
	if got := b.cachedValuationSnapshot("2330"); got != nil {
		t.Errorf("cachedValuationSnapshot(2330) = %+v, want nil (SEC EDGAR is US-only)", got)
	}
}

func TestCachedSECSnapshot_FetchesOnMissAndCaches(t *testing.T) {
	b, d := newSECTestBot(t)
	b.history = rankHistoryStub{byTicker: map[string][]data.Candle{
		"AAPL": flatCandles(120, 200, 1000),
	}}
	pct := 60.0
	fake := &fakeSECProvider{snap: &data.FundamentalSnapshot{Ticker: "AAPL", EPSAnnual: 6.5, PERatio: 30.7, PEPercentile: &pct}}
	b.secFundamentals = fake

	got := b.cachedValuationSnapshot("AAPL")
	if got == nil || got.EPSAnnual != 6.5 {
		t.Fatalf("cachedValuationSnapshot() = %+v, want EPSAnnual 6.5", got)
	}
	if fake.calls != 1 {
		t.Errorf("provider calls = %d, want 1", fake.calls)
	}

	stored, err := d.GetFundamentalSnapshot("AAPL")
	if err != nil || stored == nil {
		t.Fatalf("db.GetFundamentalSnapshot() = %v, %v, want a saved row", stored, err)
	}

	// second call within the TTL must hit the cache, not the provider again.
	got2 := b.cachedValuationSnapshot("AAPL")
	if got2 == nil || got2.EPSAnnual != 6.5 {
		t.Fatalf("cachedValuationSnapshot() (cache hit) = %+v, want EPSAnnual 6.5", got2)
	}
	if fake.calls != 1 {
		t.Errorf("provider calls after cache hit = %d, want still 1", fake.calls)
	}
}

func TestCachedSECSnapshot_FreshlySavedRowIsACacheHit(t *testing.T) {
	b, d := newSECTestBot(t)
	if err := d.SaveFundamentalSnapshot(db.FundamentalSnapshot{Ticker: "AAPL", EPSAnnual: 1.0}); err != nil {
		t.Fatalf("SaveFundamentalSnapshot() error = %v", err)
	}
	fake := &fakeSECProvider{snap: &data.FundamentalSnapshot{Ticker: "AAPL", EPSAnnual: 9.0}}
	b.secFundamentals = fake

	got := b.cachedValuationSnapshot("AAPL")
	if got == nil || got.EPSAnnual != 1.0 {
		t.Errorf("cachedValuationSnapshot() = %+v, want the cached EPSAnnual 1.0 (row is fresh, within the 90-day TTL)", got)
	}
	if fake.calls != 0 {
		t.Errorf("provider calls = %d, want 0 (fresh cache hit skips the network)", fake.calls)
	}
}

package histcache

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"argus/internal/data"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// fakeInner plays the provider: bars is its server-side series (oldest
// first), and each call is answered with the window rangeParam asks for,
// relative to the shared test clock, and recorded in calls.
type fakeInner struct {
	mu    sync.Mutex
	clk   *clock
	bars  []data.Candle
	err   error
	calls []string
}

func (f *fakeInner) GetHistory(_, rangeParam string) ([]data.Candle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, rangeParam)
	if f.err != nil {
		return nil, f.err
	}
	from, ok := rangeStart(rangeParam, f.clk.now())
	if !ok {
		return append([]data.Candle(nil), f.bars...), nil
	}
	var out []data.Candle
	for _, b := range f.bars {
		if !b.Date.Before(from.Truncate(24 * time.Hour)) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeInner) callCount(rangeParam string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == rangeParam {
			n++
		}
	}
	return n
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// series is one bar per weekday in [from, to], closing at scale*(100+i*0.1).
func series(from, to time.Time, scale float64) []data.Candle {
	var out []data.Candle
	i := 0
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		px := scale * (100 + float64(i)*0.1)
		out = append(out, data.Candle{Date: d, Open: px, High: px + 1, Low: px - 1, Close: px, Volume: 1000})
		i++
	}
	return out
}

// setup returns a cache whose clock reads Mon 2026-06-15 noon UTC, in front of
// a provider holding ten years of bars ending that same day.
func setup(t *testing.T) (*Cache, *fakeInner, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)}
	inner := &fakeInner{clk: clk, bars: series(day(2016, 6, 1), day(2026, 6, 15), 1)}
	c, err := Open(filepath.Join(t.TempDir(), "prices.db"), inner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.now = clk.now
	return c, inner, clk
}

func TestColdFillThenServedFromStore(t *testing.T) {
	c, inner, _ := setup(t)

	got, err := c.GetHistory("AAPL", "1y")
	if err != nil {
		t.Fatal(err)
	}
	if inner.callCount("10y") != 1 || len(inner.calls) != 1 {
		t.Fatalf("cold fetch should be exactly one 10y call, got %v", inner.calls)
	}
	if first := got[0].Date.UTC().Format(dateLayout); first < "2025-06-15" {
		t.Errorf("1y window starts %s, want >= 2025-06-15", first)
	}
	if last := got[len(got)-1].Date.Unix(); last != day(2026, 6, 15).Unix() {
		t.Errorf("last bar = %v, want 2026-06-15", got[len(got)-1].Date)
	}

	// A wider range inside the TTL is still a local read, and reaches back
	// past what the first caller asked for.
	all, err := c.GetHistory("AAPL", "max")
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.calls) != 1 {
		t.Errorf("second call inside TTL hit the provider: %v", inner.calls)
	}
	if len(all) <= len(got)*5 {
		t.Errorf("max returned %d bars vs 1y's %d; the store should hold the full decade", len(all), len(got))
	}
}

// The partial-bar trap: today's bar is stored mid-session, then revised. The
// next refresh must overwrite it, not mistake the revision for a split.
func TestTailRefreshOverwritesPartialBar(t *testing.T) {
	c, inner, clk := setup(t)
	if _, err := c.GetHistory("AAPL", "1y"); err != nil {
		t.Fatal(err)
	}

	last := len(inner.bars) - 1
	inner.bars[last].Close += 5 // session finished at a different price
	inner.bars = append(inner.bars, series(day(2026, 6, 16), day(2026, 6, 16), 1)...)
	clk.t = clk.t.AddDate(0, 0, 1)

	got, err := c.GetHistory("AAPL", "1y")
	if err != nil {
		t.Fatal(err)
	}
	if inner.callCount("10y") != 1 || inner.callCount("1mo") != 1 {
		t.Fatalf("want one 10y + one 1mo, got %v", inner.calls)
	}
	if got[len(got)-1].Date.Unix() != day(2026, 6, 16).Unix() {
		t.Errorf("new bar missing, last = %v", got[len(got)-1].Date)
	}
	if px := got[len(got)-2].Close; px != inner.bars[last].Close {
		t.Errorf("revised bar close = %v, want %v", px, inner.bars[last].Close)
	}
}

func TestSplitTriggersFullRefetch(t *testing.T) {
	c, inner, clk := setup(t)
	before, err := c.GetHistory("AAPL", "1y")
	if err != nil {
		t.Fatal(err)
	}

	inner.bars = series(day(2016, 6, 1), day(2026, 6, 16), 0.5) // 2:1 split, history restated
	clk.t = clk.t.AddDate(0, 0, 1)

	after, err := c.GetHistory("AAPL", "1y")
	if err != nil {
		t.Fatal(err)
	}
	if inner.callCount("10y") != 2 {
		t.Fatalf("split should force a second 10y pull, got %v", inner.calls)
	}
	if got, want := after[0].Close, before[0].Close/2; got < want*0.99 || got > want*1.01 {
		t.Errorf("old bars not restated: first close %v, want ~%v", got, want)
	}
}

func TestNoOverlapForcesFullRefetch(t *testing.T) {
	c, inner, clk := setup(t)
	if _, err := c.GetHistory("AAPL", "1y"); err != nil {
		t.Fatal(err)
	}
	inner.bars = series(day(2016, 6, 1), day(2026, 8, 20), 1)
	clk.t = day(2026, 8, 20).Add(12 * time.Hour) // stored bars end 2 months before the 1mo tail begins

	if _, err := c.GetHistory("AAPL", "1y"); err != nil {
		t.Fatal(err)
	}
	if inner.callCount("10y") != 2 {
		t.Errorf("a gap can hide a split; want a second 10y pull, got %v", inner.calls)
	}
}

func TestProviderDownServesStoredBars(t *testing.T) {
	c, inner, clk := setup(t)
	want, err := c.GetHistory("AAPL", "1y")
	if err != nil {
		t.Fatal(err)
	}
	inner.err = errors.New("yahoo down")
	clk.t = clk.t.Add(time.Hour)

	got, err := c.GetHistory("AAPL", "1y")
	if err != nil {
		t.Fatalf("stale store should still answer, got %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("got %d bars, want the %d already stored", len(got), len(want))
	}
}

func TestColdProviderErrorPropagates(t *testing.T) {
	c, inner, _ := setup(t)
	inner.err = errors.New("no data")
	if _, err := c.GetHistory("ZZZZ", "1y"); err == nil {
		t.Fatal("nothing stored and the provider failed: want an error")
	}
}

func TestUnsupportedRangeBypassesStore(t *testing.T) {
	c, inner, _ := setup(t)
	for _, r := range []string{"5d", "20y", "garbage"} {
		if _, err := c.GetHistory("AAPL", r); err != nil {
			t.Fatal(err)
		}
		if got := inner.calls[len(inner.calls)-1]; got != r {
			t.Errorf("range %q: provider saw %q, want passthrough", r, got)
		}
	}
	if inner.callCount("10y") != 0 {
		t.Errorf("passthrough must not fill the store, calls=%v", inner.calls)
	}
}

func TestStoreFailureFallsBackToProvider(t *testing.T) {
	c, inner, _ := setup(t)
	c.db.Close()
	got, err := c.GetHistory("AAPL", "1y")
	if err != nil || len(got) == 0 {
		t.Fatalf("broken store must not break the caller: %d bars, %v", len(got), err)
	}
	if inner.calls[len(inner.calls)-1] != "1y" {
		t.Errorf("fallback should pass the caller's own range, calls=%v", inner.calls)
	}
}

func TestConcurrentColdFetchHappensOnce(t *testing.T) {
	c, inner, _ := setup(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.GetHistory("AAPL", "1y"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := inner.callCount("10y"); n != 1 {
		t.Errorf("10y fetched %d times, want 1", n)
	}
}

func TestRangeStart(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want string // "" = unsupported
	}{
		{"", "2025-06-15"},
		{"1y", "2025-06-15"},
		{"3mo", "2026-03-15"},
		{"2y", "2024-06-15"},
		{"10y", "2016-06-15"},
		{"max", "2016-06-15"},
		{"ytd", "2026-01-01"},
		{"5d", ""},
		{"20y", ""},
		{"garbage", ""},
	}
	for _, tc := range cases {
		got, ok := rangeStart(tc.in, now)
		if (tc.want != "") != ok || (ok && got.Format(dateLayout) != tc.want) {
			t.Errorf("rangeStart(%q) = %v, %v; want %q", tc.in, got.Format(dateLayout), ok, tc.want)
		}
	}
}

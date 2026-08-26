package service

import (
	"strings"
	"testing"
	"time"

	"argus/internal/db"
	"argus/internal/market"
)

func TestSparklineEmpty(t *testing.T) {
	if got := Sparkline(nil); got != "" {
		t.Errorf("Sparkline(nil) = %q, want empty", got)
	}
}

func TestSparklineSinglePoint(t *testing.T) {
	got := Sparkline([]float64{1000})
	want := string(sparklineChars[len(sparklineChars)/2])
	if got != want {
		t.Errorf("Sparkline([1000]) = %q, want %q (mid-level char)", got, want)
	}
}

func TestSparklineFlatSeries(t *testing.T) {
	got := Sparkline([]float64{1000, 1000, 1000})
	want := strings.Repeat(string(sparklineChars[len(sparklineChars)/2]), 3)
	if got != want {
		t.Errorf("Sparkline(flat) = %q, want %q", got, want)
	}
}

func TestSparklineRange(t *testing.T) {
	got := Sparkline([]float64{0, 50, 100})
	runes := []rune(got)
	if len(runes) != 3 {
		t.Fatalf("Sparkline() = %q, want 3 runes", got)
	}
	if runes[0] != sparklineChars[0] {
		t.Errorf("Sparkline() first char = %q, want lowest level %q", string(runes[0]), string(sparklineChars[0]))
	}
	if runes[2] != sparklineChars[len(sparklineChars)-1] {
		t.Errorf("Sparkline() last char = %q, want highest level %q", string(runes[2]), string(sparklineChars[len(sparklineChars)-1]))
	}
}

func TestMaxDrawdownPctEmptyAndSinglePoint(t *testing.T) {
	if got := MaxDrawdownPct(nil); got != 0 {
		t.Errorf("MaxDrawdownPct(nil) = %v, want 0", got)
	}
	if got := MaxDrawdownPct([]float64{1000}); got != 0 {
		t.Errorf("MaxDrawdownPct(single point) = %v, want 0", got)
	}
}

func TestMaxDrawdownPctMonotonicallyUpIsZero(t *testing.T) {
	got := MaxDrawdownPct([]float64{1000, 1050, 1100, 1200})
	if got != 0 {
		t.Errorf("MaxDrawdownPct(monotonic up) = %v, want 0", got)
	}
}

func TestMaxDrawdownPctPicksWorstDipFromRunningPeak(t *testing.T) {
	// Peak 1200 -> trough 900 (25% drawdown) -> partial recovery to 1100
	// (still only a ~8.3% drawdown from 1200) -> the 25% dip must win, not
	// just a first-vs-last or last-seen-peak comparison.
	got := MaxDrawdownPct([]float64{1000, 1200, 900, 1100})
	want := 25.0
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("MaxDrawdownPct() = %v, want %v", got, want)
	}
}

func TestMonthRange(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)

	t.Run("ordinary month", func(t *testing.T) {
		now := time.Date(2026, time.July, 17, 9, 30, 0, 0, cst)
		from, to := MonthRange(now)
		if from != "2026-06-01" || to != "2026-06-30" {
			t.Errorf("MonthRange(2026-07-17) = %q, %q, want 2026-06-01, 2026-06-30", from, to)
		}
	})

	t.Run("january rolls back to december of the prior year", func(t *testing.T) {
		now := time.Date(2026, time.January, 1, 9, 30, 0, 0, cst)
		from, to := MonthRange(now)
		if from != "2025-12-01" || to != "2025-12-31" {
			t.Errorf("MonthRange(2026-01-01) = %q, %q, want 2025-12-01, 2025-12-31", from, to)
		}
	})
}

type mockMonthlyReportStore struct {
	points     []db.NetWorthPoint
	baseline   float64
	haveBase   bool
	count      int
	sellCount  int
	realized   float64
	benchFirst float64
	benchLast  float64
	haveBench  bool
}

func (m *mockMonthlyReportStore) GetNetWorthRange(from, to string, mk market.MarketID) ([]db.NetWorthPoint, error) {
	return m.points, nil
}

func (m *mockMonthlyReportStore) GetNetWorthOnOrBefore(date string, mk market.MarketID) (float64, bool, error) {
	return m.baseline, m.haveBase, nil
}

func (m *mockMonthlyReportStore) GetTransactionStatsByMarket(from, to string, mk market.MarketID) (int, int, float64, error) {
	return m.count, m.sellCount, m.realized, nil
}

func (m *mockMonthlyReportStore) GetSnapshotCloseRange(ticker, from, to string) (float64, float64, bool, error) {
	return m.benchFirst, m.benchLast, m.haveBench, nil
}

func TestBuildMonthlyReportBlockNoData(t *testing.T) {
	_, ok := BuildMonthlyReportBlock(&mockMonthlyReportStore{}, market.US, "2026-07-01", "2026-07-31", 0, false)
	if ok {
		t.Fatal("expected ok=false with no net worth snapshots")
	}
}

func TestBuildMonthlyReportBlockChangeAgainstPriorMonthBaseline(t *testing.T) {
	store := &mockMonthlyReportStore{
		points:   []db.NetWorthPoint{{Date: "2026-07-31", Total: 1100}},
		baseline: 1000,
		haveBase: true,
	}
	block, ok := BuildMonthlyReportBlock(store, market.US, "2026-07-01", "2026-07-31", 500, true)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !block.HaveChange || block.ChangePct != 10 {
		t.Errorf("ChangePct = %v (have=%v), want 10 (have=true)", block.ChangePct, block.HaveChange)
	}
	if !block.HaveCash || block.Cash != 500 {
		t.Errorf("Cash = %v (have=%v), want 500 (have=true)", block.Cash, block.HaveCash)
	}
}

func TestBuildMonthlyReportBlockFallsBackToFirstPointWithNoBaseline(t *testing.T) {
	store := &mockMonthlyReportStore{
		points: []db.NetWorthPoint{{Date: "2026-07-01", Total: 900}, {Date: "2026-07-31", Total: 1000}},
	}
	block, ok := BuildMonthlyReportBlock(store, market.US, "2026-07-01", "2026-07-31", 0, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := (1000.0 - 900.0) / 900.0 * 100
	if !block.HaveChange || block.ChangePct != want {
		t.Errorf("ChangePct = %v (have=%v), want %v (have=true)", block.ChangePct, block.HaveChange, want)
	}
}

func TestBuildMonthlyReportBlockSkipsChangeWithSinglePointAndNoBaseline(t *testing.T) {
	store := &mockMonthlyReportStore{
		points: []db.NetWorthPoint{{Date: "2026-07-31", Total: 1000}},
	}
	block, ok := BuildMonthlyReportBlock(store, market.US, "2026-07-01", "2026-07-31", 0, false)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if block.HaveChange {
		t.Errorf("expected HaveChange=false with no baseline and a single point, got ChangePct=%v", block.ChangePct)
	}
}

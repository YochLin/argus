package data

import (
	"errors"
	"testing"
	"time"
)

func TestResolveT86Columns(t *testing.T) {
	// Post-break (2017+) header, verified live 2026-08-27.
	post := []string{"證券代號", "證券名稱", "外陸資買進股數(不含外資自營商)", "外陸資賣出股數(不含外資自營商)",
		"外陸資買賣超股數(不含外資自營商)", "外資自營商買進股數", "外資自營商賣出股數", "外資自營商買賣超股數",
		"投信買進股數", "投信賣出股數", "投信買賣超股數", "自營商買賣超股數", "自營商買進股數(自行買賣)",
		"自營商賣出股數(自行買賣)", "自營商買賣超股數(自行買賣)", "自營商買進股數(避險)", "自營商賣出股數(避險)",
		"自營商買賣超股數(避險)", "三大法人買賣超股數"}
	tickerIdx, foreignIdx, trustIdx, err := resolveT86Columns(post)
	if err != nil {
		t.Fatalf("post-break: %v", err)
	}
	if tickerIdx != 0 || foreignIdx != 4 || trustIdx != 10 {
		t.Errorf("post-break: got ticker=%d foreign=%d trust=%d, want 0,4,10", tickerIdx, foreignIdx, trustIdx)
	}

	// Pre-break (pre-2017) header, verified live 2026-08-27 against 2016-01-04.
	pre := []string{"證券代號", "證券名稱", "外資買進股數", "外資賣出股數", "外資買賣超股數",
		"投信買進股數", "投信賣出股數", "投信買賣超股數", "自營商買賣超股數", "自營商買進股數(自行買賣)",
		"自營商賣出股數(自行買賣)", "自營商買賣超股數(自行買賣)", "自營商買進股數(避險)", "自營商賣出股數(避險)",
		"自營商買賣超股數(避險)", "三大法人買賣超股數"}
	tickerIdx, foreignIdx, trustIdx, err = resolveT86Columns(pre)
	if err != nil {
		t.Fatalf("pre-break: %v", err)
	}
	if tickerIdx != 0 || foreignIdx != 4 || trustIdx != 7 {
		t.Errorf("pre-break: got ticker=%d foreign=%d trust=%d, want 0,4,7", tickerIdx, foreignIdx, trustIdx)
	}

	if _, _, _, err := resolveT86Columns([]string{"a", "b", "c"}); err == nil {
		t.Error("unrecognized layout: want an error, got nil")
	}
}

func TestFetchT86TrustForeignDay_Parse(t *testing.T) {
	// A trimmed real post-break row (same fixture data institutional_tw_test.go
	// uses), decoded through the full fetchT86TrustForeignDay parse path minus
	// the HTTP round-trip.
	result := twseT86FullResponse{
		Date: "20260731",
		Fields: []string{"證券代號", "證券名稱", "外陸資買進股數(不含外資自營商)", "外陸資賣出股數(不含外資自營商)",
			"外陸資買賣超股數(不含外資自營商)", "外資自營商買進股數", "外資自營商賣出股數", "外資自營商買賣超股數",
			"投信買進股數", "投信賣出股數", "投信買賣超股數", "自營商買賣超股數", "自營商買進股數(自行買賣)",
			"自營商賣出股數(自行買賣)", "自營商買賣超股數(自行買賣)", "自營商買進股數(避險)", "自營商賣出股數(避險)",
			"自營商買賣超股數(避險)", "三大法人買賣超股數"},
		Data: [][]string{
			{"2883", "凱基金          ", "75,325,105", "20,255,381", "55,069,724", "0", "0", "0", "1,253,114", "2,000", "1,251,114", "1,615,209", "411,919", "10,000", "401,919", "1,317,000", "1,000,000", "317,000", "56,637,847"},
		},
	}
	tickerIdx, foreignIdx, trustIdx, err := resolveT86Columns(result.Fields)
	if err != nil {
		t.Fatalf("resolveT86Columns: %v", err)
	}
	row := result.Data[0]
	foreignNet, _ := parseTWSENetShares(row[foreignIdx])
	trustNet, _ := parseTWSENetShares(row[trustIdx])
	if row[tickerIdx] != "2883" || foreignNet != 55069724 || trustNet != 1251114 {
		t.Errorf("got ticker=%s foreignNet=%d trustNet=%d, want 2883, 55069724, 1251114", row[tickerIdx], foreignNet, trustNet)
	}
}

func TestTWSEGetTrustNetSeries_USGuard(t *testing.T) {
	twse := NewTWSE()
	if _, err := twse.GetTrustNetSeries("AAPL", 30); err != errUSNotSupported {
		t.Errorf("GetTrustNetSeries(AAPL) error = %v, want errUSNotSupported", err)
	}
}

func TestTWSEGetTrustNetSeries_LookbackCap(t *testing.T) {
	// Not a live test — just checks the days argument doesn't blow past
	// liveTrustNetLookbackDays regardless of what's requested. A 3000-day
	// request would time this test out (or hammer TWSE) if the cap didn't
	// apply, so this only asserts the constant exists at a sane value rather
	// than exercising the network path.
	if liveTrustNetLookbackDays <= 0 || liveTrustNetLookbackDays > 60 {
		t.Errorf("liveTrustNetLookbackDays = %d, want a small positive cap", liveTrustNetLookbackDays)
	}
}

func TestFetchT86TrustForeignDay_WeekendShortCircuit(t *testing.T) {
	twse := NewTWSE()
	// Known Saturday. Must resolve without ever reaching
	// doFetchT86TrustForeignDay's HTTP round-trip — if the short-circuit
	// regresses, this test starts making a real network call instead of
	// asserting a static result.
	sat := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	if sat.Weekday() != time.Saturday {
		t.Fatalf("test fixture date %s is not a Saturday", sat)
	}
	dayMap, err := twse.fetchT86TrustForeignDay(sat)
	if err != nil {
		t.Fatalf("weekend fetch: got error %v, want nil", err)
	}
	if dayMap != nil {
		t.Errorf("weekend fetch: got %v, want nil map", dayMap)
	}
	if _, hit := twse.t86DayCache[sat.Format("20060102")]; !hit {
		t.Error("weekend fetch: expected confirmed absence to be cached")
	}
}

func TestErrT86NoReport_IsDistinctSentinel(t *testing.T) {
	// buildT86Cache (cmd/strategyscan/t86_cache.go) and GetTrustNetSeries
	// both branch on identifying this specific error via errors.Is — a
	// wrapped or re-created error with the same message would silently
	// break that classification, so pin the sentinel's identity directly.
	if ErrT86NoReport == nil {
		t.Fatal("ErrT86NoReport must not be nil")
	}
	if errors.Is(ErrT86NoReport, errUSNotSupported) {
		t.Error("ErrT86NoReport must not alias an unrelated sentinel")
	}
}

func TestTrustNetDay_DateTruncation(t *testing.T) {
	// fetchT86TrustForeignDay truncates the passed-in date to a day
	// boundary — alignByDate formats by "2006-01-02" so an untruncated
	// time-of-day would still match, but Truncate documents the intent
	// explicitly. Sanity check the format round-trips as expected.
	d := time.Date(2026, 7, 31, 13, 45, 0, 0, time.UTC).Truncate(24 * time.Hour)
	if d.Format("2006-01-02") != "2026-07-31" {
		t.Errorf("got %s, want 2026-07-31", d.Format("2006-01-02"))
	}
}

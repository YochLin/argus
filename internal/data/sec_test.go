package data

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

func flatPriceCandles(t *testing.T, dateToClose map[string]float64) []Candle {
	t.Helper()
	out := make([]Candle, 0, len(dateToClose))
	for d, c := range dateToClose {
		out = append(out, Candle{Date: mustParseDate(t, d), Open: c, High: c, Low: c, Close: c})
	}
	// sort ascending by date, same contract as GetHistory
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Date.Before(out[i].Date) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func fakeFacts(t *testing.T, tag string, points map[string]float64) *secCompanyFacts {
	t.Helper()
	facts := &secCompanyFacts{}
	facts.Facts.USGAAP = make(map[string]struct {
		Units map[string][]secFactPoint `json:"units"`
	})
	var pts []secFactPoint
	for end, val := range points {
		pts = append(pts, secFactPoint{End: end, Val: val, Form: "10-K", Filed: end})
	}
	entry := facts.Facts.USGAAP[tag]
	entry.Units = map[string][]secFactPoint{"USD": pts}
	facts.Facts.USGAAP[tag] = entry
	return facts
}

func TestComputeFundamentalSnapshot_NoData(t *testing.T) {
	facts := &secCompanyFacts{}
	if got := computeFundamentalSnapshot("ZZZZ", facts, nil); got != nil {
		t.Errorf("computeFundamentalSnapshot() = %+v, want nil for a ticker with no usable tags", got)
	}
}

func TestComputeFundamentalSnapshot_EPSAndPE(t *testing.T) {
	facts := fakeFacts(t, "EarningsPerShareDiluted", map[string]float64{
		"2021-12-31": 2.0,
		"2022-12-31": 2.5,
		"2023-12-31": 3.0,
		"2024-12-31": 4.0, // latest fiscal year
	})
	candles := flatPriceCandles(t, map[string]float64{
		"2021-12-31": 20,  // PE 10
		"2022-12-31": 50,  // PE 20
		"2023-12-31": 90,  // PE 30
		"2024-12-31": 200, // PE 50 (historical, for the percentile pool)
		"2025-06-01": 400, // most recent close -> "current" price used for snap.PERatio
	})

	got := computeFundamentalSnapshot("AAPL", facts, candles)
	if got == nil {
		t.Fatal("computeFundamentalSnapshot() = nil, want a snapshot")
	}
	if got.EPSAnnual != 4.0 {
		t.Errorf("EPSAnnual = %v, want 4.0 (latest fiscal year)", got.EPSAnnual)
	}
	if got.AsOfFiscalYearEnd != "2024-12-31" {
		t.Errorf("AsOfFiscalYearEnd = %v, want 2024-12-31", got.AsOfFiscalYearEnd)
	}
	wantPE := 400.0 / 4.0 // current price / latest EPS, NOT the fiscal-year-end price
	if got.PERatio != wantPE {
		t.Errorf("PERatio = %v, want %v (current price, not fiscal-year-end price)", got.PERatio, wantPE)
	}
	if got.PEPercentile == nil {
		t.Fatal("PEPercentile = nil, want a value (4 profitable fiscal years priced)")
	}
	// historical PEs are [10, 20, 30, 50]; current PE is 100 -> above every
	// historical point -> 100th percentile.
	if *got.PEPercentile != 100.0 {
		t.Errorf("PEPercentile = %v, want 100.0 (current PE exceeds every historical PE)", *got.PEPercentile)
	}
}

func TestComputeFundamentalSnapshot_TooFewPointsForPercentile(t *testing.T) {
	facts := fakeFacts(t, "EarningsPerShareDiluted", map[string]float64{
		"2023-12-31": 2.0,
		"2024-12-31": 3.0,
	})
	candles := flatPriceCandles(t, map[string]float64{
		"2023-12-31": 20,
		"2024-12-31": 30,
		"2025-06-01": 60,
	})
	got := computeFundamentalSnapshot("NEWCO", facts, candles)
	if got == nil {
		t.Fatal("computeFundamentalSnapshot() = nil, want a snapshot (EPS/PE still computable)")
	}
	if got.PEPercentile != nil {
		t.Errorf("PEPercentile = %v, want nil (only 2 priced fiscal years, below minPEHistoryPoints)", *got.PEPercentile)
	}
	if got.PERatio <= 0 {
		t.Errorf("PERatio = %v, want > 0", got.PERatio)
	}
}

func TestComputeFundamentalSnapshot_CashFlowQuality(t *testing.T) {
	facts := &secCompanyFacts{}
	facts.Facts.USGAAP = make(map[string]struct {
		Units map[string][]secFactPoint `json:"units"`
	})
	ocf := facts.Facts.USGAAP["NetCashProvidedByUsedInOperatingActivities"]
	ocf.Units = map[string][]secFactPoint{"USD": {{End: "2024-12-31", Val: 120, Form: "10-K", Filed: "2025-01-01"}}}
	facts.Facts.USGAAP["NetCashProvidedByUsedInOperatingActivities"] = ocf

	ni := facts.Facts.USGAAP["NetIncomeLoss"]
	ni.Units = map[string][]secFactPoint{"USD": {{End: "2024-12-31", Val: 100, Form: "10-K", Filed: "2025-01-01"}}}
	facts.Facts.USGAAP["NetIncomeLoss"] = ni

	got := computeFundamentalSnapshot("QUALCO", facts, nil)
	if got == nil {
		t.Fatal("computeFundamentalSnapshot() = nil, want a snapshot")
	}
	if got.CashFlowQuality == nil {
		t.Fatal("CashFlowQuality = nil, want 1.2 (120/100)")
	}
	if *got.CashFlowQuality != 1.2 {
		t.Errorf("CashFlowQuality = %v, want 1.2", *got.CashFlowQuality)
	}
}

func TestComputeFundamentalSnapshot_MismatchedPeriodsSkipsQuality(t *testing.T) {
	facts := &secCompanyFacts{}
	facts.Facts.USGAAP = make(map[string]struct {
		Units map[string][]secFactPoint `json:"units"`
	})
	ocf := facts.Facts.USGAAP["NetCashProvidedByUsedInOperatingActivities"]
	ocf.Units = map[string][]secFactPoint{"USD": {{End: "2024-12-31", Val: 120, Form: "10-K", Filed: "2025-01-01"}}}
	facts.Facts.USGAAP["NetCashProvidedByUsedInOperatingActivities"] = ocf

	ni := facts.Facts.USGAAP["NetIncomeLoss"]
	ni.Units = map[string][]secFactPoint{"USD": {{End: "2023-12-31", Val: 100, Form: "10-K", Filed: "2024-01-01"}}}
	facts.Facts.USGAAP["NetIncomeLoss"] = ni

	got := computeFundamentalSnapshot("MISMATCH", facts, nil)
	if got != nil && got.CashFlowQuality != nil {
		t.Errorf("CashFlowQuality = %v, want nil when OCF/NetIncome don't share a fiscal year end", *got.CashFlowQuality)
	}
}

func TestGetFundamentalSnapshot_CIKLookupAndFetch(t *testing.T) {
	factsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("request missing User-Agent header")
		}
		facts := fakeFacts(t, "EarningsPerShareDiluted", map[string]float64{"2024-12-31": 5.0})
		json.NewEncoder(w).Encode(facts)
	}))
	defer factsSrv.Close()

	tickerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"0":{"cik_str":320193,"ticker":"AAPL"}}`))
	}))
	defer tickerSrv.Close()

	s := NewSEC("test-agent test@example.com")
	s.baseURL = factsSrv.URL
	s.tickerURL = tickerSrv.URL

	got, err := s.GetFundamentalSnapshot("AAPL", nil)
	if err != nil {
		t.Fatalf("GetFundamentalSnapshot() error = %v", err)
	}
	if got == nil || got.EPSAnnual != 5.0 {
		t.Errorf("GetFundamentalSnapshot() = %+v, want EPSAnnual 5.0", got)
	}

	if _, err := s.GetFundamentalSnapshot("NOSUCHTICKER", nil); err == nil {
		t.Error("GetFundamentalSnapshot(unknown ticker) error = nil, want an error")
	}
}

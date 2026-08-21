package data

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFinnhubTWGuard confirms every per-ticker Finnhub method rejects a TW
// ticker before making any HTTP request (no apiKey set, so a real request
// would fail loudly rather than silently — if any of these reach the
// network the test would hang/timeout instead of returning errTWNotSupported
// immediately).
func TestFinnhubTWGuard(t *testing.T) {
	f := NewFinnhub("")

	if _, err := f.GetQuote("2330"); err != errTWNotSupported {
		t.Errorf("GetQuote(2330) error = %v, want errTWNotSupported", err)
	}
	if _, err := f.GetNews("2330", 5); err != errTWNotSupported {
		t.Errorf("GetNews(2330) error = %v, want errTWNotSupported", err)
	}
	if _, err := f.GetFundamentals("2330"); err != errTWNotSupported {
		t.Errorf("GetFundamentals(2330) error = %v, want errTWNotSupported", err)
	}
	if _, err := f.GetFinancialStatements("2330", "annual"); err != errTWNotSupported {
		t.Errorf("GetFinancialStatements(2330) error = %v, want errTWNotSupported", err)
	}
	if _, err := f.GetAnalystRating("2330"); err != errTWNotSupported {
		t.Errorf("GetAnalystRating(2330) error = %v, want errTWNotSupported", err)
	}
	if _, err := f.GetSector("2330"); err != errTWNotSupported {
		t.Errorf("GetSector(2330) error = %v, want errTWNotSupported", err)
	}
	if _, err := f.GetEarningsSurprises("2330"); err != errTWNotSupported {
		t.Errorf("GetEarningsSurprises(2330) error = %v, want errTWNotSupported", err)
	}
}

// TestFinnhubGetEarningsSurprises fixture mirrors a real /stock/earnings
// response live-curled 2026-08-20 (AAPL) — deliberately out of
// newest-first order to confirm the method sorts oldest-first itself
// rather than trusting Finnhub's response ordering.
func TestFinnhubGetEarningsSurprises(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("symbol"); got != "AAPL" {
			t.Errorf("symbol = %q, want AAPL", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"symbol":"AAPL","estimate":1.9271,"actual":1.91,"period":"2026-06-30","surprise":-0.0171,"surprisePercent":-0.8873},
			{"symbol":"AAPL","estimate":1.8075,"actual":1.85,"period":"2025-09-30","surprise":0.0425,"surprisePercent":2.3513},
			{"symbol":"AAPL","estimate":1.9884,"actual":2.01,"period":"2026-03-31","surprise":0.0216,"surprisePercent":1.0863}
		]`))
	}))
	defer srv.Close()

	f := NewFinnhub("")
	f.baseURL = srv.URL

	got, err := f.GetEarningsSurprises("AAPL")
	if err != nil {
		t.Fatalf("GetEarningsSurprises: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantPeriods := []string{"2025-09-30", "2026-03-31", "2026-06-30"}
	for i, w := range wantPeriods {
		if got[i].Period != w {
			t.Errorf("got[%d].Period = %q, want %q (oldest-first)", i, got[i].Period, w)
		}
	}
	if got[2].SurprisePct != -0.8873 {
		t.Errorf("got[2].SurprisePct = %v, want -0.8873", got[2].SurprisePct)
	}
}

// TestFinnhubGetSector confirms finnhubIndustry and marketCapitalization are
// both extracted from a single /stock/profile2 response — fixture mirrors a
// real response live-curled 2026-08-16 (AAPL -> "Technology").
func TestFinnhubGetSector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("symbol"); got != "AAPL" {
			t.Errorf("symbol = %q, want AAPL", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ticker":"AAPL","name":"Apple Inc","finnhubIndustry":"Technology","marketCapitalization":4460857.1}`))
	}))
	defer srv.Close()

	f := NewFinnhub("")
	f.baseURL = srv.URL

	info, err := f.GetSector("AAPL")
	if err != nil {
		t.Fatalf("GetSector: %v", err)
	}
	if info.Industry != "Technology" {
		t.Errorf("GetSector(AAPL).Industry = %q, want Technology", info.Industry)
	}
	if info.MarketCapMillion != 4460857.1 {
		t.Errorf("GetSector(AAPL).MarketCapMillion = %v, want 4460857.1", info.MarketCapMillion)
	}
}

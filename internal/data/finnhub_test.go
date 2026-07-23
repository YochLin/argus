package data

import "testing"

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
}

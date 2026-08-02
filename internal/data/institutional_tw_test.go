package data

import "testing"

func TestTWSEGetInstitutionalFlow_USGuard(t *testing.T) {
	twse := NewTWSE()
	if _, err := twse.GetInstitutionalFlow("AAPL"); err != errUSNotSupported {
		t.Errorf("GetInstitutionalFlow(AAPL) error = %v, want errUSNotSupported", err)
	}
}

func TestFindInstitutionalFlowRow(t *testing.T) {
	// Real T86 field order, trimmed to two rows and the columns this parses.
	result := twseT86Response{
		Date: "20260731",
		Data: [][]string{
			{"00685L", "群益臺灣加權正2", "5,743,479", "8,228,000", "-2,484,521", "0", "0", "0", "0", "0", "0", "152,261,703", "0", "1,928,000", "-1,928,000", "261,095,903", "106,906,200", "154,189,703", "149,777,182"},
			{"2883", "凱基金          ", "75,325,105", "20,255,381", "55,069,724", "0", "0", "0", "1,253,114", "2,000", "1,251,114", "1,615,209", "411,919", "10,000", "401,919", "1,317,000", "1,000,000", "317,000", "56,637,847"},
		},
	}

	got := findInstitutionalFlowRow("2883", result)
	if got == nil {
		t.Fatal("findInstitutionalFlowRow(2883) = nil, want a row")
	}
	if got.Date != "2026-07-31" {
		t.Errorf("Date = %q, want 2026-07-31", got.Date)
	}
	if got.ForeignNet != 55069724 || got.TrustNet != 1251114 || got.DealerNet != 1615209 || got.TotalNet != 56637847 {
		t.Errorf("findInstitutionalFlowRow(2883) = %+v, want ForeignNet=55069724 TrustNet=1251114 DealerNet=1615209 TotalNet=56637847", got)
	}

	if got := findInstitutionalFlowRow("9999", result); got != nil {
		t.Errorf("findInstitutionalFlowRow(9999) = %+v, want nil (not in this session's data)", got)
	}
}

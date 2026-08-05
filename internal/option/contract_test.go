package option

import (
	"testing"
	"time"
)

func TestParseFormatRoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		occ        string
		underlying string
		right      Right
		expiry     string // YYYY-MM-DD
		strike     float64
	}{
		{"AAPL call", "AAPL260805C00310000", "AAPL", Call, "2026-08-05", 310},
		{"decimal strike", "AAPL260805C00007500", "AAPL", Call, "2026-08-05", 7.5},
		{"5-char underlying", "GOOGL260918P00150000", "GOOGL", Put, "2026-09-18", 150},
		{"6-char underlying", "ABCDEF260918C00050000", "ABCDEF", Call, "2026-09-18", 50},
		{"lowercase input", "aapl260805c00310000", "AAPL", Call, "2026-08-05", 310},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.occ)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.occ, err)
			}
			wantExpiry, _ := time.Parse("2006-01-02", c.expiry)
			if got.Underlying != c.underlying || got.Right != c.right || got.Strike != c.strike || !got.Expiry.Equal(wantExpiry) {
				t.Fatalf("Parse(%q) = %+v, want underlying=%s right=%s expiry=%s strike=%v",
					c.occ, got, c.underlying, c.right, c.expiry, c.strike)
			}

			formatted := Format(c.underlying, c.right, wantExpiry, c.strike)
			reparsed, err := Parse(formatted)
			if err != nil {
				t.Fatalf("Parse(Format(...)) error: %v", err)
			}
			if reparsed != got {
				t.Fatalf("round trip mismatch: parsed=%+v reparsed=%+v (formatted=%s)", got, reparsed, formatted)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []string{
		"", "AAPL", "AAPL260805X00310000", "AAPL2608X5C00310000",
		"AAPL260805C0031000", "1234260805C00310000", "AAAAAAA260805C00310000",
	}
	for _, occ := range cases {
		if _, err := Parse(occ); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", occ)
		}
	}
}

func TestIsOCC(t *testing.T) {
	if !IsOCC("AAPL260805C00310000") {
		t.Error("IsOCC(valid OCC) = false, want true")
	}
	if IsOCC("AAPL") {
		t.Error("IsOCC(plain ticker) = true, want false")
	}
}

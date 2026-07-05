package data

import "testing"

func TestPeriodEndDate(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2025-09-27 00:00:00", "2025-09-27"},
		{"2025-09-27", "2025-09-27"},
		{"", ""},
		{"2025", "2025"},
	}
	for _, tt := range tests {
		if got := periodEndDate(tt.in); got != tt.want {
			t.Errorf("periodEndDate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

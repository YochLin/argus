package data

import "testing"

func TestIsUSEquitySymbol(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"AAPL", true},
		{"MSFT", true},
		{"A", true},
		{"GOOGL", true}, // 5 chars, still plain letters
		{"", false},
		{"TOOLONG", false},   // > 5 chars
		{"BTC-USD", false},   // crypto pair
		{"SHOP.TO", false},   // foreign listing
		{"brk.b", false},     // lowercase, punctuation
		{"BRK.B", false},     // punctuation even if otherwise plain
		// Known limitation documented in CLAUDE.md: a plain 1-5 uppercase
		// letter symbol can't be distinguished from a real equity ticker by
		// shape alone, so a stablecoin ticker like USDE slips through. This
		// test pins down that this is current, known behavior rather than
		// an accident.
		{"USDE", true},
	}

	for _, tt := range tests {
		if got := IsUSEquitySymbol(tt.symbol); got != tt.want {
			t.Errorf("IsUSEquitySymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
		}
	}
}

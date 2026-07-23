package market

import "testing"

func TestOf(t *testing.T) {
	cases := []struct {
		ticker string
		want   MarketID
	}{
		{"AAPL", US},
		{"TSLA", US},
		{"", US},
		{"2330", TW},
		{"0050", TW},
		{"00679B", TW},
		{"5274", TW},
	}
	for _, c := range cases {
		if got := Of(c.ticker); got != c.want {
			t.Errorf("Of(%q) = %q, want %q", c.ticker, got, c.want)
		}
	}
}

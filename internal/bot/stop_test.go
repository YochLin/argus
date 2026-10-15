package bot

import "testing"

func TestParseStopArgs(t *testing.T) {
	t.Run("ticker only", func(t *testing.T) {
		ticker, price, hasPrice, err := parseStopArgs("aapl")
		if err != nil {
			t.Fatalf("parseStopArgs() error = %v", err)
		}
		if ticker != "AAPL" || price != 0 || hasPrice {
			t.Errorf("parseStopArgs() = %q, %v, %v; want AAPL, 0, false", ticker, price, hasPrice)
		}
	})

	t.Run("ticker and price", func(t *testing.T) {
		ticker, price, hasPrice, err := parseStopArgs("aapl 190.5")
		if err != nil {
			t.Fatalf("parseStopArgs() error = %v", err)
		}
		if ticker != "AAPL" || price != 190.5 || !hasPrice {
			t.Errorf("parseStopArgs() = %q, %v, %v; want AAPL, 190.5, true", ticker, price, hasPrice)
		}
	})

	for _, args := range []string{"", "AAPL 190 extra", "AAPL 0", "AAPL -5", "AAPL abc"} {
		if _, _, _, err := parseStopArgs(args); err == nil {
			t.Errorf("parseStopArgs(%q) error = nil, want error", args)
		}
	}
}

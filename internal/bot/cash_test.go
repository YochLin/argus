package bot

import (
	"testing"

	"argus/internal/market"
)

func TestParseCashArgs(t *testing.T) {
	t.Run("bare amount defaults to USD", func(t *testing.T) {
		m, amount, err := parseCashArgs("1000")
		if err != nil || m != market.US || amount != 1000 {
			t.Errorf("parseCashArgs(1000) = %v, %v, %v; want us, 1000, nil", m, amount, err)
		}
	})

	t.Run("explicit usd", func(t *testing.T) {
		m, amount, err := parseCashArgs("usd 500.50")
		if err != nil || m != market.US || amount != 500.50 {
			t.Errorf("parseCashArgs(usd 500.50) = %v, %v, %v; want us, 500.5, nil", m, amount, err)
		}
	})

	t.Run("explicit twd is case-insensitive", func(t *testing.T) {
		m, amount, err := parseCashArgs("TWD 30000")
		if err != nil || m != market.TW || amount != 30000 {
			t.Errorf("parseCashArgs(TWD 30000) = %v, %v, %v; want tw, 30000, nil", m, amount, err)
		}
	})

	for _, args := range []string{"", "gbp 100", "usd", "usd 100 extra", "usd -5", "usd abc"} {
		if _, _, err := parseCashArgs(args); err == nil {
			t.Errorf("parseCashArgs(%q) error = nil, want error", args)
		}
	}
}

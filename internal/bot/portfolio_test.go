package bot

import (
	"testing"

	"argus/internal/i18n"
	"argus/internal/market"
)

func TestLotSuffix(t *testing.T) {
	cases := []struct {
		name   string
		m      market.MarketID
		shares float64
		want   string
	}{
		{"US never gets a lot note", market.US, 1000, ""},
		{"TW under one lot", market.TW, 500, ""},
		{"TW exact multiple of a lot", market.TW, 2000, i18n.T(i18n.EN, i18n.KeyPortfolioLotSuffix, 2)},
		{"TW non-round share count", market.TW, 1500, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lotSuffix(i18n.EN, c.m, c.shares); got != c.want {
				t.Errorf("lotSuffix(%s, %v) = %q, want %q", c.m, c.shares, got, c.want)
			}
		})
	}
}

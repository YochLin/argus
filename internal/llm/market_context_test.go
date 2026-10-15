package llm

import (
	"strings"
	"testing"

	"argus/internal/i18n"
)

func TestRegimeLabel(t *testing.T) {
	tests := []struct {
		name         string
		price, ma200 float64
		want         i18n.Key
	}{
		{"above MA200 is risk-on", 500, 480, i18n.KeyRiskOn},
		{"below MA200 is risk-off", 460, 480, i18n.KeyRiskOff},
		{"exactly at MA200 is risk-off", 480, 480, i18n.KeyRiskOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := regimeLabel(tt.price, tt.ma200); got != tt.want {
				t.Errorf("regimeLabel(%v, %v) = %v, want %v", tt.price, tt.ma200, got, tt.want)
			}
		})
	}
}

func TestVIXLabel(t *testing.T) {
	tests := []struct {
		name string
		vix  float64
		want i18n.Key
	}{
		{"calm below threshold", 12, i18n.KeyVIXCalm},
		{"just under calm threshold", 14.9, i18n.KeyVIXCalm},
		{"normal at calm boundary", 15, i18n.KeyVIXNormal},
		{"normal mid-range", 20, i18n.KeyVIXNormal},
		{"normal at panic boundary", 25, i18n.KeyVIXNormal},
		{"panic above threshold", 30, i18n.KeyVIXPanic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vixLabel(tt.vix); got != tt.want {
				t.Errorf("vixLabel(%v) = %v, want %v", tt.vix, got, tt.want)
			}
		})
	}
}

func TestWriteMarketContext(t *testing.T) {
	t.Run("nil market renders nothing", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, nil)
		if sb.String() != "" {
			t.Errorf("writeMarketContext(nil) = %q, want empty", sb.String())
		}
	})

	t.Run("both zero renders nothing", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{})
		if sb.String() != "" {
			t.Errorf("writeMarketContext(all-zero) = %q, want empty", sb.String())
		}
	})

	t.Run("SPY only omits VIX line", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{Bench: "SPY", SPYPrice: 500, SPYMA50: 490, SPYMA200: 480})
		got := sb.String()
		if !strings.Contains(got, "SPY price") {
			t.Errorf("writeMarketContext() missing SPY line, got:\n%s", got)
		}
		if strings.Contains(got, "VIX") {
			t.Errorf("writeMarketContext() should omit VIX line when VIX is 0, got:\n%s", got)
		}
	})

	t.Run("VIX only omits SPY line", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{VIX: 18})
		got := sb.String()
		if strings.Contains(got, "SPY price") {
			t.Errorf("writeMarketContext() should omit SPY line when SPY data is 0, got:\n%s", got)
		}
		if !strings.Contains(got, "VIX") {
			t.Errorf("writeMarketContext() missing VIX line, got:\n%s", got)
		}
	})

	t.Run("full context renders both lines with labels", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{Bench: "SPY", SPYPrice: 500, SPYMA50: 490, SPYMA200: 480, VIX: 30})
		got := sb.String()
		if !strings.Contains(got, "risk-on") {
			t.Errorf("writeMarketContext() missing risk-on label, got:\n%s", got)
		}
		if !strings.Contains(got, "high panic") {
			t.Errorf("writeMarketContext() missing high-panic label, got:\n%s", got)
		}
	})

	t.Run("VolProxyPct renders TW's volatility-proxy line instead of VIX", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{Bench: "0050", SPYPrice: 45, SPYMA50: 44, SPYMA200: 43, VolProxyPct: 2.5})
		got := sb.String()
		if !strings.Contains(got, "Volatility proxy") {
			t.Errorf("writeMarketContext() missing volatility-proxy line, got:\n%s", got)
		}
		if strings.Contains(got, "VIX") {
			t.Errorf("writeMarketContext() should not render a VIX line when only VolProxyPct is set, got:\n%s", got)
		}
	})

	t.Run("VolProxyPct zero omits the volatility-proxy line", func(t *testing.T) {
		var sb strings.Builder
		writeMarketContext(&sb, i18n.EN, &MarketContext{Bench: "0050", SPYPrice: 45, SPYMA50: 44, SPYMA200: 43})
		if strings.Contains(sb.String(), "Volatility proxy") {
			t.Errorf("writeMarketContext() should omit volatility-proxy line when VolProxyPct is 0, got:\n%s", sb.String())
		}
	})
}

package bot

import (
	"strings"
	"testing"

	"argus/internal/data"
	"argus/internal/i18n"
	"argus/internal/service"
)

func TestFormatMomentumWatch(t *testing.T) {
	on := service.MomentumRegime{On: true, MA10: 2, MA10Prev: 1, MA20: 2, MA20Prev: 1}
	w := &service.MomentumWatch{Regime: on, Picks: []service.MomentumPick{
		{Ticker: "COIN", Pivot: 345.2, ADRPct: 5.1, Broke: true, Entry: 350, StopPct: 6,
			Quote: &data.Quote{Price: 352.1, Open: 350, Low: 329}},
		{Ticker: "AXON", Pivot: 612, ADRPct: 3.4, Mom3mPct: 40, BaseDays: 12, Quote: &data.Quote{Price: 598.4}},
	}}
	text, lines := formatMomentumWatch(i18n.ZH, "2026-10-27", w)
	for _, want := range []string{"已突破（1）", "（開盤就突破）", "⚠️ 超過 ADR", "候選，還沒突破（1）", "差 2.2%", "未驗證"} {
		if !strings.Contains(text, want) {
			t.Errorf("message lacks %q:\n%s", want, text)
		}
	}
	if !strings.HasPrefix(lines["AXON"], "• AXON") || len(lines) != 2 {
		t.Errorf("per-pick lines %v", lines)
	}

	text, _ = formatMomentumWatch(i18n.ZH, "2026-10-27", &service.MomentumWatch{})
	if !strings.Contains(text, "今天不做") {
		t.Errorf("regime-off message:\n%s", text)
	}
}

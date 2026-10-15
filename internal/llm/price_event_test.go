package llm

import (
	"strings"
	"testing"

	"argus/internal/i18n"
)

func TestBuildPriceEventPromptRendersEveryNumberAndMarksTheTrigger(t *testing.T) {
	// Opened -6% (the trigger), bought back all day, closed -0.4%: the
	// contrast Phase 20 後續 PR3 exists to put in front of the model.
	ev := PriceEventFacts{Ticker: "AAPL", GapPct: -6, ChangePct: -0.4, CumulativePct: -3.2, GapTriggered: true}
	got := buildPriceEventPrompt(i18n.EN, ev, nil)

	for _, want := range []string{"-6.0%", "-0.4%", "-3.2%"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildPriceEventPrompt() missing %q, got:\n%s", want, got)
		}
	}
	marker := i18n.T(i18n.EN, i18n.KeyPriceEventTriggerSuffix)
	if n := strings.Count(got, marker); n != 1 {
		t.Errorf("trigger marker appears %d times, want exactly 1 (the gap), got:\n%s", n, got)
	}
	if !strings.Contains(got, i18n.T(i18n.EN, i18n.KeyPriceEventGapLine, -6.0, marker)) {
		t.Errorf("trigger marker is not on the gap line, got:\n%s", got)
	}
}

func TestBuildPriceEventPromptKeepsAFlatCloseVisible(t *testing.T) {
	// A 0.0% close is a fact, not missing data — the change line renders
	// unconditionally, unlike gap/cumulative where 0 means "unavailable".
	ev := PriceEventFacts{Ticker: "AAPL", GapPct: -6, GapTriggered: true}
	got := buildPriceEventPrompt(i18n.EN, ev, nil)
	if !strings.Contains(got, i18n.T(i18n.EN, i18n.KeyPriceEventChangeLine, 0.0, "")) {
		t.Errorf("buildPriceEventPrompt() dropped the flat close line, got:\n%s", got)
	}
	if strings.Contains(got, i18n.T(i18n.EN, i18n.KeyPriceEventCumulativeLine, 0.0, "")) {
		t.Errorf("buildPriceEventPrompt() rendered an unavailable cumulative number, got:\n%s", got)
	}
}

package bot

import (
	"testing"

	"argus/internal/i18n"
)

func TestTickerActionButtons(t *testing.T) {
	buttons := tickerActionButtons(i18n.EN, "AAPL")

	if len(buttons) != 3 {
		t.Fatalf("tickerActionButtons() = %+v, want exactly 3 buttons", buttons)
	}

	wantPrefixes := []string{callbackCheckPrefix, callbackBuyPrefix, callbackSellPrefix}
	for i, btn := range buttons {
		want := wantPrefixes[i] + "AAPL"
		if btn.Data != want {
			t.Errorf("button %d Data = %q, want %q", i, btn.Data, want)
		}
	}
}

// TestQuickActionPrefixesDontCollideWithPendingActions locks in the
// "different namespace, no collision" claim from PLAN.md: a quick-action
// callback_data must never be mistaken for a pending-action confirm/reject
// one by parseCallbackData (pending_actions.go), or handleCallbackQuery's
// prefix checks would race with it.
func TestQuickActionPrefixesDontCollideWithPendingActions(t *testing.T) {
	for _, data := range []string{callbackCheckPrefix + "AAPL", callbackBuyPrefix + "AAPL", callbackSellPrefix + "AAPL"} {
		if _, _, ok := parseCallbackData(data); ok {
			t.Errorf("parseCallbackData(%q) unexpectedly succeeded — quick-action prefix collides with pending-action namespace", data)
		}
	}
}

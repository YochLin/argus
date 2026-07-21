package bot

import (
	"testing"

	"argus/internal/i18n"
)

func TestTickerActionKeyboard(t *testing.T) {
	kb := tickerActionKeyboard(i18n.EN, "AAPL")

	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 3 {
		t.Fatalf("tickerActionKeyboard() rows = %+v, want exactly one row of 3 buttons", kb.InlineKeyboard)
	}

	row := kb.InlineKeyboard[0]
	wantPrefixes := []string{callbackCheckPrefix, callbackBuyPrefix, callbackSellPrefix}
	for i, btn := range row {
		if btn.CallbackData == nil {
			t.Fatalf("button %d has nil CallbackData", i)
		}
		want := wantPrefixes[i] + "AAPL"
		if *btn.CallbackData != want {
			t.Errorf("button %d CallbackData = %q, want %q", i, *btn.CallbackData, want)
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

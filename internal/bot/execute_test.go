package bot

import (
	"context"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
)

func feePtr(v float64) *float64 { return &v }

// TestExecuteBuyValidation covers the input-validation branch shared with
// internal/web's POST /api/trade/buy (see ExecuteBuy) — invalid input never
// reaches db.RecordBuy, and still gets the same usage text /buy itself
// would send on a malformed command.
func TestExecuteBuyValidation(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)

	for _, tt := range []struct {
		name   string
		ticker string
		shares float64
		price  float64
		fee    *float64
	}{
		{"empty ticker", "", 10, 100, feePtr(0)},
		{"zero shares", "AAPL", 0, 100, feePtr(0)},
		{"negative shares", "AAPL", -1, 100, feePtr(0)},
		{"zero price", "AAPL", 10, 0, feePtr(0)},
		{"negative fee", "AAPL", 10, 100, feePtr(-1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := b.ExecuteBuy(tt.ticker, tt.shares, tt.price, tt.fee, "2026-07-01")
			if err == nil {
				t.Errorf("ExecuteBuy() error = nil, want an error for invalid input")
			}
			if want := i18n.T(i18n.EN, i18n.KeyBuyUsage); msg != want {
				t.Errorf("ExecuteBuy() msg = %q, want %q", msg, want)
			}
		})
	}
}

func TestExecuteBuySuccess(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	msg, err := b.ExecuteBuy("AAPL", 10, 200, feePtr(1), "2026-07-01")
	if err != nil {
		t.Fatalf("ExecuteBuy() error = %v", err)
	}
	if msg == "" {
		t.Errorf("ExecuteBuy() msg = %q, want a non-empty confirmation", msg)
	}

	pos, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok {
		t.Fatalf("GetPosition() = %v, %v, %v, want a recorded position", pos, ok, err)
	}
	if pos.Shares != 10 {
		t.Errorf("GetPosition().Shares = %v, want 10", pos.Shares)
	}

	watchlist, err := d.GetWatchlist()
	if err != nil || len(watchlist) != 1 || watchlist[0] != "AAPL" {
		t.Errorf("GetWatchlist() = %v, %v, want [AAPL] (buy auto-adds to watchlist)", watchlist, err)
	}

	fc := b.channel.(*fakeChannel)
	if len(fc.sent) != 1 || fc.sent[0] != msg {
		t.Errorf("channel.sent = %v, want exactly [%q] (Telegram parity)", fc.sent, msg)
	}
}

// TestExecuteSellNoPosition confirms ExecuteSell maps db.ErrNoPosition into
// a non-nil error (so internal/web can distinguish success/failure) while
// still returning the same i18n text /sell would have sent.
func TestExecuteSellNoPosition(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)

	msg, err := b.ExecuteSell(context.Background(), "AAPL", 1, 100, feePtr(0), "2026-07-01")
	if err == nil {
		t.Errorf("ExecuteSell() error = nil, want ErrNoPosition")
	}
	if want := i18n.T(i18n.EN, i18n.KeySellNoPosition, "AAPL"); msg != want {
		t.Errorf("ExecuteSell() msg = %q, want %q", msg, want)
	}
}

// TestExecuteSellPartial exercises a successful, non-closing sell — a full
// close is deliberately not exercised here since it spawns reviewClosedTrade
// against a nil llm.Client (see TestRecordSellClosedFlag's own note on why
// its test never calls handleSell/ExecuteSell for a closing sell either).
func TestExecuteSellPartial(t *testing.T) {
	b, d := newPendingActionsTestBot(t)
	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	msg, err := b.ExecuteSell(context.Background(), "AAPL", 4, 220, feePtr(0), "2026-06-10")
	if err != nil {
		t.Fatalf("ExecuteSell() error = %v", err)
	}
	if msg == "" {
		t.Errorf("ExecuteSell() msg = %q, want a non-empty confirmation", msg)
	}

	pos, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok || pos.Shares != 6 {
		t.Errorf("GetPosition() = %v, %v, %v, want 6 remaining shares", pos, ok, err)
	}
}

// TestAdjustCash covers adjustCash's two branches: a no-op when the user
// hasn't declared a cash balance via /cash, and a buy/sell nudge once they
// have (see adjustCash's doc comment on why an undeclared balance is left
// alone rather than invented from delta).
func TestAdjustCash(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	if _, err := b.ExecuteBuy("AAPL", 10, 200, feePtr(1), "2026-07-01"); err != nil {
		t.Fatalf("ExecuteBuy() error = %v", err)
	}
	if _, ok, err := b.loadCash(market.US); err != nil || ok {
		t.Fatalf("loadCash() = _, %v, %v, want ok=false with no declared balance", ok, err)
	}

	if err := b.db.SetSetting(cashSettingKey, "10000"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}

	if _, err := b.ExecuteBuy("MSFT", 5, 300, feePtr(2), "2026-07-01"); err != nil {
		t.Fatalf("ExecuteBuy() error = %v", err)
	}
	cash, ok, err := b.loadCash(market.US)
	if err != nil || !ok || cash != 10000-(5*300+2) {
		t.Errorf("loadCash() after buy = %v, %v, %v, want %v, true, nil", cash, ok, err, 10000-(5*300+2))
	}

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if _, err := b.ExecuteSell(context.Background(), "AAPL", 4, 220, feePtr(3), "2026-06-10"); err != nil {
		t.Fatalf("ExecuteSell() error = %v", err)
	}
	want := 10000.0 - (5*300 + 2) + (4*220 - 3)
	cash, ok, err = b.loadCash(market.US)
	if err != nil || !ok || cash != want {
		t.Errorf("loadCash() after sell = %v, %v, %v, want %v, true, nil", cash, ok, err, want)
	}
}

// TestExecuteSetStop exercises both ExecuteSetStop branches against a stub
// provider whose GetQuote returns a fixed price (history always fails, so
// computeStopSuggestion falls back to the live-quote path — see
// quoteOnlyProvider).
func TestExecuteSetStop(t *testing.T) {
	b, d := newPendingActionsTestBot(t)
	b.provider = quoteOnlyProvider{price: 100}
	if _, err := d.RecordBuy("AAPL", 10, 90, 0, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}

	t.Run("invalid: at or above latest close", func(t *testing.T) {
		msg, err := b.ExecuteSetStop("AAPL", 100)
		if err == nil {
			t.Errorf("ExecuteSetStop() error = nil, want an error (stop >= latest close)")
		}
		if want := i18n.T(i18n.EN, i18n.KeyStopInvalidPrice, "$100", "$100"); msg != want {
			t.Errorf("ExecuteSetStop() msg = %q, want %q", msg, want)
		}
	})

	t.Run("valid", func(t *testing.T) {
		msg, err := b.ExecuteSetStop("AAPL", 80)
		if err != nil {
			t.Fatalf("ExecuteSetStop() error = %v", err)
		}
		if msg == "" {
			t.Errorf("ExecuteSetStop() msg = %q, want a non-empty confirmation", msg)
		}
		pos, ok, err := d.GetPosition("AAPL")
		if err != nil || !ok || pos.StopPrice != 80 {
			t.Errorf("GetPosition().StopPrice = %v, %v, %v, want 80", pos, ok, err)
		}
	})

	t.Run("no position", func(t *testing.T) {
		msg, err := b.ExecuteSetStop("MSFT", 50)
		if err != db.ErrNoPosition {
			t.Errorf("ExecuteSetStop() error = %v, want db.ErrNoPosition", err)
		}
		if want := i18n.T(i18n.EN, i18n.KeyStopNoPosition, b.tickerLabel("MSFT")); msg != want {
			t.Errorf("ExecuteSetStop() msg = %q, want %q", msg, want)
		}
	})
}

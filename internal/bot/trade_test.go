package bot

import "testing"

func TestParseTradeArgs(t *testing.T) {
	t.Run("ticker shares price", func(t *testing.T) {
		ticker, shares, price, fee, feeSet, date, err := parseTradeArgs("aapl 10 205.5")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "AAPL" || shares != 10 || price != 205.5 || fee != 0 || feeSet || date != "" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %v, %q; want AAPL, 10, 205.5, 0, false, \"\"", ticker, shares, price, fee, feeSet, date)
		}
	})

	t.Run("with fee", func(t *testing.T) {
		ticker, shares, price, fee, feeSet, date, err := parseTradeArgs("MSFT 5 400 1.5")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "MSFT" || shares != 5 || price != 400 || fee != 1.5 || !feeSet || date != "" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %v, %q; want MSFT, 5, 400, 1.5, true, \"\"", ticker, shares, price, fee, feeSet, date)
		}
	})

	t.Run("with date, no fee", func(t *testing.T) {
		ticker, shares, price, fee, feeSet, date, err := parseTradeArgs("AAPL 10 200 2026-01-15")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if ticker != "AAPL" || shares != 10 || price != 200 || fee != 0 || feeSet || date != "2026-01-15" {
			t.Errorf("parseTradeArgs() = %q, %v, %v, %v, %v, %q; want AAPL, 10, 200, 0, false, 2026-01-15", ticker, shares, price, fee, feeSet, date)
		}
	})

	t.Run("with fee and date, either order", func(t *testing.T) {
		for _, args := range []string{"AAPL 10 200 1.5 2026-01-15", "AAPL 10 200 2026-01-15 1.5"} {
			ticker, shares, price, fee, feeSet, date, err := parseTradeArgs(args)
			if err != nil {
				t.Fatalf("parseTradeArgs(%q) error = %v", args, err)
			}
			if ticker != "AAPL" || shares != 10 || price != 200 || fee != 1.5 || !feeSet || date != "2026-01-15" {
				t.Errorf("parseTradeArgs(%q) = %q, %v, %v, %v, %v, %q; want AAPL, 10, 200, 1.5, true, 2026-01-15", args, ticker, shares, price, fee, feeSet, date)
			}
		}
	})

	// Phase 13 §11.3: an explicit 0 must be distinguishable from an omitted
	// fee, since only the latter triggers TW's fee auto-calc.
	t.Run("explicit zero fee is feeSet, unlike an omitted one", func(t *testing.T) {
		_, _, _, fee, feeSet, _, err := parseTradeArgs("2330 1000 100")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if fee != 0 || feeSet {
			t.Errorf("parseTradeArgs(no fee) = fee %v, feeSet %v; want 0, false", fee, feeSet)
		}

		_, _, _, fee, feeSet, _, err = parseTradeArgs("2330 1000 100 0")
		if err != nil {
			t.Fatalf("parseTradeArgs() error = %v", err)
		}
		if fee != 0 || !feeSet {
			t.Errorf("parseTradeArgs(fee=0) = fee %v, feeSet %v; want 0, true", fee, feeSet)
		}
	})

	for _, args := range []string{
		"",
		"AAPL",
		"AAPL 10",
		"AAPL 10 200 1 2",
		"AAPL 10 200 2026-01-15 2026-02-01",
		"AAPL 10 200 1 2 2026-01-15",
		"AAPL 0 200",
		"AAPL -1 200",
		"AAPL 10 0",
		"AAPL 10 -5",
		"AAPL 10 200 -1",
		"AAPL abc 200",
		"AAPL 10 200 2026-13-40",
	} {
		if _, _, _, _, _, _, err := parseTradeArgs(args); err == nil {
			t.Errorf("parseTradeArgs(%q) error = nil, want error", args)
		}
	}
}

func TestRecordSellClosedFlag(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	if _, err := d.RecordBuy("AAPL", 10, 200, 0, "2026-06-01"); err != nil {
		t.Fatalf("RecordBuy() error = %v", err)
	}
	if err := d.SetStopPrice("AAPL", 180); err != nil {
		t.Fatalf("SetStopPrice() error = %v", err)
	}

	msg, closed, stopPrice, err := b.recordSell("AAPL", 4, 220, 0, false, "2026-06-10", "")
	if err != nil {
		t.Errorf("recordSell() error = %v, want nil for a valid partial sell", err)
	}
	if closed {
		t.Errorf("recordSell() closed = true for a partial sell, want false; msg = %q", msg)
	}
	if stopPrice != 180 {
		t.Errorf("recordSell() stopPrice = %v, want 180 (read before the sell)", stopPrice)
	}

	msg, closed, stopPrice, err = b.recordSell("AAPL", 6, 230, 0, false, "2026-06-20", "")
	if err != nil {
		t.Errorf("recordSell() error = %v, want nil for a valid closing sell", err)
	}
	if !closed {
		t.Errorf("recordSell() closed = false for a sell down to 0 shares, want true; msg = %q", msg)
	}
	if stopPrice != 180 {
		t.Errorf("recordSell() stopPrice = %v, want 180 for the sell that fully closed the position", stopPrice)
	}

	msg, closed, stopPrice, err = b.recordSell("AAPL", 1, 200, 0, false, "2026-06-21", "")
	if err == nil {
		t.Errorf("recordSell() error = nil, want ErrNoPosition (no position left)")
	}
	if closed {
		t.Errorf("recordSell() closed = true on an error path (no position left), want false; msg = %q", msg)
	}
	if stopPrice != 0 {
		t.Errorf("recordSell() stopPrice = %v, want 0 on an error path", stopPrice)
	}
}

package bot

import (
	"strings"
	"testing"
	"time"

	"argus/internal/data"
)

const testOptionSymbol = "AAPL260918C00320000"

// fakeOptionChain implements data.OptionChainProvider with a fixed chain,
// for /option select tests — no live Yahoo call.
type fakeOptionChain struct {
	expirations []time.Time
	quotes      map[time.Time][]data.OptionQuote
}

func (f fakeOptionChain) GetOptionExpirations(string) ([]time.Time, error) {
	return f.expirations, nil
}
func (f fakeOptionChain) GetOptionChain(_ string, expiry time.Time) ([]data.OptionQuote, error) {
	return f.quotes[expiry], nil
}

// TestOptionBuySellRoundTrip mirrors docs/phase-12-options.md §9's
// end-to-end acceptance checklist: /obuy then /osell against the same
// contract, checking the confirmation text carries the right market value
// and realized P&L.
func TestOptionBuySellRoundTrip(t *testing.T) {
	b, d := newPendingActionsTestBot(t)

	b.handleOBuy(testOptionSymbol + " 2 5.40")
	fc := b.channel.(*fakeChannel)
	if len(fc.sent) != 1 || !strings.Contains(fc.sent[0], "2") {
		t.Fatalf("obuy confirmation = %v", fc.sent)
	}
	pos, ok, err := d.GetOptionPosition(testOptionSymbol)
	if err != nil || !ok || pos.Contracts != 2 || pos.AvgPremium != 5.40 {
		t.Fatalf("GetOptionPosition after obuy = %+v ok=%v err=%v", pos, ok, err)
	}

	fc.sent = nil
	b.handleOSell(testOptionSymbol + " 1 7.20")
	if len(fc.sent) != 1 {
		t.Fatalf("osell confirmation count = %d, want 1", len(fc.sent))
	}
	if !strings.Contains(fc.sent[0], "180.00") {
		t.Errorf("osell confirmation = %q, want it to mention realized P&L 180.00", fc.sent[0])
	}
	pos, ok, err = d.GetOptionPosition(testOptionSymbol)
	if err != nil || !ok || pos.Contracts != 1 {
		t.Fatalf("GetOptionPosition after partial close = %+v ok=%v err=%v", pos, ok, err)
	}
}

// TestOptionCrossesZeroRejected covers the same 2-long/sell-3 rejection as
// the db-layer test, through the /osell command path this time.
func TestOptionCrossesZeroRejected(t *testing.T) {
	b, d := newPendingActionsTestBot(t)
	b.handleOBuy(testOptionSymbol + " 2 5.40")

	fc := b.channel.(*fakeChannel)
	fc.sent = nil
	b.handleOSell(testOptionSymbol + " 3 5.00")
	if len(fc.sent) != 1 || !strings.Contains(fc.sent[0], testOptionSymbol) {
		t.Fatalf("osell crossing zero: sent = %v", fc.sent)
	}
	pos, ok, err := d.GetOptionPosition(testOptionSymbol)
	if err != nil || !ok || pos.Contracts != 2 {
		t.Fatalf("position after rejected cross = %+v ok=%v err=%v, want untouched 2 contracts", pos, ok, err)
	}
}

// TestOAssignShortPutCreatesStockPosition covers §9 item 5: assignment on a
// short position must generate the matching stock trade via the existing
// RecordBuy/RecordSell path.
func TestOAssignShortPutCreatesStockPosition(t *testing.T) {
	putSymbol := "AAPL260918P00320000"
	b, d := newPendingActionsTestBot(t)

	// Sell to open a cash-secured put.
	b.handleOSell(putSymbol + " 1 4.00")

	fc := b.channel.(*fakeChannel)
	fc.sent = nil
	b.handleOAssign(putSymbol + " 2026-09-18")
	if len(fc.sent) != 1 {
		t.Fatalf("oassign sent = %v, want exactly one confirmation", fc.sent)
	}

	if _, ok, _ := d.GetOptionPosition(putSymbol); ok {
		t.Error("option position should be closed after assignment")
	}
	stockPos, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok {
		t.Fatalf("GetPosition(AAPL) after assignment = %v %v %v, want a stock position from the assignment", stockPos, ok, err)
	}
	if stockPos.Shares != 100 || stockPos.AvgCost != 320 {
		t.Errorf("stock position after put assignment = %+v, want 100 shares @ 320", stockPos)
	}
}

// TestOAssignRequiresShort ensures /oassign refuses a long position (the
// user meant /oexercise) without touching the ledger.
func TestOAssignRequiresShort(t *testing.T) {
	b, d := newPendingActionsTestBot(t)
	b.handleOBuy(testOptionSymbol + " 1 5.40")

	fc := b.channel.(*fakeChannel)
	fc.sent = nil
	b.handleOAssign(testOptionSymbol)
	if len(fc.sent) != 1 {
		t.Fatalf("oassign on long: sent = %v", fc.sent)
	}
	if _, ok, _ := d.GetOptionPosition(testOptionSymbol); !ok {
		t.Error("long option position should be untouched by a rejected /oassign")
	}
}

// TestNakedCallWarning confirms an uncovered short call triggers the
// at-write-time warning (§3.5) when the underlying isn't held at all.
func TestNakedCallWarning(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)

	b.handleOSell(testOptionSymbol + " 1 5.40")
	fc := b.channel.(*fakeChannel)
	if len(fc.sent) != 1 || !strings.Contains(fc.sent[0], "Naked call warning") {
		t.Errorf("osell naked call: sent = %v, want a naked call warning", fc.sent)
	}
}

// TestNoNakedCallWarningWhenCovered confirms a covered call (enough shares
// held) doesn't trigger the warning.
func TestNoNakedCallWarningWhenCovered(t *testing.T) {
	b, d := newPendingActionsTestBot(t)
	if _, err := d.RecordBuy("AAPL", 100, 300, 0, "2026-08-01"); err != nil {
		t.Fatalf("RecordBuy: %v", err)
	}

	b.handleOSell(testOptionSymbol + " 1 5.40")
	fc := b.channel.(*fakeChannel)
	if len(fc.sent) != 1 || strings.Contains(fc.sent[0], "Naked call warning") {
		t.Errorf("osell covered call: sent = %v, want no naked call warning", fc.sent)
	}
}

func TestParseOptionTradeArgs(t *testing.T) {
	symbol, contracts, premium, fee, date, err := parseOptionTradeArgs(testOptionSymbol + " 2 5.40 1.5 2026-08-01")
	if err != nil {
		t.Fatalf("parseOptionTradeArgs: %v", err)
	}
	if symbol != testOptionSymbol || contracts != 2 || premium != 5.40 || fee != 1.5 || date != "2026-08-01" {
		t.Errorf("parsed = %q %v %v %v %q", symbol, contracts, premium, fee, date)
	}

	if _, _, _, _, _, err := parseOptionTradeArgs("AAPL 2 5.40"); err == nil {
		t.Error("parseOptionTradeArgs(plain ticker) expected error, got nil")
	}
}

func TestPortfolioIncludesOptionsSection(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)
	b.handleOBuy(testOptionSymbol + " 1 5.40")

	fc := b.channel.(*fakeChannel)
	fc.sent = nil
	b.handlePortfolio()

	var sawSection bool
	for _, s := range fc.sent {
		if strings.Contains(s, "Option Positions") {
			sawSection = true
		}
	}
	if !sawSection {
		t.Errorf("handlePortfolio() sent = %v, want an options section", fc.sent)
	}
}

func TestHandleOption_LongCallDefault(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)
	b.provider = quoteOnlyProvider{price: 315}

	expiry := time.Now().AddDate(0, 0, 40)
	b.optionChain = fakeOptionChain{
		expirations: []time.Time{expiry},
		quotes: map[time.Time][]data.OptionQuote{
			expiry: {
				// ATM call: passes liquidity gate and LongCall's delta band.
				{ContractSymbol: "AAPL-ATM", Right: "C", Strike: 315, Bid: 4.9, Ask: 5.1, Volume: 100, OpenInterest: 1000, ImpliedVolatility: 0.30, Expiration: expiry},
				// Illiquid: fails the OI gate.
				{ContractSymbol: "AAPL-ILLIQUID", Right: "C", Strike: 316, Bid: 4.9, Ask: 5.1, Volume: 1, OpenInterest: 1, ImpliedVolatility: 0.30, Expiration: expiry},
			},
		},
	}

	b.handleOption("AAPL")
	fc := b.channel.(*fakeChannel)
	if len(fc.sent) != 1 {
		t.Fatalf("handleOption sent = %v, want exactly 1 candidate line", fc.sent)
	}
	if !strings.Contains(fc.sent[0], "AAPL-ATM") {
		t.Errorf("handleOption sent = %q, want it to name the ATM contract", fc.sent[0])
	}
}

func TestHandleOption_NoCandidates(t *testing.T) {
	b, _ := newPendingActionsTestBot(t)
	b.provider = quoteOnlyProvider{price: 315}
	b.optionChain = fakeOptionChain{} // no expirations at all

	b.handleOption("AAPL")
	fc := b.channel.(*fakeChannel)
	if len(fc.sent) != 1 || !strings.Contains(fc.sent[0], "AAPL") {
		t.Errorf("handleOption(no candidates) sent = %v", fc.sent)
	}
}

func TestParseOptionSelectArgs(t *testing.T) {
	ticker, profile, err := parseOptionSelectArgs("AAPL csp")
	if err != nil || ticker != "AAPL" || profile.Name != "CSP" {
		t.Errorf("parseOptionSelectArgs(AAPL csp) = %q %+v %v", ticker, profile, err)
	}

	ticker, profile, err = parseOptionSelectArgs("AAPL")
	if err != nil || ticker != "AAPL" || profile.Name != "LongCall" {
		t.Errorf("parseOptionSelectArgs(AAPL) = %q %+v %v, want default LongCall", ticker, profile, err)
	}

	if _, _, err := parseOptionSelectArgs("AAPL bogus"); err == nil {
		t.Error("parseOptionSelectArgs(AAPL bogus) expected error, got nil")
	}
}

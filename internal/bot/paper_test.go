package bot

import (
	"math"
	"path/filepath"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/paper"
)

func almostEqualPaper(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func newPaperTestBot(t *testing.T) *Bot {
	t.Helper()
	mainDB, err := db.New(filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatalf("db.New(main) error = %v", err)
	}
	t.Cleanup(func() { mainDB.Close() })

	paperDB, err := db.New(filepath.Join(t.TempDir(), "paper.db"))
	if err != nil {
		t.Fatalf("db.New(paper) error = %v", err)
	}
	t.Cleanup(func() { paperDB.Close() })

	return NewWithChannel(&fakeChannel{}, Config{
		DB:                  mainDB,
		Provider:            noDataProvider{},
		History:             noDataProvider{},
		Lang:                i18n.EN,
		PaperDB:             paperDB,
		PaperInitialCashUSD: 100000,
		PaperInitialCashTWD: 1000000,
		PaperMaxPositionPct: 25,
	})
}

// TestPaperAccount_LoadPersistRoundTrip exercises PR3's load -> apply ->
// persist -> reload cycle end to end against a real (temp-dir) paper.db —
// the trading rules themselves are internal/paper's job to test (PR1); this
// only checks that state survives the round trip through SQLite.
func TestPaperAccount_LoadPersistRoundTrip(t *testing.T) {
	b := newPaperTestBot(t)

	acct, err := b.loadPaperAccount(market.US)
	if err != nil {
		t.Fatalf("loadPaperAccount() error = %v", err)
	}
	if acct.Cash != 100000 {
		t.Fatalf("seeded cash = %v, want 100000", acct.Cash)
	}
	if len(acct.Holdings) != 0 {
		t.Fatalf("fresh account should have no holdings, got %v", acct.Holdings)
	}

	cfg := b.paperConfig(market.US)
	trade, ok := acct.ApplySignal(paper.Signal{Date: "2024-01-02", Ticker: "AAPL", Action: "BUY", Price: 150}, 150, 3, cfg)
	if !ok {
		t.Fatalf("expected BUY to execute")
	}
	if err := b.persistPaperTrade(acct, trade); err != nil {
		t.Fatalf("persistPaperTrade() error = %v", err)
	}

	reloaded, err := b.loadPaperAccount(market.US)
	if err != nil {
		t.Fatalf("reload loadPaperAccount() error = %v", err)
	}
	if !almostEqualPaper(reloaded.Cash, acct.Cash) {
		t.Errorf("reloaded cash = %v, want %v", reloaded.Cash, acct.Cash)
	}
	h, held := reloaded.Holdings["AAPL"]
	if !held {
		t.Fatalf("reloaded account is missing the AAPL holding")
	}
	if !almostEqualPaper(h.Shares, trade.Shares) {
		t.Errorf("reloaded shares = %v, want %v", h.Shares, trade.Shares)
	}
	if !almostEqualPaper(h.AvgCost, trade.Price) {
		t.Errorf("reloaded avg cost = %v, want %v", h.AvgCost, trade.Price)
	}
	if !almostEqualPaper(h.Stop, trade.Stop) {
		t.Errorf("reloaded stop = %v, want %v", h.Stop, trade.Stop)
	}
	if !almostEqualPaper(h.Peak, trade.Price) {
		t.Errorf("reloaded peak (no snapshot history yet, should fall back to avg cost) = %v, want %v", h.Peak, trade.Price)
	}

	// Stop-loss exit: MarkClose below the stop, persist, then verify the
	// transactions row's realized_pnl and stop_price snapshot are correct —
	// the stop_price comes back from paper.db's own positions row (set by
	// SetStopPrice during the BUY above), not from anything this test passes
	// in directly, so this also checks that plumbing.
	exitClose := trade.Stop - 1
	exits := reloaded.MarkClose("2024-01-03", map[string]float64{"AAPL": exitClose}, nil, cfg)
	if len(exits) != 1 || exits[0].Reason != "stop" {
		t.Fatalf("expected one stop-loss exit, got %+v", exits)
	}
	if err := b.persistPaperTrade(reloaded, exits[0]); err != nil {
		t.Fatalf("persistPaperTrade(exit) error = %v", err)
	}

	txs, err := b.paperDB.GetTransactions("AAPL")
	if err != nil {
		t.Fatalf("GetTransactions() error = %v", err)
	}
	var sell *db.Transaction
	for i := range txs {
		if txs[i].Side == "SELL" {
			sell = &txs[i]
		}
	}
	if sell == nil {
		t.Fatalf("no SELL transaction recorded, got %+v", txs)
	}
	if !almostEqualPaper(sell.RealizedPnL, exits[0].RealizedPnL) {
		t.Errorf("transaction realized_pnl = %v, want %v", sell.RealizedPnL, exits[0].RealizedPnL)
	}
	if !almostEqualPaper(sell.StopPrice, trade.Stop) {
		t.Errorf("transaction stop_price snapshot = %v, want %v", sell.StopPrice, trade.Stop)
	}

	if _, held := reloaded.Holdings["AAPL"]; held {
		t.Errorf("in-memory account still shows AAPL held after stop-loss exit")
	}
	finalAcct, err := b.loadPaperAccount(market.US)
	if err != nil {
		t.Fatalf("final loadPaperAccount() error = %v", err)
	}
	if _, held := finalAcct.Holdings["AAPL"]; held {
		t.Errorf("reloaded account still shows AAPL held after stop-loss exit")
	}
}

func TestPaperAccount_DisabledIsNoop(t *testing.T) {
	b := NewWithChannel(&fakeChannel{}, Config{Lang: i18n.EN})
	b.handlePaper("") // must not panic with paperDB nil

	fc := b.channel.(*fakeChannel)
	if len(fc.sent) != 1 {
		t.Fatalf("expected exactly one disabled-notice message, got %v", fc.sent)
	}
}

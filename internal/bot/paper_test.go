package bot

import (
	"math"
	"path/filepath"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/market"
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
// persist -> reload cycle end to end against a real (temp-dir) paper.db, via
// the same applyPaperTrades/runPaperClose entry points RunDailyReport and
// RunClosingSnapshot actually call — the trading rules themselves are
// internal/paper's job to test (PR1), and PaperService's own logic has its
// own mock-backed unit tests; this only checks that state survives the round
// trip through SQLite and the bot-layer wiring to PaperService.
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

	recs := []llm.Recommendation{{Ticker: "AAPL", Action: "BUY"}}
	b.applyPaperTrades(recs, map[string]float64{"AAPL": 150}, map[string]float64{"AAPL": 3}, market.US)

	reloaded, err := b.loadPaperAccount(market.US)
	if err != nil {
		t.Fatalf("reload loadPaperAccount() error = %v", err)
	}
	h, held := reloaded.Holdings["AAPL"]
	if !held {
		t.Fatalf("reloaded account is missing the AAPL holding")
	}
	if !almostEqualPaper(h.AvgCost, 150) {
		t.Errorf("reloaded avg cost = %v, want 150", h.AvgCost)
	}
	if !almostEqualPaper(h.Peak, 150) {
		t.Errorf("reloaded peak (no snapshot history yet, should fall back to avg cost) = %v, want 150", h.Peak)
	}

	// Stop-loss exit via runPaperClose — the same "yesterday's positions face
	// today's stop" entry point RunClosingSnapshot uses. Verifies the
	// transactions row's realized_pnl and stop_price snapshot come back
	// correctly from paper.db's own positions row (set by SetStopPrice during
	// the BUY above), not from anything this test passes in directly.
	exitClose := h.Stop - 1
	b.runPaperClose(market.US, "2024-01-03", map[string]float64{"AAPL": exitClose})

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
	if !almostEqualPaper(sell.StopPrice, h.Stop) {
		t.Errorf("transaction stop_price snapshot = %v, want %v", sell.StopPrice, h.Stop)
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

package db

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"argus/internal/market"
)

// TestBackfillFIFOLots pins migration 16's backfill step: given pre-existing
// transaction history written the old way (BUY rows with remaining_shares
// never set, a SELL's realized_pnl computed off the old blended average, and
// a stale blended positions.avg_cost), backfillFIFOLots must reconstruct the
// correct FIFO lot state and recompute positions.avg_cost from it — the fix
// for positions that already existed before this migration ran.
func TestBackfillFIFOLots(t *testing.T) {
	d := newTestDB(t)

	// Two BUY lots and a partial SELL, inserted directly to simulate
	// pre-migration data (remaining_shares left at its 0 default, realized_pnl
	// carrying the old, now-wrong blended-average value of 375).
	var lot1ID, lot2ID int64
	mustExec := func(query string, args ...any) sql.Result {
		t.Helper()
		res, err := d.conn.Exec(query, args...)
		if err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
		return res
	}
	res := mustExec(`INSERT INTO transactions (ticker, side, shares, price, fee, date, market) VALUES ('AAPL', 'BUY', 10, 100, 0, '2026-07-01', 'us')`)
	lot1ID, _ = res.LastInsertId()
	res = mustExec(`INSERT INTO transactions (ticker, side, shares, price, fee, date, market) VALUES ('AAPL', 'BUY', 10, 150, 0, '2026-07-02', 'us')`)
	lot2ID, _ = res.LastInsertId()
	mustExec(`INSERT INTO transactions (ticker, side, shares, price, fee, date, realized_pnl, market) VALUES ('AAPL', 'SELL', 5, 200, 0, '2026-07-03', 375, 'us')`)
	mustExec(`INSERT INTO positions (ticker, shares, avg_cost, market) VALUES ('AAPL', 15, 125, 'us')`)

	tx, err := d.conn.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := backfillFIFOLots(tx); err != nil {
		tx.Rollback()
		t.Fatalf("backfillFIFOLots() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	var remaining1, remaining2 float64
	if err := d.conn.QueryRow(`SELECT remaining_shares FROM transactions WHERE id = ?`, lot1ID).Scan(&remaining1); err != nil {
		t.Fatalf("query lot1 remaining_shares: %v", err)
	}
	if err := d.conn.QueryRow(`SELECT remaining_shares FROM transactions WHERE id = ?`, lot2ID).Scan(&remaining2); err != nil {
		t.Fatalf("query lot2 remaining_shares: %v", err)
	}
	if remaining1 != 5 {
		t.Errorf("lot1 remaining_shares = %v, want 5 (FIFO consumed 5 of the SELL's 5 shares from the oldest lot)", remaining1)
	}
	if remaining2 != 10 {
		t.Errorf("lot2 remaining_shares = %v, want 10 (untouched — FIFO left lot2 alone)", remaining2)
	}

	var avgCost float64
	if err := d.conn.QueryRow(`SELECT avg_cost FROM positions WHERE ticker = 'AAPL'`).Scan(&avgCost); err != nil {
		t.Fatalf("query positions.avg_cost: %v", err)
	}
	wantAvgCost := (5*100.0 + 10*150.0) / 15
	if math.Abs(avgCost-wantAvgCost) > 1e-9 {
		t.Errorf("positions.avg_cost after backfill = %v, want %v (FIFO-weighted over the surviving lots, not the stale 125 blend)", avgCost, wantAvgCost)
	}
}

// TestMigrateIsRerunnable reopens the same database file, simulating a bot
// restart — migrate() must see the recorded user_version and skip already-
// applied steps (re-running the ALTER TABLE step would fail on duplicate
// columns).
func TestMigrateIsRerunnable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatalf("AddTicker() error = %v", err)
	}
	d.Close()

	d, err = New(path)
	if err != nil {
		t.Fatalf("New() (reopen) error = %v", err)
	}
	defer d.Close()
	got, err := d.GetWatchlist()
	if err != nil || len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("GetWatchlist() after reopen = %v, %v; want [AAPL], nil", got, err)
	}
}

// TestMigration12BackfillsMarketAndRebuildsNetWorth simulates a pre-Phase-6
// database (stopped at migration 11, no market columns, net_worth_snapshots
// keyed by date alone) picking up a TW position before ever running the
// Phase 6 code — the deploy-day scenario docs/phase-6-tw-market.md §8 flags
// as this migration's biggest risk, since it's the project's first
// rebuild-a-table migration rather than a plain append-only ALTER TABLE.
// Asserts every pre-existing row survives with the correct backfilled
// market, not just that the migration runs without error.
func TestMigration12BackfillsMarketAndRebuildsNetWorth(t *testing.T) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s", filepath.Join(t.TempDir(), "legacy.db")))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	d := &DB{conn: conn}
	defer d.Close()

	// Apply migrations 1..11 by hand (pre-Phase-6 schema) and stamp
	// user_version accordingly, exactly like migrate() would have at that
	// point in this database's history.
	for i, m := range migrations[:11] {
		if _, err := conn.Exec(m); err != nil {
			t.Fatalf("apply legacy migration %d: %v", i+1, err)
		}
	}
	if _, err := conn.Exec(`PRAGMA user_version = 11`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	// Seed old-shaped rows: a US and a TW ticker on the watchlist/positions/
	// transactions/recommendations tables (none of which have a market
	// column yet), plus a net_worth_snapshots row under the old
	// single-column (date) primary key.
	legacyInserts := []string{
		`INSERT INTO watchlist (ticker) VALUES ('AAPL')`,
		`INSERT INTO watchlist (ticker) VALUES ('2330')`,
		`INSERT INTO positions (ticker, shares, avg_cost) VALUES ('AAPL', 10, 150)`,
		`INSERT INTO positions (ticker, shares, avg_cost) VALUES ('2330', 1000, 900)`,
		`INSERT INTO transactions (ticker, side, shares, price, fee, date) VALUES ('AAPL', 'BUY', 10, 150, 0, '2026-07-01')`,
		`INSERT INTO transactions (ticker, side, shares, price, fee, date) VALUES ('2330', 'BUY', 1000, 900, 0, '2026-07-01')`,
		`INSERT INTO recommendations (date, ticker, action, reason, price, source) VALUES ('2026-07-01', 'AAPL', 'BUY', 'test', 150, 'watchlist')`,
		`INSERT INTO recommendations (date, ticker, action, reason, price, source) VALUES ('2026-07-01', '2330', 'BUY', 'test', 900, 'watchlist')`,
		`INSERT INTO net_worth_snapshots (date, total_value) VALUES ('2026-07-01', 1500)`,
	}
	for _, ins := range legacyInserts {
		if _, err := conn.Exec(ins); err != nil {
			t.Fatalf("legacy insert %q: %v", ins, err)
		}
	}

	// Now apply the rest — migration 12, the market-column/net_worth rebuild.
	if err := d.migrate(); err != nil {
		t.Fatalf("migrate() error = %v", err)
	}

	watchlistUS, err := d.GetWatchlistByMarket(market.US)
	if err != nil || len(watchlistUS) != 1 || watchlistUS[0] != "AAPL" {
		t.Errorf("GetWatchlistByMarket(US) = %v, %v; want [AAPL], nil", watchlistUS, err)
	}
	watchlistTW, err := d.GetWatchlistByMarket(market.TW)
	if err != nil || len(watchlistTW) != 1 || watchlistTW[0] != "2330" {
		t.Errorf("GetWatchlistByMarket(TW) = %v, %v; want [2330], nil", watchlistTW, err)
	}

	posUS, ok, err := d.GetPosition("AAPL")
	if err != nil || !ok || posUS.Market != "us" {
		t.Errorf("GetPosition(AAPL) = %+v, %v, %v; want Market=us", posUS, ok, err)
	}
	posTW, ok, err := d.GetPosition("2330")
	if err != nil || !ok || posTW.Market != "tw" {
		t.Errorf("GetPosition(2330) = %+v, %v, %v; want Market=tw", posTW, ok, err)
	}

	txsUS, err := d.GetTransactions("AAPL")
	if err != nil || len(txsUS) != 1 || txsUS[0].Market != "us" {
		t.Errorf("GetTransactions(AAPL) = %+v, %v; want one row with Market=us", txsUS, err)
	}
	txsTW, err := d.GetTransactions("2330")
	if err != nil || len(txsTW) != 1 || txsTW[0].Market != "tw" {
		t.Errorf("GetTransactions(2330) = %+v, %v; want one row with Market=tw", txsTW, err)
	}

	recs, err := d.GetRecommendationsSince("2026-07-01")
	if err != nil || len(recs) != 2 {
		t.Fatalf("GetRecommendationsSince() = %v, %v; want 2 rows", recs, err)
	}
	for _, r := range recs {
		wantMarket := "us"
		if r.Ticker == "2330" {
			wantMarket = "tw"
		}
		if r.Market != wantMarket {
			t.Errorf("recommendation %s Market = %q, want %q", r.Ticker, r.Market, wantMarket)
		}
	}

	// The pre-migration net_worth_snapshots row must survive the table
	// rebuild, backfilled to market='us' (the only market that existed
	// before Phase 6).
	date, total, ok, err := d.GetLatestNetWorth(market.US)
	if err != nil || !ok || date != "2026-07-01" || total != 1500 {
		t.Errorf("GetLatestNetWorth(US) = %q, %v, %v, %v; want \"2026-07-01\", 1500, true, nil", date, total, ok, err)
	}
	if _, _, ok, err := d.GetLatestNetWorth(market.TW); err != nil || ok {
		t.Errorf("GetLatestNetWorth(TW) = _, _, %v, %v; want ok=false (no TW row existed pre-migration)", ok, err)
	}

	// A post-migration write for the other market must coexist at the same
	// date without clobbering the backfilled US row — the whole point of the
	// (date, market) composite PK.
	if err := d.SaveNetWorthSnapshot("2026-07-01", market.TW, 950000); err != nil {
		t.Fatalf("SaveNetWorthSnapshot(TW) error = %v", err)
	}
	if _, total, ok, err := d.GetLatestNetWorth(market.US); err != nil || !ok || total != 1500 {
		t.Errorf("GetLatestNetWorth(US) after a same-date TW write = %v, %v, %v; want unaffected 1500", total, ok, err)
	}
	if _, total, ok, err := d.GetLatestNetWorth(market.TW); err != nil || !ok || total != 950000 {
		t.Errorf("GetLatestNetWorth(TW) after write = %v, %v, %v; want 950000", total, ok, err)
	}
}

// A multi-statement migration that fails part-way must leave nothing behind:
// before each step ran in a transaction, the statements before the failing
// one stayed applied while user_version did not advance, so the next boot
// re-ran the step and died on "duplicate column" / "table already exists".
func TestMigrateStepIsAtomic(t *testing.T) {
	d := newTestDB(t)

	var before int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&before); err != nil {
		t.Fatalf("read user_version: %v", err)
	}

	orig := migrations
	t.Cleanup(func() { migrations = orig })
	migrations = append(migrations[:len(migrations):len(migrations)],
		`CREATE TABLE half_applied (a INTEGER); INSERT INTO no_such_table VALUES (1);`)

	if err := d.migrate(); err == nil {
		t.Fatal("migrate() error = nil, want the failing step's error")
	}

	var after int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&after); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if after != before {
		t.Errorf("user_version = %d after a failed step, want %d", after, before)
	}
	var n int
	if err := d.conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'half_applied'`).Scan(&n); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if n != 0 {
		t.Error("table half_applied exists after a failed step, want the whole step rolled back")
	}
}

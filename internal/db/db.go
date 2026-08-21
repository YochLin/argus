package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"argus/internal/market"
)

// ErrNoPosition is returned by RecordSell when there is no open position for
// the ticker to sell from.
var ErrNoPosition = errors.New("no position for ticker")

// ErrInsufficientShares is returned by RecordSell when the sell size exceeds
// the currently held shares — this project only tracks long positions, so
// selling more than is held would go negative, which isn't representable.
var ErrInsufficientShares = errors.New("insufficient shares for sell")

// ErrCrossesZero is returned by RecordOption when an order would flip a
// position from long to short (or vice versa) in one trade — e.g. selling 3
// contracts against a 2-long position. Rejected rather than split into
// "close 2, open short 1" automatically: ponytail: allowing it would double
// RecordOption's branch count for a mistake nobody makes by accident (see
// docs/phase-12-options.md §3.3).
var ErrCrossesZero = errors.New("order crosses zero — close the position before reversing it")

type DB struct {
	conn *sql.DB
}

type Watchlist struct {
	Ticker  string
	AddedAt time.Time
}

type DailySnapshot struct {
	ID            int64
	Ticker        string
	Date          string
	Open          float64
	Close         float64
	High          float64
	Low           float64
	Volume        int64
	ChangePercent float64
	CreatedAt     time.Time
}

type Recommendation struct {
	ID        int64
	Date      string
	Ticker    string
	Action    string // BUY / SELL / HOLD ("" for rows saved before the column existed)
	Reason    string
	Price     float64 // price at recommendation time (0 for rows saved before the column existed)
	Source    string  // "watchlist" / "movers" / "scan" ("" for rows saved before the column existed — display as "watchlist")
	Market    string  // "us" / "tw" — derived from Ticker via market.Of at write time, never caller-supplied (Phase 6, see migration 12)
	CreatedAt time.Time
}

// Position is the current open holding for a ticker: total shares and the
// cost-basis-weighted average price paid across all buys, net of sells.
// StopPrice is Phase 3.11's per-trade stop-loss level (0 = unset, meaning
// checkStopLossAlerts falls back to the global STOP_LOSS_PCT for this
// ticker) — see SetStopPrice. It belongs to the open position, not the
// ticker itself: RecordSell deletes the positions row on a full close, and
// the stop price correctly disappears with it, while RecordBuy topping up
// an existing position deliberately leaves it untouched (whether to adjust
// a stop after adding shares is the user's call, not automatic).
type Position struct {
	Ticker    string
	Shares    float64
	AvgCost   float64
	StopPrice float64
	Market    string // "us" / "tw" — derived from Ticker via market.Of at write time, never caller-supplied (Phase 6, see migration 12)
	UpdatedAt time.Time
}

// Transaction is one recorded buy or sell. RealizedPnL is only meaningful
// for SELL rows (0 for BUY) — proceeds minus the shares' cost basis at
// AvgCost, minus fee. StopPrice (Phase 8 PR1, migration 13) is likewise
// SELL-only: the position's stop_price at the moment of sale, 0 for every
// BUY row and for any SELL predating this migration or made with no stop
// set — see RecordSell and docs/phase-8-trader-analytics.md §3.1.
type Transaction struct {
	ID          int64
	Ticker      string
	Side        string // BUY / SELL
	Shares      float64
	Price       float64
	Fee         float64
	Date        string
	RealizedPnL float64
	StopPrice   float64
	Market      string // "us" / "tw" — derived from Ticker via market.Of at write time, never caller-supplied (Phase 6, see migration 12)
	CreatedAt   time.Time
}

// New opens the main read-write connection used by the bot process itself.
// busy_timeout is set (see OpenReadOnly/OpenForWrites below) so that a
// second connection to the same file briefly holding a write lock — the MCP
// subprocess's own writable connection, see OpenForWrites — makes this
// connection wait rather than fail outright with "database is locked";
// both sides are low-frequency single-user writers, so contention should be
// rare, but there was no reason to leave the default (no wait at all) now
// that a second writer process exists at all.
func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		return nil, err
	}
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	if err := d.seedSP500(); err != nil {
		return nil, err
	}
	if err := d.seedTW150(); err != nil {
		return nil, err
	}
	return d, nil
}

// OpenReadOnly opens a second connection to the SQLite database at path for
// a separate process that must never write to it — the MCP server
// subprocess's read-only query tools (see internal/mcptools and PLAN.md's
// Phase 3.5 "DB 唯讀查詢工具" item), which relaxes that phase's original
// "don't touch the DB at all" decision now that it's clear a
// same-process-family read-only connection carries none of the
// cross-process write-conflict risk that decision was guarding against.
//
// Unlike New, this skips migrate()/seedSP500() (both write) — it assumes
// the schema already exists, which holds in practice since this connection
// is only ever opened after the main bot process has already opened (and
// migrated) the database.
//
// modernc.org/sqlite's "mode=ro" DSN query parameter is silently ignored —
// confirmed against the driver's own test suite (all_test.go's
// TestInMemory: "// This parameter should be ignored"); this pure-Go
// driver always opens with SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE
// regardless of what the DSN says, so "mode=ro" would be a lie here. Real
// enforcement instead comes from `PRAGMA query_only = ON`, applied via the
// driver's `_pragma` DSN parameter: SQLite rejects any write statement on
// a query_only connection with "attempt to write a readonly database",
// independent of how the file was opened. The `file:` prefix is required
// for the driver to honor the query string at all — without it, the
// driver's DSN parser strips everything after `?` before opening.
func OpenReadOnly(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=query_only(1)", path))
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

// OpenForWrites opens a second, genuinely writable connection to the SQLite
// database at path for a separate process — the MCP server subprocess's
// watchlist write-pilot tools (add_to_watchlist/remove_from_watchlist, see
// internal/mcptools and PLAN.md's Phase 3.5 "watchlist 寫入工具" item).
// Deliberately a distinct connection from OpenReadOnly (not just "the same
// thing minus query_only") so the four pre-existing read-only tools keep
// their hard DB-level guarantee that they can never write, even if a future
// bug tried to route a write through them — only the tools that are
// explicitly meant to mutate data get a handle capable of it.
//
// Like OpenReadOnly, this skips migrate()/seedSP500() (schema ownership
// stays with the main bot process) and sets busy_timeout so a write here
// waits rather than immediately erroring if it briefly races the main
// process's own connection (see New's doc comment).
func OpenForWrites(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

// migrations is the ordered list of incremental schema steps. The DB's
// PRAGMA user_version records how many have been applied, so migrate() only
// runs the ones past it — this is how columns get added to existing
// databases, which bare CREATE TABLE IF NOT EXISTS can't do. Append new
// steps at the end; never edit or reorder ones that have shipped, since
// deployed databases have already recorded them as applied.
var migrations = []string{
	// 1: base schema. Kept idempotent (IF NOT EXISTS) because databases
	// created before versioning existed have these tables at user_version 0.
	`
	CREATE TABLE IF NOT EXISTS watchlist (
		ticker TEXT PRIMARY KEY,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS daily_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		date TEXT NOT NULL,
		open REAL,
		close REAL,
		high REAL,
		low REAL,
		volume INTEGER,
		change_percent REAL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(ticker, date)
	);

	CREATE TABLE IF NOT EXISTS recommendations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date TEXT NOT NULL,
		ticker TEXT NOT NULL,
		reason TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 2: signal_states remembers the last state we notified per
	// (ticker, signal family) so daily signal checks can diff against it —
	// MACD golden/death-cross detection and RSI dedup both live here.
	// recommendations gains the explicit action (BUY/SELL/HOLD) and the
	// price at recommendation time, which /track compares against later.
	`
	CREATE TABLE IF NOT EXISTS signal_states (
		ticker TEXT NOT NULL,
		signal TEXT NOT NULL,
		state TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (ticker, signal)
	);

	ALTER TABLE recommendations ADD COLUMN action TEXT NOT NULL DEFAULT '';
	ALTER TABLE recommendations ADD COLUMN price REAL NOT NULL DEFAULT 0;
	`,
	// 3: positions/transactions back Phase 2's asset tracking. positions
	// holds one row per ticker with the cost-basis-weighted average price
	// (RecordBuy/RecordSell keep it in sync); transactions is the full
	// buy/sell log, including realized_pnl for sells. net_worth_snapshots
	// records total position value once a day (RunClosingSnapshot) so a net
	// worth curve can be drawn later.
	`
	CREATE TABLE IF NOT EXISTS positions (
		ticker TEXT PRIMARY KEY,
		shares REAL NOT NULL,
		avg_cost REAL NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		side TEXT NOT NULL,
		shares REAL NOT NULL,
		price REAL NOT NULL,
		fee REAL NOT NULL DEFAULT 0,
		date TEXT NOT NULL,
		realized_pnl REAL NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS net_worth_snapshots (
		date TEXT PRIMARY KEY,
		total_value REAL NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 4: universe is Phase 2.6's candidate scan pool — much bigger than
	// watchlist, seeded once from an embedded S&P 500 list (see universe.go's
	// seedSP500) plus whatever the user adds manually via /universe add.
	// scan_hits logs which universe tickers the daily scan job found a fresh
	// RSI/MACD signal on (no uniqueness constraint: a ticker can log more than
	// one hit the same day) so the same evening's daily report can pull
	// today's rows and upgrade those tickers into LLM candidates.
	`
	CREATE TABLE IF NOT EXISTS universe (
		ticker TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS scan_hits (
		ticker TEXT NOT NULL,
		date TEXT NOT NULL,
		reason TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 5: recommendations gains a source column ("watchlist"/"movers"/"scan")
	// so /track can break its hit rate down by which candidate-sourcing path
	// actually produced a given call (Phase 3.8's deferred-from-2.6 "成效對照").
	// Existing rows get "" rather than being backfilled to "watchlist" — the
	// read path treats an empty source as "watchlist" for display, keeping
	// this migration a single cheap ALTER TABLE.
	`ALTER TABLE recommendations ADD COLUMN source TEXT NOT NULL DEFAULT '';`,
	// 6: settings is a generic single-value key/value table, first used by
	// Phase 3.6's manually-declared cash balance (key "cash_balance") — see
	// GetSetting/SetSetting. Generic rather than a dedicated cash_balance
	// table since "a table that stores a single value" is exactly what
	// PLAN.md's Phase 3.6 item asked for, and any future single-value
	// setting (there will likely be more as this grows into a broader
	// personal assistant, per CLAUDE.md's project description) reuses this
	// table instead of its own one-off migration.
	`
	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 7: thesis holds one free-text holding rationale per ticker (Phase 3.6
	// expansion's "論點日誌"), set/overwritten wholesale by /thesis — a
	// dedicated table rather than another settings key, since settings is for
	// single global values and this is one row per ticker. Deliberately no
	// history (no timestamped multi-entry log): a single-user low-frequency
	// bot doesn't need a thesis audit trail, just "what do I currently
	// believe about this position."
	`
	CREATE TABLE IF NOT EXISTS thesis (
		ticker TEXT PRIMARY KEY,
		thesis TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 8: pending_actions backs Phase 4's write-gating infrastructure (see
	// pending_actions.go) — a write tool running in the MCP subprocess (e.g.
	// record_buy/record_sell) has no Telegram bot of its own to ask for
	// confirmation, so it can only leave a proposal here; status moves
	// pending -> sent -> confirmed/rejected, driven by the main bot process.
	// No foreign key to any other table: action_type plus a free-form JSON
	// payload is enough for the bot to know what to execute once confirmed,
	// which keeps this table reusable for any future write-gated action
	// type, not just trades.
	`
	CREATE TABLE IF NOT EXISTS pending_actions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		action_type TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 9: universe gains a soft-delete flag for Phase 2.6 追加項's S&P 500
	// refresh (see docs/phase-2.6-universe-refresh.md and SyncSP500) — a
	// user's manual /universe remove must "stick" against a future re-sync
	// of the embedded ticker list, but a hard DELETE leaves no way to tell
	// "the user removed this" apart from "this ticker was never seeded" once
	// the row is gone. RemoveUniverseTicker now sets removed=1 instead of
	// deleting; GetUniverse/seedSP500's count check both then need to filter
	// or ignore it appropriately (see their own doc comments).
	`ALTER TABLE universe ADD COLUMN removed INTEGER NOT NULL DEFAULT 0;`,
	// 10: trade_lessons backs Phase 3.9's reflect-then-inject feedback loop
	// (see docs/research-tradingagents.md's "反思回饋迴路" section) — the
	// short, distilled takeaway ReviewTrade's prompt already asks for (see
	// KeyLessonMarker) gets parsed out and stored here, so a later
	// /recommend/daily report can inject it back into the prompt instead of
	// it only ever living in a Telegram message history. One row per
	// closed-trade review (both the automatic post-sell path and manual
	// /review both write here) — no uniqueness constraint, since re-running
	// /review on the same round is expected to produce a fresh row rather
	// than silently no-op.
	`
	CREATE TABLE IF NOT EXISTS trade_lessons (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		date TEXT NOT NULL,
		lesson TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
	// 11: positions gains a per-trade stop-loss price (Phase 3.11 PR1, see
	// docs/phase-3.11-trade-risk-management.md §3.1) — 0 is a safe "unset"
	// sentinel since a real stock price is never 0, mirroring universe's
	// removed flag and recommendations' price/source columns as a single
	// cheap ALTER TABLE. Set via /stop or SetStopPrice; checkStopLossAlerts
	// (internal/bot/jobs.go) prefers this over the global STOP_LOSS_PCT
	// whenever it's set for a given position.
	`ALTER TABLE positions ADD COLUMN stop_price REAL NOT NULL DEFAULT 0;`,
	// 12: Phase 6 PR1's market column — watchlist/positions/transactions/
	// recommendations each gain a "us"/"tw" market tag (see
	// docs/phase-6-tw-market.md §4.2) so per-market queries (a TW-only
	// closing snapshot, a two-book /portfolio, a per-market web dashboard
	// filter) don't have to re-derive it from ticker shape via SQL. The
	// UPDATE ... GLOB backfill is defensive (a pre-Phase-6 database has no TW
	// rows to begin with) rather than load-bearing, but costs nothing to run.
	// market.Of is the single source of truth this backfill (and every write
	// path from here on) mirrors — see that function's doc comment.
	//
	// net_worth_snapshots' PK is date alone, and SQLite can't ALTER a
	// primary key — the whole table is rebuilt with PK (date, market)
	// instead, backfilling every existing row as 'us' (the only market that
	// existed before this migration). This is the project's first
	// rebuild-a-table migration rather than an append-only ALTER TABLE; see
	// docs/phase-6-tw-market.md §8 for why this is flagged as the phase's
	// biggest single risk and why a pre-deploy backup check matters here
	// specifically.
	`
	ALTER TABLE watchlist       ADD COLUMN market TEXT NOT NULL DEFAULT 'us';
	ALTER TABLE positions       ADD COLUMN market TEXT NOT NULL DEFAULT 'us';
	ALTER TABLE transactions    ADD COLUMN market TEXT NOT NULL DEFAULT 'us';
	ALTER TABLE recommendations ADD COLUMN market TEXT NOT NULL DEFAULT 'us';
	UPDATE watchlist       SET market = 'tw' WHERE ticker GLOB '[0-9]*';
	UPDATE positions       SET market = 'tw' WHERE ticker GLOB '[0-9]*';
	UPDATE transactions    SET market = 'tw' WHERE ticker GLOB '[0-9]*';
	UPDATE recommendations SET market = 'tw' WHERE ticker GLOB '[0-9]*';

	CREATE TABLE net_worth_snapshots_new (
		date        TEXT NOT NULL,
		market      TEXT NOT NULL DEFAULT 'us',
		total_value REAL NOT NULL,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (date, market)
	);
	INSERT INTO net_worth_snapshots_new (date, market, total_value, created_at)
		SELECT date, 'us', total_value, created_at FROM net_worth_snapshots;
	DROP TABLE net_worth_snapshots;
	ALTER TABLE net_worth_snapshots_new RENAME TO net_worth_snapshots;
	`,
	// 13: transactions gains stop_price (Phase 8 PR1, see
	// docs/phase-8-trader-analytics.md §3.1) — only SELL rows ever get a
	// non-zero value, written by RecordSell from the position's stop_price
	// at the moment of sale (before that row is deleted on a full close).
	// BUY rows stay 0: a stop is normally set after entry via /stop, so
	// "the stop at entry" usually doesn't exist yet. 0 is the same unset/
	// pre-migration sentinel positions.stop_price already uses. This is
	// what lets R-multiple (realized P&L ÷ initial risk) be computed per
	// closed round from here on — Phase 3.11's stop-loss data finally has
	// a place to persist past a position closing out.
	`ALTER TABLE transactions ADD COLUMN stop_price REAL NOT NULL DEFAULT 0;`,
	// 14: buy_alerts backs the "notify me when a ticker reaches a price I'd
	// buy at" feature — the mirror image of positions.stop_price, but many
	// rows per ticker (no PK on ticker) since a user wants several price
	// points watched at once, and a ticker doesn't need an open position
	// (unlike stop_price, which lives on the positions row itself). Modeled
	// on trade_lessons/scan_hits' "many rows, no uniqueness constraint"
	// shape rather than positions/watchlist's one-row-per-ticker shape. See
	// internal/db/buy_alerts.go.
	`
	CREATE TABLE IF NOT EXISTS buy_alerts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		market TEXT NOT NULL,
		price REAL NOT NULL,
		direction TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_buy_alerts_ticker ON buy_alerts(ticker);
	`,
	// 15: Phase 12's options ledger. OCC symbols never enter positions.ticker
	// (see docs/phase-12-options.md §2.1 — market.Of would silently
	// misclassify one as a plain US stock, and every downstream consumer of
	// positions/daily_snapshots/signals would then compute something
	// confidently wrong) — two independent tables instead.
	// underlying/right/strike/expiry are always derived by option.Parse at
	// write time, never caller-supplied — same convention as the market
	// column. contracts is signed (+long/-short) so RecordOption's realized
	// P&L formula is the one place that formula is written (see
	// internal/db/options.go). iv_history is unrelated to the ledger itself
	// but shares this migration deliberately — it's a daily ATM-IV snapshot
	// with no consumer for 6-12 months (until there's enough history for an
	// IV rank/percentile), and Yahoo has no historical IV endpoint to
	// backfill from, so not starting the clock now means never having it.
	`
	CREATE TABLE IF NOT EXISTS option_positions (
		contract_symbol TEXT PRIMARY KEY,
		underlying      TEXT NOT NULL,
		right           TEXT NOT NULL,
		strike          REAL NOT NULL,
		expiry          TEXT NOT NULL,
		multiplier      INTEGER NOT NULL DEFAULT 100,
		contracts       REAL NOT NULL,
		avg_premium     REAL NOT NULL,
		stop_premium    REAL NOT NULL DEFAULT 0,
		updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS option_transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_symbol TEXT NOT NULL,
		underlying TEXT NOT NULL,
		right TEXT NOT NULL,
		strike REAL NOT NULL,
		expiry TEXT NOT NULL,
		multiplier INTEGER NOT NULL DEFAULT 100,
		action TEXT NOT NULL,
		contracts REAL NOT NULL,
		premium REAL NOT NULL,
		fee REAL NOT NULL DEFAULT 0,
		date TEXT NOT NULL,
		realized_pnl REAL NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_option_tx_underlying ON option_transactions(underlying);

	CREATE TABLE IF NOT EXISTS iv_history (
		underlying TEXT NOT NULL,
		date       TEXT NOT NULL,
		atm_iv     REAL NOT NULL,
		dte        INTEGER NOT NULL,
		PRIMARY KEY (underlying, date)
	);
	`,
	// 16: transactions gains remaining_shares — the unconsumed share count of
	// a BUY row's purchase lot, always 0 for SELL rows (same sentinel
	// convention as stop_price, migration 13). This switches cost-basis
	// tracking from a single blended weighted average (positions.avg_cost,
	// reblended on every buy and left untouched by a sell) to real FIFO lot
	// matching, which is what both US and TW brokers actually use: a sell
	// consumes the *oldest* open lot(s) first. The two methods agree until
	// the first partial sell after multiple buys at different prices — from
	// there on the blended average silently diverges from the broker's true
	// remaining cost basis. RecordBuy/RecordSell below do the FIFO
	// bookkeeping going forward; this migration's Go-code backfill step
	// (migrate() special-cases index 16 to call backfillFIFOLots after this
	// ALTER TABLE) fixes it for history already on record — the first
	// non-pure-SQL migration in this codebase, since FIFO replay needs
	// per-ticker running state a single SQL statement can't carry.
	`ALTER TABLE transactions ADD COLUMN remaining_shares REAL NOT NULL DEFAULT 0;`,

	// 17: transactions gains ext_id, an idempotency key for trades synced
	// in from an external source (Phase 16's Shioaji auto-bookkeeping) —
	// the brokerage's own per-deal dseq. Empty string (manual/CSV/paper
	// trades, the vast majority of rows) is excluded from the unique
	// index rather than given a NOT NULL UNIQUE constraint, since '' isn't
	// a real identity and would collide with itself past the first row.
	`
	ALTER TABLE transactions ADD COLUMN ext_id TEXT NOT NULL DEFAULT '';
	CREATE UNIQUE INDEX IF NOT EXISTS idx_tx_ext_id ON transactions(ext_id) WHERE ext_id != '';
	`,
	// 18: Phase 21's "論點日誌" turns thesis from one overwritable row per
	// ticker into an append-only journal — thesis_entries(ticker, text,
	// created_at), "current" thesis is just its latest row. See
	// GetThesis/SetThesis/GetThesisEntriesInRange (internal/db/thesis.go) for
	// the read/write rationale. Existing rows carry over as each ticker's
	// first entry (dated at their old updated_at) so no history is lost,
	// then the old thesis table is dropped — GetThesis/SetThesis were its
	// only callers and both now point at thesis_entries. The unique index on
	// (ticker, date(created_at)) is what SetThesis's ON CONFLICT upsert
	// targets for its "one edit per ticker per day" rule.
	`
	CREATE TABLE IF NOT EXISTS thesis_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		text TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_thesis_entries_ticker_day ON thesis_entries(ticker, date(created_at));
	INSERT INTO thesis_entries (ticker, text, created_at)
		SELECT ticker, thesis, updated_at FROM thesis;
	DROP TABLE thesis;
	`,

	// 19: price_events backs Phase 20's gap/big-move event log (see
	// docs/phase-20-price-event-log.md) — one row per (ticker, date), written
	// by RunClosingSnapshot when signals.CheckPriceEvent fires. summary is
	// left '' when the per-run LLM writeup cap (top 3 by move size) was
	// exceeded — no separate has_writeup flag, an empty string already means
	// "recorded but not written up." The unique index is this table's
	// dedup mechanism (HasPriceEvent checks it before writing) rather than
	// signal_states/Detector-style stateful tracking, since RunClosingSnapshot
	// already only runs once per market per day.
	`
	CREATE TABLE IF NOT EXISTS price_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		market TEXT NOT NULL,
		date TEXT NOT NULL,
		gap_pct REAL NOT NULL DEFAULT 0,
		change_pct REAL NOT NULL DEFAULT 0,
		summary TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_price_events_ticker_date ON price_events(ticker, date);
	`,

	// 20: price_events gains cumulative_pct, a multi-day decline event
	// alongside migration 19's single-day gap_pct/change_pct — same
	// (ticker, date) row/unique index, RunClosingSnapshot merges a same-day
	// single-day and cumulative hit into one row rather than the unique
	// index rejecting a second write.
	`ALTER TABLE price_events ADD COLUMN cumulative_pct REAL NOT NULL DEFAULT 0;`,

	// 21: fundamental_snapshots caches Phase 23 PR6's SEC EDGAR-derived
	// valuation/cash-flow summary, one row per US ticker (SEC EDGAR has no
	// ADR/20-F coverage, docs/phase-23-strategy-data-uplift.md §4.5, so a
	// TW ticker never gets a row here — no market column needed). Upserted
	// on ticker, refreshed on a 90-day TTL read against fetched_at (see
	// internal/data/sec.go for the fetch+compute side, internal/db's own
	// GetFundamentalSnapshot/SaveFundamentalSnapshot for the cache
	// read/write). pe_percentile/cash_flow_quality are nullable: a ticker
	// with too little annual history or a loss-making fiscal year simply
	// omits that line rather than showing a fabricated number.
	`
	CREATE TABLE IF NOT EXISTS fundamental_snapshots (
		ticker TEXT PRIMARY KEY,
		eps_annual REAL NOT NULL DEFAULT 0,
		pe_ratio REAL NOT NULL DEFAULT 0,
		pe_percentile REAL,
		ocf REAL NOT NULL DEFAULT 0,
		net_income REAL NOT NULL DEFAULT 0,
		cash_flow_quality REAL,
		as_of_fiscal_year_end TEXT NOT NULL DEFAULT '',
		fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,

	// 22: llm_runs backs Phase 19's LLM-input-transparency audit trail (see
	// docs/phase-19-llm-transparency.md) — one row per GenerateRecommendations
	// call (/recommend and the daily report both funnel through it), storing
	// the full, unprojected prompt input (input_json — see
	// recordLLMRun/internal/bot/pipeline.go for why nothing gets trimmed) and
	// the model's raw reply (output_raw) side by side, so a bad recommendation
	// can be diagnosed as "bad data in" vs "model made it up" instead of
	// guessing. model/latency_ms come from llm.Client.GenerateRecommendations'
	// own return values; no tokens column — ACP has no per-token usage to
	// report (see client.go's existing comment on that). watchlist_count/
	// candidate_count/news_count/candle_gap_count are computed once at write
	// time (not in the doc's original schema) so ListLLMRuns' list view can
	// show a summary without pulling every row's input_json over the wire,
	// which is the whole reason ListLLMRuns excludes it. candle_gap_count
	// specifically replaces a client-side (TS) weekday-only heuristic with
	// internal/market.IsTradingDay's real NYSE calendar (US only — TW falls
	// back to the same weekday-only check, since internal/market has no TW
	// calendar) — see pipeline.go's countCandleGaps. No retention policy
	// (§3's decision 5): add a DELETE alongside the existing backup job
	// later if the table ever grows large enough to matter.
	`
	CREATE TABLE IF NOT EXISTS llm_runs (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		kind             TEXT NOT NULL,
		market           TEXT NOT NULL,
		model            TEXT NOT NULL,
		latency_ms       INTEGER NOT NULL,
		created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
		input_json       TEXT NOT NULL,
		output_raw       TEXT NOT NULL,
		watchlist_count  INTEGER NOT NULL DEFAULT 0,
		candidate_count  INTEGER NOT NULL DEFAULT 0,
		news_count       INTEGER NOT NULL DEFAULT 0,
		candle_gap_count INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_llm_runs_created ON llm_runs(created_at DESC);
	`,

	// 23: blocked_news_sources backs Phase 19 PR2's news-source blacklist
	// (see docs/phase-19-llm-transparency.md §5) — one global table, not
	// scoped by market/ticker (media quality is a property of the media
	// outlet, and TW/US source names don't overlap anyway). source is the
	// exact string NewsItem.Source carries (Finnhub's `source`, Yahoo's
	// Publisher, cnyes'/Google News' own source field), compared
	// case-insensitively + trimmed by the filter (internal/data/newsfilter.go)
	// since the four providers don't agree on casing/whitespace.
	`
	CREATE TABLE IF NOT EXISTS blocked_news_sources (
		source     TEXT PRIMARY KEY,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`,
}

func (d *DB) migrate() error {
	var version int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		if _, err := d.conn.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if i+1 == 16 {
			if err := backfillFIFOLots(d.conn); err != nil {
				return fmt.Errorf("migration 16 backfill: %w", err)
			}
		}
		// PRAGMA doesn't support parameter binding.
		if _, err := d.conn.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			return fmt.Errorf("set user_version %d: %w", i+1, err)
		}
	}
	return nil
}

// fifoLot is one BUY transaction's purchase lot state while backfillFIFOLots
// replays history — remaining starts at shares and is decremented by later
// SELLs in the same ticker, oldest lot first.
type fifoLot struct {
	id             int64
	remaining      float64
	price, fee     float64
	originalShares float64
}

// backfillFIFOLots is migration 16's one-time Go-code step (see that
// migration's doc comment): it replays every ticker's transaction history in
// date order to compute each BUY row's true remaining_shares under FIFO —
// the same lot-consumption logic RecordSell now uses going forward — then
// recomputes positions.avg_cost for every currently open position from the
// resulting open lots, which is what actually fixes the reported mismatch
// for positions that already existed before this migration ran.
func backfillFIFOLots(conn *sql.DB) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, ticker, side, shares, price, fee FROM transactions ORDER BY ticker, date, id`)
	if err != nil {
		return err
	}
	openLots := make(map[string][]*fifoLot)
	for rows.Next() {
		var id int64
		var ticker, side string
		var shares, price, fee float64
		if err := rows.Scan(&id, &ticker, &side, &shares, &price, &fee); err != nil {
			rows.Close()
			return err
		}
		switch side {
		case "BUY":
			openLots[ticker] = append(openLots[ticker], &fifoLot{id: id, remaining: shares, price: price, fee: fee, originalShares: shares})
		case "SELL":
			lots := openLots[ticker]
			needed := shares
			for len(lots) > 0 && needed > 1e-9 {
				lot := lots[0]
				consume := math.Min(needed, lot.remaining)
				lot.remaining -= consume
				needed -= consume
				if lot.remaining <= 1e-9 {
					lots = lots[1:]
				}
			}
			openLots[ticker] = lots
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, lots := range openLots {
		for _, lot := range lots {
			if lot.remaining <= 1e-9 {
				continue
			}
			if _, err := tx.Exec(`UPDATE transactions SET remaining_shares = ? WHERE id = ?`, lot.remaining, lot.id); err != nil {
				return err
			}
		}
	}

	posRows, err := tx.Query(`SELECT ticker FROM positions`)
	if err != nil {
		return err
	}
	var tickers []string
	for posRows.Next() {
		var t string
		if err := posRows.Scan(&t); err != nil {
			posRows.Close()
			return err
		}
		tickers = append(tickers, t)
	}
	if err := posRows.Err(); err != nil {
		posRows.Close()
		return err
	}
	posRows.Close()

	for _, ticker := range tickers {
		lots := openLots[ticker]
		var totalShares, totalCost float64
		for _, lot := range lots {
			if lot.remaining <= 1e-9 {
				continue
			}
			costPerShare := lot.price + lot.fee/lot.originalShares
			totalShares += lot.remaining
			totalCost += lot.remaining * costPerShare
		}
		if totalShares <= 1e-9 {
			continue
		}
		if _, err := tx.Exec(`UPDATE positions SET avg_cost = ? WHERE ticker = ?`, totalCost/totalShares, ticker); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) AddTicker(ticker string) error {
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO watchlist (ticker, market) VALUES (?, ?)`, ticker, string(market.Of(ticker)))
	return err
}

func (d *DB) RemoveTicker(ticker string) error {
	_, err := d.conn.Exec(`DELETE FROM watchlist WHERE ticker = ?`, ticker)
	return err
}

// GetWatchlist returns every watchlist ticker regardless of market, for
// callers that want "the whole list" (chat context injection, /list,
// /status with no argument) — see GetWatchlistByMarket for a single-market
// query.
func (d *DB) GetWatchlist() ([]string, error) {
	rows, err := d.conn.Query(`SELECT ticker FROM watchlist ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tickers = append(tickers, t)
	}
	return tickers, rows.Err()
}

// GetWatchlistByMarket returns watchlist tickers for one market only (Phase
// 6, market.US or market.TW) — used by the per-market jobs (closing
// snapshot, and PR1's US-only daily-report/recommend input gathering) that
// must not mix the two.
func (d *DB) GetWatchlistByMarket(m market.MarketID) ([]string, error) {
	rows, err := d.conn.Query(`SELECT ticker FROM watchlist WHERE market = ? ORDER BY ticker`, string(m))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tickers = append(tickers, t)
	}
	return tickers, rows.Err()
}

func (d *DB) SaveSnapshot(s DailySnapshot) error {
	_, err := d.conn.Exec(`
		INSERT OR REPLACE INTO daily_snapshots
		(ticker, date, open, close, high, low, volume, change_percent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Ticker, s.Date, s.Open, s.Close, s.High, s.Low, s.Volume, s.ChangePercent,
	)
	return err
}

// GetSnapshotClose returns the stored closing price for ticker on date, or
// ok=false if no snapshot exists for that day.
func (d *DB) GetSnapshotClose(ticker, date string) (close float64, ok bool, err error) {
	err = d.conn.QueryRow(
		`SELECT close FROM daily_snapshots WHERE ticker = ? AND date = ?`,
		ticker, date,
	).Scan(&close)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return close, true, nil
}

// GetLatestSnapshot returns the most recent daily_snapshots row for ticker
// regardless of date, or ok=false if none exists yet. Used by chat context
// injection, which wants "whatever we last saw" rather than a specific
// day's close.
func (d *DB) GetLatestSnapshot(ticker string) (DailySnapshot, bool, error) {
	s := DailySnapshot{Ticker: ticker}
	err := d.conn.QueryRow(`
		SELECT date, open, close, high, low, volume, change_percent
		FROM daily_snapshots WHERE ticker = ? ORDER BY date DESC LIMIT 1`,
		ticker,
	).Scan(&s.Date, &s.Open, &s.Close, &s.High, &s.Low, &s.Volume, &s.ChangePercent)
	if err == sql.ErrNoRows {
		return DailySnapshot{}, false, nil
	}
	if err != nil {
		return DailySnapshot{}, false, err
	}
	return s, true, nil
}

func (d *DB) SaveRecommendations(date string, recs []Recommendation) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, r := range recs {
		_, err := tx.Exec(`INSERT INTO recommendations (date, ticker, action, reason, price, source, market) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			date, r.Ticker, r.Action, r.Reason, r.Price, r.Source, string(market.Of(r.Ticker)))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetRecommendationsSince returns all recommendations dated fromDate or later
// (dates are lexicographically comparable YYYY-MM-DD strings), oldest first.
func (d *DB) GetRecommendationsSince(fromDate string) ([]Recommendation, error) {
	rows, err := d.conn.Query(
		`SELECT id, date, ticker, action, reason, price, source, market FROM recommendations
		 WHERE date >= ? ORDER BY date, id`,
		fromDate,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price, &r.Source, &r.Market); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// GetSignalState returns the last recorded state for (ticker, signal), or ""
// if none has been recorded yet.
func (d *DB) GetSignalState(ticker, signal string) (string, error) {
	var state string
	err := d.conn.QueryRow(
		`SELECT state FROM signal_states WHERE ticker = ? AND signal = ?`,
		ticker, signal,
	).Scan(&state)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return state, nil
}

// SetSignalState upserts the current state for (ticker, signal).
func (d *DB) SetSignalState(ticker, signal, state string) error {
	_, err := d.conn.Exec(`
		INSERT INTO signal_states (ticker, signal, state, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker, signal) DO UPDATE SET
			state = excluded.state,
			updated_at = excluded.updated_at`,
		ticker, signal, state,
	)
	return err
}

// lotAvgCost returns ticker's total remaining shares and FIFO-weighted
// average cost across its open BUY lots (transactions rows with
// remaining_shares > 0) — the single place RecordBuy/RecordSell derive
// positions.shares/avg_cost from, rather than incrementally blending, so
// avg_cost always reflects only the purchase lots that haven't been sold
// yet. A lot's own cost per share is price + fee/shares (that buy's fee
// spread only over its own shares, not the ticker's whole position).
func lotAvgCost(tx *sql.Tx, ticker string) (shares, avgCost float64, err error) {
	rows, err := tx.Query(`
		SELECT remaining_shares, price, fee, shares FROM transactions
		WHERE ticker = ? AND side = 'BUY' AND remaining_shares > 1e-9`, ticker)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var totalShares, totalCost float64
	for rows.Next() {
		var remaining, price, fee, origShares float64
		if err := rows.Scan(&remaining, &price, &fee, &origShares); err != nil {
			return 0, 0, err
		}
		totalShares += remaining
		totalCost += remaining * (price + fee/origShares)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if totalShares <= 1e-9 {
		return 0, 0, nil
	}
	return totalShares, totalCost / totalShares, nil
}

// RecordBuy records a BUY transaction as a new FIFO purchase lot
// (remaining_shares = shares) and recomputes the ticker's position from
// every open lot via lotAvgCost. It returns the position as it stands after
// the buy.
func (d *DB) RecordBuy(ticker string, shares, price, fee float64, date string) (Position, error) {
	return d.RecordBuyExt(ticker, shares, price, fee, date, "")
}

// RecordBuyExt is RecordBuy plus extID, an idempotency key from an external
// trade source (Phase 16's Shioaji sync) — "" (RecordBuy's behavior) for
// every other caller (web/paper/CSV import). A non-empty extID that's
// already on record fails with a unique-constraint error from the
// migration-17 partial index; callers that sync from an external source
// are expected to pre-filter against that, not rely on this as their only
// dedup check.
func (d *DB) RecordBuyExt(ticker string, shares, price, fee float64, date, extID string) (Position, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return Position{}, err
	}
	defer tx.Rollback()

	m := string(market.Of(ticker))

	if _, err := tx.Exec(`
		INSERT INTO transactions (ticker, side, shares, price, fee, date, remaining_shares, market, ext_id)
		VALUES (?, 'BUY', ?, ?, ?, ?, ?, ?, ?)`,
		ticker, shares, price, fee, date, shares, m, extID,
	); err != nil {
		return Position{}, err
	}

	totalShares, avgCost, err := lotAvgCost(tx, ticker)
	if err != nil {
		return Position{}, err
	}

	if _, err := tx.Exec(`
		INSERT INTO positions (ticker, shares, avg_cost, market, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ticker) DO UPDATE SET
			shares = excluded.shares,
			avg_cost = excluded.avg_cost,
			updated_at = excluded.updated_at`,
		ticker, totalShares, avgCost, m,
	); err != nil {
		return Position{}, err
	}

	if err := tx.Commit(); err != nil {
		return Position{}, err
	}
	return Position{Ticker: ticker, Shares: totalShares, AvgCost: avgCost, Market: m}, nil
}

// RecordSell records a SELL transaction against an existing position using
// FIFO lot matching — the oldest open BUY lot(s) are consumed first, same as
// both US and TW brokers — to compute this sale's realized P&L and the
// remaining position's cost basis (lotAvgCost over whatever lots are left).
// It returns ErrNoPosition if there is nothing open for the ticker, or
// ErrInsufficientShares if shares exceeds what's held — this project only
// tracks long positions, so short-selling isn't representable. Selling the
// full position deletes the positions row rather than leaving a zero-share
// one behind.
func (d *DB) RecordSell(ticker string, shares, price, fee float64, date string) (Position, float64, error) {
	return d.RecordSellExt(ticker, shares, price, fee, date, "")
}

// RecordSellExt is RecordSell plus extID — see RecordBuyExt's doc comment.
func (d *DB) RecordSellExt(ticker string, shares, price, fee float64, date, extID string) (Position, float64, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return Position{}, 0, err
	}
	defer tx.Rollback()

	var existingShares, stopPrice float64
	err = tx.QueryRow(`SELECT shares, stop_price FROM positions WHERE ticker = ?`, ticker).Scan(&existingShares, &stopPrice)
	if err == sql.ErrNoRows {
		return Position{}, 0, ErrNoPosition
	}
	if err != nil {
		return Position{}, 0, err
	}
	if shares > existingShares {
		return Position{}, 0, ErrInsufficientShares
	}

	lotRows, err := tx.Query(`
		SELECT id, remaining_shares, price, fee, shares FROM transactions
		WHERE ticker = ? AND side = 'BUY' AND remaining_shares > 1e-9
		ORDER BY date, id`, ticker)
	if err != nil {
		return Position{}, 0, err
	}
	type openLot struct {
		id                                int64
		remaining, price, fee, origShares float64
	}
	var lots []openLot
	for lotRows.Next() {
		var l openLot
		if err := lotRows.Scan(&l.id, &l.remaining, &l.price, &l.fee, &l.origShares); err != nil {
			lotRows.Close()
			return Position{}, 0, err
		}
		lots = append(lots, l)
	}
	if err := lotRows.Err(); err != nil {
		lotRows.Close()
		return Position{}, 0, err
	}
	lotRows.Close()

	needed := shares
	var costBasis float64
	for _, lot := range lots {
		if needed <= 1e-9 {
			break
		}
		consume := math.Min(needed, lot.remaining)
		costBasis += consume * (lot.price + lot.fee/lot.origShares)
		if _, err := tx.Exec(`UPDATE transactions SET remaining_shares = ? WHERE id = ?`, lot.remaining-consume, lot.id); err != nil {
			return Position{}, 0, err
		}
		needed -= consume
	}
	if needed > 1e-9 {
		// positions.shares (checked above) is supposed to always equal the
		// sum of open lots' remaining_shares — this means the two are out of
		// sync, a data-integrity bug rather than a normal error a caller
		// should retry.
		return Position{}, 0, fmt.Errorf("db: RecordSell: %s open lots %v shares short of the %v requested — position/lot data out of sync", ticker, shares-needed, shares)
	}

	realizedPnL := shares*price - costBasis - fee
	remainingShares := existingShares - shares
	var avgCost float64

	if math.Abs(remainingShares) < 1e-9 {
		remainingShares = 0
		if _, err := tx.Exec(`DELETE FROM positions WHERE ticker = ?`, ticker); err != nil {
			return Position{}, 0, err
		}
	} else {
		_, avgCost, err = lotAvgCost(tx, ticker)
		if err != nil {
			return Position{}, 0, err
		}
		if _, err := tx.Exec(`
			UPDATE positions SET shares = ?, avg_cost = ?, updated_at = CURRENT_TIMESTAMP WHERE ticker = ?`,
			remainingShares, avgCost, ticker,
		); err != nil {
			return Position{}, 0, err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO transactions (ticker, side, shares, price, fee, date, realized_pnl, stop_price, market, ext_id)
		VALUES (?, 'SELL', ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticker, shares, price, fee, date, realizedPnL, stopPrice, string(market.Of(ticker)), extID,
	); err != nil {
		return Position{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return Position{}, 0, err
	}
	return Position{Ticker: ticker, Shares: remainingShares, AvgCost: avgCost, Market: string(market.Of(ticker))}, realizedPnL, nil
}

// TransactionExtIDExists reports whether a transactions row with this
// ext_id has already been recorded (Phase 16's Shioaji sync dedup key —
// see migration 17). Always false for "" since that's not a real identity.
func (d *DB) TransactionExtIDExists(extID string) (bool, error) {
	if extID == "" {
		return false, nil
	}
	var exists bool
	err := d.conn.QueryRow(`SELECT EXISTS(SELECT 1 FROM transactions WHERE ext_id = ?)`, extID).Scan(&exists)
	return exists, err
}

// GetPosition returns the current position for ticker, or ok=false if
// there's no open position.
func (d *DB) GetPosition(ticker string) (Position, bool, error) {
	p := Position{Ticker: ticker}
	err := d.conn.QueryRow(
		`SELECT shares, avg_cost, stop_price, market, updated_at FROM positions WHERE ticker = ?`, ticker,
	).Scan(&p.Shares, &p.AvgCost, &p.StopPrice, &p.Market, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return Position{}, false, nil
	}
	if err != nil {
		return Position{}, false, err
	}
	return p, true, nil
}

// GetPositions returns every open position, ordered by ticker.
func (d *DB) GetPositions() ([]Position, error) {
	rows, err := d.conn.Query(`SELECT ticker, shares, avg_cost, stop_price, market, updated_at FROM positions ORDER BY ticker`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var positions []Position
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.Ticker, &p.Shares, &p.AvgCost, &p.StopPrice, &p.Market, &p.UpdatedAt); err != nil {
			return nil, err
		}
		positions = append(positions, p)
	}
	return positions, rows.Err()
}

// SetStopPrice sets ticker's per-trade stop-loss price (Phase 3.11 PR1, see
// the Position.StopPrice field comment). Returns ErrNoPosition — the same
// sentinel RecordSell uses, checked the same way via errors.Is — when there
// is no open position for ticker to set it on, detected via RowsAffected
// rather than a separate existence check.
func (d *DB) SetStopPrice(ticker string, price float64) error {
	res, err := d.conn.Exec(`UPDATE positions SET stop_price = ? WHERE ticker = ?`, price, ticker)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoPosition
	}
	return nil
}

// GetLatestRecommendations returns each ticker's most recent recommendation
// row (by insertion order, i.e. highest id) in one batched query, keyed by
// ticker — tickers with no recommendation history are simply absent from the
// map. Backs Phase 3.8's recommendation continuity: the caller feeds "what
// did we say last time" into today's prompt so a reversal comes with an
// explanation instead of silently contradicting an earlier call.
func (d *DB) GetLatestRecommendations(tickers []string) (map[string]Recommendation, error) {
	if len(tickers) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(tickers))
	args := make([]any, len(tickers))
	for i, t := range tickers {
		placeholders[i] = "?"
		args[i] = t
	}
	query := fmt.Sprintf(`
		SELECT id, date, ticker, action, reason, price, market FROM recommendations
		WHERE id IN (
			SELECT MAX(id) FROM recommendations WHERE ticker IN (%s) GROUP BY ticker
		)`, strings.Join(placeholders, ","))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]Recommendation)
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price, &r.Market); err != nil {
			return nil, err
		}
		out[r.Ticker] = r
	}
	return out, rows.Err()
}

// GetEarliestBuyDate returns the date of ticker's first recorded BUY
// transaction, or ok=false if there is none on record. Used to anchor the
// trailing-stop peak-close lookup to "since this position was first opened"
// rather than an arbitrary window.
func (d *DB) GetEarliestBuyDate(ticker string) (string, bool, error) {
	var date sql.NullString
	err := d.conn.QueryRow(
		`SELECT MIN(date) FROM transactions WHERE ticker = ? AND side = 'BUY'`,
		ticker,
	).Scan(&date)
	if err != nil {
		return "", false, err
	}
	if !date.Valid {
		return "", false, nil
	}
	return date.String, true, nil
}

// GetPeakClose returns the highest daily_snapshots close recorded for ticker
// on or after sinceDate, or ok=false if there's no snapshot in that range
// yet. Backs the trailing-stop check's running-high, computed on demand from
// existing snapshot history rather than a separately maintained column.
func (d *DB) GetPeakClose(ticker, sinceDate string) (float64, bool, error) {
	var peak sql.NullFloat64
	err := d.conn.QueryRow(
		`SELECT MAX(close) FROM daily_snapshots WHERE ticker = ? AND date >= ?`,
		ticker, sinceDate,
	).Scan(&peak)
	if err != nil {
		return 0, false, err
	}
	if !peak.Valid {
		return 0, false, nil
	}
	return peak.Float64, true, nil
}

// GetTransactions returns every recorded buy/sell for ticker, oldest first —
// transactions has been write-only (RecordBuy/RecordSell) until Phase 3.8's
// sell-review feature needed to read the full history back to segment it
// into trade rounds (see bot.lastClosedRound).
func (d *DB) GetTransactions(ticker string) ([]Transaction, error) {
	rows, err := d.conn.Query(`
		SELECT id, ticker, side, shares, price, fee, date, realized_pnl, stop_price, market, created_at
		FROM transactions WHERE ticker = ? ORDER BY date, id`,
		ticker,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.Ticker, &t.Side, &t.Shares, &t.Price, &t.Fee, &t.Date, &t.RealizedPnL, &t.StopPrice, &t.Market, &t.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

// GetAllTransactions returns every recorded buy/sell across every ticker,
// oldest first — the whole-table counterpart to GetTransactions(ticker).
// internal/web's daily P&L replay engine needs this: reconstructing shares
// held on any given day requires walking every transaction from the very
// first one, not a bounded window (unlike GetTransactionStats' date-range
// aggregate, which only needs a window's totals, not per-row detail).
func (d *DB) GetAllTransactions() ([]Transaction, error) {
	rows, err := d.conn.Query(`
		SELECT id, ticker, side, shares, price, fee, date, realized_pnl, stop_price, market, created_at
		FROM transactions ORDER BY date, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.Ticker, &t.Side, &t.Shares, &t.Price, &t.Fee, &t.Date, &t.RealizedPnL, &t.StopPrice, &t.Market, &t.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

// GetCloseExtremes returns the highest and lowest daily_snapshots close for
// ticker within [from, to] inclusive, or ok=false if there's no snapshot in
// that range. Unlike GetPeakClose (which only has a lower bound — right for
// a still-open position's running high), a closed trade's review needs both
// ends of a bounded window so a ticker that's kept trading (and setting new
// highs) after the position was closed doesn't leak into "how far the exit
// was from the period's high."
func (d *DB) GetCloseExtremes(ticker, from, to string) (high, low float64, ok bool, err error) {
	var h, l sql.NullFloat64
	err = d.conn.QueryRow(
		`SELECT MAX(close), MIN(close) FROM daily_snapshots WHERE ticker = ? AND date BETWEEN ? AND ?`,
		ticker, from, to,
	).Scan(&h, &l)
	if err != nil {
		return 0, 0, false, err
	}
	if !h.Valid {
		return 0, 0, false, nil
	}
	return h.Float64, l.Float64, true, nil
}

// GetRecommendationsForTicker returns ticker's recommendations dated within
// [from, to] inclusive, oldest first — a single-ticker, bounded-window
// counterpart to GetRecommendationsSince (which is whole-table and only has
// a lower bound, right for /track's rolling-window scan but not for a closed
// trade's fixed holding period).
func (d *DB) GetRecommendationsForTicker(ticker, from, to string) ([]Recommendation, error) {
	rows, err := d.conn.Query(
		`SELECT id, date, ticker, action, reason, price, source, market FROM recommendations
		 WHERE ticker = ? AND date BETWEEN ? AND ? ORDER BY date, id`,
		ticker, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Recommendation
	for rows.Next() {
		var r Recommendation
		if err := rows.Scan(&r.ID, &r.Date, &r.Ticker, &r.Action, &r.Reason, &r.Price, &r.Source, &r.Market); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// GetRealizedPnL sums realized_pnl across every SELL transaction ever
// recorded in market, for /portfolio's per-market cumulative realized P&L
// line (Phase 6: money never sums across markets, see
// docs/phase-6-tw-market.md §3.2).
func (d *DB) GetRealizedPnL(m market.MarketID) (float64, error) {
	var total sql.NullFloat64
	if err := d.conn.QueryRow(`SELECT SUM(realized_pnl) FROM transactions WHERE side = 'SELL' AND market = ?`, string(m)).Scan(&total); err != nil {
		return 0, err
	}
	return total.Float64, nil
}

// SaveNetWorthSnapshot upserts the total position value for (date, market) —
// see net_worth_snapshots' migration-12 PK rebuild (Phase 6): the two
// markets' totals are always tracked as separate rows, never summed, since
// TWD and USD don't convert.
func (d *DB) SaveNetWorthSnapshot(date string, m market.MarketID, total float64) error {
	_, err := d.conn.Exec(`
		INSERT INTO net_worth_snapshots (date, market, total_value)
		VALUES (?, ?, ?)
		ON CONFLICT(date, market) DO UPDATE SET total_value = excluded.total_value`,
		date, string(m), total,
	)
	return err
}

// GetLatestNetWorth returns the most recent net_worth_snapshots row for
// market regardless of date, or ok=false if none exists yet — same "most
// recent regardless of date" shape as GetLatestSnapshot, for the weekly
// review's net-worth line (Phase 3.6 PR2) and Phase 3.11's accountValue.
func (d *DB) GetLatestNetWorth(m market.MarketID) (date string, total float64, ok bool, err error) {
	err = d.conn.QueryRow(
		`SELECT date, total_value FROM net_worth_snapshots WHERE market = ? ORDER BY date DESC LIMIT 1`,
		string(m),
	).Scan(&date, &total)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return date, total, true, nil
}

// GetNetWorthOnOrBefore returns the most recent net_worth_snapshots row for
// market with date <= the given date, or ok=false if none exists — used to
// find a baseline "about a week ago" even when that exact date wasn't a
// trading day (weekend/holiday), same reasoning as GetPeakClose's date-range
// query.
func (d *DB) GetNetWorthOnOrBefore(date string, m market.MarketID) (float64, bool, error) {
	var total float64
	err := d.conn.QueryRow(
		`SELECT total_value FROM net_worth_snapshots WHERE market = ? AND date <= ? ORDER BY date DESC LIMIT 1`,
		string(m), date,
	).Scan(&total)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return total, true, nil
}

// NetWorthPoint is one date/total pair from net_worth_snapshots, returned by
// GetNetWorthRange for Phase 3.6 追加項's monthly report (see
// docs/phase-3.6-monthly-report.md) — the first reader to want a whole
// range of points rather than a single latest/on-or-before value.
type NetWorthPoint struct {
	Date  string
	Total float64
}

// GetNetWorthRange returns every net_worth_snapshots row for market with
// date in [from, to] inclusive, date ascending — the monthly report's raw
// input for its sparkline/drawdown calculations (both pure functions over
// this slice, see bot.sparkline/bot.maxDrawdownPct).
func (d *DB) GetNetWorthRange(from, to string, m market.MarketID) ([]NetWorthPoint, error) {
	rows, err := d.conn.Query(
		`SELECT date, total_value FROM net_worth_snapshots WHERE market = ? AND date BETWEEN ? AND ? ORDER BY date`,
		string(m), from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []NetWorthPoint
	for rows.Next() {
		var p NetWorthPoint
		if err := rows.Scan(&p.Date, &p.Total); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetTransactionStats summarizes every transaction with date in [from, to]
// inclusive, for the monthly report's trade-count and realized-P&L lines.
// sellCount is returned separately from realized (rather than the caller
// inferring "no sells" from realized == 0) because a month with SELLs that
// happen to net to exactly zero must still render its realized-P&L line —
// see docs/phase-3.6-monthly-report.md's distinction between "nothing to
// show" (skip) and "the number is zero" (show it).
func (d *DB) GetTransactionStats(from, to string) (count, sellCount int, realized float64, err error) {
	var sellCountNull sql.NullInt64
	var realizedNull sql.NullFloat64
	err = d.conn.QueryRow(`
		SELECT COUNT(*),
		       SUM(CASE WHEN side = 'SELL' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN side = 'SELL' THEN realized_pnl ELSE 0 END)
		FROM transactions WHERE date BETWEEN ? AND ?`,
		from, to,
	).Scan(&count, &sellCountNull, &realizedNull)
	if err != nil {
		return 0, 0, 0, err
	}
	return count, int(sellCountNull.Int64), realizedNull.Float64, nil
}

// GetTransactionStatsByMarket is GetTransactionStats scoped to one market
// (Phase 6 PR2, see docs/phase-6-tw-market.md §5.3) — RunMonthlyReport's
// per-market blocks must not sum a US and a TW transaction into the same
// count/realized-P&L figure, so this filters by the transactions.market
// column (populated at write time by RecordBuy/RecordSell, see that
// migration's doc comment) rather than reusing the whole-table query.
func (d *DB) GetTransactionStatsByMarket(from, to string, m market.MarketID) (count, sellCount int, realized float64, err error) {
	var sellCountNull sql.NullInt64
	var realizedNull sql.NullFloat64
	err = d.conn.QueryRow(`
		SELECT COUNT(*),
		       SUM(CASE WHEN side = 'SELL' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN side = 'SELL' THEN realized_pnl ELSE 0 END)
		FROM transactions WHERE date BETWEEN ? AND ? AND market = ?`,
		from, to, string(m),
	).Scan(&count, &sellCountNull, &realizedNull)
	if err != nil {
		return 0, 0, 0, err
	}
	return count, int(sellCountNull.Int64), realizedNull.Float64, nil
}

// GetSnapshotCloseRange returns ticker's first and last daily_snapshots
// close with date in [from, to] inclusive, or ok=false if fewer than two
// rows exist in that range (a single row can't express a period change).
// Unlike GetSnapshotClose (one exact date), this is the monthly report's
// "SPY start vs. end of month" lookup — first/last by date, not by row
// insertion order.
func (d *DB) GetSnapshotCloseRange(ticker, from, to string) (first, last float64, ok bool, err error) {
	rows, err := d.conn.Query(
		`SELECT close FROM daily_snapshots WHERE ticker = ? AND date BETWEEN ? AND ? ORDER BY date`,
		ticker, from, to,
	)
	if err != nil {
		return 0, 0, false, err
	}
	defer rows.Close()

	var closes []float64
	for rows.Next() {
		var c float64
		if err := rows.Scan(&c); err != nil {
			return 0, 0, false, err
		}
		closes = append(closes, c)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, false, err
	}
	if len(closes) < 2 {
		return 0, 0, false, nil
	}
	return closes[0], closes[len(closes)-1], true, nil
}

// GetRecentCloses returns ticker's n most recent daily_snapshots closes
// recorded so far, oldest first — RunClosingSnapshot's cumulative-decline
// check calls this before writing today's own row, so the result is exactly
// "the last n sessions before today," no date filter needed. Returns fewer
// than n (possibly zero) rows if daily_snapshots doesn't have enough history
// yet for ticker, which the caller treats as "not enough data, skip."
func (d *DB) GetRecentCloses(ticker string, n int) ([]float64, error) {
	rows, err := d.conn.Query(
		`SELECT close FROM (SELECT close, date FROM daily_snapshots WHERE ticker = ? ORDER BY date DESC LIMIT ?) sub ORDER BY date ASC`,
		ticker, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var closes []float64
	for rows.Next() {
		var c float64
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		closes = append(closes, c)
	}
	return closes, rows.Err()
}

// GetDailySnapshotsForTickers returns every daily_snapshots row for any of
// tickers dated within [from, to] inclusive, ordered by ticker then date —
// a batched IN query (same one-call-not-N-calls principle as
// GetLatestRecommendations/GetLessonsForTickers) rather than a per-ticker
// loop, since internal/web's daily P&L replay engine needs the full
// multi-ticker OHLCV series for a date range in one shot (every other
// daily_snapshots reader in this file returns a single scalar/pair, never a
// multi-row series across tickers). Returns nil, nil for an empty tickers.
func (d *DB) GetDailySnapshotsForTickers(tickers []string, from, to string) ([]DailySnapshot, error) {
	if len(tickers) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(tickers))
	args := make([]any, 0, len(tickers)+2)
	for i, t := range tickers {
		placeholders[i] = "?"
		args = append(args, t)
	}
	args = append(args, from, to)

	query := fmt.Sprintf(`
		SELECT id, ticker, date, open, close, high, low, volume, change_percent, created_at
		FROM daily_snapshots
		WHERE ticker IN (%s) AND date BETWEEN ? AND ?
		ORDER BY ticker, date`, strings.Join(placeholders, ","))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailySnapshot
	for rows.Next() {
		var s DailySnapshot
		if err := rows.Scan(&s.ID, &s.Ticker, &s.Date, &s.Open, &s.Close, &s.High, &s.Low, &s.Volume, &s.ChangePercent, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetSetting returns the stored value for key, or ok=false if it's never
// been set.
func (d *DB) GetSetting(key string) (string, bool, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSetting upserts value for key.
func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		key, value,
	)
	return err
}

// ResetTradingData wipes every positions/transactions/daily_snapshots/
// net_worth_snapshots row plus the cash_balance* settings keys. Not
// market-scoped — daily_snapshots has no market column to filter by (see
// migration 12's doc comment for which four tables got one) — so this is a
// full wipe, not a per-market one. Meant only for a dedicated
// single-purpose database like Phase 11's paper.db (see
// internal/bot/paper.go's handlePaper "reset confirm" path): calling this
// against the bot's real db.DB would destroy the user's actual trade
// history, and there is deliberately no other caller anywhere in this
// codebase.
func (d *DB) ResetTradingData() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"DELETE FROM positions",
		"DELETE FROM transactions",
		"DELETE FROM daily_snapshots",
		"DELETE FROM net_worth_snapshots",
		"DELETE FROM settings WHERE key IN ('cash_balance', 'cash_balance_twd')",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Backup writes a consistent point-in-time copy of the database to destPath
// via SQLite's VACUUM INTO, which is safe to run against a live database
// (unlike copying the file directly, which can catch it mid-write or miss
// WAL-journaled pages). destPath is bound as a parameter rather than
// interpolated into the SQL string, even though it's caller-controlled
// (config, not user input) — no reason to hand-format a file path into SQL
// when the driver can bind it safely.
func (d *DB) Backup(destPath string) error {
	_, err := d.conn.Exec(`VACUUM INTO ?`, destPath)
	return err
}

func (d *DB) Close() error {
	return d.conn.Close()
}

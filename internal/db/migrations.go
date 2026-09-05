package db

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

	// 24: notifications backs Phase 24 Stage 2's in-app notification history
	// (internal/notification's InAppNotificationStore) — the same background
	// alerts a Telegram push shows (stop-loss, restricted-stock, price
	// events, ...), kept so a future Web/App surface has history to read.
	// No market/ticker column: Event.Type + Text already identify what
	// happened, and not every event is ticker-scoped (e.g. job_panic).
	`
	CREATE TABLE IF NOT EXISTS notifications (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		type       TEXT NOT NULL,
		text       TEXT NOT NULL,
		level      TEXT NOT NULL,
		read       INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);
	`,

	// 25: podcast_insights backs the /podcast command (internal/bot/podcast.go)
	// — a stock-focused podcast transcript, pasted as a URL, gets fetched via
	// internal/webfetch and LLM-extracted into per-ticker (or ticker-less,
	// for a macro-only observation) rows. Append-only log, same
	// no-dedup/no-uniqueness-constraint convention as trade_lessons/scan_hits:
	// re-running /podcast on the same episode is expected to add fresh rows,
	// not silently no-op. No episode_date column — created_at (when it was
	// ingested) is enough for a single-user bot; the episode's own date, if
	// ever needed, is still in source_title/source_url for a human to read.
	`
	CREATE TABLE IF NOT EXISTS podcast_insights (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		source_url   TEXT NOT NULL,
		source_title TEXT NOT NULL,
		ticker       TEXT NOT NULL DEFAULT '',
		market       TEXT NOT NULL DEFAULT '',
		stance       TEXT NOT NULL,
		thesis       TEXT NOT NULL,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_podcast_insights_ticker ON podcast_insights(ticker);
	`,

	// 26: podcast_insights.derived_from (see PodcastInsight's doc comment) —
	// "" for a row grounded in something the transcript actually said, and a
	// short note (e.g. "NVDA: GPU demand") for a downstream-beneficiary row
	// the model inferred from its own supply-chain knowledge rather than the
	// transcript naming it directly — same "unverified, LLM's own claim"
	// shape as llm.ExploreNomination, so it needs to stay visibly
	// distinguishable from a grounded mention.
	`
	ALTER TABLE podcast_insights ADD COLUMN derived_from TEXT NOT NULL DEFAULT '';
	`,

	// 27: sell_followups backs Phase 26's post-sell follow-up review (see
	// docs/phase-26-sell-followup.md) — one row per (ticker, exit_date),
	// written only after a follow-up message actually sends successfully.
	// The unique index on (ticker, exit_date) IS the dedup mechanism (no
	// signal_states-style state machine needed, same as price_events
	// migration 18): RunClosingSnapshot already runs at most once per
	// market per day, so "row exists" is exactly "already followed up on
	// this exit." Writing only on success is deliberate, not an oversight —
	// an LLM failure or a not-yet-5-trading-days history simply leaves no
	// row, so bot.checkSellFollowups retries the same candidate the next
	// closing snapshot, up to its own 30-day age cutoff.
	// review_date is when the follow-up actually ran (may be later than the
	// 5th trading day if a retry was needed); price_at_review is always
	// that 5th trading day's close regardless of when the retry happened —
	// kept as two separate columns so a backfilled row is distinguishable
	// from an on-time one.
	`
	CREATE TABLE IF NOT EXISTS sell_followups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT NOT NULL,
		market TEXT NOT NULL,
		exit_date TEXT NOT NULL,
		review_date TEXT NOT NULL,
		exit_price REAL NOT NULL DEFAULT 0,
		price_at_review REAL NOT NULL DEFAULT 0,
		pct_since_exit REAL NOT NULL DEFAULT 0,
		verdict TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_sell_followups_ticker_exit ON sell_followups(ticker, exit_date);
	`,
}

# Phase 2.6 — Candidate Pool Scanning

Implements PLAN.md's Phase 2.6 items 1–3: a much larger *universe* of tickers than the
watchlist, scanned daily in chunks for the same RSI/MACD signals already used on the
watchlist, so `/recommend`/daily-report candidates surface because something concrete
happened (oversold, golden cross) rather than only because a ticker is on a "trending"
list.

## Seed data

`internal/db/sp500_tickers.txt` (embedded via `go:embed`) is 503 ticker symbols, one per
line, extracted from the `Symbol` column of
`https://raw.githubusercontent.com/datasets/s-and-p-500-companies/main/data/constituents.csv`
(the exact source PLAN.md names — a GitHub dataset that syncs Wikipedia's S&P 500
constituents list). Only the ticker is ever used, so this embeds a plain symbol list rather
than the full CSV — the full CSV's other columns (HQ location, etc.) have quoting
complications (`"Saint Paul, Minnesota"`) that a plain ticker list sidesteps entirely, and
nothing in this codebase reads sector/founding-year/etc. data.

## Schema (migration 4)

- `universe (ticker PRIMARY KEY, source, added_at)` — `source` is `'sp500'` (seeded) or
  `'manual'` (`/universe add`).
- `scan_hits (ticker, date, reason, created_at)` — no uniqueness constraint, since a ticker
  can log more than one signal hit the same day (e.g. both RSI oversold and a MACD golden
  cross); read back grouped by `db.GetScanHits`.

## Seeding is one-time, not resynced

`db.New()` calls `seedSP500()` right after `migrate()`. It checks
`SELECT COUNT(*) FROM universe WHERE source = 'sp500'` and only bulk-inserts if that's zero
— i.e. only on a database's first-ever startup. This is deliberate: a monthly refresh/diff
against the live S&P 500 list is explicitly deferred in PLAN.md ("這步可以先不做"), and
without a refresh mechanism, re-syncing on every startup would silently undo a user's
`/universe remove` of a seeded ticker. `TestSeedSP500` in `internal/db/db_test.go` covers
exactly this: remove a seeded ticker, call `seedSP500()` again, and confirm it stays gone.

## `/universe` command

`/universe` (no args) shows a count summary by source — deliberately never the full
~505-ticker list, which would exceed Telegram's practical message size for no benefit.
`/universe add TICKER` / `/universe remove TICKER` manage the manual tier; remove works on
any source (a user can prune an unwanted S&P 500 seed ticker too, and it won't come back —
see above).

## Chunking: stateless, no cursor table

`universeScanChunk(tickers, chunkCount, dayIndex)` (`internal/bot/bot.go`) is a pure
function: it splits `tickers` into `chunkCount` contiguous slices and returns the one at
`dayIndex % chunkCount`. `scanChunkCount = 5`, matching the closing-snapshot cadence of
Tue–Sat (5 US trading days/week) — ~503 tickers / 5 ≈ 100/day, matching PLAN.md's example
numbers. Called with `time.Now().In(cst).YearDay()` as `dayIndex`.

This was chosen over a persisted "scan cursor" row specifically to avoid a new piece of
mutable state to keep consistent across restarts/crashes. The tradeoff: chunk *boundaries*
shift slightly whenever universe membership changes (a ticker added/removed mid-cycle
shifts everything after it by one slot), so coverage isn't a mathematically exact "every
ticker exactly once every 5 calendar days" guarantee — it's "every ticker gets covered
roughly every 5 trading days, with occasional day-early or day-late reshuffling." PLAN.md
explicitly tolerates staleness on the order of months for this data, so this was an easy
tradeoff to accept. `TestUniverseScanChunkFullCoverage` in `internal/bot/bot_test.go` proves
the property that actually matters: a full `scanChunkCount`-call cycle against a *fixed*
ticker list covers every ticker exactly once with no gaps or duplicates.

## Scan job

`bot.RunUniverseScan` (new scheduler cron, 05:45 CST Tue–Sat — after the 05:30 closing
snapshot, before the 06:00 backup):

1. Loads the universe and watchlist, filters out any universe ticker already on the
   watchlist (the watchlist already gets a full signal check daily in `RunDailyReport` — no
   need to double-check it here, and doing so would also mean two different code paths
   racing to write the same `signal_states` row for that ticker).
2. Picks today's chunk via `universeScanChunk`.
3. For each ticker: fetches history from `HistoryProvider` (Yahoo) and reuses the
   **existing** `checkStatefulSignals` unchanged — the same RSI/MACD dedup-via-`signal_states`
   logic already used for the watchlist. This is safe to share because the watchlist and
   universe-scan ticker sets are disjoint by construction (step 1's filter), so there's no
   `(ticker, family)` key collision between the two jobs' writes.
4. Sleeps `universeScanRequestDelay` (300ms) between tickers to throttle Yahoo requests, per
   PLAN.md's explicit note not to hammer it — a ~100-ticker chunk takes roughly 30 seconds.
5. Any non-empty signal result is a hit: logged to `scan_hits` via `db.SaveScanHit`.
6. Silent like `RunClosingSnapshot` — logs only, no Telegram message. The eventual daily
   report is the user-facing surface for anything a scan hit produces.

## Candidate upgrade

Both `handleRecommend` and `RunDailyReport` now call `b.loadScanHits()` (today's
`db.GetScanHits`) alongside `GetMarketMovers()`, and merge the two into the final candidate
list via the new pure `mergeCandidates(movers, scanHits, watchlist)` — movers first
(preserves existing dedup/ordering behavior), then any not-already-present scan-hit ticker,
excluding anything on the watchlist. `fetchStockData` gained a `scanReasons` parameter
(alongside the existing `positions`/`earnings` maps) that sets the new
`llm.StockData.ScanReason` field for any ticker with a hit — rendered in the LLM prompt via
`KeyScanHitLine`, the same attach-and-render pattern already used for `Position`/`Earnings`.

Note this means the scan-hit upgrade applies to *both* `/recommend` and the daily report,
matching PLAN.md item 3's wording exactly ("進入 `/recommend`/daily report 的候選清單") —
not just the daily report, even though the scan job itself only runs once a day at 05:45.
An on-demand `/recommend` later in the day still benefits from that morning's scan.

## Explicitly deferred / out of scope

- **Monthly S&P 500 refresh** (diffing the live list, adding/removing `source='sp500'`
  rows): PLAN.md marks this "可以先不做" — skipped.
- **Item 4 (recording candidate source on `recommendations` for `/track` hit-rate
  breakdown)**: marked "考慮" (consider) in PLAN.md with no consumer built yet. Skipped
  rather than half-plumbed — a `source` column nothing reads is exactly the kind of
  speculative addition worth avoiding. Revisit when `/track` actually wants to break down by
  source.
- **Item 5 (two-stage LLM exploration)**: explicitly depends on Phase 3's still-unbuilt news
  work in PLAN.md. Out of scope here.

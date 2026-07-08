# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**Argus** is a personal (single-user) US stock monitoring bot that talks over Telegram. Built in Go, runs
in Docker, persists to SQLite. There is no multi-tenant/multi-user design anywhere — `chatID` is a single
fixed value from env, not a per-user table. The name (and the Go module path, `argus`) reflects an intent
to grow this beyond stocks into a broader personal assistant — the free-form `Chat` mode in `internal/llm`
is the first step in that direction; don't assume every feature here is stock-specific when extending it.
Two other things are today's implementation choices, not permanent constraints (see the README's "Vision"
section for the user-facing version of this): **Telegram** is currently the only messaging channel
(`internal/bot`) — a second channel should get its own package behind a shared interface, not be bolted
onto `bot.Bot`. **Claude via ACP** is currently the only LLM provider (`internal/llm`) — supporting another
provider is a real future direction, but ACP's session model (one-shot `prompt` calls vs. the persistent
`Chat` session) won't map 1:1 onto every provider's API, so that'll need a proper interface boundary
rather than a quick swap.

## Commands

```bash
go build ./...              # build everything
go run ./cmd/bot             # run locally (reads .env via godotenv)
go run ./cmd/bot mcp         # run as an MCP server over stdio instead (see internal/mcptools)
go vet ./...                 # static checks
docker compose up --build    # build + run in Docker (uses .env, mounts ./data -> /app/data)
```

There's no broad test suite; `internal/i18n` has the one exception (`go test ./internal/i18n/...`), which
checks the zh/en message tables stay in sync — see that package's entry below. Setup: copy `.env.example`
to `.env` and fill in `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` (required) and `FINNHUB_API_KEY`
(optional). The LLM needs no API key — run `claude` once on this machine and log in with your Claude
Pro/Max account first (see `internal/llm` below); Node.js (`npx`) must also be installed since the bot
shells out to an ACP agent process.

## Architecture

Flow: `cmd/bot/main.go` wires everything together — loads env, opens SQLite, builds the data provider
chain, constructs the LLM client, constructs the Telegram `bot.Bot`, registers the daily cron job, then
runs the Telegram long-poll loop until SIGINT/SIGTERM.

- `internal/data` — `Provider` interface (`GetQuote`, `GetNews`, `GetMarketMovers`). `finnhub.go` and
  `yahoo.go` each implement it independently. `provider.go`'s `Multi` wraps an ordered list of providers
  and tries each in sequence, falling back to the next on error — Finnhub is primary (if
  `FINNHUB_API_KEY` is set), Yahoo is the fallback. Any new data source should implement `Provider` and
  get appended to the list in `main.go`, not special-cased elsewhere. `FINNHUB_API_KEY` truthiness is
  checked by non-empty string, so a placeholder value left in `.env` silently enables Finnhub with a
  bad key (every quote wastes a doomed 401 request before falling back to Yahoo) — blank it out
  entirely if you don't have a real key.
  Yahoo-specific gotchas (found by testing against the live endpoint, not just reading the code):
  Yahoo's chart API `meta` object has no `regularMarketOpen` field despite what its name suggests — Open
  must be read from `indicators.quote[0].open` the same way High/Low already are, or it silently comes
  back as 0. An invalid/delisted ticker doesn't error either; it returns HTTP 200 with a real-looking but
  all-zero `meta` (`GetQuote` treats `RegularMarketPrice == 0` as "no data" to catch this — same pattern
  Finnhub already used for `result.C == 0`). `GetMarketMovers`'s `/trending/US` endpoint returns crypto
  pairs (`BTC-USD`) and foreign listings (`SHOP.TO`) mixed in with US equities and carries no
  asset-class field to filter on properly — `isUSEquitySymbol` filters by symbol shape (plain 1-5
  uppercase letters) as a heuristic; it's not perfect (e.g. `USDE`, a stablecoin ticker, slips through)
  but removes the two biggest offenders. `Yahoo.GetQuote`'s `Quote.Timestamp` comes from the chart
  meta's `regularMarketTime` (falling back to `time.Now()` only if absent) — keep it that way, since
  the closing-snapshot job relies on a real exchange timestamp to tell a fresh close from a US-holiday
  stale quote. Finnhub's `/quote` endpoint has no volume field at all (only
  `c/h/l/o/pc/t`), so `Finnhub.GetQuote`'s `Quote.Volume` is always 0 — this is a real API limitation,
  not a parsing bug; Claude has been observed calling this out unprompted in `/check` output.
  `internal/data/fundamentals.go` adds `Fundamentals` (ratios, from `/stock/metric`) and
  `FinancialStatement` (10-K/10-Q line items, from `/stock/financials-reported`) — both **Finnhub-only**,
  exposed via a separate `FundamentalsProvider` interface rather than folded into `Provider`/`Multi`,
  because Yahoo's equivalent (`quoteSummary`) now requires a crumb/cookie handshake (confirmed via live
  testing: returns 401 `Invalid Crumb` unauthenticated) that we deliberately don't implement. Finnhub's
  free tier also blocks `/stock/candle` entirely (`"You don't have access to this resource"`), so
  `Quote.Volume` can't be backfilled that way either without a paid plan. Statement line items are
  extracted from raw XBRL by matching a short list of known `concept` name aliases per field (see `find`
  in `GetFinancialStatements`) since filers don't all tag the same line item with the same XBRL concept —
  add more aliases if a ticker comes back with an unexpectedly-zero field rather than assuming the data
  doesn't exist. Finnhub's free tier is rate-limited to 60 req/min, so fundamentals are only fetched for
  watchlist tickers (`fetchStockData(..., includeFundamentals: true)`), never for the broad market-mover
  candidate list, which can be 15+ tickers. `HistoryProvider` (`GetHistory`, daily closes for RSI/MACD) is
  the mirror image of `FundamentalsProvider`: Yahoo-only, no `Multi` wrapper, because Finnhub's free tier
  blocks `/stock/candle` entirely — same constraint as the `Quote.Volume` gap above, just hitting a
  different feature this time. `EarningsProvider` (`earnings.go`) is Finnhub-only for the same reason as
  `FundamentalsProvider` — no Yahoo equivalent. `GetUpcomingEarnings(tickers, days)` fetches Finnhub's
  `/calendar/earnings` **without** a `symbol` filter (that param only accepts one ticker, so a per-ticker
  loop would cost one request each) and filters the whole-market response down to the requested tickers
  client-side via the pure, unit-tested `filterEarningsCalendar` — one API call regardless of watchlist
  size, unlike the fundamentals path's per-ticker calls. `MarketNewsProvider` (`marketnews.go`) is
  Finnhub-only for the same reason again: `GetMarketNews(limit)` hits `/news?category=general`, which
  (unlike `/company-news`) isn't scoped to any ticker — it's the whole-market/macro news source for the
  `/recommend`/daily-report news summary, not per-ticker headlines. No client-side filtering logic here
  (unlike `filterEarningsCalendar`), so no dedicated test file — same as `finnhub.go`'s other simple
  passthrough methods.
- `internal/db` — thin wrapper around `database/sql` + `modernc.org/sqlite` (pure-Go, no cgo). Owns nine
  tables: `watchlist`, `daily_snapshots`, `recommendations` (with `action` BUY/SELL/HOLD and `price` at
  recommendation time, both read back by `/track`), `signal_states` (last-notified state per
  ticker+signal family, backing MACD cross detection and RSI dedup), `positions` (one row per ticker,
  shares + weighted-average cost), `transactions` (the full buy/sell log, `realized_pnl` populated only
  for SELL rows), `net_worth_snapshots` (total position value by date), `universe` (the Phase 2.6 scan
  candidate pool — ticker + `source`, `'sp500'` or `'manual'`), and `scan_hits` (ticker/date/reason log
  of daily universe-scan signal hits, no uniqueness constraint since one ticker can log more than one
  hit a day). `universe.go`'s `seedSP500` bulk-inserts an embedded S&P 500 ticker list
  (`sp500_tickers.txt`, `go:embed`) into `universe` the first time `New()` ever sees an empty `sp500`
  source — deliberately not re-synced on every startup, so a user's manual `/universe remove` of a
  seeded ticker sticks (a monthly refresh/diff is a known, deliberately deferred gap — see
  docs/phase-2.6-universe-scan.md). Migrations are versioned via
  `PRAGMA user_version`: `migrate()` applies each entry of the ordered `migrations` slice past the
  recorded version — append new steps at the end, never edit or reorder shipped ones (deployed DBs have
  already recorded them as applied). Step 1 (the base tables) stays `IF NOT EXISTS`-idempotent because
  databases created before versioning existed sit at user_version 0 with those tables already present.
  `RecordBuy`/`RecordSell` are the only writers of `positions`/`transactions` and both wrap their
  read-modify-write in a transaction: `RecordBuy` recomputes the weighted-average cost
  (`(existingShares*existingCost + shares*price + fee) / totalShares`, so fees are folded into cost
  basis), `RecordSell` computes `realizedPnL = (price-avgCost)*shares - fee` and deletes the `positions`
  row outright once shares hits ~0 rather than leaving a zero-share row — it returns `ErrNoPosition` /
  `ErrInsufficientShares` (sentinel errors, checked with `errors.Is` in `bot.handleSell`) rather than
  ever going negative, since this project only tracks long positions. Both take an explicit `date`
  param (not always "today") so `/buy`/`/sell` can backdate a historical trade; `RecordBuy`'s
  weighted-average math is order-independent so backdated buys are safe in any order, but a backdated
  sell's `realizedPnL` is computed against whatever `avg_cost` the position holds *at call time* — enter
  backdated sells oldest-first or the realized P&L won't match what it would have been on that date.
  `GetLatestSnapshot` (most recent `daily_snapshots` row for a ticker, regardless of date) backs the
  chat context injection in `internal/bot` — see below. `Backup` runs `VACUUM INTO ?` (safe against a
  live DB, unlike copying the file directly) for the daily backup job.
- `internal/i18n` — every user/LLM-facing string in the project, split into exactly two files by design:
  `zh.go` (Traditional Chinese, the original default) and `en.go` (English), both keyed by the `Key`
  constants declared in `i18n.go`. `T(lang, key, args...)` does the lookup + `fmt.Sprintf`. This covers two
  different kinds of text that are easy to conflate: `internal/bot`'s UI copy (fixed strings/templates),
  and `internal/llm`'s system prompts + prompt-template text that Claude is instructed to follow — the
  latter isn't just cosmetic, since `KeyReasonMarker` ("原因:" / "Reason:") is both what the prompt asks
  Claude to emit *and* what `parseRecommendations` matches on to parse the reply; change one without the
  other and `/recommend`/`RunDailyReport` silently return zero recommendations. Every `Key` must have an
  entry in both `zh.go` and `en.go` with the same number of `fmt.Sprintf` verbs in the same order — call
  sites pass one set of positional args and reuse it for whichever table `T` picks. `i18n_test.go`
  (`TestTablesMatch`) enforces this automatically; run it after adding or editing any key. Language is
  selected once at startup via `BOT_LANGUAGE` (`zh`/`en`, default `zh`), threaded through
  `signals.NewDetector`, `llm.NewClient`, and `bot.New` — there's no per-message or per-user override, by
  the same single-user-bot design as `chatID`.
- `internal/llm` — `Client` talks to an LLM through an ordered chain of `Provider`s (`provider.go`:
  `Prompt` for one-shot calls, `NewChatSession`/`ChatSession.Send`/`Close` for a persistent multi-turn
  session) rather than any one backend directly — `[]backend` (provider + that provider's own
  recommend/check/chat model strings, since model aliases are provider-specific vocabulary — Claude's
  "opus"/"sonnet" mean nothing to a different backend), tried in order with fallthrough on error, same
  shape as `data.Multi` just for LLM calls. `NewClient` always seeds `backends[0]` with `acpProvider`
  (defined in `acp_provider.go`), which drives Claude through the **Agent Client Protocol (ACP)**, not the
  Anthropic API SDK. `internal/llm/acp` itself (`conn.go` + `session.go`) knows nothing about Claude — it's
  a generic ACP JSON-RPC-over-stdio transport/handshake driver (`initialize` → `session/new` →
  `session/prompt`, accumulating `session/update` text chunks) reusable by any ACP-speaking agent;
  `acp.StartSession(ctx, command, args, cwd, meta)` takes the launch command and the `_meta` payload as
  plain parameters rather than knowing what's inside them, since `_meta`'s contents are an
  implementation-specific ACP extension, not part of the base protocol. Everything Claude-specific lives in
  `acp_provider.go` instead: `claudeAgentCommand()` (resolves the `npx @agentclientprotocol/claude-agent-acp`
  launch, overridable via `CLAUDE_ACP_COMMAND`) and `startClaudeSession()`, which builds the
  `_meta.disableBuiltInTools`/`_meta.systemPrompt`/`_meta.claudeCode.options.model` fields that only
  `claude-agent-acp` understands before calling `acp.StartSession`. If a future backend also turns out to
  speak ACP (there's no other one today), it gets its own `<name>_provider.go` supplying its own
  command/meta — `internal/llm/acp` doesn't change. `acpProvider.Prompt` spawns a fresh `claude-agent-acp`
  subprocess per call and closes it once the reply arrives (one-shot, nothing to remember between calls),
  while `acpProvider.NewChatSession` returns an `acpChatSession` wrapping a single ACP session kept open
  across calls (`Client.chatSession`, lazily started, guarded by `Client.chatMu`) so the agent retains
  conversation history for free-form back-and-forth — `ResetChat` closes it early, and `Client.Close`
  (called once on bot shutdown in `main.go`) closes whatever's still open so the subprocess doesn't get
  orphaned when the bot exits. `acpProvider` authenticates via the operator's local `claude` CLI login
  (Claude Pro/Max subscription) instead of a metered `ANTHROPIC_API_KEY` — do not reintroduce an API key
  path without an explicit reason, that was a deliberate choice to avoid API billing. Every ACP session
  disables built-in tools and runs from `os.TempDir()` with a custom system prompt (a different one for
  chat vs. analysis — see `chatSystemPrompt`), so the agent never gets tool access and never picks up this
  repo's own CLAUDE.md/skills — keep both of those in place if you touch `startClaudeSession`.
  `Client.AddFallback` appends a second `backend` to the chain — `main.go` calls it with
  `antigravity_provider.go`'s `AntigravityProvider` (Google's Antigravity CLI, `agy -p`) only when
  `ANTIGRAVITY_ENABLED=true`, deliberately opt-in rather than presence-of-config-gated like Finnhub, because
  of the tradeoff below. `c.prompt`/`c.startChatSession` walk `c.backends` in order and return the first
  success, exactly like `data.Multi` (no special-casing a "quota exceeded" error over any other failure).
  For `Chat`, the chain is only consulted when (re)starting a session; once open, later calls reuse it
  until it errors, and the *next* call restarts from `backends[0]` again — so a session that fell back to
  Antigravity mid-conversation doesn't get stuck avoiding Claude forever, but falling back does lose
  whatever history the old session held (a `Provider`'s chat memory lives inside its own session, not in
  `Client`). Unlike `acpProvider`, `AntigravityProvider` always passes `--sandbox`: `agy -p` auto-approves
  every tool call it makes, including `write_file`, with no working read-only/plan-mode flag for
  non-interactive runs — a known upstream gap the user explicitly accepted the risk of rather than an
  oversight in this code (see PLAN.md's architecture-debt entry) — `--sandbox` contains the blast radius in
  an isolated container, it does not stop tool use, and requires the VPS to have a working
  sandbox/container runtime available to `agy`. `antigravityChatSession` also replays the full conversation
  transcript on every turn from Go-side state rather than resuming a backing session, because `agy -p` has
  no reliable session id to resume against (unlike `acpChatSession`, which relies on the ACP process's own
  memory). None of `AntigravityProvider` has been exercised against a real, logged-in `agy` install yet —
  treat its behavior as unverified until it has been, particularly the reported non-TTY stdout-drop bug
  (`ANTIGRAVITY_CLI_COMMAND` is the escape hatch to point at a wrapper if that bites, same pattern as
  `CLAUDE_ACP_COMMAND`). Model per call for the Claude backend is configurable
  (`CLAUDE_RECOMMEND_MODEL` / `CLAUDE_CHECK_MODEL` /
  `CLAUDE_CHAT_MODEL` env vars, default `opus` / `sonnet` / `sonnet`); the Antigravity fallback shares one
  `ANTIGRAVITY_MODEL` across all three call sites instead (empty uses `agy`'s own default) — it's a rarely
  invoked fallback path, not worth three separate knobs. `GenerateRecommendations`'s output
  is plain text following a hand-specified format (`[TICKER: X]` / `<reason marker>: ...` blocks) that
  `parseRecommendations` parses with string matching, not JSON — if you change that prompt's expected
  output shape, update the parser in lockstep (and see `internal/i18n`'s entry above for why the reason
  marker specifically must stay wired
  through `i18n.KeyReasonMarker` rather than a hardcoded literal). When `marketNews` is non-empty,
  `GenerateRecommendations` also asks the model (via `KeyRecMarketSummaryTask`) to emit a
  `[MARKET SUMMARY]` block (`i18n.KeyMarketSummaryMarker`) *before* its `[TICKER: ...]` blocks — the same
  raw reply gets a second, independent extraction pass via `parseMarketSummary(raw, marker)`, which grabs
  everything between the marker line and the first `[TICKER:` line. This works without touching
  `parseRecommendations` at all: it already ignores any text before the first `[TICKER:` line (it only
  starts collecting once it sees one), so the prepended summary block is silently skipped by the existing
  parser. `Chat` has no such format to parse; its
  reply is sent to the user verbatim. All LLM-facing prompts and bot-facing copy go through
  `internal/i18n` now — don't add a new hardcoded zh or en string in this package, add a `Key` instead.
  `StockData.Position` (a minimal `{Shares, AvgCost}` struct, deliberately not `db.Position` so this
  package doesn't import `internal/db`) is optional and set by `internal/bot`'s `fetchStockData` for any
  ticker the user holds — `writeStockSection` renders it as an unrealized-P&L line computed against the
  quote already in the same section, so cost basis is available for both `/recommend` and daily-report
  prompts wherever a position exists. `StockData.Earnings` (`{Date, DaysUntil}`, `DaysUntil` precomputed
  by the caller so this package doesn't do date math against "now") is the same pattern for an upcoming
  earnings report — `writeStockSection` renders `KeyEarningsLine` as a warning so the model doesn't call
  BUY on something reporting earnings tomorrow. `StockData.ScanReason` (`*string`) is the same
  attach-and-render pattern again for Phase 2.6's universe scan: set only for a candidate that was
  surfaced by a signal hit rather than the market-movers list, rendered via `KeyScanHitLine`.
- `internal/signals` — pure functions/struct for rule-based technical signals (price % threshold, RSI,
  MACD) independent of Telegram/LLM/DB. That purity is preserved for the stateful checks too:
  `CheckRSIState` and `CheckMACDCross` take the previously persisted state as a parameter and return the
  new state for the caller to persist — the DB round-trip lives in `bot.checkStatefulSignals`
  (`db.signal_states`), not here. `RunDailyReport` uses `CheckQuote` plus these two stateful checks, fed
  by `HistoryProvider.GetHistory` (see `internal/data` above): RSI only alerts on newly entering
  overbought/oversold (no repeat while it stays there), and MACD only alerts on an actual golden/death
  cross (`macd_golden_cross`/`macd_death_cross`), with a first-ever observation just recording the
  baseline silently. The stateless `CheckRSI`/`CheckMACD` still exist (the latter fires every call while
  a trend holds — that's why the daily path doesn't use it). `MACD`'s signal line is a genuine EMA9 over
  the MACD series (needs 26+9 closes to warm up, returns all-zero before that) — don't collapse it back
  to the single-point approximation this used to be (signal line hardcoded to 0), that gave wrong
  bullish/bearish reads. `Signal.Message` text goes through `internal/i18n` (`NewDetector(lang)`), same
  as everything in `internal/bot` — don't hardcode a new message string here either.
- `internal/scheduler` — thin wrapper around `robfig/cron` fixed to `time.FixedZone("CST", 8*3600)`
  (Taiwan time) rather than a loaded `time.Location`, specifically so it works in the `alpine` Docker
  image without needing the `tzdata` package installed. Five jobs: the daily report (21:00 CST daily),
  the closing snapshot (05:30 CST Tue–Sat — a US session ends at 04:00 or 05:00 CST the next morning
  depending on daylight saving, so 05:30 is past the close in both; Sun/Mon mornings follow no US
  session and are excluded at the cron level, while US holidays are handled by the job itself skipping
  stale quotes), the universe scan (`AddUniverseScan`, 05:45 CST Tue–Sat — after the closing snapshot,
  before the backup), log rotation (`AddLogRotation`, 00:00 CST daily — `lumberjack.Logger` only rotates
  on size by itself, so this cron call to `Rotate()` is what makes it an actual daily rotation), and the
  SQLite backup (`AddBackup`, 06:00 CST daily, after the closing snapshot so each backup includes that
  day's post-close data).
- `internal/bot` — Telegram command dispatch (`/add`, `/remove`, `/list`, `/status`, `/recommend`,
  `/check`, `/track`, `/buy`, `/sell`, `/portfolio`, `/dailyreport`, `/fundamentals`, `/universe`,
  `/reset`) plus three scheduler-invoked jobs: `RunDailyReport` (21:00 CST, pre-open), `RunClosingSnapshot`
  (05:30 CST Tue–Sat, post-close), and `RunUniverseScan` (05:45 CST Tue–Sat — see below). The former two:
  `RunClosingSnapshot` writes each watchlist ticker's completed-session OHLCV to
  `daily_snapshots` dated one day back in Taiwan terms (that's the US trading date at that hour) and
  skipping quotes whose timestamp is >12h old (US market holiday — the providers return the prior
  session, which would otherwise be filed under the wrong date). `/track [days]` reads `recommendations`
  back (the only reader) and scores each BUY/SELL against today's price for a hit rate; it prefers the
  `price` stored at recommendation time and falls back to the `daily_snapshots` close for older rows.
  `/buy`/`/sell` (`handleBuy`/`handleSell`, parsed by the shared `parseTradeArgs`) wrap
  `db.RecordBuy`/`RecordSell`; `/buy` also calls `db.AddTicker` so a bought position is always on the
  watchlist (positions are never auto-removed from it on a full sell — the user may still want to watch
  it). `parseTradeArgs` accepts an optional trailing date (`YYYY-MM-DD`, order-independent with the
  optional fee — distinguished by shape via `tradeDateRe`) for backdating historical trades; omitted
  date defaults to today in the handler, not the parser, so the parser stays a pure function (see
  `bot_test.go`'s `TestParseTradeArgs`). `/portfolio` (`handlePortfolio`) lists every `db.GetPositions()` row against a live quote for
  market value and unrealized P&L, plus `db.GetRealizedPnL()`'s all-time SELL total.
  `RunClosingSnapshot` calls `recordNetWorthSnapshot` after its per-ticker loop, reusing the quotes it
  already fetched for watchlist tickers (falling back to a direct quote fetch for any position ticker
  that isn't on the watchlist) to total position value into `net_worth_snapshots`.
  `loadPositions` (called once per `/recommend`/`RunDailyReport` run) builds a `ticker -> db.Position`
  map that `fetchStockData` attaches to `llm.StockData.Position` — this is how a held ticker's cost
  basis and unrealized P&L% reach the recommendation prompt (see `internal/llm`'s `KeyPositionLine`) so
  a SELL/HOLD call has an actual P&L to reason against, not just price action. `loadEarnings` is the
  same shape for `data.EarningsProvider` (nil-checked exactly like `Bot.fundamentals`): a single bulk
  call covering watchlist ∪ candidate tickers within `earningsPromptWindowDays` (14), attached by
  `fetchStockData` as `llm.StockData.Earnings` so a BUY call doesn't walk into next-day earnings
  volatility. `checkEarningsAlerts` is the separate, narrower-window (`earningsAlertDays`, 3) proactive
  Telegram reminder — called only from `RunDailyReport`, not `/recommend` (same asymmetry as
  `checkStatefulSignals` below), deduped via `signal_states` under the literal family string
  `"earnings"` (not one of `signals.FamilyRSI`/`FamilyMACD`, since this isn't a price-derived technical
  signal — `internal/signals` stays scoped to those) with `state` holding the earnings date string
  itself, so a ticker only re-alerts once its *next* earnings date rolls around. Both `handleRecommend`
  and `RunDailyReport` fetch `GetMarketMovers()` *before* building any `llm.StockData`, specifically so
  `loadEarnings` can cover the combined watchlist+candidate ticker set in one call rather than two.
  `loadMarketNews` is the same nil-checked-optional-provider shape again, for
  `data.MarketNewsProvider`: both callers fetch it once and pass it into
  `llm.GenerateRecommendations`, which returns `(summary, recs, err)` — `summary` is prepended as its own
  Telegram message by `sendAndSaveRecommendations` (via `KeyMarketNewsSummaryTitle`) when non-empty, but
  is never written to `recommendations` since it isn't a per-ticker call `/track` can score.
  `RunUniverseScan` is Phase 2.6's chunked candidate-pool scan (see docs/phase-2.6-universe-scan.md for
  the full design): each run picks a rotating slice of the `universe` table (excluding watchlist
  tickers) via the pure `universeScanChunk(tickers, scanChunkCount, dayIndex)` — stateless, no persisted
  cursor, `scanChunkCount=5` to match the Tue–Sat cadence — and reuses `checkStatefulSignals` unchanged
  (safe since the watchlist and universe-scan ticker sets never overlap, so no `signal_states` key
  collision). Hits are logged to `scan_hits`; both `handleRecommend` and `RunDailyReport` call
  `loadScanHits` (today's rows) and merge them into the candidate list via the pure
  `mergeCandidates(movers, scanHits, watchlist)`, attaching the hit reason as `llm.StockData.ScanReason`
  via `fetchStockData`'s `scanReasons` parameter. `Run`'s `dispatch` splits incoming messages two ways: commands each get their own goroutine
  (`go b.handleMessage(...)`) so a slow one like `/recommend` can't block a quick one like `/status` sent
  right after — but plain-text chat messages go on `chatQueue` instead, drained one at a time by the single
  `chatWorker` goroutine, so replies come back in the order the user actually sent them. That ordering
  guarantee is the reason chat isn't just `go b.handleChat(...)` too: it shares one persistent LLM session,
  and answering two chat messages concurrently could let the second reach that session before the first.
  `handleChat` (used only by `chatWorker`) is the bot's free-form chat mode, backed by `llm.Client`'s
  persistent session (see `internal/llm` below) — separate from the one-shot analysis commands. Every
  chat message is prefixed with `buildChatContext`'s output before being sent to the LLM: a read-only
  summary (watchlist ∪ position tickers, each one's `db.GetLatestSnapshot`, cost basis/unrealized P&L
  for held ones) rendered by the pure, independently-tested `formatChatContext`. This deliberately reads
  local `daily_snapshots` instead of live quotes — fetching a live quote per ticker on every chat
  message would add real latency to a conversational flow — and deliberately prefixes *every* message
  rather than injecting once per session, since ACP auth has no metered token cost (ordinary API pricing
  concerns don't apply — see the Pro/Max note below), so data freshness wins over the context bloat of
  repeating it each turn. `/reset`
  clears that persistent session's memory via `llm.Client.ResetChat`. `RunDailyReport` and `/recommend`
  share almost identical logic (fetch watchlist + market-mover candidates, run signal detection, call
  the LLM, then the shared `sendAndSaveRecommendations` tail: send results and persist ticker + action +
  reason + current price to `recommendations`) — when changing one, check whether the other needs the
  same change. `Bot.fundamentals` is a
  `data.FundamentalsProvider` that's `nil` whenever `FINNHUB_API_KEY` isn't set — every fundamentals
  code path (`/fundamentals`, and the fundamentals branch in `handleCheck`/`fetchStockData`) must nil-check
  it and degrade gracefully rather than erroring, since this data source is optional by design.
  `RunDailyReport`/`RunClosingSnapshot` both `defer b.recoverJobPanic(...)` — the bot runs unattended on
  a VPS, so a panic in either scheduler-invoked job must not just kill that goroutine silently; it logs
  and sends a Telegram alert (`KeyJobPanic`) instead. `RunClosingSnapshot`'s early-return on a failed
  `db.GetWatchlist()` also now sends `KeyWatchlistQueryFailed` (previously log-only), matching how
  `RunDailyReport` already handled the same failure — keep new whole-job-abort paths in either function
  user-visible; per-ticker failures inside the loop (one quote fetch failing, etc.) should stay log-only
  so a bad day for one ticker doesn't spam a Telegram alert.
- `internal/mcptools` — Phase 3.5's read-only MCP (Model Context Protocol) tool surface for chat, using
  the official `github.com/modelcontextprotocol/go-sdk`. `NewServer(lang, provider, history, fundamentals,
  earnings)` builds an `*mcp.Server` and registers seven tools (`tools.go`'s `registerTools`): `get_quote`/
  `get_history`/`get_news`/`get_market_movers` unconditionally, `get_fundamentals`/
  `get_financial_statements`/`get_upcoming_earnings` only when `fundamentals`/`earnings` are non-nil —
  same nil-check-and-degrade shape as `Bot.fundamentals`, so a client's `tools/list` never advertises a
  tool that would always fail without a Finnhub key. `Run` serves the result on `mcp.StdioTransport`.
  `NewServer` is kept separate from `Run` specifically so tests can add tools and connect over
  `mcp.NewInMemoryTransports` without going through stdio (see `tools_test.go`'s `connectTool`). Reached
  via `cmd/bot/main.go`'s `mcp` subcommand (`argus mcp` runs this instead of the Telegram bot) — branch on
  `os.Args[1] == "mcp"` happens *before* `godotenv.Load()`/`mustEnv` in `main()`, since the MCP subprocess
  (spawned by an ACP chat session via `os.Executable()`, never invoked directly by a human) needs none of
  the Telegram-specific env setup; `runMCPServer` does its own minimal `godotenv.Load()` +
  `FINNHUB_API_KEY` + `BOT_LANGUAGE` read and builds the same `Multi(finnhub?, yahoo)` chain `main()`
  does — kept as a separate, duplicated block rather than a shared helper since the two call sites need
  almost entirely disjoint env vars. Running the same binary as a subcommand — rather than a separately
  built/deployed server — is deliberate: it can never drift out of version sync with the running bot.
  `log` output in the `mcp` subprocess stays on its default stderr, **never** get redirected onto
  `os.Stdout` the way `main()`'s `io.MultiWriter` does for the Telegram path — `os.Stdout` here is the
  live MCP JSON-RPC stream (`mcp.StdioTransport`), and anything else written to it corrupts the protocol.
  Keep this package's dependency graph to `internal/data` + `internal/i18n` only (no
  `internal/db`/`internal/llm`/`internal/bot` imports) — see PLAN.md's Phase 3.5 rationale for why
  (provider-neutral tool surface that survives an `internal/llm` provider swap). This cost something
  concrete: `get_fundamentals`/`get_financial_statements` want the exact full-field-dump formatting
  `internal/bot`'s `/fundamentals` command already has (`formatFundamentals`/`formatFinancialStatement` in
  `bot.go`, built from `internal/i18n`'s granular per-field keys like `KeyPE`/`KeyROE`/`KeyStatementTitle`
  — see that package's entry above), but can't import `internal/bot` to reuse the functions themselves, so
  `tools.go` has its own copies of `formatFundamentals`/`formatFinancialStatement`/`commaf` that call the
  *same* i18n keys — keep those two implementations in sync by hand if either one's field list changes;
  don't let them drift into using different keys for the same field. Every tool handler routes errors
  through `ts.mcpErr(key, args...)` (an `i18n.T`-formatted `fmt.Errorf`) rather than returning a
  provider's raw Go error — the SDK's generic `AddTool` wrapper auto-packs a returned `error` into
  `CallToolResult{IsError: true}`, so a raw `err` would leak an untranslated, implementation-detail string
  like `"yahoo: no data for %s"` straight to the chat model regardless of `BOT_LANGUAGE`. An empty-but-
  no-error result (e.g. `get_news` finding zero headlines) is treated as an `IsError` result too, via the
  same helper — a chat model needs to be able to tell "no news" from "the tool call actually failed"
  instead of silently getting an empty string either way.

## Key behaviors to preserve

- LLM-backed commands (`/recommend`, `/check`) must reply immediately with an `i18n.KeyAnalyzing`/
  `KeyAnalyzingTicker` placeholder before the (slow) LLM call, since Telegram requests otherwise appear to
  hang. `handleChat` does the same with `KeyThinking`.
- `main.go` must call `llmClient.Close()` on shutdown (currently a `defer` right after construction) —
  the persistent chat session's `claude-agent-acp` subprocess has no other way to get cleaned up if the
  bot exits mid-conversation.
- The daily report is scheduled for 21:00 CST/Taiwan time — before US market open — via cron spec
  `"0 0 21 * * *"` in `scheduler.go`. The closing snapshot runs at `"0 30 5 * * 2-6"` (05:30 CST
  Tue–Sat, after US close); it dates snapshots one day back in Taiwan terms and must keep skipping
  quotes older than ~12h, or US-holiday reruns of the previous session get filed under the wrong date.
- `parseRecommendations` matches two i18n-driven markers, not one: `KeyActionMarker` ("動作:" /
  "Action:") and `KeyReasonMarker` ("原因:" / "Reason:"). Both appear in the `KeyRecTaskBlock` prompt
  template and must stay in lockstep with the parser (same constraint as the reason marker note in
  `internal/i18n` above). Actions are normalized to exactly BUY/SELL/HOLD; anything else parses as ""
  so `/track` and the display never see a made-up action word. A third marker,
  `KeyMarketSummaryMarker` ("[MARKET SUMMARY]", same in both languages), is independent of the two
  above — it's what `parseMarketSummary` looks for, and it must stay wired through
  `KeyRecMarketSummaryTask`'s `%s` verb rather than a hardcoded literal, same reasoning as the other two.
- `Multi` provider fallback depends on provider order in `main.go` (Finnhub before Yahoo); don't reorder
  without reason since Finnhub is considered the more reliable/richer source.
- The Dockerfile/docker-compose setup predates the ACP-based LLM client and has **not** been updated for
  it: the `alpine` image has no Node.js, and the Pro/Max login (macOS Keychain locally) has no equivalent
  credential path solved for a Linux container yet. Running the bot in Docker currently only works for the
  Telegram/data/DB parts, not `/recommend` or `/check`. Treat this as an open problem, not an oversight, if
  asked to containerize this.
- Migration steps append at the end of `db.migrations` and never get edited/reordered once shipped (see
  `internal/db` above) — this now also applies to `positions`/`transactions`/`net_worth_snapshots`
  (migration 3). `db.Backup`'s `VACUUM INTO` runs against the live DB via the same `*sql.DB` handle the
  bot uses, so it must stay a plain read (no schema/write locks held across it) or it'll contend with
  normal request handling.
- Log rotation (`AddLogRotation`) and backups (`AddBackup`) both run at fixed CST times distinct from the
  daily report/closing snapshot (00:00 and 06:00 respectively) — keep the backup after 05:30 (closing
  snapshot) so a day's backup always includes that day's post-close data, and don't move log rotation
  onto the same minute as another cron job for no reason (keeps log lines from either job from
  interleaving confusingly around a rotation boundary).

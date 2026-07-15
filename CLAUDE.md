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
  candidate list, which can be 15+ tickers. `HistoryProvider` (`GetHistory`, daily closes for RSI/MACD/moving
  averages) is the mirror image of `FundamentalsProvider`: Yahoo-only, no `Multi` wrapper, because Finnhub's
  free tier blocks `/stock/candle` entirely — same constraint as the `Quote.Volume` gap above, just hitting
  a different feature this time. `GetHistory`'s window is `range=1y` (Phase 3.7) rather than the `3mo` it
  used to be — a 200-day moving average needs ~200 trading days of closes, and the existing RSI(14)/MACD
  callers are unaffected since both only read the tail of the slice regardless of how much history sits in
  front of it. `EarningsProvider` (`earnings.go`) is Finnhub-only for the same reason as
  `FundamentalsProvider` — no Yahoo equivalent. `GetUpcomingEarnings(tickers, days)` fetches Finnhub's
  `/calendar/earnings` **without** a `symbol` filter (that param only accepts one ticker, so a per-ticker
  loop would cost one request each) and filters the whole-market response down to the requested tickers
  client-side via the pure, unit-tested `filterEarningsCalendar` — one API call regardless of watchlist
  size, unlike the fundamentals path's per-ticker calls. `MarketNewsProvider` (`marketnews.go`) is
  Finnhub-only for the same reason again: `GetMarketNews(limit)` hits `/news?category=general`, which
  (unlike `/company-news`) isn't scoped to any ticker — it's the whole-market/macro news source for the
  `/recommend`/daily-report news summary, not per-ticker headlines. No client-side filtering logic here
  (unlike `filterEarningsCalendar`), so no dedicated test file — same as `finnhub.go`'s other simple
  passthrough methods. `AnalystRatingProvider` (`analystrating.go`, Phase 3.7) is Finnhub-only for the
  same reason as `FundamentalsProvider` — and unlike `EarningsProvider`'s calendar, `/stock/recommendation`
  has no whole-market/unfiltered form at all, so `GetAnalystRating(ticker)` is a genuine one-ticker-per-call
  endpoint like `GetFundamentals`, not a client-side-filtered batch call like `filterEarningsCalendar`.
  Finnhub documents the response as most-recent-period-first but `GetAnalystRating` sorts by `period`
  defensively anyway before picking "current" vs. "previous" month, since trusting undocumented response
  order for that distinction is the kind of thing that fails silently. `AnalystRating.HasPrev` is false
  when Finnhub has only one period on record (a newly-covered ticker) — the `Prev*` fields are all zero in
  that case, and must not be read as "no analysts currently rate this stock."
- `internal/db` — thin wrapper around `database/sql` + `modernc.org/sqlite` (pure-Go, no cgo). Owns nine
  tables: `watchlist`, `daily_snapshots`, `recommendations` (with `action` BUY/SELL/HOLD, `price` at
  recommendation time, and `source` — `"watchlist"`/`"movers"`/`"scan"`, migration 5, `""` for rows saved
  before the column existed — all read back by `/track`, see `internal/bot` below), `signal_states` (last-notified state per
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
  live DB, unlike copying the file directly) for the daily backup job. `GetLatestRecommendations(tickers)`
  (Phase 3.8) batches "each ticker's newest recommendation" into one query
  (`WHERE id IN (SELECT MAX(id) ... GROUP BY ticker)`) rather than a per-ticker loop, same principle as
  `filterEarningsCalendar`'s single whole-market call in `internal/data`. `GetEarliestBuyDate`/
  `GetPeakClose` back the trailing-stop check's running-high: the peak is computed on demand from
  `daily_snapshots` closes on/after the ticker's first BUY date rather than maintained as a running column
  — `GetPeakClose` returns `ok=false` (via `sql.NullFloat64`, since `MAX()` on zero matching rows returns
  SQL NULL, not zero rows) when there's no snapshot in range yet, and callers must treat that as "skip the
  check," not as a peak of 0. `GetLatestNetWorth` (most recent `net_worth_snapshots` row regardless of
  date) and `GetNetWorthOnOrBefore(date)` (most recent row with `date <=` the given date, e.g. a
  weekend/holiday with no snapshot of its own) are Phase 3.6 PR2's first readers of a table that had
  none before — `bot.weeklyNetWorthLine` uses both together to diff "now" against "about a week ago"
  for the weekly review's opening line. `GetNetWorthOnOrBefore` uses a plain `ORDER BY date DESC LIMIT 1`
  (not `MAX()`), so it returns `sql.ErrNoRows` — not a NULL row — when nothing matches; unlike
  `GetPeakClose` it maps that to `ok=false` via an explicit `err == sql.ErrNoRows` check rather than
  `sql.NullFloat64`.
  `pending_actions` (migration 8, `pending_actions.go`) backs Phase 4's write-gating infrastructure: a
  write tool running in the MCP subprocess (`internal/mcptools`) has no Telegram bot of its own to ask
  for confirmation, so it can only leave a proposal here (`CreatePendingAction`) as `PendingActionStatusPending`.
  Status moves through exactly `pending → sent → confirmed|rejected`; `MarkPendingActionSent`/
  `ResolvePendingAction` are both `UPDATE ... WHERE id=? AND status=<expected-current>`, and the caller
  must check `RowsAffected() > 0` (returned as the method's `ok` bool) — this is the *only* guard against
  a double-tap on the same Telegram inline button (or a callback arriving after the row was already
  resolved some other way) executing the underlying trade twice; there is no separate locking mechanism.
  `PendingActionRecordBuy`/`PendingActionRecordSell` (the `action_type` strings) are declared here rather
  than in `internal/mcptools` (which creates rows) or `internal/bot` (which interprets them) specifically
  because both of those packages already import `internal/db` — unlike this codebase's other
  can't-share-an-import duplication (e.g. `formatFundamentals`), a hand-synced pair of string constants
  here would be a real footgun. See [docs/phase-4-write-gating.md](docs/phase-4-write-gating.md).
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
  `StockData.Technicals` (`{RSI14, MACDTrend, MA20, MA50, MA200}`, Phase 3.7) is the same pattern once
  more, set by `bot.computeTechnicals` from `HistoryProvider.GetHistory` — unlike Position/Earnings/
  ScanReason it's attached unconditionally for every ticker `fetchStockData` builds (watchlist *and*
  candidates), not gated behind a bulk-prefetched map, since candidates are exactly where the model most
  needs trend context before calling a fresh BUY and Yahoo's history endpoint carries none of Finnhub's
  rate-limit concern. `MACDTrend` is a plain string mirroring `signals.MACDTrend`'s own vocabulary
  ("bullish"/"bearish"/"" for not-enough-history) rather than an import of `internal/signals`, same
  reasoning as Position/Earnings staying package-local mini-structs. `MA20`/`MA50`/`MA200` are each 0 when
  there isn't enough history to compute them (`signals.MA` returns 0 as a sentinel, e.g. `MA200` on a
  recent IPO) — `writeStockSection` renders each moving-average line independently and skips any that are
  0 rather than showing a misleading `$0.00`, so `KeyTechnicalsMALine` is a single reusable
  `"%s MA%d ($%.2f)"` line (label/period/value), not three separate per-period keys.
  `KeyFundamentalsSummaryLine` also grew five more `%`-verbs in this phase for fields that were already
  being fetched but never rendered: `EPS`, `CurrentRatio`, `MarketCapMillion`, and `Week52High`/
  `Week52Low` converted to "% from 52-week high/low" via a `pctFrom(price, ref)` helper that returns 0
  when `ref` is 0 (a ticker Finnhub hasn't got a 52-week range for yet) rather than dividing by zero.
  `StockData.PrevRec` (Phase 3.8, `*PrevRecommendation`) is the same attach-and-render pattern once more,
  for recommendation continuity: `bot.loadPrevRecs`/`fetchStockData` only set it when a prior
  `db.Recommendation.Action` is non-empty (skips legacy pre-action rows and any the model failed to parse
  an action out of), and `DaysAgo` is precomputed by the caller exactly like `Earnings.DaysUntil` so this
  package still does no date math of its own. `KeyRecTaskBlock`'s prompt text was extended (no new `%s`
  verbs, same four in the same order) to tell the model that a reversal from `PrevRec.Action` needs an
  explicit "what changed" in its reasoning, not just a restated conclusion. `Client.WeeklyReview`
  (Phase 3.6 PR2) is the same one-shot-session shape as `InsightPortfolio` (Phase 3.6's portfolio-level
  analysis behind `/insight` — concentration risk, thesis check, add/reduce/stop-loss suggestions across
  every held position) and reuses `checkModel` for the same reason — it takes the same `positions`/
  `cash`/`haveCash` arguments as `InsightPortfolio` plus one more, `trackSummary` (a pre-rendered
  hit-rate/avg-return string, `""` when there's no recommendation history yet), folded into
  `buildWeeklyReviewPrompt` as its own section so the model's comment on recommendation accuracy comes
  out of the same call as its portfolio judgment rather than a second LLM round-trip.
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
  image without needing the `tzdata` package installed. Six jobs: the daily report (23:30 CST daily —
  at least an hour into the US session, see `AddDailyReport`'s doc comment for why 23:30 specifically),
  the closing snapshot (05:30 CST Tue–Sat — a US session ends at 04:00 or 05:00 CST the next morning
  depending on daylight saving, so 05:30 is past the close in both; Sun/Mon mornings follow no US
  session and are excluded at the cron level, while US holidays are handled by the job itself skipping
  stale quotes), the universe scan (`AddUniverseScan`, 05:45 CST Tue–Sat — after the closing snapshot,
  before the backup), the weekly review (`AddWeeklyReview`, Phase 3.6 PR2, 09:00 CST Sunday — no
  market-open/close time to align with since it's a weekend read rather than a reactive alert; by
  Sunday morning the most recent `net_worth_snapshots`/`daily_snapshots` row is already Friday's close,
  written by Saturday's 05:30 closing-snapshot run), log rotation (`AddLogRotation`, 00:00 CST daily —
  `lumberjack.Logger` only rotates on size by itself, so this cron call to `Rotate()` is what makes it
  an actual daily rotation), and the SQLite backup (`AddBackup`, 06:00 CST daily, after the closing
  snapshot so each backup includes that day's post-close data).
- `internal/render` — Telegram/chat-facing text formatting shared between `internal/bot` and
  `internal/mcptools`: `Fundamentals`/`FinancialStatement`/`Commaf`, depending only on `internal/data` +
  `internal/i18n` (same constraint `internal/mcptools` needs — see that package's entry below). Pulled
  out of a 2026-07 `internal/bot` refactor (see docs/refactor-internal-bot.md) that retired a hand-synced duplicate: the
  same ~15-line field-by-field assembly used to live once in `bot.go` and once in `mcptools/tools.go`
  because `mcptools` can't import `internal/bot`, with a CLAUDE.md/PLAN note to keep them in sync by
  hand. `internal/bot`'s `/fundamentals` command and `internal/mcptools`'s `get_fundamentals`/
  `get_financial_statements` tools both call into this package now; `mcptools`'s wrappers just prepend
  their MCP-specific ticker-header line before calling it (see that package's entry).
- `internal/webfetch` — Phase 3's "article digestion" chat mode: `ExtractURL(text)` (pure, regex-based)
  finds the first `http(s)` URL in a chat message, and `Fetch(ctx, url)` downloads and extracts that
  page's readable text via `golang.org/x/net/html`, skipping `script`/`style`/`nav`/`header`/`footer`/
  `aside`/etc. subtrees. This is bot-side fetching by design, same principle as `internal/data`'s
  providers — chat's ACP session runs with `disableBuiltInTools: true` (see `internal/llm`), so the
  agent has no network access of its own and never will; a second messaging channel or a future ACP
  tool-enabled mode doesn't change that, `internal/bot` still has to fetch and hand over plain text.
  `Fetch` treats extracted text under ~200 characters as a failure (`webfetch: extracted content too
  short`), not a successful-but-empty result — that's the observed signature of a paywall or a
  JS-rendered page whose initial HTML is mostly chrome with no article body; a non-2xx status and a
  non-`text/html` content type are also errors. Callers must treat any `Fetch` error as "couldn't read
  this page" and degrade gracefully rather than surfacing the raw error to the LLM (see `internal/bot`
  below) — there's no article text for the model to reason about in that case, so forwarding it wastes a
  call and invites the model to fabricate a summary. Extracted text is capped at `maxArticleRunes`
  (20,000 chars) — Claude via ACP has no metered per-token cost, but an unbounded page still means an
  unbounded, slower prompt.
- `internal/bot` — Telegram command dispatch (`/add`, `/remove`, `/list`, `/status`, `/recommend`,
  `/check`, `/track`, `/buy`, `/sell`, `/portfolio`, `/dailyreport`, `/fundamentals`, `/universe`,
  `/reset`) plus three scheduler-invoked jobs: `RunDailyReport` (23:30 CST, ~1–2h into the US session), `RunClosingSnapshot`
  (05:30 CST Tue–Sat, post-close), and `RunUniverseScan` (05:45 CST Tue–Sat — see below). Split across
  five files (2026-07 refactor, see docs/refactor-internal-bot.md; pure mechanical move, no behavior
  change) along the transport-vs-business line: `bot.go` (the `Bot` struct, `Config`, `New`,
  `Run`/`dispatch`/`chatWorker`/`Send`, `handleMessage`'s command routing table), `handlers.go` (the
  command handlers and their pure helpers, e.g. `parseTradeArgs`/`trackHit`/`trackSourceStats`),
  `jobs.go` (the three scheduler jobs and their job-only checks — signal detection, earnings alerts,
  stop-loss/trailing-stop), `pipeline.go` (`fetchStockData` and its `load*`/`compute*` helpers,
  `mergeCandidates`, `recommendationSources`, `gatherRecommendationInputs`,
  `sendAndSaveRecommendations` — see `RunDailyReport`'s entry below), and `format.go` (pure
  formatting/date helpers with no other home). The `bot.go`/`jobs.go`+`pipeline.go` boundary is drawn so
  a future second messaging channel (a deferred item — see PLAN.md's 架構債 "訊息通道介面") has
  somewhere to land without another reshuffle. `New` takes a `Config` struct (token, chatID, DB, the
  four data providers, LLM client, lang, thresholds) rather than its former 12 positional parameters.
  The former two:
  `RunClosingSnapshot` writes each watchlist ticker's completed-session OHLCV to
  `daily_snapshots` dated one day back in Taiwan terms (that's the US trading date at that hour) and
  skipping quotes whose timestamp is >12h old (US market holiday — the providers return the prior
  session, which would otherwise be filed under the wrong date). It also calls `snapshotBenchmark`
  (Phase 3.8), which snapshots `benchmarkTicker` (`"SPY"`, a plain const, not env-configurable) into
  `daily_snapshots` under the same date and stale-quote guard — SPY is deliberately never added to the
  `watchlist` table (it's not a holding, and `/list` shouldn't show it), so this is the only place SPY
  data enters the DB. `/track [days]` reads `recommendations` back (the only reader) and scores each
  BUY/SELL for a hit rate; it prefers the `price` stored at recommendation time and falls back to the
  `daily_snapshots` close for older rows. The hit rule itself (`trackHit`, pure) is Phase 3.8's
  relative-to-market benchmark: when a same-period SPY close is on record, BUY only counts as a hit if
  the ticker beat SPY's move and SELL only if it underperformed SPY — otherwise ("in a rally, everything
  looks like a good BUY") it falls back to the pre-Phase-3.8 absolute-direction rule (BUY up / SELL
  down), which is also what a date predating `snapshotBenchmark`'s rollout falls back to. `summarizeTrack`
  (pure, over `trackRow` values `handleTrack` builds from live quotes) folds hit rate into average
  BUY/SELL magnitude and — only rendered when more than one source appears in the window, via
  `sortedSourceKeys` for deterministic output — a hit-rate breakdown by `recommendations.source`
  (`displaySource` maps a stored `""` to `"watchlist"` for display, never rewriting the DB row itself).
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
  via `fetchStockData`'s `scanReasons` parameter. `computeTechnicals` (Phase 3.7) is `fetchStockData`'s and
  `handleCheck`'s shared helper for `llm.StockData.Technicals`: one `b.history.GetHistory(ticker)` call
  reduced via `signals.RSI`/`signals.MACDTrend`/`signals.MA` into RSI(14), MACD trend, and MA20/50/200 —
  a history-fetch failure logs and returns nil (degrades exactly like the fundamentals fetch beside it),
  never aborts the ticker. This duplicates the `GetHistory` call `RunDailyReport`'s own signal-check loop
  already makes for watchlist tickers (`checkStatefulSignals`'s stateful RSI/MACD-cross alerting) — the
  two aren't merged into one call because they serve different purposes (dedup-by-persisted-state alerts
  vs. raw values for the prompt) and Yahoo's history endpoint has no Finnhub-style rate-limit concern to
  justify the coupling. `Run`'s `dispatch` splits incoming messages two ways: commands each get their own goroutine
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
  repeating it each turn. `handleChat` also checks every message for a URL via `webfetch.ExtractURL`
  first (Phase 3's article digestion mode) and, if found, branches to `handleChatArticle` instead:
  `webfetch.Fetch` pulls the page's text, wraps it in `i18n.KeyArticleTaskBlock` (title/URL/content/the
  user's original message, so a comment alongside the link like "這對 NVDA 有沒有影響" still reaches the
  model) and sends *that* through the same persistent session in place of the raw message — still one
  chat turn, not a separate one-shot call, so the digestion stays in the conversation's memory for
  follow-up questions. A `webfetch.Fetch` error (dead link, paywall, JS-only page) is reported straight
  to the user via `KeyArticleFetchFailed` and never reaches the LLM — see `internal/webfetch`'s entry
  above for why. `/reset`
  clears that persistent session's memory via `llm.Client.ResetChat`. `RunDailyReport` and `/recommend`
  used to share their data-assembly head by a hand-maintained "when changing one, check the other" note;
  the 2026-07 `internal/bot` refactor (docs/refactor-internal-bot.md) replaced that with `pipeline.go`'s
  `gatherRecommendationInputs`, which both call first thing and which returns a `recommendationInputs`
  struct (watchlist ∪ candidate tickers, positions/earnings/market-news/prior-recs, and the resulting
  `[]llm.StockData` for both ticker sets) — a new prompt input now gets wired in exactly once. Each
  caller still owns its own middle: `RunDailyReport` runs signal detection, earnings alerts, and the
  stop-loss/trailing-stop checks (Phase 3.8) against the struct's fields (nothing in
  `gatherRecommendationInputs` itself has a side effect), and both share the `sendAndSaveRecommendations`
  tail — send results and persist ticker + action + reason + current price + source to
  `recommendations`. The `source` value comes from `recommendationSources(watchlist, candidates,
  scanHits)` (pure; called right before `sendAndSaveRecommendations` in both `handleRecommend` and
  `RunDailyReport`), which labels every ticker `"watchlist"`/`"scan"`/`"movers"` — a ticker in both
  `scanHits` and the candidate list is labeled `"scan"` (the more specific reason it was surfaced, see
  `llm.StockData.ScanReason`), and a ticker present in both `watchlist` and `candidates` (shouldn't
  happen given `mergeCandidates` already excludes watchlist tickers, but guarded anyway) keeps
  `"watchlist"` rather than whichever loop happened to run last. `Bot.fundamentals` is a
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
  `checkStopLossAlerts`/`checkTrailingStopAlerts` (Phase 3.8) are `RunDailyReport`-only, same asymmetry as
  `checkEarningsAlerts` — rule-based exit-discipline warnings, not something `/recommend` triggers.
  Both share one dedup helper, `breachAlertDecision(adverseMovePct, thresholdPct, prevState)`: a pure
  function (no DB/Telegram calls) that turns "how far has this moved against me" into
  breached/shouldAlert/newState, so the alert-once-then-reset logic lives in exactly one place instead of
  being duplicated per check. `signal_states` for both is a fixed `"breached"`/`""` string (not a
  computed value like RSI's state), under their own families (`"stop_loss"`/`"trailing_stop"`) so the two
  checks — and the unrelated `checkStatefulSignals` RSI/MACD dedup — never collide. `checkTrailingStopAlerts`
  computes its running-high on demand via two new DB reads, `GetEarliestBuyDate` (ticker's first BUY
  transaction date) and `GetPeakClose` (max `daily_snapshots` close on/after that date) — no separate
  running-high column, since a held ticker is always on the watchlist (via `/buy`'s auto-add) and so
  already accumulates daily snapshots. Both checks share `priceFor(ticker, prices)` — prefer an
  already-fetched quote from the caller's prefetch map, else one direct `GetQuote` fallback — which also
  replaced `recordNetWorthSnapshot`'s previously-inlined copy of the same fallback. `STOP_LOSS_PCT`/
  `TRAILING_STOP_PCT` (env, `Bot.stopLossPct`/`trailingStopPct`) are positive percentages; either at 0
  disables that check entirely rather than firing on every position. Phase 3.8's other item,
  recommendation continuity, is prompt-only (not an alert): `loadPrevRecs` wraps the new batched
  `db.GetLatestRecommendations(tickers)` (one `MAX(id) ... GROUP BY ticker` query, not a per-ticker
  loop — same one-call-not-N-calls principle as `loadEarnings`), and `fetchStockData` attaches it as
  `llm.StockData.PrevRec` for both `/recommend` and `RunDailyReport` (unlike the stop-loss checks, this
  is prompt input, not a proactive push, so both call sites get it) — see `internal/llm`'s entry below
  for the rendering side. `RunWeeklyReview` (Phase 3.6 PR2, `jobs.go`) is a fourth scheduler-invoked job
  (09:00 CST Sunday): the same per-position data assembly `handleInsight` uses (positions, technicals,
  fundamentals, earnings, thesis, vs-SPY, cash — `/insight`'s command handler), wrapped with a
  `weeklyNetWorthLine` opening line and a `renderEarningsPreview` block appended after the LLM's reply.
  `computeTrackRows(days)` (`pipeline.go`) is `handleTrack`'s core computation pulled out into its own
  method precisely so this job could reuse it: it returns both `rows` (for `summarizeTrack`) and `lines`
  (the full per-recommendation display `/track` itself renders) so neither caller repeats the
  quote/SPY-fetching logic — `handleTrack` is now a thin wrapper that renders `lines` verbatim and
  appends `renderTrackSummary(rows)`'s output, while `RunWeeklyReview` only needs `rows`, discarding
  `lines` (same "compute once, let callers use what they need" shape as `fetchStockData`).
  `renderTrackSummary` (pure, `format.go`) is the hit-rate/avg-return/by-source block factored out of
  `handleTrack` so both call sites render it identically; it returns `""` when nothing's been
  evaluated yet, so `RunWeeklyReview` can tell "no track data" apart from "data says nothing hit" and
  feed an empty `trackSummary` into `llm.Client.WeeklyReview` rather than an empty section header.
  `renderEarningsPreview(earnings, days)` (pure, `pipeline.go`) is deliberately not the same rendering
  `writeStockSection` already does per-position (`llm.StockData.Earnings`) — it's a consolidated,
  soonest-first list across every holding in one block, a distinct "plan for the coming week" view from
  `checkEarningsAlerts`'s narrower 3-day proactive alert (see `earningsAlertDays` above); returns `""`
  when nothing falls in the window so the block is skipped entirely rather than shown empty.
  `weeklyNetWorthLine` (`jobs.go`) reads `db.GetLatestNetWorth`/`GetNetWorthOnOrBefore` to diff the
  latest `net_worth_snapshots` value against roughly a week prior, returning `""` (skip, not a
  misleading 0%) when either read comes up empty — e.g. a fresh install, or the very first week after
  `RunClosingSnapshot` started writing snapshots. This job was deliberately wired up only after several
  manual `/insight` runs had proven the underlying prompt (see docs/phase-3.6-portfolio-insight.md) —
  an unproven prompt landing straight in an automatic Sunday push was the one thing the two-PR split
  for this phase was designed to avoid. `pending_actions.go` (Phase 4) is the bot-side half of the
  write-gating flow whose MCP-side half is `internal/mcptools`'s `record_buy`/`record_sell`: `Run`'s
  update loop now branches `update.CallbackQuery != nil` to `handleCallbackQuery` before falling through
  to ordinary message dispatch — the first non-`Message` update type this project handles.
  `sendPendingActionPrompts` is called from `handleChat`/`handleChatArticle` right after the LLM reply is
  sent (the only point in the chat flow where a tool call could have run that turn); it queries
  `db.GetPendingActionsByStatus(PendingActionStatusPending)`, sends each one a confirm/reject inline
  keyboard, and marks it `sent`. `handleCallbackQuery` always answers the callback (clears Telegram's
  button spinner) before doing anything else, then edits the original message in place with the outcome
  rather than sending a new one. `resolvePendingAction` does the atomic `sent → confirmed|rejected`
  transition via `db.ResolvePendingAction` and, only on a winning confirm, calls `executePendingAction`,
  which dispatches on `action_type` to `recordBuy`/`recordSell` — the same two methods `handleBuy`/
  `handleSell` call, pulled out of those handlers specifically so a confirmed chat-tool trade produces
  byte-identical confirmation text to typing `/buy`/`/sell` directly rather than a second implementation
  that could drift from the first. `tradePayload` here is `internal/mcptools`'s own copy with matching
  `json` tags (decode side, not encode) — same can't-share-an-import duplication as `formatFundamentals`.
  See [docs/phase-4-write-gating.md](docs/phase-4-write-gating.md).
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
  Keep this package's dependency graph narrow (`internal/data` + `internal/i18n`, plus `internal/db` for
  the Phase 3.5 追加項 read/write DB tools and `internal/render` for shared formatting — never
  `internal/llm`/`internal/bot`) — see PLAN.md's Phase 3.5 rationale for why (provider-neutral tool
  surface that survives an `internal/llm` provider swap). `get_fundamentals`/`get_financial_statements`
  want the exact full-field-dump formatting `internal/bot`'s `/fundamentals` command already has, built
  from `internal/i18n`'s granular per-field keys like `KeyPE`/`KeyROE`/`KeyStatementTitle` — this used to
  be two hand-synced copies of the same ~15-line assembly (one in `bot.go`, one in `tools.go`) before the
  2026-07 `internal/bot` refactor (docs/refactor-internal-bot.md) pulled the shared body out into
  `internal/render.Fundamentals`/`FinancialStatement`/`Commaf` (depends only on `data`+`i18n`, so both
  packages can import it); `tools.go`'s `formatFundamentals`/`formatFinancialStatement` are now thin
  wrappers that just prepend the MCP-specific ticker-header line. Every tool handler routes errors
  through `ts.mcpErr(key, args...)` (an `i18n.T`-formatted `fmt.Errorf`) rather than returning a
  provider's raw Go error — the SDK's generic `AddTool` wrapper auto-packs a returned `error` into
  `CallToolResult{IsError: true}`, so a raw `err` would leak an untranslated, implementation-detail string
  like `"yahoo: no data for %s"` straight to the chat model regardless of `BOT_LANGUAGE`. An empty-but-
  no-error result (e.g. `get_news` finding zero headlines) is treated as an `IsError` result too, via the
  same helper — a chat model needs to be able to tell "no news" from "the tool call actually failed"
  instead of silently getting an empty string either way. Every provider-hitting handler routes through
  `tools.go`'s `withCache(ctx, key, ttl, build)` — a cache hit returns immediately (no rate-limiter token
  consumed, no provider call); a miss waits on `ratelimit.go`'s hand-rolled `tokenBucket` (capacity 5,
  refill 0.5/sec — deliberately half of Finnhub's 60 req/min ceiling, since this subprocess can't see
  what the bot's own prefetch paths are doing against the same API key concurrently) before calling
  `build`, and only caches a successful result so a failed call stays retryable. TTL is tiered by how
  often the underlying data actually changes (`quoteCacheTTL` 30s, `shortCacheTTL` 5min for
  news/movers, `longCacheTTL` 1h for history/fundamentals/statements/earnings) — a new tool that hits a
  provider should go through this same helper rather than calling the provider directly, or it bypasses
  both the cache and the Finnhub rate-limit protection. See
  [docs/phase-3.5-mcp-rate-limit.md](docs/phase-3.5-mcp-rate-limit.md) for the full rationale.
  `record_buy`/`record_sell` (`trade_write_tools.go`, Phase 4) are gated on `ts.writeDB != nil` like the
  watchlist write pilot, but unlike those two, they never call a `db.RecordBuy`/`RecordSell`-shaped
  write directly — they only validate input and call `db.CreatePendingAction`, reporting the new
  `db.PendingAction` id back to the model as a proposal awaiting Telegram confirmation. This MCP
  subprocess has no Telegram bot of its own to show a confirm/reject keyboard with; `internal/bot` picks
  up `pending`-status rows after the chat turn that created them (see that package's entry). See
  [docs/phase-4-write-gating.md](docs/phase-4-write-gating.md).

## Key behaviors to preserve

- LLM-backed commands (`/recommend`, `/check`) must reply immediately with an `i18n.KeyAnalyzing`/
  `KeyAnalyzingTicker` placeholder before the (slow) LLM call, since Telegram requests otherwise appear to
  hang. `handleChat` does the same with `KeyThinking`.
- `main.go` must call `llmClient.Close()` on shutdown (currently a `defer` right after construction) —
  the persistent chat session's `claude-agent-acp` subprocess has no other way to get cleaned up if the
  bot exits mid-conversation.
- The daily report is scheduled for 23:30 CST/Taiwan time — at least an hour into the US session (see
  `scheduler.go`'s `AddDailyReport` doc comment for the DST-vs-standard-time reasoning behind that
  specific time) — via cron spec `"0 30 23 * * *"` in `scheduler.go`. The closing snapshot runs at
  `"0 30 5 * * 2-6"` (05:30 CST Tue–Sat, after US close); it dates snapshots one day back in Taiwan
  terms and must keep skipping quotes older than ~12h, or US-holiday reruns of the previous session get
  filed under the wrong date.
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

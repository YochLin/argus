# AGENTS.md

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
to `.env` and fill in `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` — since Phase 17 these may be left
blank, in which case the process still starts with Telegram disabled entirely (no bot, no scheduled
reports) and everything else running, so they can be filled in later from the web dashboard's Settings
page — plus `FINNHUB_API_KEY` (optional). The LLM needs no API key — run `claude` once on this machine and log in with your Claude
Pro/Max account first (see `internal/llm` below); Node.js (`npx`) must also be installed since the bot
shells out to an ACP agent process.

## Architecture

Flow: `cmd/bot/main.go` wires everything together — loads env, opens SQLite, builds the data provider
chain, constructs the LLM client, constructs the Telegram `bot.Bot`, registers the daily cron job, then
runs the Telegram long-poll loop until SIGINT/SIGTERM.

- `internal/data` — `Provider` interface (`GetQuote`/`GetNews`/`GetMarketMovers`), implemented
  independently by `finnhub.go` (primary) and `yahoo.go` (fallback via `Multi`). Separate optional
  interfaces (`FundamentalsProvider`, `HistoryProvider`, `EarningsProvider`, `MarketNewsProvider`,
  `AnalystRatingProvider`, `OptionChainProvider`) cover data Finnhub-only or Yahoo-only supports, each
  nil-checked by callers rather than folded into `Provider`. TW market support (`market.Of`-gated) spans
  `yahoo.go` (`.TW`/`.TWO` suffix resolution), `finmind.go` (TW fundamentals), `twse_movers.go` (TW
  market movers), `cnyes.go` (TW market news), `googlenews.go` (TW per-ticker Chinese news — a keyless
  Google News RSS search wired into the `Multi` chain between Finnhub and Yahoo, TW-only so the US path
  is unchanged), and `tw_earnings.go` (a statutory-deadline earnings proxy, no real API). `options.go`'s
  `OptionChainProvider` (US-only, Phase 12) is implemented by `yahoo.go`, which authenticates with a
  cookie + crumb handshake (`ensureCrumb`, cached on the `Yahoo` struct, retried once on a 401) that no
  other Yahoo endpoint needs — quote/history/news stay on the plain (cookie-less) `client`.
  `GetHistory` rewrites `rangeParam == "max"` to `"10y"` before hitting the wire — Yahoo's chart API
  silently ignores `interval=1d` for `range=max` and returns quarterly bars instead (live-verified:
  AAPL "max" comes back as 168 bars, matching "3mo"), which would otherwise corrupt anything computed
  off Yahoo history at that range (e.g. `/recs`/Distributions' h=1 return, MAE/MFE) without erroring.
  `sec.go`'s `FundamentalHistoryProvider` (US-only, Phase 23 PR6) wraps SEC EDGAR's free, keyless XBRL
  `companyfacts` API for valuation percentile (self-relative P/E, US EPS × price since Yahoo's free-tier
  fundamentals-timeseries caps out at 4 years) and cash-flow quality (OCF/NetIncome, same fiscal year) —
  briefing material only (never a ranking/filter factor). Only 10-K annual points are used (no TTM
  reconstruction from quarterly 10-Qs — false precision for a briefing line); `SEC_USER_AGENT` must
  contain a real, working email since SEC's edge filter technically requires an email-shaped UA and a bad
  one just gets the IP silently blocked, not bounced. `finmind.go`'s `GetFundamentalSnapshot` (Phase 23
  PR7) is the same `FundamentalHistoryProvider` interface's TW implementation — `TaiwanStockPER` already
  reports PER per trading day, so its own history is the percentile pool directly, no price series to
  align against EPS dates the way SEC's US path needs; valuation only, no cash-flow-quality equivalent for
  TW yet (`TaiwanStockFinancialStatements` has no cash-flow-statement fields). Both share one
  `internal/db` cache (`fundamental_snapshots`, see below), dispatched by `bot.cachedValuationSnapshot`.
  `earnings.go`'s `EarningsSurpriseProvider` (US-only, Phase 23 PR8) wraps Finnhub's `/stock/earnings`
  (live-verified 2026-08-20: free tier caps at the trailing 4 quarters regardless of a `from`/`to` range —
  unlike the Yahoo/SEC percentile cases, 4 quarters is exactly the right depth for a beat/miss-streak
  signal, not a crippled version of something that needed more). No DB cache — unlike PR6/PR7's valuation
  snapshot, Finnhub's response is already the whole useful window every call, so it rides the same
  in-memory `dataCache`/`slowDataCacheTTL` as Fundamentals/AnalystRating/InsiderTx.
  Full rationale, live-endpoint gotchas, and TW-specific design notes: **[docs/architecture/data.md](docs/architecture/data.md)**.

- `internal/option` — Phase 12's pure functions for US equity options, independent of
  `internal/db`/`internal/bot` like `internal/signals`. `contract.go`'s `Parse`/`Format`/`IsOCC` handle
  OCC/OSI symbols (e.g. `AAPL260805C00310000`); `Parse` anchors from the right (date+right+strike is
  always exactly 15 characters) so it doesn't depend on a fixed-width, space-padded underlying — live
  data has no such padding, only the textbook OCC spec does. `greeks.go` self-computes Black-Scholes
  greeks (`math.Erfc` for the normal CDF) since Yahoo's free option chain has none; `mark.go`'s `Mark`
  (mid-price, falling back to last only when bid/ask is unusable) is the single entry point for "what is
  this contract worth" everywhere else in the codebase — a thin contract's `lastPrice` can be a
  days-stale zombie trade. `select.go`'s `Select` screens a chain down to `Candidate`s matching a
  `Profile` (`LongCall`/`LongPut`/`CSP`/`CoveredCall` — delta/DTE bands and the liquidity gate are all
  `Profile` fields, not constants, since they're real-world calibration knobs); the liquidity gate
  (OI/volume/spread%) is checked before delta — a contract with a wide spread loses money even when the
  direction call is right. Full design: **[docs/phase-12-options.md](docs/phase-12-options.md)**.

- `internal/db` — thin wrapper around `database/sql` + `modernc.org/sqlite` (pure-Go, no cgo). Nine
  tables (`watchlist`, `daily_snapshots`, `recommendations`, `signal_states`, `positions`,
  `transactions`, `net_worth_snapshots`, `universe`, `scan_hits`) plus `pending_actions` (Phase 4
  write-gating) — migrations are versioned via `PRAGMA user_version`, append-only in `db.migrations`,
  never edited/reordered once shipped. `RecordBuy`/`RecordSell` own all `positions`/`transactions`
  writes with weighted-average cost and realized P&L math. TW support added a `market` column to four
  tables (migration 12) and rebuilt `net_worth_snapshots`'s primary key to `(date, market)`. Phase 12
  (migration 15) added `option_positions`/`option_transactions` (`options.go`) as two fully independent
  tables — an OCC symbol never enters `positions.ticker` — with `RecordOption` as their single write
  path so the signed-`contracts` realized-P&L formula (docs/phase-12-options.md §3.2) only exists once.
  Phase 23 PR6/PR7 (migration 21) added `fundamental_snapshots` (`fundamentals.go`) — one upserted row per
  ticker (no `market` column needed; a ticker's own value already disambiguates) caching either
  `internal/data.SEC` (US) or FinMind's `TaiwanStockPER` (TW) valuation percentile/cash-flow quality on a
  90-day TTL that `bot.cachedValuationSnapshot` checks before ever calling either source;
  `pe_percentile`/`cash_flow_quality` are nullable columns, not 0-sentinel, since a ticker with too little
  history is a real "unknown," not a real zero.
  Full table/method rationale: **[docs/architecture/db.md](docs/architecture/db.md)**.

- `internal/i18n` — every user/LLM-facing string, split into `zh.go` (default) and `en.go`, keyed by
  `Key` constants in `i18n.go`. `T(lang, key, args...)` does lookup + `fmt.Sprintf`; `TestTablesMatch`
  enforces both tables stay in sync (same keys, same verb count). Covers both bot UI copy and LLM
  prompt text — `KeyReasonMarker`/`KeyActionMarker` are parsed by `parseRecommendations`, so prompt and
  parser must change together. Language is selected once at startup via `BOT_LANGUAGE`, no per-message
  override. Details: **[docs/architecture/i18n.md](docs/architecture/i18n.md)**.

- `internal/llm` — `Client` talks to an LLM through an ordered chain of `Provider`s (one-shot `Prompt`
  or persistent `ChatSession`), always seeded with `acpProvider` (Claude via the Agent Client Protocol,
  authenticated through the operator's local `claude` CLI login rather than a metered API key) and
  optionally falling back to `AntigravityProvider` (Google's Antigravity CLI, opt-in via
  `ANTIGRAVITY_ENABLED`). `GenerateRecommendations` parses plain-text `[TICKER: X]`/`Action:`/`Reason:`
  blocks (not JSON); `Chat` replies go through verbatim. `StockData` carries optional attach-and-render
  fields (`Position`, `Earnings`, `ScanReason`, `Technicals`, `Candles`, `PrevRec`) that degrade
  independently when their source data isn't available. Also home to `WeeklyReview`, `InsightPortfolio`,
  and `ReviewTrade` (closed-trade analysis). Full provider/prompt rationale:
  **[docs/architecture/llm.md](docs/architecture/llm.md)**.

- `internal/signals` — pure functions for rule-based technical signals (RSI, MACD, Stochastic KD,
  Bollinger Bandwidth, MA Alignment, Volume-Price, New High, Relative Strength, Lowest Close) and
  strategy screens (Squeeze Breakout, Box Bottom Rebound, and Phase 14's Trend Breakout/Trend Pullback),
  independent of Telegram/LLM/DB. Every screen follows the same three-layer shape: `Check<Name>Exact(candles,
  params) bool` (pure boolean gate), `<Name>(candles, params) *StrategyHit` (wraps the exact check with a
  5-day lookback so a signal from a few days ago still surfaces), and `Detector.Check<Name>(ticker, candles,
  prevState) (*Signal, newState)` (adds alert-once-per-occurrence dedup on top). All four screens take a
  `ScreenParams` (from `DefaultScreenParams(market.MarketID)`, Phase 13) rather than package-level constants,
  since thresholds differ between TW and US. Stateful checks (`CheckRSIState`, `CheckMACDCross`, etc.) take
  and return persisted state as parameters — the DB round-trip lives in `internal/bot`, not here. Details:
  **[docs/architecture/signals.md](docs/architecture/signals.md)**.

- `internal/scheduler` — thin wrapper around `robfig/cron` fixed to `time.FixedZone("CST", 8*3600)`
  (avoids needing `tzdata` in the Alpine Docker image). Registers the daily report, closing snapshot
  (US + TW variants), universe scan, weekly review, log rotation, and SQLite backup jobs at fixed CST
  times. Full cron-time rationale: **[docs/architecture/scheduler.md](docs/architecture/scheduler.md)**.

- `internal/market` — pure, dependency-free NYSE trading-calendar logic (`IsTradingDay`/`IsHoliday`,
  computed per-year including a Meeus/Jones/Butcher Good Friday calculation) plus `MarketID`/`Of(ticker)`
  — the project-wide single source of truth for US-vs-TW ticker classification (leading digit = TW).
  Known gap: only fixed annual holidays are covered, not ad-hoc closures. Details:
  **[docs/architecture/market.md](docs/architecture/market.md)**.

- `internal/render` — Telegram/chat-facing text formatting (`Fundamentals`/`FinancialStatement`/
  `Commaf`) shared between `internal/bot` and `internal/mcptools`, depending only on `internal/data` +
  `internal/i18n` so both packages can import it without a hand-synced duplicate. Details:
  **[docs/architecture/render.md](docs/architecture/render.md)**.

- `internal/webfetch` — Phase 3's "article digestion" chat mode: `ExtractURL` (pure regex) finds a URL
  in a chat message, `Fetch` downloads and extracts readable text via `golang.org/x/net/html`, treating
  extraction under ~200 chars as a failure (paywall/JS-rendered signature). Extracted text is capped at
  20,000 runes. Details: **[docs/architecture/webfetch.md](docs/architecture/webfetch.md)**.

- `internal/bot` — Telegram command dispatch (`/add`, `/remove`, `/list`, `/status`, `/recommend`,
  `/check`, `/track`, `/buy`, `/sell`, `/portfolio`, `/dailyreport`, `/fundamentals`, `/universe`,
  `/stop`, `/review`, `/reset`, and more) plus scheduler-invoked jobs (`RunDailyReport`,
  `RunClosingSnapshot`, `RunUniverseScan`, `RunWeeklyReview`). Split across five files along the
  transport-vs-business line: `bot.go` (dispatch), `handlers.go` (command handlers), `jobs.go`
  (scheduler jobs + alert checks), `pipeline.go` (recommendation data assembly), `format.go` (pure
  formatting helpers). `internal/bot/channel.go`'s `Channel` interface abstracts the transport —
  `telegram.go` is the only file that imports `tgbotapi`, leaving room for a future second messaging
  channel. `pending_actions.go` is the bot-side half of Phase 4's write-gating flow (confirm/reject
  inline keyboards for MCP-proposed trades). Risk management (`/stop`, stop-loss/trailing-stop/target/
  MA5-break alerts, position sizing) and TW market support (per-market watchlist/portfolio/snapshot/
  cash handling) are both fully wired through this package. `options.go` (Phase 12) adds `/obuy`/
  `/osell`/`/oassign`/`/oexercise` — deliberately not folded into `/buy`/`/sell`'s OCC-autodetection,
  since the argument shapes differ (contracts vs. shares, per-share premium vs. price) — plus
  `/portfolio`'s options section and the expiry-scan job (hung off `RunClosingSnapshot(US)`, resolved
  only via a `pending_actions` confirm/reject, never automatically — an ITM expiry is an
  assignment/exercise, not a zero). `/option TICKER [call|put|csp|cc]` (defaults to `call`) runs
  `internal/option.Select` against a live chain and replies with the passing contracts. Phase 14's Trend
  Breakout/Trend Pullback strategies are wired into `checkStatefulSignals` (`jobs.go`) alongside the
  existing Squeeze Breakout/Box Bottom Rebound screens; Trend Breakout additionally short-circuits its own
  signal via `revenueGrowthOK` when `ScreenParams.RequireRevenueGrowth` is set — a fundamentals-based gate
  evaluated in `internal/bot` (reading `MonthRevenueYoYPct` for TW vs `RevenueGrowthYoY` for US via
  `b.cachedFundamentals`), not in `internal/signals`, since the latter stays DB/network-free.
  Full command-by-command and job-by-job rationale: **[docs/architecture/bot.md](docs/architecture/bot.md)**.

- `internal/mcptools` — Phase 3.5's MCP (Model Context Protocol) tool surface for chat, using the
  official `github.com/modelcontextprotocol/go-sdk`. Registers read tools (`get_quote`, `get_history`,
  `get_news`, `get_market_movers`, `get_fundamentals`, `get_financial_statements`,
  `get_upcoming_earnings`, `get_portfolio`) plus Phase 4's gated write tools (`record_buy`/`record_sell`,
  which only create a `pending_action` for the bot to confirm via Telegram, never write directly).
  Reached via `cmd/bot/main.go`'s `mcp` subcommand — same binary as the bot, so it can never drift out of
  version sync. All provider calls go through `withCache`+`tokenBucket` rate limiting. Dependency graph
  stays narrow (`internal/data`/`internal/db`/`internal/render`/`internal/i18n`, never `internal/llm`/
  `internal/bot`) so the tool surface survives an LLM provider swap. Details:
  **[docs/architecture/mcptools.md](docs/architecture/mcptools.md)**.

- `internal/web` — Phase 5's read-only web dashboard: an in-process HTTP server gated by `WEB_ADDR`,
  sharing the bot's live `data.Provider` chain and `*db.DB` connection directly. `pnl.go`'s pure
  `DailyPnL`/`CumulativeCurve`/KPI functions are the daily-P&L replay engine backing every dashboard
  view. The embedded frontend build lives at `internal/web/dist` (React/Vite source at repo-root `web/`);
  only `index.html` is committed as a placeholder, CI builds the real SPA. Every market-scoped API
  endpoint takes a `?market=us|tw` query parameter (TW support added in Phase 6 PR1) so a TWD position
  never gets replayed through the same P&L curve as a USD one. `risk.go`'s `/api/risk` (`m=us` only)
  reports CSP locked cash and, per underlying with an open short call, locked shares vs. what's actually
  held — a naked call (locked > held) is flagged, the one case in Phase 12 where a write can leave the
  user exposed without their noticing. `options.go`'s `/api/options` (`m=us` only, always 200 — no
  feature flag, unlike `/api/paper`) reuses `buildOptionCollateral` for its own collateral summary
  rather than duplicating it, and has no P&L curve — `daily_snapshots`/`DailyPnL` don't cover option
  market value (no free historical option price source), so this page only ever shows realized P&L on
  closed trades, never a portfolio-value line that would silently omit open option exposure.
  `settings.go`'s `/api/settings` (Phase 17) is the one endpoint that writes outside SQLite: it patches
  the connection/credential env vars in `.env` line-by-line (never `godotenv.Write`, which would reorder
  the file and drop every comment) and then `os.Exit(1)`s so the supervisor restarts the process into the
  new config — there is no hot reload, matching the codebase-wide "wire everything once at boot" rule.
  Its whitelist admits a variable on "would a wrong value still boot?", not "is it a credential":
  `DB_PATH` and the other paths are excluded permanently, since a typo there would crash-loop before
  `web.New` and leave no UI to fix it from. Details:
  **[docs/architecture/web.md](docs/architecture/web.md)**.

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

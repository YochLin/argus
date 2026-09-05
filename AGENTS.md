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
go run ./cmd/server         # run locally (HTTP always on, Telegram attached if configured)
go run ./cmd/server mcp     # run as an MCP server over stdio instead (see internal/mcptools)
go vet ./...                # static checks
docker compose up --build   # build + run in Docker (uses .env, mounts ./data -> /app/data)
```

There's no broad test suite; `internal/i18n` has the one exception (`go test ./internal/i18n/...`), which
checks the zh/en message tables stay in sync — see that package's entry below. Setup: copy `.env.example`
to `.env` and fill in `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` — since Phase 17 these may be left
blank, in which case the process starts with the Telegram transport disabled (no inbound commands, no
outbound messages) but everything else — scheduled jobs included — still running, so they can be filled
in later from the web dashboard's Settings page — plus `FINNHUB_API_KEY` (optional). The LLM needs no API key — run `claude` once on this machine and log in with your Claude
Pro/Max account first (see `internal/llm` below); Node.js (`npx`) must also be installed since the bot
shells out to an ACP agent process.

## Architecture

Flow: `internal/app`'s `Boot` wires everything together — loads env (`app.Load`), opens SQLite, builds
the data provider chain (`app.NewCoreProviders`), constructs the LLM client, constructs the Telegram
`bot.Bot` (a headless one, `bot.NewHeadless`, when no token/chat id is configured — never nil, so every
cron job and the web dashboard's write seam keep working with Telegram off; only `App.Run`'s long-poll
loop is gated), constructs the web server, registers the cron jobs;
`App.Run` then starts all of them and blocks until SIGINT/SIGTERM, and `App.Close` releases them in
reverse order. `cmd/server/main.go` is the only entrypoint (Phase 24 Stage 3): it defaults `WEB_ADDR`
to `app.DefaultWebAddr` (`127.0.0.1:8080`) so the HTTP/API surface is always on, and carries the
`mcp`/`eval`/`backtest` subcommands, which branch before any of the daemon setup. `mcp` in particular
has to stay in *this* binary: `llm.argusMCPServer` spawns a chat session's MCP server as
`os.Executable() mcp`, so a deployed binary that ignored the argument would boot a second copy of the
whole daemon against the live DB. The old Telegram-first `cmd/bot` is gone — once nothing but the
long-poll loop was gated on a token, it differed only in what an empty `WEB_ADDR` meant. Deploy units,
the Dockerfile and `docker-compose.yml` all build `./cmd/server`; the deployed binary is still installed
as `~/apps/argus/argus`, so `deploy/argus.service` is unchanged.

- `internal/data` — `Provider` interface (`GetQuote`/`GetNews`/`GetMarketMovers`), implemented
  independently by `finnhub.go` (primary) and `yahoo.go` (fallback via `Multi`). Separate optional
  interfaces (`FundamentalsProvider`, `HistoryProvider`, `EarningsProvider`, `MarketNewsProvider`,
  `AnalystRatingProvider`, `OptionChainProvider`) cover data Finnhub-only or Yahoo-only supports, each
  nil-checked by callers rather than folded into `Provider`. TW market support (`market.Of`-gated) spans
  `yahoo.go` (`.TW`/`.TWO` suffix resolution), `finmind.go` (TW fundamentals), `twse_movers.go` (TW
  market movers), `cnyes.go` (TW market news), `googlenews.go` (TW per-ticker Chinese news — a keyless
  Google News RSS search wired into the `Multi` chain between Finnhub and Yahoo, TW-only so the US path
  is unchanged), `tw_earnings.go` (a statutory-deadline earnings proxy, no real API), and
  `twse_calendar.go` (`TWTradingDayProvider`, TWSE's own published open/close schedule — the same
  `*TWSE` instance as `twse_movers.go`/`institutional_tw.go`, cached per calendar year — backing
  `RunTWMorningBriefing`'s market-closed gate; a schedule entry whose name marks a trading-resumption/
  last-trading-day boundary around a multi-day break, e.g. "農曆春節後開始交易日", is itself a real
  trading day and filtered out rather than treated as a holiday). `options.go`'s
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
  history is a real "unknown," not a real zero. Migration 25 added `podcast_insights`
  (`podcast_insights.go`) — an append-only log of `/podcast`'s LLM-extracted per-stock/macro market
  views, same no-dedup convention as `trade_lessons`; `ticker`/`market` default to `''` rather than
  being nullable, since a macro-only row (no single stock attached) is a normal row shape here, not a
  missing value. Migration 26 added `derived_from`, also `''` by default, populated only for a
  downstream-beneficiary row the model proposed from its own supply-chain knowledge (e.g. a TW foundry
  benefiting from an
  NVDA demand story) rather than something the transcript named directly — kept as its own column so
  that weaker, self-inferred claim stays visibly distinguishable from a grounded mention. Migration 27
  added `sell_followups` (`sell_followups.go`) — Phase 26's post-sell follow-up dedup table, one row per
  `(ticker, exit_date)` unique index, written only after a follow-up message actually sends (see
  `internal/bot`'s `sell_followup.go` entry below for why that's deliberate, not an oversight).
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
  and `ReviewTrade` (closed-trade analysis). `ExtractPodcastInsights` is `/podcast`'s one-shot call over
  a fetched transcript, reusing `checkModel` (same reasoning as `ExploreCandidates`); its
  `parsePodcastInsights` parser mirrors `parseExploreNominations` but flushes a block even when its
  `[TICKER: ]` is empty, since a macro-only observation with no single stock attached is still a real
  result, not a parse failure. The prompt also invites a downstream-beneficiary block (e.g. a TW
  supplier that benefits from a US chipmaker's demand story mentioned in the transcript) marked with a
  `DerivedFrom` line — an `ExploreNomination`-style "model's own claim, not grounded in the source
  text" case nested inside an already-unverified struct, so it's its own field rather than folded into
  Thesis where a reader couldn't tell a transcript quote from a supply-chain guess. Full provider/prompt
  rationale: **[docs/architecture/llm.md](docs/architecture/llm.md)**.

- `internal/signals` — pure functions for rule-based technical signals (RSI, MACD, Stochastic KD,
  Bollinger Bandwidth, MA Alignment, Volume-Price, New High, Relative Strength, Lowest Close) and
  strategy screens (Squeeze Breakout, Box Bottom Rebound, and Phase 14's Trend Breakout/Trend Pullback),
  independent of Telegram/LLM/DB. `mtf.go`'s 【日週共振穿越】 is the one *watchlist* tag here rather than
  an entry signal, and the only screen whose annotation depends on the market. Since 2026-09-04 the rule
  is a two-timeframe **angle** gate — daily MA5/MA7 and weekly MA5/MA7 each rising at ≥20°/≥30°, plus
  RSI14 ≤ 80 — where the angle is `atan(MA rise / that timeframe's trailing stdev of bar-to-bar moves)`,
  i.e. volatility-normalised so it matches what a chart reader sees on an autoscaled y-axis. It replaced
  a `close just crossed MA30` version; the two are structurally near-mutually-exclusive (a steep weekly
  MA means the cross already happened), which is why the cross leg is gone rather than kept alongside.
  It is a STATE, not an event: the alert semantics come from `MTFCross`'s 5-day lookback plus the
  `Detector` dedup, which together give "fires when true after being false for 5 bars" — and that is the
  form that was measured. It takes no `ScreenParams` on purpose — the thresholds are the rule as
  measured, and a knob would invite re-tuning against the same slices the result came from. **It fires on
  BOTH markets, but only TW's numbers are positive**: TW carries `i18n.KeyStrategyUnvalidatedMTF`
  (+4.8pp/2.7σ early, +5.7pp/3.2σ late, within-ticker), US carries `i18n.KeyStrategyMTFCrossUS` and is
  measured NEGATIVE past 1 SE in both splits (−6.9pp/10.9σ, −3.7pp/4.8σ) — it fires there only because
  the user asked to observe it, and `service.ComputeTechnicals` still keeps the LLM prompt path TW-only
  so a measured-harmful screen never reaches the recommendation prompt. See `CheckMTFCrossExact` for the
  full numbers, for the long list of variants measured and rejected (平轉向上, RSI caps on US, market
  breadth gates, %/week slopes), for the survivorship caveat, and for the two weekly-resampling bugs
  (lookahead; day-to-day instead of bar-to-bar MA comparison) it went through. Every screen follows the same three-layer shape: `Check<Name>Exact(candles,
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

- `internal/service` — the business-logic layer Phase 24 pulled out of `internal/bot`: 15 services
  (`RiskService`, `ScanService`, `PaperService`, `OptionsService`, `BrokerSyncService`,
  `PortfolioService`, `SnapshotService`, `RecommendationService`, `TradeService`, `WatchlistService`,
  plus the smaller `NewsPicker`/candidate-ranking/technicals/report/briefing helpers) built on 15 narrow
  `XxxStore` interfaces (`RiskStore`, `ScanStore`, `PaperStore`, `PortfolioStore`, ... — one per
  service, each with exactly one production implementation, `*db.DB`, and a test fake), so a service can
  be unit-tested without opening SQLite. `internal/bot`/`internal/web`/`internal/mcptools` all call into
  this layer rather than each independently reimplementing the same rule — **when a value or function
  already lives here, don't write a second copy in a caller package just because it used to be
  unexported in `internal/bot`; that "can't import bot's unexported X" reasoning stopped being true the
  moment the logic moved to `service`.** Confirmed already-centralized and not to be re-duplicated:
  `BenchmarkFor`, `CashSettingKey`, `NewsPicker`, `OptionMark`, `ComputeTechnicals`/`ComputeMarketRegime`,
  `TrackHit`, `TradePayload`. `internal/bot` itself has shrunk to Telegram transport + command parsing +
  message assembly (see below) — most of what its doc entry described pre-Phase-24 as "business logic"
  now actually lives here.

- `internal/bot` — Telegram command dispatch (`/add`, `/remove`, `/list`, `/status`, `/recommend`,
  `/check`, `/track`, `/buy`, `/sell`, `/portfolio`, `/dailyreport`, `/fundamentals`, `/universe`,
  `/stop`, `/review`, `/reset`, and more) plus scheduler-invoked jobs (`RunDailyReport`,
  `RunClosingSnapshot`, `RunUniverseScan`, `RunWeeklyReview`). Split across five files along the
  transport-vs-business line: `bot.go` (dispatch), `handlers.go` (command handlers), `jobs.go`
  (scheduler jobs + alert checks), `pipeline.go` (recommendation data assembly), `format.go` (pure
  formatting helpers) — post-Phase-24, most of the actual rule logic behind those handlers/jobs is a call
  into `internal/service` (see above); what's left here is Telegram-specific glue: parsing command
  arguments, assembling `service` calls, and formatting the reply. `internal/bot/channel.go`'s `Channel`
  interface abstracts the transport —
  `telegram.go` is the only file that imports `tgbotapi`, leaving room for a future second messaging
  channel, and `headless.go` is the no-op implementation a Telegram-less process runs on (Phase 24
  Stage 3: the absence of a transport must not take the orchestration with it). `pending_actions.go` is the bot-side half of Phase 4's write-gating flow (confirm/reject
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
  `podcast.go`'s `/podcast <url>` reuses `internal/webfetch`'s fetch path (same as the chat article-
  digestion mode) but, unlike that free-form chat reply, runs a dedicated one-shot LLM call
  (`llm.ExtractPodcastInsights`) that parses structured per-stock/macro market views out of a pasted
  podcast/video transcript and persists each one via `db.SavePodcastInsight` — building a queryable
  log of past outside-source views rather than a one-off summary. `sell_followup.go`'s
  `checkSellFollowups` (Phase 26) rides `RunClosingSnapshot`'s tail for both markets (no new cron entry)
  and, for a ticker whose most recent fully closed round trip exited 5 trading days ago, runs a second
  `llm.ReviewTrade` looking back at how it traded since — "5 trading days" is counted off
  `b.history.GetHistory`'s daily candle count past the exit date, not a trading calendar, since a candle
  only exists for a session that actually traded (US holidays, TW multi-day breaks, and individual-ticker
  halts are then all automatically correct); the `sell_followups` table's `(ticker, exit_date)` row is
  only written once the follow-up message actually sends, so an LLM failure or not-yet-enough history
  just retries on the next closing snapshot rather than being treated as done.
  Full command-by-command and job-by-job rationale: **[docs/architecture/bot.md](docs/architecture/bot.md)**.

- `internal/mcptools` — Phase 3.5's MCP (Model Context Protocol) tool surface for chat, using the
  official `github.com/modelcontextprotocol/go-sdk`. Registers read tools (`get_quote`, `get_history`,
  `get_news`, `get_market_movers`, `get_fundamentals`, `get_financial_statements`,
  `get_upcoming_earnings`, `get_portfolio`) plus Phase 4's gated write tools (`record_buy`/`record_sell`,
  which only create a `pending_action` for the bot to confirm via Telegram, never write directly).
  Reached via `cmd/server/main.go`'s `mcp` subcommand — same binary as the daemon, so it can never drift out of
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
  `apiauth.go`/`apiv1.go`/`apiv1_resources.go`/`ws.go` (Phase 24 Stage 4) are the `/api/v1` surface
  aimed at a future mobile app and at scripts — JWT (`JWT_SECRET`) or `X-API-Key` (`API_KEY`) auth, a
  `{success, data, error, timestamp}` envelope on every response, and a `/api/v1/ws` WebSocket fed by
  `internal/notification`'s `WebSocketHub`. It is registered only when `JWT_SECRET` **and**
  `WEB_PASSWORD` are both set, and it is deliberately *separate* from `auth.go`'s Phase 10 cookie auth
  (a browser session vs. a token-bearing client with no cookie jar; they share `WEB_PASSWORD` and
  nothing else). The route table lives in `apiV1Handlers` as data so `openapi_test.go` can fail when
  **[docs/openapi.yaml](docs/openapi.yaml)** drifts from it — update the spec in the same PR as a route.
  `settings.go`'s `/api/settings` (Phase 17) is the one endpoint that writes outside SQLite: it patches
  the connection/credential env vars in `.env` line-by-line (never `godotenv.Write`, which would reorder
  the file and drop every comment) and then `os.Exit(1)`s so the supervisor restarts the process into the
  new config — there is no hot reload, matching the codebase-wide "wire everything once at boot" rule.
  Its whitelist admits a variable on "would a wrong value still boot?", not "is it a credential":
  `DB_PATH` and the other paths are excluded permanently, since a typo there would crash-loop before
  `web.New` and leave no UI to fix it from. Details:
  **[docs/architecture/web.md](docs/architecture/web.md)**.

- `internal/paper` — Phase 11's pure rule engine shared by `argus backtest`'s historical replay
  (`cmd/server/backtest.go`) and the live paper account's forward accumulation
  (`internal/bot/paper.go`): `Account.ApplySignal`/`MarkClose`/`Equity` are the entire trading rulebook,
  fed plain values by both callers (same "no DB/network/Telegram" discipline as `internal/signals`/
  `internal/receval`) so backtest and live behavior can't structurally diverge. Details:
  **[docs/phase-11-paper-account.md](docs/phase-11-paper-account.md)** §1.

- `internal/receval` — scores the `recommendations` table against actual subsequent price action for
  the `eval` subcommand; pure functions over plain structs (`db.Recommendation` rows + `data.Candle`
  history handed in by the caller), same no-DB/network discipline as `internal/signals`/`internal/paper`.
  Details: **[docs/offline-rec-eval.md](docs/offline-rec-eval.md)**.

- `internal/notification` — Phase 24 Stage 2's event bus: the seam between business logic that decides
  something is alert-worthy (stop-loss breach, restricted-stock warning, price event, ...) and the
  channel(s) that deliver it (Telegram today, plus an in-app store so a future Web/App surface has
  history to read). Doesn't replace synchronous command replies (`/list`, `/portfolio`, ...) — those
  still go straight through `bot.Channel.Send`.

- `internal/logger` — the application's small logging facade (`log/slog` underneath), imported as
  `logger` by 57 files across the codebase so the bot, scheduler, web server, and CLI tools share one
  handler/level configuration.

- `cmd/strategyscan` — the standalone research tool (4,688 lines, independent of the daemon) behind
  every strategy-screen go/no-go decision referenced elsewhere in this file (Phase 14 trend
  breakout/pullback, Phase 25's §8.x studies, etc.) — its output numbers are the evidence trail for
  "tried this, didn't clear the bar." Deliberately not deduped against `internal/paper` for small
  overlaps like `maxDrawdownPct` (three implementations, different input types) — that's a kept boundary
  between the research tool and the live trading path, not an oversight.

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

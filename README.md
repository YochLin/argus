# Argus

A personal assistant, currently wearing a stock-monitoring hat and talking over Telegram. Neither is
meant to be permanent.

## Why "Argus"

Argus is named after the hundred-eyed giant of Greek myth, charged with watching over things no matter
where he looked. Today this bot watches stocks: your watchlist, technical signals, fundamentals, options,
and daily market movers, across both the US and Taiwan markets. It's built as a personal assistant first
and a stock bot second. The free-form chat mode (just message it, no `/` command needed) is the first
step toward that, and future features aren't expected to all be stock-related. If you're extending this
project, don't assume everything belongs under "finance."

## Vision

Three things about today's implementation are conveniences, not commitments:

- **Just stocks.** The domain today, not the ceiling (see "Why Argus" above).
- **Just Telegram.** `internal/bot` only speaks Telegram right now, but the plan is to support other
  messaging channels too (Discord, Slack, a plain CLI, whatever). Adding a second channel means pulling
  the Telegram-specific parts behind a shared interface rather than bolting the new channel directly onto
  `bot.Bot`.
- **Just Claude.** `internal/llm` talks to Claude via ACP today, specifically to use a Claude Pro/Max
  subscription instead of a metered API. Supporting other LLM providers is a real future direction, not
  just Claude forever, but swapping providers isn't a find-and-replace: ACP's session model (one-shot per
  call vs. the persistent chat session) doesn't necessarily map 1:1 onto other providers' APIs, so this
  needs a proper interface boundary, not a quick patch.

## What it does today

**Watchlist and alerts**

- Add/remove tickers per market (US or Taiwan) and check live quotes on demand.
- Rule-based alerts on price moves, RSI overbought/oversold, MACD golden/death crosses, and a handful of
  strategy screens (squeeze breakout, box bottom rebound, trend breakout/pullback), checked daily against
  your watchlist. Alerts dedupe: RSI only fires on newly entering an extreme zone, MACD only on an actual
  cross, not every day a trend holds.
- A larger scan universe (seeded from the S&P 500 plus manual `/universe add` tickers) gets checked daily
  in rotating chunks for the same signals; a hit both feeds into that day's recommendations and, for
  strategy screens, pushes straight to Telegram.
- Manual stop-loss prices (`/stop`) plus automatic stop-loss and trailing-stop alerts on open positions,
  tuned separately for US and Taiwan (Taiwan trades under a daily ±10% price limit, so the thresholds
  differ). Both dedupe: a breach alerts once and resets once the position recovers.
- One-shot price-cross watches (`/buyalert`) for a level worth knowing about without opening a position.

**LLM analysis**

- Ask for an instant read on any ticker (`/check`) or a daily recommendation pass across current holdings
  and worthwhile opportunities from the watchlist, market movers, and universe scan (`/recommend`).
- `/track` reviews past recommendations against today's prices. A BUY/SELL only counts as a hit if it
  beat or underperformed SPY over the same period (plain up/down when no same-period SPY data exists),
  alongside average return magnitude and a hit-rate breakdown by where the candidate came from.
- Each ticker's last recommendation feeds back into the next prompt, so a reversal comes with an
  explanation of what changed instead of a context-free flip-flop.
- Open positions and their cost basis feed into `/recommend` and the daily report, and free-form chat is
  prefixed with a read-only summary of your watchlist and positions, so both can reason about actual P&L
  and answer questions like "which of my watchlist tickers dropped the most recently."
- `/insight` runs a portfolio-wide read across everything you hold; `/review <ticker>` runs a closed-trade
  postmortem and saves the takeaway as a standing lesson; `/thesis <ticker> <text>` attaches your own
  standing investment thesis to a ticker so future analysis can be checked against it.
- `/podcast <url>` fetches a transcript and has the LLM extract structured per-stock and macro market
  views out of it, building a searchable log of outside opinions you've fed it over time.
- Earnings within 14 days get a warning in the recommendation prompt, plus a Telegram reminder inside 3
  days, so a BUY call doesn't walk into next-day earnings volatility.
- General market/macro news opens the recommendation reply before the per-ticker calls.
- Fundamentals (P/E, margins, growth, key 10-K/10-Q line items for the US; PER/PBR, dividend yield,
  quarterly EPS/revenue/margin for Taiwan) surface in `/fundamentals`, `/check`, and `/recommend` when the
  relevant data provider is configured.

**Trading and portfolio**

- `/buy` and `/sell` record trades against a weighted-average cost basis (both take an optional backdated
  date for migrating historical cost basis); `/undo` removes a mistaken entry.
- `/portfolio` shows market value and unrealized/realized P&L per market; `/cash` tracks a declared cash
  balance alongside it. Total position value snapshots daily after close, building the history behind the
  web dashboard's P&L curve.
- A live paper-trading account (`/paper`) mirrors the same rule engine used by `argus backtest`'s
  historical replay, so the two can't structurally diverge.
- Options trading (US only): `/obuy`, `/osell`, `/oassign`, `/oexercise` manage contract positions
  end-to-end including expiry resolution, and `/option <ticker> [call|put|csp|cc]` screens a live option
  chain against a delta/DTE/liquidity profile and replies with the passing contracts.
- `/events` shows the recorded log of large single-day price moves.

**Taiwan market**

Taiwan support isn't a stripped-down side mode. It gets its own watchlist, portfolio, and cash tracking;
FinMind for fundamentals and Chinese company-name resolution; TWSE for market movers and a real trading
calendar (so morning briefings respect actual holidays, not just weekends); and, when configured, a
running Sinopac (永豐證券) Shioaji session for broker-grade quotes plus a scheduled sync that pulls your
real positions and cash and proposes trades for confirmation, instead of guessing them from Yahoo's
suffix-matched TW quotes.

**Reliability**

- A rotating daily log (kept about a week), a daily SQLite backup (kept about two weeks by default), and
  a Telegram alert if a scheduled job panics or can't even read the watchlist.
- An optional fallback LLM provider (Google's Antigravity CLI) if every Claude call fails, e.g. a usage
  cap.

**Web dashboard and API** *(always on; address via `WEB_ADDR`)*

KPI cards (net P&L, win rate, profit factor, expectancy, max drawdown), a cumulative P&L curve, open
positions with live quotes, calendar and monthly views, a risk page (flags a naked call when locked
shares exceed what you actually hold), an options collateral summary, and buy/sell/stop/watchlist forms
once a dashboard password is set. A separate `/api/v1` surface (JWT or API-key auth, plus a WebSocket
feed) exists for scripts and a future mobile client. No login or HTTPS by default beyond the optional
password. This is meant for private access over Tailscale or an SSH tunnel, not a public port.

**Chat tools (MCP)**

Free-form chat can call a set of MCP tools directly: read tools for quotes, history, technicals, news,
fundamentals, earnings, insider transactions, and portfolio state, plus gated write tools for recording a
buy/sell or editing the watchlist. A gated write only creates a pending action for you to confirm in
Telegram; it never writes directly.

This is single-user by design: one Telegram chat ID, no accounts, no multi-tenant data model.

## How it's built

Go, SQLite (pure-Go driver, no cgo), and Telegram's bot API (today's messaging channel, see Vision). US
market data comes from Finnhub (primary, optional) with Yahoo Finance as a keyless fallback, plus SEC
EDGAR for a longer fundamentals history than Yahoo's free tier gives. Taiwan data comes from Yahoo/TWSE
by default, FinMind for fundamentals, and optionally a live Sinopac Shioaji session for broker-grade
quotes. The LLM side talks to Claude (today's provider, see Vision) through the **Agent Client Protocol
(ACP)**, authenticating via your existing Claude Pro/Max subscription (the `claude` CLI login) instead of
a metered API key, so running this bot doesn't rack up separate API bills. See `AGENTS.md` for the
deeper architectural notes if you're modifying the code.

## Getting started

**Prerequisites:**

- Go 1.25+
- Node.js (`npx` on your `PATH`), since the bot shells out to a local ACP agent process and also builds
  the web dashboard's frontend (see below)
- The `claude` CLI installed and logged in once on this machine with a Claude Pro/Max account
- A Telegram bot token ([BotFather](https://t.me/BotFather)) and your chat ID
- Optional: a [Finnhub](https://finnhub.io/) API key for US fundamentals and richer quotes/news, a
  [FinMind](https://finmindtrade.com/analysis/#/login) token for Taiwan fundamentals, and a
  `SEC_USER_AGENT` (a real contact email; SEC blocks anything else) for extended US fundamentals history

**Setup:**

```bash
cp .env.example .env
# fill in TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID, or leave both blank and
# configure them later from the web dashboard's Settings page

npm --prefix web ci && npm --prefix web run build   # build the web dashboard (see below)
go build ./...      # sanity-check the build
go run ./cmd/server # run it
```

TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID may be left blank: the process starts with Telegram disabled (no
inbound commands, no outbound messages) but everything else, scheduled jobs included, still runs, and
they can be filled in later from the dashboard. Every other credential in `.env.example` (Finnhub,
FinMind, SEC, Sinopac, JWT/API secrets) works the same way and can be set from that same Settings page.

The `npm run build` step is only needed once (or whenever `web/` changes); it builds straight into
`internal/web/dist`, which `go:embed` packs into the binary. A placeholder is committed there so
`go build ./...`/`go test ./...` still work on a fresh clone before you've run it, just without a real
dashboard UI. Set `WEB_ADDR` in `.env` (e.g. `127.0.0.1:8090`) to pick the address it's served on; left
blank it defaults to `127.0.0.1:8080`, loopback only, since the dashboard has no auth of its own until
you set `WEB_PASSWORD`.

No `ANTHROPIC_API_KEY` is needed or wanted; leave it unset.

Set `BOT_LANGUAGE=en` in `.env` to switch the bot's replies and the LLM's analysis to English; leave it
unset (or `zh`) for the Traditional Chinese default. It's a single startup-time setting, not a
per-message toggle, since this is a single-user bot with no per-user preference table.

`go run ./cmd/server mcp` runs the same binary as an MCP server over stdio instead of the daemon, which
is what free-form chat shells out to for its tool calls.

**Running in Docker** works for the Telegram/data/DB parts, but `/recommend`, `/check`, `/dailyreport`,
and chat currently do **not** work in the containerized setup: the `alpine` image has no Node.js, and the
Pro/Max login has no solved credential path inside a Linux container yet. This is a known, open
limitation, not an oversight. See `AGENTS.md` if you want to tackle it.

## Using the bot

Talk to it in Telegram:

| Command | What it does |
|---|---|
| `/add <ticker>` | Add a ticker to your watchlist |
| `/remove <ticker>` | Remove a ticker |
| `/list` | Show your watchlist |
| `/status [ticker]` | Live quote(s), all watchlist tickers or just one |
| `/check <ticker>` | Instant LLM analysis of one ticker |
| `/recommend` | LLM read on current holdings and worthwhile opportunities |
| `/track [days]` | Past recommendations vs. today's prices and the resulting hit rate (default 7 days) |
| `/buy <ticker> <shares> <price> [fee] [date]` | Record a purchase; folds into the ticker's cost basis and auto-adds it to your watchlist |
| `/sell <ticker> <shares> <price> [fee] [date]` | Record a sale against an open position and report the realized P&L |
| `/undo <id>` | Remove a mistaken buy/sell entry |
| `/portfolio` | Every open position's market value and unrealized P&L, plus cumulative realized P&L and options |
| `/cash [amount]` | View or set your declared cash balance |
| `/stop <ticker> [price]` | View, or set, a manual stop-loss price |
| `/buyalert <ticker> <price>` | One-shot alert when price crosses a level |
| `/obuy`, `/osell`, `/oassign`, `/oexercise` | Open, close, assign, or exercise an option position (US only) |
| `/option <ticker> [call\|put\|csp\|cc]` | Screen a live option chain for contracts matching a delta/DTE/liquidity profile |
| `/paper [reset]` | View, or reset, the live paper-trading account |
| `/insight` | LLM read across your whole portfolio |
| `/review <ticker>` | LLM postmortem on a closed trade, saved as a lesson |
| `/thesis <ticker> <text>` | Attach a standing investment thesis to a ticker |
| `/podcast <url>` | Extract per-stock/macro views from a transcript |
| `/fundamentals <ticker>` | Raw valuation/profitability/financial-statement data |
| `/universe [add\|remove <ticker>]` | Show, or edit, the daily scan universe |
| `/events` | Log of recorded large price moves |
| `/sinopac sync` | Sync live positions/cash from your Sinopac Shioaji account (if configured) |
| `/dailyreport`, `/morningreport`, `/monthlyreport` | Manually trigger a scheduled report |
| `/reset` | Clear the chat mode's conversation memory |
| _(anything else)_ | Free-form chat, no command needed, just send a message |

## Project status

This is a personal side project, evolving as needs come up rather than following a fixed roadmap. Expect
the feature set, the stock-only scope, the Telegram-only channel, and the Claude-only provider to all
keep shifting, see Vision above.

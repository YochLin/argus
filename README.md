# Argus

A personal assistant, currently wearing a US-stock-monitoring hat and talking over Telegram — neither
of which is meant to be permanent.

## Why "Argus"

Argus is named after the hundred-eyed giant of Greek myth, charged with watching over things no
matter where he looked. Today this bot only watches stocks: your watchlist, technical signals,
fundamentals, and daily market movers. But it's built as a personal assistant first, a stock bot
second — the free-form chat mode (just message it, no `/` command needed) is the first step toward
that, and future features aren't expected to all be stock-related. If you're extending this project,
don't assume everything belongs under "finance."

## Vision

Three things about today's implementation are conveniences, not commitments:

- **Just stocks** — the domain today, not the ceiling (see "Why Argus" above).
- **Just Telegram** — `internal/bot` only speaks Telegram right now, but the plan is to support other
  messaging channels too (Discord, Slack, a plain CLI, whatever). If you're adding a second channel,
  expect to pull the Telegram-specific parts behind a shared interface rather than bolting the new
  channel directly onto `bot.Bot`.
- **Just Claude** — `internal/llm` talks to Claude via ACP today, specifically to use a Claude Pro/Max
  subscription instead of a metered API. Supporting other LLM providers is a real future direction, not
  just Claude forever — but note that swapping providers isn't a find-and-replace: ACP's session model
  (one-shot per call vs. the persistent chat session) doesn't necessarily map 1:1 onto other providers'
  APIs, so this will need a proper interface boundary, not a quick patch.

## What it does today

- **Watchlist tracking** — add/remove tickers, check live quotes on demand
- **Rule-based alerts** — price-move thresholds, RSI (overbought/oversold), and MACD (trend
  momentum), checked daily against your watchlist
- **LLM-powered analysis** — ask for an instant read on any ticker, or a daily set of 3–5
  recommendations drawn from your watchlist plus the day's broad market movers
- **Bilingual (zh/en)** — every bot reply and LLM prompt is available in Traditional Chinese (default) or
  English, switched with one env var — see Getting Started
- **Fundamentals** — P/E, margins, growth, and key 10-K/10-Q line items, when a Finnhub API key is
  configured
- **Free-form chat** — message the bot without a command and it remembers the conversation, separate
  from the one-shot analysis commands
- **Daily report** — an automatic summary pushed every day before US market open (21:00 Taiwan time)

This is single-user by design: one Telegram chat ID, no accounts, no multi-tenant data model.

## How it's built

Go, SQLite (pure-Go driver, no cgo), and Telegram's bot API (today's messaging channel — see Vision).
Market data comes from Finnhub (primary, optional) with Yahoo Finance as a keyless fallback. The LLM
side talks to Claude (today's provider — see Vision) through the **Agent Client Protocol (ACP)**,
authenticating via your existing Claude Pro/Max subscription (the `claude` CLI login) instead of a
metered API key — so running this bot doesn't rack up separate API bills. See `CLAUDE.md` for the
deeper architectural notes if you're modifying the code.

## Getting started

**Prerequisites:**

- Go 1.25+
- Node.js (`npx` on your `PATH`) — the bot shells out to a local ACP agent process
- The `claude` CLI installed and logged in once on this machine with a Claude Pro/Max account
- A Telegram bot token ([BotFather](https://t.me/BotFather)) and your chat ID
- (Optional) A [Finnhub](https://finnhub.io/) API key for fundamentals and richer quotes/news

**Setup:**

```bash
cp .env.example .env
# fill in TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID (required),
# FINNHUB_API_KEY (optional), and review the CLAUDE_*_MODEL settings

go build ./...     # sanity-check the build
go run ./cmd/bot    # run it
```

No `ANTHROPIC_API_KEY` is needed or wanted — leave it unset.

Set `BOT_LANGUAGE=en` in `.env` to switch the bot's replies and the LLM's analysis to English; leave it
unset (or `zh`) for the Traditional Chinese default. It's a single startup-time setting, not a
per-message toggle — this is a single-user bot with no per-user preference table.

**Running in Docker** works for the Telegram/data/DB parts, but `/recommend`, `/check`, `/dailyreport`,
and chat currently do **not** work in the containerized setup (the `alpine` image has no Node.js, and
the Pro/Max login has no solved credential path inside a Linux container yet). This is a known,
open limitation, not an oversight — see `CLAUDE.md` if you want to tackle it.

## Using the bot

Talk to it in Telegram:

| Command | What it does |
|---|---|
| `/add <ticker>` | Add a ticker to your watchlist |
| `/remove <ticker>` | Remove a ticker |
| `/list` | Show your watchlist |
| `/status [ticker]` | Live quote(s) — all watchlist tickers, or just one |
| `/check <ticker>` | Instant LLM analysis of one ticker |
| `/recommend` | LLM picks 3–5 tickers to look at today, from your watchlist + market movers |
| `/fundamentals <ticker>` | Raw valuation/profitability/financial-statement data (requires Finnhub key) |
| `/dailyreport` | Manually trigger the daily report (normally runs automatically at 21:00 Taiwan time) |
| `/reset` | Clear the chat mode's conversation memory |
| _(anything else)_ | Free-form chat — no command needed, just send a message |

## Project status

This is a personal side project, evolving as needs come up rather than following a fixed roadmap.
Expect the feature set, the stock-only scope, the Telegram-only channel, and the Claude-only provider
to all keep shifting — see Vision above.

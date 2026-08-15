# Backend application boundary

Argus started as a Telegram bot, but Telegram should be treated as a delivery
adapter rather than the owner of investment workflows.

The target dependency direction is:

```text
Telegram / Web / App / MCP
             ↓
     internal/service
             ↓
 internal/data + internal/db
```

`internal/service` contains application use cases and returns transport-neutral
results. It must not know how to send Telegram messages, encode HTTP responses,
or render an LLM review. Each adapter owns those concerns.

The first migrated use case is trading. `service.TradeService` owns input
validation, buy/sell persistence, realized P&L results, stop-price capture for
closed trades, and automatic watchlist inclusion after a buy. The bot still
owns the Telegram-specific confirmation text, cash-balance nudging, stop-price
suggestions, and asynchronous closed-trade review. Keeping those pieces in the
bot during this first step preserves existing behavior while creating a seam
for Web and App callers.

The next shared use cases are `WatchlistService` and `PortfolioService`.
`WatchlistService` owns ticker normalization and watchlist writes for the bot,
web write routes, and MCP write tools; TradeService's intentional "buy implies
watchlist" side effect remains inside the trade use case. `PortfolioService` produces a
market-scoped open-position snapshot with parallel best-effort quote valuation,
cash, realized P&L, and account value. Bot portfolio output, the web
status/dashboard/risk views, and MCP's `get_portfolio` tool render that snapshot
instead of each repeating the same position valuation loop.

`RecommendationTrackingService` now owns the shared `/track` workflow used by
the Telegram bot and MCP's `get_recommendation_stats` tool. It resolves stored
entry prices, fetches current prices, selects the correct market benchmark
(`SPY` for US and `0050` for TW), applies the relative hit rule with an
absolute-direction fallback, and aggregates by source and market. Adapters
still own localized lines, summaries, caching, and tool/command validation.

Future extraction should follow the same vertical-slice pattern:

1. Define a narrow service input/result and persistence interface.
2. Move one complete business workflow behind the service.
3. Keep adapter formatting and channel side effects outside the service.
4. Add service tests with fakes before changing another adapter.

This is intentionally a modular monolith, not a microservice split. The bot,
web server, and future app API can share one process and one SQLite connection
until deployment scale makes a separate process worthwhile.

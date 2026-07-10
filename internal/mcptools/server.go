// Package mcptools implements Argus's MCP (Model Context Protocol) server —
// mostly a read-only tool surface a chat session can call into for on-demand
// data (see PLAN.md's Phase 3.5), plus two narrowly-scoped write tools
// (add_to_watchlist/remove_from_watchlist — watchlist_write_tools.go) as
// that phase's lowest-risk write-path pilot. Deliberately narrow dependency
// surface (only internal/data + internal/i18n, plus internal/db for the
// Phase 3.5 "追加項" DB tools — see db.OpenReadOnly/OpenForWrites' doc
// comments for why those imports are safe despite this package's original
// "don't touch the DB" decision) so this stays a thin, provider-neutral
// adapter rather than pulling in internal/llm or internal/bot — see
// tools.go's formatFundamentals/commaf and db_tools.go's duplicated
// track-scoring helpers for what that narrower boundary costs (small,
// deliberate duplication instead of importing internal/bot).
package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
)

// Version is the MCP server's reported implementation version. Argus has no
// release/version process, so this is a static placeholder rather than
// something threaded through from a build flag.
const Version = "0.1.0"

const (
	// rateLimiterCapacity is how many tool calls can burst through
	// instantly (e.g. a model firing off get_quote for several watchlist
	// tickers back to back) before throttling kicks in.
	rateLimiterCapacity = 5
	// rateLimiterRefillPerSecond throttles sustained tool-call cadence to
	// this many calls/sec once the burst capacity is drained — well under
	// Finnhub's free-tier 60 req/min ceiling (~1/sec) on purpose, to leave
	// headroom for whatever the bot's own prefetch paths (RunDailyReport,
	// /recommend, RunUniverseScan) are doing against the same Finnhub API
	// key concurrently, since this MCP subprocess has no visibility into
	// that usage. See PLAN.md's Phase 3.5 "API 防護" item.
	rateLimiterRefillPerSecond = 0.5 // 30 req/min sustained
)

// NewServer builds the argus MCP server and registers every read-only data
// tool this build has a provider for. Kept separate from Run so tests can
// connect over an in-memory transport (mcp.NewInMemoryTransports) without
// going through stdio.
//
// fundamentals and earnings are optional (nil whenever FINNHUB_API_KEY
// isn't set, same as Bot.fundamentals) — their tools are simply not
// registered when nil, per PLAN.md's "Finnhub-only tools ... 於 tools/list
// 階段就不註冊" decision. provider and history are never nil in practice
// (Multi always wraps at least Yahoo, and history is Yahoo-only but always
// constructed — see cmd/bot/main.go), so their tools are unconditional.
// database and writeDatabase are the same nil-check-and-degrade shape once
// more: nil whenever db.OpenReadOnly/OpenForWrites failed (see
// runMCPServer), so a DB hiccup takes down only the tools that depend on
// the connection that failed, not the whole MCP server. The two are
// deliberately separate connections/parameters, not one dual-purpose one —
// see db.OpenForWrites' doc comment for why the four read-only query tools
// keep a hard DB-level guarantee that they can never write.
//
// Every tool handler is routed through a shared per-process TTL cache and
// token-bucket rate limiter (tools.go's withCache) — once this server is
// wired into a live chat session (a later Phase 3.5 checklist item), tool
// call cadence is driven by the model, not bot-side prefetch logic, so this
// process has to protect Finnhub's rate limit itself instead of relying on
// callers to behave. The two write tools bypass withCache entirely (see
// watchlist_write_tools.go) — caching a mutation's result makes no sense,
// and local SQLite writes need no Finnhub-rate-limit protection.
func NewServer(lang i18n.Lang, provider data.Provider, history data.HistoryProvider, fundamentals data.FundamentalsProvider, earnings data.EarningsProvider, database *db.DB, writeDatabase *db.DB) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "argus",
		Version: Version,
	}, nil)
	registerTools(server, &toolset{
		lang:         lang,
		provider:     provider,
		history:      history,
		fundamentals: fundamentals,
		earnings:     earnings,
		db:           database,
		writeDB:      writeDatabase,
		cache:        newTTLCache(),
		limiter:      newTokenBucket(rateLimiterCapacity, rateLimiterRefillPerSecond),
	})
	return server
}

// Run starts the argus MCP server on stdio and blocks until the connection
// closes or ctx is cancelled. This is what cmd/bot's "mcp" subcommand
// invokes — an ACP chat session launches the same binary as a subprocess
// (os.Executable()) rather than a separately deployed server, so the tool
// surface can never drift out of version sync with the running bot.
func Run(ctx context.Context, lang i18n.Lang, provider data.Provider, history data.HistoryProvider, fundamentals data.FundamentalsProvider, earnings data.EarningsProvider, database *db.DB, writeDatabase *db.DB) error {
	server := NewServer(lang, provider, history, fundamentals, earnings, database, writeDatabase)
	return server.Run(ctx, &mcp.StdioTransport{})
}

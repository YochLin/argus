// Package mcptools implements Argus's MCP (Model Context Protocol) server —
// the read-only tool surface a chat session can call into for on-demand data
// (see PLAN.md's Phase 3.5). Deliberately narrow dependency surface (only
// internal/data + internal/i18n) so this stays a thin, provider-neutral
// adapter rather than pulling in internal/db, internal/llm, or internal/bot
// — see tools.go's formatFundamentals/commaf for what that boundary costs
// (small, deliberate duplication instead of importing internal/bot).
package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/data"
	"argus/internal/i18n"
)

// Version is the MCP server's reported implementation version. Argus has no
// release/version process, so this is a static placeholder rather than
// something threaded through from a build flag.
const Version = "0.1.0"

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
func NewServer(lang i18n.Lang, provider data.Provider, history data.HistoryProvider, fundamentals data.FundamentalsProvider, earnings data.EarningsProvider) *mcp.Server {
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
	})
	return server
}

// Run starts the argus MCP server on stdio and blocks until the connection
// closes or ctx is cancelled. This is what cmd/bot's "mcp" subcommand
// invokes — an ACP chat session launches the same binary as a subprocess
// (os.Executable()) rather than a separately deployed server, so the tool
// surface can never drift out of version sync with the running bot.
func Run(ctx context.Context, lang i18n.Lang, provider data.Provider, history data.HistoryProvider, fundamentals data.FundamentalsProvider, earnings data.EarningsProvider) error {
	server := NewServer(lang, provider, history, fundamentals, earnings)
	return server.Run(ctx, &mcp.StdioTransport{})
}

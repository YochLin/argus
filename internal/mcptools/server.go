// Package mcptools implements Argus's MCP (Model Context Protocol) server —
// the read-only tool surface a chat session can call into for on-demand data
// (see PLAN.md's Phase 3.5). This is a skeleton: no tools are registered yet,
// that lands in a follow-up. Deliberately narrow dependency surface (only
// internal/data + internal/i18n once tools are added) so this stays a thin,
// provider-neutral adapter rather than pulling in internal/db or
// internal/llm.
package mcptools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the MCP server's reported implementation version. Argus has no
// release/version process, so this is a static placeholder rather than
// something threaded through from a build flag.
const Version = "0.1.0"

// NewServer builds the argus MCP server with no tools registered yet. Kept
// separate from Run so tests can add tools and connect over an in-memory
// transport (mcp.NewInMemoryTransports) without going through stdio.
func NewServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    "argus",
		Version: Version,
	}, nil)
}

// Run starts the argus MCP server on stdio and blocks until the connection
// closes or ctx is cancelled. This is what cmd/bot's "mcp" subcommand
// invokes — an ACP chat session launches the same binary as a subprocess
// (os.Executable()) rather than a separately deployed server, so the tool
// surface can never drift out of version sync with the running bot.
func Run(ctx context.Context) error {
	server := NewServer()
	return server.Run(ctx, &mcp.StdioTransport{})
}

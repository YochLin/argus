package llm

import "context"

// Provider is the seam between internal/llm's prompt building/response
// parsing and the underlying LLM backend's session lifecycle. Client talks
// to exactly one Provider today (acpProvider, driving Claude via ACP) — the
// interface exists so a second backend can be added later as a fallback
// (see PLAN.md's "LLM provider 介面 + Google 備援" architecture-debt item)
// without changing GenerateRecommendations/CheckStock/Chat or their callers.
//
// It covers both session lifecycles Client needs: a one-shot Prompt for
// GenerateRecommendations/CheckStock (nothing to remember between calls),
// and a persistent NewChatSession for Chat's free-form back-and-forth (the
// session keeps conversation history across calls until Close).
type Provider interface {
	// Prompt runs a single turn: start a session, send text, return the
	// reply, tear the session down.
	Prompt(ctx context.Context, systemPrompt, model, text string) (string, error)

	// NewChatSession starts a persistent multi-turn session with the argus
	// MCP tool set scope names. A provider that can't attach MCP servers
	// ignores scope and runs tool-less.
	NewChatSession(ctx context.Context, systemPrompt, model string, scope MCPScope) (ChatSession, error)
}

// MCPScope picks which argus MCP tool set a chat session gets — the value is
// the `argus mcp <scope>` argument the session's MCP subprocess is launched
// with (see argusMCPServer).
type MCPScope string

const (
	// MCPFull is `argus mcp`: market data, the portfolio, and the gated
	// trade/watchlist write tools — Telegram's chat.
	MCPFull MCPScope = ""
	// MCPWealth is `argus mcp wealth`: the wealth records only (assets,
	// cash flows, policies, goals, profile) and nothing that can touch a
	// trade, so the wealth assistant can't propose a buy.
	MCPWealth MCPScope = "wealth"
)

// ChatSession is one persistent multi-turn conversation against a Provider.
type ChatSession interface {
	// Send sends one user turn and returns the reply. The session retains
	// conversation history across calls until Close.
	Send(ctx context.Context, text string) (string, error)
	// Close ends the session and releases its underlying resources (e.g. the
	// backing agent subprocess).
	Close() error
}

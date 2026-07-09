package llm

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"argus/internal/llm/acp"
)

// acpProvider is the Provider implementation that drives Claude through the
// Agent Client Protocol (internal/llm/acp) — the only Provider Client is
// built with today. internal/llm/acp itself knows nothing about Claude (it's
// a generic ACP transport/handshake driver reusable by any ACP-speaking
// agent); everything Claude-specific — which binary to launch and the
// `_meta` fields only claude-agent-acp understands — lives in this file.
type acpProvider struct{}

func (acpProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	// One-shot analysis calls (GenerateRecommendations/CheckStock) stay
	// tool-less: their output is hand-parsed by parseRecommendations, and
	// the prefetch paths that build their prompts already control Finnhub
	// rate-limit exposure directly (see PLAN.md's Phase 3.5 "一次性指令加
	// 工具——暫緩"). Only Chat gets the MCP tool surface.
	session, err := startClaudeSession(ctx, systemPrompt, model, false)
	if err != nil {
		return "", err
	}
	defer session.Close()

	reply, err := session.Prompt(ctx, text)
	if err != nil {
		return "", fmt.Errorf("acp: %w", err)
	}
	return strings.TrimSpace(reply), nil
}

func (acpProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (ChatSession, error) {
	session, err := startClaudeSession(ctx, systemPrompt, model, true)
	if err != nil {
		return nil, err
	}
	return &acpChatSession{session: session}, nil
}

// startClaudeSession launches claude-agent-acp and opens an ACP session with
// all built-in tool use disabled — this bot only ever wants a text analysis
// back, never file/shell/network tool access. withMCPTools additionally
// wires in the argus MCP server (internal/mcptools) so the agent can call
// its read-only data tools (get_quote/get_history/...) — Phase 3.5's "掛進
// chat session" item; disableBuiltInTools is a claude-agent-acp _meta
// extension with no bearing on MCP servers, which are connected through the
// base ACP session/new "mcpServers" field instead, so the two don't
// conflict.
//
// Runs from os.TempDir(), a neutral, non-project directory: claude-agent-acp
// resolves .claude/settings.json and CLAUDE.md relative to cwd, and we don't
// want this repo's own coding-agent instructions bleeding into
// stock-analysis/chat prompts. systemPrompt replaces the agent's default
// "Claude Code" persona entirely. model is a Claude model alias/ID (e.g.
// "opus", "sonnet"); empty uses the agent's own default.
func startClaudeSession(ctx context.Context, systemPrompt, model string, withMCPTools bool) (*acp.Session, error) {
	command, args := claudeAgentCommand()

	meta := map[string]any{"disableBuiltInTools": true}
	if systemPrompt != "" {
		meta["systemPrompt"] = systemPrompt
	}
	if model != "" {
		meta["claudeCode"] = map[string]any{"options": map[string]any{"model": model}}
	}

	var mcpServers []acp.MCPServer
	if withMCPTools {
		server, err := argusMCPServer()
		if err != nil {
			// Missing tools shouldn't take chat down entirely — degrade to
			// the tool-less session chat already worked with before this
			// wiring existed, and let the caller notice via the log.
			log.Printf("llm: argus MCP server unavailable, starting chat session without tools: %v", err)
		} else {
			mcpServers = []acp.MCPServer{server}
		}
	}

	session, err := acp.StartSession(ctx, command, args, os.TempDir(), mcpServers, meta)
	if err != nil {
		return nil, fmt.Errorf("acp: %w", err)
	}
	return session, nil
}

// argusMCPServer resolves this same running binary's absolute path and
// builds the stdio MCP server config for its own "mcp" subcommand (see
// internal/mcptools) — reused as-is rather than a separately deployed
// server, so the tool surface can never drift out of version sync with the
// running bot (same rationale as mcptools.Run's doc comment).
//
// Only FINNHUB_API_KEY and BOT_LANGUAGE are threaded through explicitly:
// claude-agent-acp launches this subprocess itself (not this Go process
// directly) with a bare environment and a cwd this package doesn't control
// (os.TempDir(), see startClaudeSession) — runMCPServer's own
// godotenv.Load() has no .env file to find there, so these two env vars
// (already loaded into this process's own environment by main()'s
// godotenv.Load() at startup) are the subprocess's only source of the
// config main() otherwise reads straight from .env.
func argusMCPServer() (acp.MCPServer, error) {
	exe, err := os.Executable()
	if err != nil {
		return acp.MCPServer{}, fmt.Errorf("resolve own executable: %w", err)
	}
	env := map[string]string{}
	if v := os.Getenv("FINNHUB_API_KEY"); v != "" {
		env["FINNHUB_API_KEY"] = v
	}
	if v := os.Getenv("BOT_LANGUAGE"); v != "" {
		env["BOT_LANGUAGE"] = v
	}
	return acp.MCPServer{
		Name:    "argus",
		Command: exe,
		Args:    []string{"mcp"},
		Env:     env,
	}, nil
}

// claudeAgentCommand resolves how to launch claude-agent-acp. Defaults to
// `npx`, which works with no setup beyond having Node installed; set
// CLAUDE_ACP_COMMAND to a globally-installed binary (e.g. after
// `npm install -g @agentclientprotocol/claude-agent-acp`) to skip npx's
// resolution overhead on every call.
func claudeAgentCommand() (string, []string) {
	if custom := os.Getenv("CLAUDE_ACP_COMMAND"); custom != "" {
		return custom, nil
	}
	return "npx", []string{"-y", "@agentclientprotocol/claude-agent-acp"}
}

// acpChatSession adapts *acp.Session — which already keeps conversation
// history inside the underlying claude-agent-acp process — to the
// ChatSession interface.
type acpChatSession struct {
	session *acp.Session
}

func (s *acpChatSession) Send(ctx context.Context, text string) (string, error) {
	reply, err := s.session.Prompt(ctx, text)
	if err != nil {
		return "", fmt.Errorf("acp: %w", err)
	}
	return strings.TrimSpace(reply), nil
}

func (s *acpChatSession) Close() error {
	return s.session.Close()
}

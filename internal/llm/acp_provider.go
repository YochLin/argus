package llm

import (
	"context"
	"fmt"
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
	session, err := startClaudeSession(ctx, systemPrompt, model)
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
	session, err := startClaudeSession(ctx, systemPrompt, model)
	if err != nil {
		return nil, err
	}
	return &acpChatSession{session: session}, nil
}

// startClaudeSession launches claude-agent-acp and opens an ACP session with
// all built-in tool use disabled — this bot only ever wants a text analysis
// back, never file/shell/network tool access.
//
// Runs from os.TempDir(), a neutral, non-project directory: claude-agent-acp
// resolves .claude/settings.json and CLAUDE.md relative to cwd, and we don't
// want this repo's own coding-agent instructions bleeding into
// stock-analysis/chat prompts. systemPrompt replaces the agent's default
// "Claude Code" persona entirely. model is a Claude model alias/ID (e.g.
// "opus", "sonnet"); empty uses the agent's own default.
func startClaudeSession(ctx context.Context, systemPrompt, model string) (*acp.Session, error) {
	command, args := claudeAgentCommand()

	meta := map[string]any{"disableBuiltInTools": true}
	if systemPrompt != "" {
		meta["systemPrompt"] = systemPrompt
	}
	if model != "" {
		meta["claudeCode"] = map[string]any{"options": map[string]any{"model": model}}
	}

	session, err := acp.StartSession(ctx, command, args, os.TempDir(), meta)
	if err != nil {
		return nil, fmt.Errorf("acp: %w", err)
	}
	return session, nil
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

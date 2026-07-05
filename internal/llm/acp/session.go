package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Session is a single ACP conversation against a running claude-agent-acp
// process, authenticated via the operator's local `claude` CLI login
// (Claude Pro/Max subscription) rather than a billed API key.
type Session struct {
	conn      *Conn
	sessionID string
}

// StartSession launches the ACP agent, completes the initialize handshake,
// and opens a session with all built-in tool use disabled — this bot only
// ever wants a text analysis back, never file/shell/network tool access.
//
// cwd should be a neutral, non-project directory: the agent resolves
// .claude/settings.json and CLAUDE.md relative to it, and we don't want this
// repo's own coding-agent instructions bleeding into stock-analysis prompts.
// systemPrompt replaces the agent's default "Claude Code" persona entirely.
// model is a Claude model alias/ID (e.g. "opus", "sonnet"); empty uses the
// agent's own default.
func StartSession(ctx context.Context, cwd, systemPrompt, model string) (*Session, error) {
	command, args := agentCommand()
	conn, err := Dial(ctx, command, args...)
	if err != nil {
		return nil, err
	}

	if _, err := conn.Call(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{},
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	meta := map[string]any{"disableBuiltInTools": true}
	if systemPrompt != "" {
		meta["systemPrompt"] = systemPrompt
	}
	if model != "" {
		meta["claudeCode"] = map[string]any{"options": map[string]any{"model": model}}
	}

	result, err := conn.Call(ctx, "session/new", map[string]any{
		"cwd":        cwd,
		"mcpServers": []any{},
		"_meta":      meta,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("session/new: %w", err)
	}

	var parsed struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		conn.Close()
		return nil, fmt.Errorf("session/new: parse response: %w", err)
	}

	return &Session{conn: conn, sessionID: parsed.SessionID}, nil
}

// Prompt sends a single user turn and returns the concatenated text of the
// agent's reply.
func (s *Session) Prompt(ctx context.Context, text string) (string, error) {
	var reply strings.Builder
	s.conn.NotifyHandler = func(method string, params json.RawMessage) {
		if method != "session/update" {
			return
		}
		var update struct {
			SessionID string `json:"sessionId"`
			Update    struct {
				SessionUpdate string `json:"sessionUpdate"`
				Content       struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"update"`
		}
		if err := json.Unmarshal(params, &update); err != nil {
			return
		}
		if update.SessionID != s.sessionID || update.Update.SessionUpdate != "agent_message_chunk" {
			return
		}
		if update.Update.Content.Type == "text" {
			reply.WriteString(update.Update.Content.Text)
		}
	}

	if _, err := s.conn.Call(ctx, "session/prompt", map[string]any{
		"sessionId": s.sessionID,
		"prompt": []map[string]any{
			{"type": "text", "text": text},
		},
	}); err != nil {
		return "", fmt.Errorf("session/prompt: %w", err)
	}

	return reply.String(), nil
}

// Close shuts down the underlying agent process.
func (s *Session) Close() error {
	return s.conn.Close()
}

// agentCommand resolves how to launch the ACP agent. Defaults to `npx`,
// which works with no setup beyond having Node installed; set
// CLAUDE_ACP_COMMAND to a globally-installed binary (e.g. after
// `npm install -g @agentclientprotocol/claude-agent-acp`) to skip npx's
// resolution overhead on every call.
func agentCommand() (string, []string) {
	if custom := os.Getenv("CLAUDE_ACP_COMMAND"); custom != "" {
		return custom, nil
	}
	return "npx", []string{"-y", "@agentclientprotocol/claude-agent-acp"}
}

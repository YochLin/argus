package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Session is a single ACP conversation against a running agent process.
type Session struct {
	conn      *Conn
	sessionID string
}

// StartSession launches command (with args) as the agent process, completes
// the ACP initialize handshake, and opens a session with the given cwd and
// _meta payload.
//
// meta is opaque to this package — its shape is an implementation-specific
// ACP extension, not part of the base protocol, so it means whatever the
// launched agent's own ACP adapter defines (e.g. claude-agent-acp's
// disableBuiltInTools/systemPrompt/claudeCode.options.model — see
// llm.acpProvider, which builds that map). This package only knows how to
// launch a process and speak the ACP handshake against it, not which agent
// it is or what that agent's private fields mean.
func StartSession(ctx context.Context, command string, args []string, cwd string, meta map[string]any) (*Session, error) {
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

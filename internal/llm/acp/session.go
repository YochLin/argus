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

// MCPServer describes one stdio-transport MCP server for the agent to
// connect to for a session (the ACP spec's session/new mcpServers field —
// this package only implements the stdio variant since that's the only one
// Argus needs today). Name becomes the tool-name prefix the agent uses for
// this server's tools (mcp__<Name>__<tool>, standard Claude Agent SDK MCP
// tool naming) — see Conn's permission auto-approval in conn.go, which
// trusts exactly the servers passed in here and nothing else.
type MCPServer struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

func (s MCPServer) wireFormat() map[string]any {
	env := make([]map[string]string, 0, len(s.Env))
	for k, v := range s.Env {
		env = append(env, map[string]string{"name": k, "value": v})
	}
	return map[string]any{
		"name":    s.Name,
		"command": s.Command,
		"args":    s.Args,
		"env":     env,
	}
}

// StartSession launches command (with args) as the agent process, completes
// the ACP initialize handshake, and opens a session with the given cwd,
// mcpServers, and _meta payload.
//
// meta is opaque to this package — its shape is an implementation-specific
// ACP extension, not part of the base protocol, so it means whatever the
// launched agent's own ACP adapter defines (e.g. claude-agent-acp's
// disableBuiltInTools/systemPrompt/claudeCode.options.model — see
// llm.acpProvider, which builds that map). This package only knows how to
// launch a process and speak the ACP handshake against it, not which agent
// it is or what that agent's private fields mean. mcpServers, by contrast,
// is part of the base ACP protocol (not an extension), so it's a typed
// parameter rather than folded into meta.
func StartSession(ctx context.Context, command string, args []string, cwd string, mcpServers []MCPServer, meta map[string]any) (*Session, error) {
	conn, err := Dial(ctx, command, args...)
	if err != nil {
		return nil, err
	}
	conn.trustPermissionsFor(mcpServers)

	if _, err := conn.Call(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{},
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	mcpServersPayload := make([]map[string]any, len(mcpServers))
	for i, s := range mcpServers {
		mcpServersPayload[i] = s.wireFormat()
	}
	result, err := conn.Call(ctx, "session/new", map[string]any{
		"cwd":        cwd,
		"mcpServers": mcpServersPayload,
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

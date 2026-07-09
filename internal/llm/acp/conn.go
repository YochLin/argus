// Package acp implements just enough of the Agent Client Protocol (ACP) to
// drive a local claude-agent-acp process from Go: a JSON-RPC 2.0 connection
// over newline-delimited JSON on the subprocess's stdin/stdout.
package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

// Conn is a JSON-RPC 2.0 connection to an ACP agent subprocess.
type Conn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	nextID int64

	mu      sync.Mutex
	pending map[string]chan rpcMessage

	// trustedToolPrefixes lists "mcp__<server>__" prefixes this connection
	// auto-approves session/request_permission calls for — exactly the MCP
	// servers this same process configured via StartSession's mcpServers
	// param (trustPermissionsFor), since those are servers we launched
	// ourselves rather than ones the agent decided to connect to on its
	// own. Everything else (built-in tools, if disableBuiltInTools were
	// ever off; any other MCP server) is denied, same as before MCP
	// support existed.
	trustedToolPrefixes []string

	// NotifyHandler is invoked for every agent->client notification
	// (a message with a method but no id), e.g. "session/update".
	NotifyHandler func(method string, params json.RawMessage)
}

// trustPermissionsFor records the MCP servers this connection should
// auto-approve tool-call permission requests for. Must be called before the
// session/new call that actually connects them, so a permission request
// arriving early never races an unset trust list.
func (c *Conn) trustPermissionsFor(servers []MCPServer) {
	prefixes := make([]string, len(servers))
	for i, s := range servers {
		prefixes[i] = "mcp__" + s.Name + "__"
	}
	c.trustedToolPrefixes = prefixes
}

// Dial starts the given command and speaks ACP over its stdin/stdout.
func Dial(ctx context.Context, name string, args ...string) (*Conn, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start %s: %w", name, err)
	}

	c := &Conn{
		cmd:     cmd,
		stdin:   stdin,
		pending: make(map[string]chan rpcMessage),
	}
	go c.readLoop(stdout, &stderr)
	return c, nil
}

func (c *Conn) readLoop(stdout io.Reader, stderr *bytes.Buffer) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		switch {
		case len(msg.ID) > 0 && msg.Method == "":
			// Response to one of our requests.
			key := string(msg.ID)
			c.mu.Lock()
			ch, ok := c.pending[key]
			if ok {
				delete(c.pending, key)
			}
			c.mu.Unlock()
			if ok {
				ch <- msg
			}
		case len(msg.ID) > 0 && msg.Method != "":
			c.handleAgentRequest(msg)
		case msg.Method != "":
			if c.NotifyHandler != nil {
				c.NotifyHandler(msg.Method, msg.Params)
			}
		}
	}
	// Any pending calls will never resolve once the process is gone;
	// unblock them rather than leaving callers to hang until ctx timeout.
	c.mu.Lock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		msg := rpcMessage{Error: &rpcError{Message: fmt.Sprintf("acp: agent process exited: %s", stderr.String())}}
		ch <- msg
	}
	c.mu.Unlock()
}

func (c *Conn) respondError(id json.RawMessage, code int, message string) {
	resp := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   rpcError        `json:"error"`
	}{"2.0", id, rpcError{code, message}}
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	b = append(b, '\n')
	c.stdin.Write(b)
}

func (c *Conn) respondResult(id json.RawMessage, result any) {
	resp := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result}
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	b = append(b, '\n')
	c.stdin.Write(b)
}

// handleAgentRequest answers an agent -> client request. The only one this
// client understands is session/request_permission for a tool call from one
// of the MCP servers this connection itself configured (trustedToolPrefixes)
// — auto-approved, since we chose to launch that server. Everything else
// (fs/terminal access, a permission request outside every trusted prefix, or
// one this client can't even parse) is rejected: either with a proper
// RequestPermissionResponse reject outcome when the request parses far
// enough to pick one, or a blanket JSON-RPC "not supported" error otherwise.
func (c *Conn) handleAgentRequest(msg rpcMessage) {
	if msg.Method == "session/request_permission" && c.respondToPermissionRequest(msg) {
		return
	}
	c.respondError(msg.ID, -32601, "method not supported by this client")
}

// respondToPermissionRequest answers a session/request_permission call,
// approving iff the tool call's name (ACP renders it as toolCall.title for
// any tool the client has no built-in rendering for, which includes every
// MCP tool — see agentclientprotocol/claude-agent-acp's tools.js default
// case) starts with one of trustedToolPrefixes. Reports whether it sent a
// response at all — false means the request didn't parse or none of the
// offered options matched the desired allow/reject kind, so the caller
// should fall back to the generic "not supported" rejection instead of
// leaving the agent's request hanging.
func (c *Conn) respondToPermissionRequest(msg rpcMessage) bool {
	var req struct {
		ToolCall struct {
			Title string `json:"title"`
		} `json:"toolCall"`
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if err := json.Unmarshal(msg.Params, &req); err != nil {
		return false
	}

	trusted := false
	for _, prefix := range c.trustedToolPrefixes {
		if strings.HasPrefix(req.ToolCall.Title, prefix) {
			trusted = true
			break
		}
	}

	wantKind, fallbackKind := "reject_once", "reject_always"
	if trusted {
		wantKind, fallbackKind = "allow_once", "allow_always"
	}
	optionID := ""
	for _, o := range req.Options {
		if o.Kind == wantKind {
			optionID = o.OptionID
			break
		}
	}
	if optionID == "" {
		for _, o := range req.Options {
			if o.Kind == fallbackKind {
				optionID = o.OptionID
				break
			}
		}
	}
	if optionID == "" {
		return false
	}

	c.respondResult(msg.ID, map[string]any{
		"outcome": map[string]any{"outcome": "selected", "optionId": optionID},
	})
	return true
}

// Call sends a JSON-RPC request and blocks until the matching response
// arrives or ctx is done.
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	idJSON := json.RawMessage(fmt.Sprintf("%d", id))

	ch := make(chan rpcMessage, 1)
	c.mu.Lock()
	c.pending[string(idJSON)] = ch
	c.mu.Unlock()

	req := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  any             `json:"params,omitempty"`
	}{"2.0", idJSON, method, params}

	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("acp: marshal %s: %w", method, err)
	}
	b = append(b, '\n')
	if _, err := c.stdin.Write(b); err != nil {
		return nil, fmt.Errorf("acp: write %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		if msg.Error != nil {
			return nil, fmt.Errorf("acp: %s: %s (code %d)", method, msg.Error.Message, msg.Error.Code)
		}
		return msg.Result, nil
	}
}

// Close terminates the underlying agent process.
func (c *Conn) Close() error {
	c.stdin.Close()
	if c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

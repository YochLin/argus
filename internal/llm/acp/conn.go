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

	// NotifyHandler is invoked for every agent->client notification
	// (a message with a method but no id), e.g. "session/update".
	NotifyHandler func(method string, params json.RawMessage)
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
			// Agent -> client request (e.g. permission/fs/terminal). This bot
			// never grants tool access, so there's nothing to honor here.
			c.respondError(msg.ID, -32601, "method not supported by this client")
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

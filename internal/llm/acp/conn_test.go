package acp

import (
	"bytes"
	"encoding/json"
	"testing"
)

// nopWriteCloser lets tests give a Conn a plain bytes.Buffer as its stdin so
// respondToPermissionRequest/respondError's writes can be inspected without
// a real agent subprocess on the other end.
type nopWriteCloser struct{ *bytes.Buffer }

func (nopWriteCloser) Close() error { return nil }

func newTestConn(trustedToolPrefixes []string) (*Conn, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	c := &Conn{
		stdin:               nopWriteCloser{buf},
		pending:             make(map[string]chan rpcMessage),
		trustedToolPrefixes: trustedToolPrefixes,
	}
	return c, buf
}

func decodeResponse(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (raw: %s)", err, buf.String())
	}
	return resp
}

func selectedOptionID(t *testing.T, resp map[string]any) string {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("response has no result field: %#v", resp)
	}
	outcome, ok := result["outcome"].(map[string]any)
	if !ok {
		t.Fatalf("result has no outcome field: %#v", result)
	}
	optionID, _ := outcome["optionId"].(string)
	return optionID
}

func permissionRequestMsg(t *testing.T, toolTitle string, options []map[string]string) rpcMessage {
	t.Helper()
	opts := make([]map[string]any, len(options))
	for i, o := range options {
		opts[i] = map[string]any{"optionId": o["optionId"], "kind": o["kind"]}
	}
	params, err := json.Marshal(map[string]any{
		"toolCall": map[string]any{"title": toolTitle},
		"options":  opts,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return rpcMessage{ID: json.RawMessage(`"1"`), Method: "session/request_permission", Params: params}
}

func TestHandleAgentRequest_TrustedMCPToolApproved(t *testing.T) {
	c, buf := newTestConn([]string{"mcp__argus__"})
	msg := permissionRequestMsg(t, "mcp__argus__get_quote", []map[string]string{
		{"optionId": "allow_always", "kind": "allow_always"},
		{"optionId": "allow", "kind": "allow_once"},
		{"optionId": "reject", "kind": "reject_once"},
	})

	c.handleAgentRequest(msg)

	got := selectedOptionID(t, decodeResponse(t, buf))
	if got != "allow" {
		t.Errorf("optionId = %q, want %q (allow_once)", got, "allow")
	}
}

func TestHandleAgentRequest_UntrustedMCPToolRejected(t *testing.T) {
	c, buf := newTestConn([]string{"mcp__argus__"})
	msg := permissionRequestMsg(t, "mcp__someothserver__do_something", []map[string]string{
		{"optionId": "allow_always", "kind": "allow_always"},
		{"optionId": "allow", "kind": "allow_once"},
		{"optionId": "reject", "kind": "reject_once"},
	})

	c.handleAgentRequest(msg)

	got := selectedOptionID(t, decodeResponse(t, buf))
	if got != "reject" {
		t.Errorf("optionId = %q, want %q (reject_once)", got, "reject")
	}
}

func TestHandleAgentRequest_NoTrustedServersRejectsEverything(t *testing.T) {
	c, buf := newTestConn(nil)
	msg := permissionRequestMsg(t, "mcp__argus__get_quote", []map[string]string{
		{"optionId": "allow", "kind": "allow_once"},
		{"optionId": "reject", "kind": "reject_once"},
	})

	c.handleAgentRequest(msg)

	got := selectedOptionID(t, decodeResponse(t, buf))
	if got != "reject" {
		t.Errorf("optionId = %q, want %q (reject_once)", got, "reject")
	}
}

func TestHandleAgentRequest_FallsBackToAlwaysKindWhenOnceMissing(t *testing.T) {
	c, buf := newTestConn([]string{"mcp__argus__"})
	msg := permissionRequestMsg(t, "mcp__argus__get_quote", []map[string]string{
		{"optionId": "allow_always", "kind": "allow_always"},
		{"optionId": "reject_always", "kind": "reject_always"},
	})

	c.handleAgentRequest(msg)

	got := selectedOptionID(t, decodeResponse(t, buf))
	if got != "allow_always" {
		t.Errorf("optionId = %q, want %q", got, "allow_always")
	}
}

func TestHandleAgentRequest_UnparsablePermissionRequestFallsBackToError(t *testing.T) {
	c, buf := newTestConn([]string{"mcp__argus__"})
	msg := rpcMessage{ID: json.RawMessage(`"1"`), Method: "session/request_permission", Params: json.RawMessage(`not json`)}

	c.handleAgentRequest(msg)

	resp := decodeResponse(t, buf)
	if _, hasResult := resp["result"]; hasResult {
		t.Fatalf("expected an error response, got a result: %#v", resp)
	}
	if _, hasError := resp["error"]; !hasError {
		t.Fatalf("expected an error response, got: %#v", resp)
	}
}

func TestHandleAgentRequest_OtherMethodsRejectedWithProtocolError(t *testing.T) {
	c, buf := newTestConn([]string{"mcp__argus__"})
	msg := rpcMessage{ID: json.RawMessage(`"1"`), Method: "fs/write_text_file", Params: json.RawMessage(`{}`)}

	c.handleAgentRequest(msg)

	resp := decodeResponse(t, buf)
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected an error response, got: %#v", resp)
	}
	if code, _ := errObj["code"].(float64); code != -32601 {
		t.Errorf("error code = %v, want -32601", errObj["code"])
	}
}

func TestMCPServerWireFormat(t *testing.T) {
	s := MCPServer{
		Name:    "argus",
		Command: "/usr/local/bin/argus",
		Args:    []string{"mcp"},
		Env:     map[string]string{"FINNHUB_API_KEY": "secret"},
	}

	got := s.wireFormat()

	if got["name"] != "argus" || got["command"] != "/usr/local/bin/argus" {
		t.Fatalf("unexpected name/command: %#v", got)
	}
	args, ok := got["args"].([]string)
	if !ok || len(args) != 1 || args[0] != "mcp" {
		t.Fatalf("unexpected args: %#v", got["args"])
	}
	env, ok := got["env"].([]map[string]string)
	if !ok || len(env) != 1 || env[0]["name"] != "FINNHUB_API_KEY" || env[0]["value"] != "secret" {
		t.Fatalf("unexpected env: %#v", got["env"])
	}
}

func TestMCPServerWireFormat_EmptyEnvIsEmptyArrayNotNull(t *testing.T) {
	s := MCPServer{Name: "argus", Command: "/usr/local/bin/argus", Args: []string{"mcp"}}

	b, err := json.Marshal(s.wireFormat())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Env []map[string]string `json:"env"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Env == nil {
		t.Errorf("env marshaled as null, want an empty array (schema requires Array<EnvVariable>, not optional)")
	}
}

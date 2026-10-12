package acp

import (
	"encoding/json"
	"testing"
)

func feed(r *replyCollector, sessionID, kind, text string) {
	u := map[string]any{"sessionId": sessionID, "update": map[string]any{"sessionUpdate": kind, "content": map[string]any{"type": "text", "text": text}}}
	b, _ := json.Marshal(u)
	r.add(b)
}

func TestReplyCollector(t *testing.T) {
	cases := []struct {
		name    string
		updates [][3]string // session, kind, text
		want    string
	}{
		{"no tools", [][3]string{{"s", "agent_message_chunk", "Hello "}, {"s", "agent_message_chunk", "there"}}, "Hello there"},
		{"narration before tool calls is dropped", [][3]string{
			{"s", "agent_message_chunk", "Let me check. "},
			{"s", "tool_call", ""},
			{"s", "agent_message_chunk", "Both saved, proposing an update."},
			{"s", "tool_call_update", ""},
			{"s", "agent_message_chunk", "已加入草稿。"},
		}, "已加入草稿。"},
		{"nothing after the last tool keeps everything", [][3]string{
			{"s", "agent_message_chunk", "已加入草稿。"},
			{"s", "tool_call", ""},
			{"s", "agent_message_chunk", "  \n"},
		}, "已加入草稿。  \n"},
		{"other sessions are ignored", [][3]string{{"other", "agent_message_chunk", "x"}, {"s", "agent_message_chunk", "y"}}, "y"},
	}
	for _, c := range cases {
		r := &replyCollector{sessionID: "s"}
		for _, u := range c.updates {
			feed(r, u[0], u[1], u[2])
		}
		if got := r.text(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
)

// fakeChatter answers every turn with reply (or err), reports fresh on the
// turns listed in freshTurns (1-based), and records what it was sent.
type fakeChatter struct {
	reply      string
	err        error
	freshTurns map[int]bool
	block      chan struct{} // when set, Chat waits on it
	entered    chan struct{} // when set, Chat signals it on entry
	during     func()        // when set, Chat runs it — the model's tool calls

	sent   []string
	keys   []llm.ChatKey
	resets int
}

func (f *fakeChatter) Chat(ctx context.Context, key llm.ChatKey, text string) (string, bool, error) {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.block != nil {
		<-f.block
	}
	if f.during != nil {
		f.during()
	}
	f.sent = append(f.sent, text)
	f.keys = append(f.keys, key)
	if f.err != nil {
		return "", false, f.err
	}
	return f.reply, f.freshTurns[len(f.sent)], nil
}

func (f *fakeChatter) ResetChat(key llm.ChatKey) { f.resets++ }

func newAssistantTestServer(t *testing.T, chat AssistantChatter) (*Server, *http.Cookie) {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := New(Config{DB: d, Password: "pw", Lang: i18n.EN, Assistant: chat})
	return s, loginAndGetCookie(t, s, "pw")
}

func postAssistant(t *testing.T, s *Server, cookie *http.Cookie, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func assistantMessages(t *testing.T, rec *httptest.ResponseRecorder) []assistantMessage {
	t.Helper()
	var resp struct {
		Messages []assistantMessage `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
	}
	return resp.Messages
}

func getConversation(t *testing.T, s *Server) []assistantMessage {
	t.Helper()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/wealth/assistant", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d: %s", rec.Code, rec.Body.String())
	}
	return assistantMessages(t, rec)
}

func roles(msgs []assistantMessage) string {
	var r []string
	for _, m := range msgs {
		r = append(r, m.Role)
	}
	return strings.Join(r, ",")
}

func TestWealthAssistant_ChatStoresTheTurn(t *testing.T) {
	chat := &fakeChatter{reply: "What's the rate?", freshTurns: map[int]bool{1: true}}
	s, cookie := newAssistantTestServer(t, chat)

	if got := getConversation(t, s); len(got) != 0 {
		t.Fatalf("new conversation = %+v, want empty", got)
	}

	rec := postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "  I have a mortgage  "})
	if rec.Code != http.StatusOK {
		t.Fatalf("chat status = %d: %s", rec.Code, rec.Body.String())
	}
	added := assistantMessages(t, rec)
	// The very first turn is fresh too, but there was nothing to forget: no divider.
	if roles(added) != "user,ai" || added[0].Text != "I have a mortgage" || added[1].Text != "What's the rate?" {
		t.Errorf("added = %+v, want the trimmed user text and the reply", added)
	}
	if len(chat.keys) != 1 || chat.keys[0] != llm.ChatWealth {
		t.Errorf("chat keys = %v, want the wealth key", chat.keys)
	}
	if got := getConversation(t, s); roles(got) != "user,ai" {
		t.Errorf("stored conversation = %s, want user,ai", roles(got))
	}
}

func TestWealthAssistant_FreshSessionAfterTurnsAddsDivider(t *testing.T) {
	chat := &fakeChatter{reply: "ok", freshTurns: map[int]bool{1: true, 2: false, 3: true}}
	s, cookie := newAssistantTestServer(t, chat)
	for i := 0; i < 3; i++ {
		if rec := postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "hi"}); rec.Code != http.StatusOK {
			t.Fatalf("turn %d status = %d", i+1, rec.Code)
		}
	}
	if got := roles(getConversation(t, s)); got != "user,ai,user,ai,div,user,ai" {
		t.Errorf("conversation = %s, want a divider before the third turn only", got)
	}
}

func TestWealthAssistant_FailureStoresNothing(t *testing.T) {
	chat := &fakeChatter{err: errors.New("model down")}
	s, cookie := newAssistantTestServer(t, chat)
	rec := postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "hello"})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if got := getConversation(t, s); len(got) != 0 {
		t.Errorf("conversation after a failed turn = %+v, want nothing stored (the text goes back to the input box)", got)
	}
}

func TestWealthAssistant_GoTradesMarker(t *testing.T) {
	chat := &fakeChatter{reply: "Stocks come from trade records.\n" + llm.WealthGoTradesMarker}
	s, cookie := newAssistantTestServer(t, chat)
	added := assistantMessages(t, postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "I own TSMC"}))
	ai := added[len(added)-1]
	if ai.Text != "Stocks come from trade records." || ai.Action != "trades" {
		t.Errorf("ai message = %+v, want the marker stripped and action trades", ai)
	}
	if got := getConversation(t, s); got[len(got)-1].Action != "trades" {
		t.Errorf("stored ai message lost its action: %+v", got)
	}
}

func TestWealthAssistant_Topic(t *testing.T) {
	chat := &fakeChatter{reply: "ok"}
	s, cookie := newAssistantTestServer(t, chat)

	added := assistantMessages(t, postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "hi", Topic: "insure"}))
	if !strings.Contains(chat.sent[0], wealthTopics["insure"]) || !strings.HasSuffix(chat.sent[0], "hi") {
		t.Errorf("sent %q, want the insurance topic hint before the text", chat.sent[0])
	}
	if added[0].Text != "hi" {
		t.Errorf("stored user text = %q, want it without the hint", added[0].Text)
	}

	if rec := postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "hi", Topic: "nope"}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown topic status = %d, want 400", rec.Code)
	}
}

func TestWealthAssistant_RejectsBadInput(t *testing.T) {
	s, cookie := newAssistantTestServer(t, &fakeChatter{reply: "ok"})
	for name, text := range map[string]string{"empty": "   ", "too long": strings.Repeat("字", assistantMaxRunes+1)} {
		if rec := postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: text}); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestWealthAssistant_OneTurnAtATime(t *testing.T) {
	chat := &fakeChatter{reply: "ok", block: make(chan struct{}), entered: make(chan struct{})}
	s, cookie := newAssistantTestServer(t, chat)

	done := make(chan int)
	go func() {
		done <- postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "first"}).Code
	}()
	<-chat.entered // the first turn is inside the model call, holding the lock
	if rec := postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "second"}); rec.Code != http.StatusConflict {
		t.Errorf("second turn while the first runs: status = %d, want 409", rec.Code)
	}
	close(chat.block)
	if code := <-done; code != http.StatusOK {
		t.Errorf("first turn status = %d, want 200", code)
	}
}

func TestWealthAssistant_NewClearsAndResets(t *testing.T) {
	chat := &fakeChatter{reply: "ok"}
	s, cookie := newAssistantTestServer(t, chat)
	postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "hi"})

	if rec := postAssistant(t, s, cookie, "/api/wealth/assistant/new", struct{}{}); rec.Code != http.StatusOK {
		t.Fatalf("new status = %d: %s", rec.Code, rec.Body.String())
	}
	if chat.resets != 1 {
		t.Errorf("ResetChat called %d times, want 1", chat.resets)
	}
	if got := getConversation(t, s); len(got) != 0 {
		t.Errorf("conversation after 開新對話 = %+v, want empty", got)
	}
}

func TestWealthAssistant_Gates(t *testing.T) {
	s, _ := newAssistantTestServer(t, &fakeChatter{reply: "ok"})
	if rec := postAssistant(t, s, nil, "/api/wealth/assistant/chat", assistantChatRequest{Text: "hi"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("chat without login: status = %d, want 401", rec.Code)
	}

	noModel, cookie := newAssistantTestServer(t, nil)
	if rec := postAssistant(t, noModel, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "hi"}); rec.Code != http.StatusConflict {
		t.Errorf("chat with no model wired: status = %d, want 409", rec.Code)
	}
}

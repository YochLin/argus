package llm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"argus/internal/i18n"
)

// fakeReplyProvider is a minimal Provider that always returns a fixed reply
// for Prompt — enough to exercise Client.GenerateRecommendations' post-parse
// logic without spawning a real ACP subprocess (same purpose as
// NewClientWithProvider's doc comment describes for internal/bot's E2E test).
type fakeReplyProvider struct {
	reply string
	err   error
}

func (f fakeReplyProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	return f.reply, f.err
}

func (f fakeReplyProvider) NewChatSession(ctx context.Context, systemPrompt, model string, scope MCPScope) (ChatSession, error) {
	return nil, errors.New("not implemented")
}

func TestGenerateRecommendations_ParseFailure(t *testing.T) {
	reply := "just some prose with no [TICKER: ...] blocks"
	c := NewClientWithProvider(fakeReplyProvider{reply: reply}, "", "", "", i18n.EN)
	summary, recs, raw, _, _, err := c.GenerateRecommendations(context.Background(), nil, nil, nil, nil, nil, false)
	if !errors.Is(err, ErrRecommendationParseFailed) {
		t.Fatalf("GenerateRecommendations() err = %v, want ErrRecommendationParseFailed", err)
	}
	if recs != nil || summary != "" {
		t.Errorf("GenerateRecommendations() = (%q, %+v), want (\"\", nil) on parse failure", summary, recs)
	}
	// Phase 19: raw must still come back on a parse failure — that's exactly
	// the case a caller most needs to record for later auditing.
	if raw != reply {
		t.Errorf("GenerateRecommendations() raw = %q, want the model's reply even on parse failure", raw)
	}
}

func TestGenerateRecommendations_EmptyReplyIsNotParseFailure(t *testing.T) {
	c := NewClientWithProvider(fakeReplyProvider{reply: ""}, "", "", "", i18n.EN)
	_, recs, _, _, _, err := c.GenerateRecommendations(context.Background(), nil, nil, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("GenerateRecommendations() err = %v, want nil for an empty (not malformed) reply", err)
	}
	if len(recs) != 0 {
		t.Errorf("GenerateRecommendations() recs = %+v, want empty", recs)
	}
}

func TestGenerateRecommendations_ParsesSuccessfully(t *testing.T) {
	reply := "[TICKER: AAPL]\nAction: BUY\nReason: strong earnings\n"
	c := NewClientWithProvider(fakeReplyProvider{reply: reply}, "", "", "", i18n.EN)
	_, recs, _, _, _, err := c.GenerateRecommendations(context.Background(), nil, nil, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("GenerateRecommendations() err = %v, want nil", err)
	}
	if len(recs) != 1 || recs[0].Ticker != "AAPL" {
		t.Errorf("GenerateRecommendations() recs = %+v, want one AAPL recommendation", recs)
	}
}

type fakeChatSession struct {
	sendFunc func(text string) (string, error)
}

func (s *fakeChatSession) Send(ctx context.Context, text string) (string, error) {
	if s.sendFunc != nil {
		return s.sendFunc(text)
	}
	return "default reply", nil
}

func (s *fakeChatSession) Close() error {
	return nil
}

type fakeChatProvider struct {
	session ChatSession
	err     error
}

func (f fakeChatProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	return "", errors.New("not implemented")
}

func (f fakeChatProvider) NewChatSession(ctx context.Context, systemPrompt, model string, scope MCPScope) (ChatSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

func TestChat_FallbackWhenSendFailsOnInitialTurn(t *testing.T) {
	// Primary provider returns a session whose Send fails (like acpProvider hitting rate limit)
	primary := fakeChatProvider{
		session: &fakeChatSession{
			sendFunc: func(text string) (string, error) {
				return "", errors.New("acp: session limit hit")
			},
		},
	}
	// Fallback provider returns a working session
	fallback := fakeChatProvider{
		session: &fakeChatSession{
			sendFunc: func(text string) (string, error) {
				return "fallback agy reply", nil
			},
		},
	}

	c := NewClientWithProvider(primary, "", "", "", i18n.EN)
	c.AddFallback(fallback, "", "", "")

	reply, _, err := c.Chat(context.Background(), ChatTelegram, "hello")
	if err != nil {
		t.Fatalf("Chat() unexpected error: %v", err)
	}
	if reply != "fallback agy reply" {
		t.Errorf("Chat() reply = %q, want %q", reply, "fallback agy reply")
	}
}

func TestChat_FallbackWhenSendFailsMidConversation(t *testing.T) {
	turn := 0
	primarySession := &fakeChatSession{
		sendFunc: func(text string) (string, error) {
			turn++
			if turn == 1 {
				return "primary reply turn 1", nil
			}
			return "", errors.New("acp: session limit hit on turn 2")
		},
	}
	primary := fakeChatProvider{session: primarySession}

	fallbackSession := &fakeChatSession{
		sendFunc: func(text string) (string, error) {
			return "fallback reply turn 2", nil
		},
	}
	fallback := fakeChatProvider{session: fallbackSession}

	c := NewClientWithProvider(primary, "", "", "", i18n.EN)
	c.AddFallback(fallback, "", "", "")

	// Turn 1: Primary succeeds
	reply1, _, err := c.Chat(context.Background(), ChatTelegram, "turn 1")
	if err != nil {
		t.Fatalf("Turn 1 Chat() unexpected error: %v", err)
	}
	if reply1 != "primary reply turn 1" {
		t.Errorf("Turn 1 reply = %q, want %q", reply1, "primary reply turn 1")
	}

	// Turn 2: Primary fails, Chat automatically falls back to fallback provider
	reply2, _, err := c.Chat(context.Background(), ChatTelegram, "turn 2")
	if err != nil {
		t.Fatalf("Turn 2 Chat() unexpected error: %v", err)
	}
	if reply2 != "fallback reply turn 2" {
		t.Errorf("Turn 2 reply = %q, want %q", reply2, "fallback reply turn 2")
	}
}

// sessionCountingProvider opens a new numbered session on every
// NewChatSession call; each session's reply names its own number and the
// system prompt it was opened with, so a test can tell sessions apart.
type sessionCountingProvider struct {
	opened *int
	closed *int
}

func (p sessionCountingProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	return "", errors.New("not implemented")
}

func (p sessionCountingProvider) NewChatSession(ctx context.Context, systemPrompt, model string, scope MCPScope) (ChatSession, error) {
	*p.opened++
	n := *p.opened
	return &countedSession{n: n, systemPrompt: systemPrompt, closed: p.closed}, nil
}

type countedSession struct {
	n            int
	systemPrompt string
	closed       *int
}

func (s *countedSession) Send(ctx context.Context, text string) (string, error) {
	return fmt.Sprintf("session %d (%s)", s.n, s.systemPrompt), nil
}

func (s *countedSession) Close() error {
	*s.closed++
	return nil
}

func newCountingClient(t *testing.T) (c *Client, opened, closed *int) {
	t.Helper()
	opened, closed = new(int), new(int)
	c = NewClientWithProvider(sessionCountingProvider{opened: opened, closed: closed}, "", "", "", i18n.EN)
	return c, opened, closed
}

func mustChat(t *testing.T, c *Client, key ChatKey) (string, bool) {
	t.Helper()
	reply, fresh, err := c.Chat(context.Background(), key, "hi")
	if err != nil {
		t.Fatalf("Chat(%s) error: %v", key, err)
	}
	return reply, fresh
}

func TestChat_KeysAreSeparateSessions(t *testing.T) {
	c, opened, _ := newCountingClient(t)
	const other ChatKey = "other"
	c.chatSpecs[other] = chatSpec{systemPrompt: i18n.KeySystemPromptChat}

	r1, fresh1 := mustChat(t, c, ChatTelegram)
	r2, fresh2 := mustChat(t, c, other)
	r3, fresh3 := mustChat(t, c, ChatTelegram)

	if *opened != 2 {
		t.Fatalf("opened %d sessions, want 2 (one per key)", *opened)
	}
	if r1 == r2 {
		t.Errorf("two keys answered from the same session: %q", r1)
	}
	if r3 != r1 {
		t.Errorf("telegram's second turn = %q, want its own session %q back", r3, r1)
	}
	if !fresh1 || !fresh2 || fresh3 {
		t.Errorf("fresh = %v, %v, %v; want true, true, false", fresh1, fresh2, fresh3)
	}
}

func TestChat_ResetOnlyClosesItsOwnKey(t *testing.T) {
	c, opened, closed := newCountingClient(t)
	const other ChatKey = "other"
	c.chatSpecs[other] = chatSpec{systemPrompt: i18n.KeySystemPromptChat}
	mustChat(t, c, ChatTelegram)
	otherReply, _ := mustChat(t, c, other)

	c.ResetChat(ChatTelegram)
	if *closed != 1 {
		t.Fatalf("ResetChat closed %d sessions, want 1", *closed)
	}
	if r, fresh := mustChat(t, c, other); r != otherReply || fresh {
		t.Errorf("other key after telegram reset = %q (fresh %v), want its old session %q", r, fresh, otherReply)
	}
	if _, fresh := mustChat(t, c, ChatTelegram); !fresh {
		t.Error("telegram after reset should get a fresh session")
	}
	if *opened != 3 {
		t.Errorf("opened %d sessions, want 3", *opened)
	}

	c.Close()
	if *closed != 3 {
		t.Errorf("after Close, closed %d sessions, want all 3", *closed)
	}
}

func TestChat_UnknownKey(t *testing.T) {
	c, opened, _ := newCountingClient(t)
	if _, _, err := c.Chat(context.Background(), "nope", "hi"); err == nil {
		t.Fatal("Chat with an unregistered key should fail")
	}
	if *opened != 0 {
		t.Errorf("opened %d sessions for an unknown key, want 0", *opened)
	}
}

func TestChat_SessionAgesOut(t *testing.T) {
	c, opened, closed := newCountingClient(t)
	const k ChatKey = "aging"
	c.chatSpecs[k] = chatSpec{systemPrompt: i18n.KeySystemPromptChat, maxIdle: 30 * time.Minute, maxTurns: 3}
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	// Turn limit: turns 1-3 share a session, turn 4 opens a new one.
	for i, wantFresh := range []bool{true, false, false, true} {
		if _, fresh := mustChat(t, c, k); fresh != wantFresh {
			t.Errorf("turn %d fresh = %v, want %v", i+1, fresh, wantFresh)
		}
	}
	if *opened != 2 || *closed != 1 {
		t.Fatalf("after 4 turns opened=%d closed=%d, want 2 and 1", *opened, *closed)
	}

	// Idle limit: within 30 minutes keeps the session, past it starts over.
	now = now.Add(29 * time.Minute)
	if _, fresh := mustChat(t, c, k); fresh {
		t.Error("29 minutes idle should keep the session")
	}
	now = now.Add(31 * time.Minute)
	if _, fresh := mustChat(t, c, k); !fresh {
		t.Error("31 minutes idle should start a fresh session")
	}
}

func TestChat_TelegramNeverAgesOut(t *testing.T) {
	c, opened, _ := newCountingClient(t)
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	for i := 0; i < 50; i++ {
		mustChat(t, c, ChatTelegram)
		now = now.Add(24 * time.Hour)
	}
	if *opened != 1 {
		t.Errorf("telegram opened %d sessions over 50 days of turns, want 1 (only /reset starts over)", *opened)
	}
}

func TestMCPArgs(t *testing.T) {
	if got := mcpArgs(MCPFull); !slices.Equal(got, []string{"mcp"}) {
		t.Errorf("mcpArgs(MCPFull) = %v, want [mcp]", got)
	}
	if got := mcpArgs(MCPWealth); !slices.Equal(got, []string{"mcp", "wealth"}) {
		t.Errorf("mcpArgs(MCPWealth) = %v, want [mcp wealth]", got)
	}
}

// scopeRecordingProvider records the MCPScope each chat session was opened with.
type scopeRecordingProvider struct{ scopes *[]MCPScope }

func (p scopeRecordingProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	return "", errors.New("not implemented")
}

func (p scopeRecordingProvider) NewChatSession(ctx context.Context, systemPrompt, model string, scope MCPScope) (ChatSession, error) {
	*p.scopes = append(*p.scopes, scope)
	return &fakeChatSession{}, nil
}

func TestChat_OpensSessionWithItsKeysScope(t *testing.T) {
	var scopes []MCPScope
	c := NewClientWithProvider(scopeRecordingProvider{scopes: &scopes}, "", "", "", i18n.EN)
	const w ChatKey = "wealth-test"
	c.chatSpecs[w] = chatSpec{systemPrompt: i18n.KeySystemPromptChat, mcpScope: MCPWealth}

	mustChat(t, c, ChatTelegram)
	mustChat(t, c, w)
	if !slices.Equal(scopes, []MCPScope{MCPFull, MCPWealth}) {
		t.Errorf("sessions opened with scopes %q, want telegram on the full set and the other key on wealth", scopes)
	}
}

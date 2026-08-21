package llm

import (
	"context"
	"errors"
	"testing"

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

func (f fakeReplyProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (ChatSession, error) {
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

func (f fakeChatProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (ChatSession, error) {
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

	reply, err := c.Chat(context.Background(), "hello")
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
	reply1, err := c.Chat(context.Background(), "turn 1")
	if err != nil {
		t.Fatalf("Turn 1 Chat() unexpected error: %v", err)
	}
	if reply1 != "primary reply turn 1" {
		t.Errorf("Turn 1 reply = %q, want %q", reply1, "primary reply turn 1")
	}

	// Turn 2: Primary fails, Chat automatically falls back to fallback provider
	reply2, err := c.Chat(context.Background(), "turn 2")
	if err != nil {
		t.Fatalf("Turn 2 Chat() unexpected error: %v", err)
	}
	if reply2 != "fallback reply turn 2" {
		t.Errorf("Turn 2 reply = %q, want %q", reply2, "fallback reply turn 2")
	}
}

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
	c := NewClientWithProvider(fakeReplyProvider{reply: "just some prose with no [TICKER: ...] blocks"}, "", "", "", i18n.EN)
	summary, recs, err := c.GenerateRecommendations(context.Background(), nil, nil, nil, nil, nil, false)
	if !errors.Is(err, ErrRecommendationParseFailed) {
		t.Fatalf("GenerateRecommendations() err = %v, want ErrRecommendationParseFailed", err)
	}
	if recs != nil || summary != "" {
		t.Errorf("GenerateRecommendations() = (%q, %+v), want (\"\", nil) on parse failure", summary, recs)
	}
}

func TestGenerateRecommendations_EmptyReplyIsNotParseFailure(t *testing.T) {
	c := NewClientWithProvider(fakeReplyProvider{reply: ""}, "", "", "", i18n.EN)
	_, recs, err := c.GenerateRecommendations(context.Background(), nil, nil, nil, nil, nil, false)
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
	_, recs, err := c.GenerateRecommendations(context.Background(), nil, nil, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("GenerateRecommendations() err = %v, want nil", err)
	}
	if len(recs) != 1 || recs[0].Ticker != "AAPL" {
		t.Errorf("GenerateRecommendations() recs = %+v, want one AAPL recommendation", recs)
	}
}

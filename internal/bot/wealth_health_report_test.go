package bot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/service"
)

// viewLLM answers the AI view's one-off session with reply.
type viewLLM struct {
	reply string
	err   error
}

func (p viewLLM) Prompt(context.Context, string, string, string) (string, error) {
	return "", errors.New("fake: prompt not used")
}
func (p viewLLM) NewChatSession(context.Context, string, string, llm.MCPScope) (llm.ChatSession, error) {
	return viewSession(p), nil
}

type viewSession viewLLM

func (s viewSession) Send(context.Context, string) (string, error) { return s.reply, s.err }
func (s viewSession) Close() error                                 { return nil }

func TestRunWealthHealthReportListsChecksAndStoresAIView(t *testing.T) {
	b, d, ch := newNetworthTestBot(t)
	b.llm = llm.NewClientWithProvider(viewLLM{reply: "[SUMMARY] Overlap\n- Two policies both pay a daily hospital benefit."}, "", "", "", i18n.EN)
	gap := 84000.0
	items := []service.CheckItem{
		{Code: "emergency", Severity: "do", Gap: &gap},
		{Code: "salary", Severity: "data"},
		{Code: "goals", Severity: "tip"},
		{Code: "stale", Severity: "tip"},
	}
	b.SetWealthChecks(func(time.Time) (service.WealthChecks, error) { return service.WealthChecks{Items: items}, nil })

	b.RunWealthHealthReport(context.Background())

	if len(ch.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(ch.sent))
	}
	msg := ch.sent[0]
	for _, want := range []string{"🔴 Build up the emergency fund (NT$84,000)", "⚪ Fill in your annual salary", "1 more", "Two policies both pay", "not investment"} {
		if !strings.Contains(msg, want) {
			t.Errorf("report missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Update stale records") {
		t.Errorf("report lists more than three checks:\n%s", msg)
	}
	v := service.LoadWealthAIView(d)
	if v == nil || v.Summary != "Overlap" || len(v.Points) != 1 || v.GeneratedAt != time.Now().Format("2006-01-02") {
		t.Fatalf("stored view = %+v", v)
	}

	// A failed view keeps last month's on the home page and still sends.
	b.llm = llm.NewClientWithProvider(viewLLM{err: errors.New("acp down")}, "", "", "", i18n.EN)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // skip the retries
	b.RunWealthHealthReport(ctx)
	if len(ch.sent) != 2 || strings.Contains(ch.sent[1], "AI view") {
		t.Fatalf("second report = %q", ch.sent[1:])
	}
	if v := service.LoadWealthAIView(d); v == nil || v.Summary != "Overlap" {
		t.Errorf("failed view replaced the stored one: %+v", v)
	}
}

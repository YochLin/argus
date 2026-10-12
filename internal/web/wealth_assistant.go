package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/llm"
	"argus/internal/logger"
)

// The wealth assistant (Phase 28, docs/phase-28-wealth-ai-chat.md §5.2): a
// chat on the /w/* pages where the model reads the user's wealth records
// through `argus mcp wealth` and asks about what's missing. This file is the
// conversation half — A3. Drafts and writing them (B2) come later; for now
// the assistant can only talk.

// AssistantChatter is the seam onto *llm.Client the assistant routes need —
// an interface only so tests can fake the model.
type AssistantChatter interface {
	Chat(ctx context.Context, key llm.ChatKey, text string) (reply string, fresh bool, err error)
	ResetChat(key llm.ChatKey)
}

type assistantStore interface {
	ListAssistantMessages() ([]db.AssistantMessage, error)
	AddAssistantMessages(msgs []db.AssistantMessage) ([]db.AssistantMessage, error)
	ClearAssistantMessages() error
}

// assistantMaxRunes caps one message. Generous for "here are all my
// accounts" in one go, small enough that a pasted statement doesn't land in
// the model's context whole.
const assistantMaxRunes = 4000

// wealthTopics maps the page an entry point opened the assistant from (the
// /w/<topic> path segment the frontend sends) to how the model is told
// about it. Model-facing only and the model reads either language, so these
// stay English inside the localized KeyWealthTopicHint.
var wealthTopics = map[string]string{
	"balance": "the balance sheet page (accounts, property and loans)",
	"cash":    "the cash flow page (regular monthly income and spending)",
	"insure":  "the insurance page (policies and cover)",
	"retire":  "the retirement page (retirement age, spending and contributions)",
	"goals":   "the goals page",
	"alloc":   "the allocation page",
	"funds":   "the funds and DCA page",
}

// assistantMessage is one message on the wire. Action is "trades" on an ai
// turn whose reply carried llm.WealthGoTradesMarker.
type assistantMessage struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Action    string `json:"action,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// assistantMeta is what wealth_assistant_messages.meta holds.
type assistantMeta struct {
	Action string `json:"action,omitempty"`
}

func toAssistantMessages(rows []db.AssistantMessage) []assistantMessage {
	out := make([]assistantMessage, 0, len(rows))
	for _, m := range rows {
		var meta assistantMeta
		if err := json.Unmarshal([]byte(m.Meta), &meta); err != nil {
			logger.Errorf("wealth assistant: message %d meta: %v", m.ID, err)
		}
		out = append(out, assistantMessage{ID: m.ID, Role: m.Role, Text: m.Text, Action: meta.Action, CreatedAt: m.CreatedAt})
	}
	return out
}

// requireAssistant 409s when no model seam was wired, same shape as
// requireTrade.
func (s *Server) requireAssistant(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.assistant == nil || s.assistantDB == nil {
			writeError(w, http.StatusConflict, "the assistant is not available")
			return
		}
		next(w, r)
	}
}

// handleWealthAssistantGet returns the stored conversation. Ungated like
// every other wealth read — a read-only server still shows it.
func (s *Server) handleWealthAssistantGet(w http.ResponseWriter, r *http.Request) {
	if s.assistantDB == nil {
		writeJSON(w, http.StatusOK, map[string]any{"messages": []assistantMessage{}})
		return
	}
	rows, err := s.assistantDB.ListAssistantMessages()
	if err != nil {
		logger.Errorf("wealth assistant: list messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load the conversation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": toAssistantMessages(rows)})
}

type assistantChatRequest struct {
	Text  string `json:"text"`
	Topic string `json:"topic"`
}

// handleWealthAssistantChat sends one message and returns the messages it
// added. Nothing is stored unless the model answered: on a failure the
// frontend puts the text back in the input box (the design's error state),
// so a stored user message would be a duplicate when they resend it.
func (s *Server) handleWealthAssistantChat(w http.ResponseWriter, r *http.Request) {
	var req assistantChatRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if utf8.RuneCountInString(text) > assistantMaxRunes {
		writeError(w, http.StatusBadRequest, "message is too long")
		return
	}
	prompt := text
	if req.Topic != "" {
		topic, ok := wealthTopics[req.Topic]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown topic")
			return
		}
		prompt = i18n.T(s.lang, i18n.KeyWealthTopicHint, topic) + text
	}

	// One turn at a time: a second tab sending mid-reply would interleave
	// with the first in the model's session.
	if !s.assistantMu.TryLock() {
		writeError(w, http.StatusConflict, "the assistant is still answering")
		return
	}
	defer s.assistantMu.Unlock()

	prior, err := s.assistantDB.ListAssistantMessages()
	if err != nil {
		logger.Errorf("wealth assistant: list messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load the conversation")
		return
	}

	// WithoutCancel: a reload mid-reply shouldn't throw the answer away — it
	// is stored and shows up on the next load. llm.Chat bounds the call.
	reply, fresh, err := s.assistant.Chat(context.WithoutCancel(r.Context()), llm.ChatWealth, prompt)
	if err != nil {
		logger.Errorf("wealth assistant: chat: %v", err)
		writeError(w, http.StatusBadGateway, "the assistant didn't answer")
		return
	}

	var meta assistantMeta
	if strings.Contains(reply, llm.WealthGoTradesMarker) {
		reply = strings.TrimSpace(strings.ReplaceAll(reply, llm.WealthGoTradesMarker, ""))
		meta.Action = "trades"
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the reply")
		return
	}

	var add []db.AssistantMessage
	// A fresh session after earlier turns means the model forgot them; the
	// divider tells the user why it may ask again.
	if fresh && len(prior) > 0 {
		add = append(add, db.AssistantMessage{Role: "div"})
	}
	add = append(add,
		db.AssistantMessage{Role: "user", Text: text},
		db.AssistantMessage{Role: "ai", Text: reply, Meta: string(metaJSON)},
	)
	added, err := s.assistantDB.AddAssistantMessages(add)
	if err != nil {
		logger.Errorf("wealth assistant: save turn: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save the reply")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": toAssistantMessages(added)})
}

// handleWealthAssistantNew is 開新對話: forget the model's session and delete
// the stored conversation. Drafts (B2) are kept — they're what the user
// already said.
func (s *Server) handleWealthAssistantNew(w http.ResponseWriter, r *http.Request) {
	if !s.assistantMu.TryLock() {
		writeError(w, http.StatusConflict, "the assistant is still answering")
		return
	}
	defer s.assistantMu.Unlock()
	s.assistant.ResetChat(llm.ChatWealth)
	if err := s.assistantDB.ClearAssistantMessages(); err != nil {
		logger.Errorf("wealth assistant: clear messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to clear the conversation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": []assistantMessage{}})
}

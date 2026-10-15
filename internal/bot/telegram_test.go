package bot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestSendRetriesWithoutMarkdown pins the fallback in Send: a message whose
// dynamic text carries a stray Markdown character (here "國巨*", TWSE's real
// short name for 2327) gets 400'd by Telegram, and must still reach the user
// as plain text rather than being logged and dropped.
func TestSendRetriesWithoutMarkdown(t *testing.T) {
	var mu sync.Mutex
	var texts []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "getMe") {
			w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"t","username":"t_bot"}}`))
			return
		}
		if r.FormValue("parse_mode") != "" {
			w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities: Can't find end of the entity starting at byte offset 7"}`))
			return
		}
		mu.Lock()
		texts = append(texts, r.FormValue("text"))
		mu.Unlock()
		w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1}}}`))
	}))
	defer server.Close()

	api, err := tgbotapi.NewBotAPIWithAPIEndpoint("token", server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("NewBotAPIWithAPIEndpoint() error = %v", err)
	}
	c := &telegramChannel{api: api, chatID: 1}

	want := "已記錄 *國巨*(2327)* 的持有論點：被動元件龍頭"
	c.Send(want)

	mu.Lock()
	defer mu.Unlock()
	if len(texts) != 1 || texts[0] != want {
		t.Errorf("plain-text retry not delivered, got %q", texts)
	}
}

func TestSplitMessage(t *testing.T) {
	t.Run("short text is returned as a single chunk", func(t *testing.T) {
		got := splitMessage("hello\nworld\n", 100)
		if len(got) != 1 || got[0] != "hello\nworld\n" {
			t.Errorf("splitMessage() = %v, want single unchanged chunk", got)
		}
	})

	t.Run("splits on line boundaries, never mid-line", func(t *testing.T) {
		// Each line is 6 runes ("AAAA\n"/"BBBB\n" etc are 5, use 6 to be
		// explicit); limit of 10 fits one line per chunk, not two.
		text := "aaaaa\nbbbbb\nccccc\n"
		got := splitMessage(text, 10)
		if len(got) != 3 {
			t.Fatalf("splitMessage() = %v (len %d), want 3 chunks", got, len(got))
		}
		for i, want := range []string{"aaaaa\n", "bbbbb\n", "ccccc\n"} {
			if got[i] != want {
				t.Errorf("chunk %d = %q, want %q", i, got[i], want)
			}
		}
		// Reassembling every chunk must reproduce the original text exactly —
		// splitting must never drop or duplicate content.
		if strings.Join(got, "") != text {
			t.Errorf("chunks don't reassemble to the original text")
		}
	})

	t.Run("packs multiple short lines into one chunk up to the limit", func(t *testing.T) {
		text := "ab\ncd\nef\ngh\n"
		got := splitMessage(text, 6)
		if len(got) != 2 || got[0] != "ab\ncd\n" || got[1] != "ef\ngh\n" {
			t.Errorf("splitMessage() = %v, want [\"ab\\ncd\\n\" \"ef\\ngh\\n\"]", got)
		}
	})

	t.Run("a single line longer than the limit is hard-split rather than dropped", func(t *testing.T) {
		text := "abcdefghij"
		got := splitMessage(text, 4)
		if strings.Join(got, "") != text {
			t.Errorf("splitMessage() chunks %v don't reassemble to %q", got, text)
		}
		for _, c := range got {
			if utf8.RuneCountInString(c) > 4 {
				t.Errorf("chunk %q exceeds limit 4", c)
			}
		}
	})
}

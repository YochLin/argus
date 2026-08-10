package bot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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

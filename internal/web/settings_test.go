package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newSettingsTestServer mirrors New's own /api/settings registration over the
// same fakes newTradeTestServer uses, and swaps exitForRestart out so a save
// doesn't take the test binary down with it. The returned channel receives
// once the (goroutine-launched) restart fires — see waitRestart/refuseRestart.
func newSettingsTestServer(t *testing.T, envPath string) (*Server, chan struct{}) {
	t.Helper()
	s := newTradeTestServer("secret", &fakeTrade{}, &fakeWatchlistDB{})
	s.envPath = envPath
	s.mux.HandleFunc("GET /api/settings", s.requireWritable(s.requireAuth(s.handleSettingsGet)))
	s.mux.HandleFunc("POST /api/settings", s.requireWritable(s.requireAuth(s.handleSettingsSave)))

	restarted := make(chan struct{}, 1)
	original := exitForRestart
	exitForRestart = func() { restarted <- struct{}{} }
	t.Cleanup(func() { exitForRestart = original })
	return s, restarted
}

func waitRestart(t *testing.T, restarted chan struct{}) {
	t.Helper()
	select {
	case <-restarted:
	case <-time.After(2 * time.Second):
		t.Error("a successful save must trigger the restart")
	}
}

// refuseRestart needs no timeout: a rejected save returns before ever
// launching the goroutine, so anything in the channel by now is a real bug.
func refuseRestart(t *testing.T, restarted chan struct{}) {
	t.Helper()
	select {
	case <-restarted:
		t.Error("a rejected save must not restart the process")
	default:
	}
}

func postSettings(t *testing.T, s *Server, cookie *http.Cookie, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(raw))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestSettingsGet_SecretsNeverLeaveTheProcess(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "super-secret-token")
	t.Setenv("TELEGRAM_CHAT_ID", "12345")
	t.Setenv("FINNHUB_API_KEY", "")
	t.Setenv("SEC_USER_AGENT", "argus me@example.com")

	s, _ := newSettingsTestServer(t, filepath.Join(t.TempDir(), ".env"))
	cookie := loginAndGetCookie(t, s, "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "super-secret-token") {
		t.Fatal("response body leaked a secret value")
	}

	var resp settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	byKey := make(map[string]settingResponse, len(resp.Settings))
	for _, item := range resp.Settings {
		byKey[item.Key] = item
	}
	if len(byKey) != len(settingKeys) {
		t.Errorf("got %d settings, want %d (one per whitelisted key)", len(byKey), len(settingKeys))
	}
	if got := byKey["TELEGRAM_BOT_TOKEN"]; got.Value != "" || !got.IsSet {
		t.Errorf("TELEGRAM_BOT_TOKEN = %+v, want empty value with isSet true", got)
	}
	if got := byKey["FINNHUB_API_KEY"]; got.IsSet {
		t.Errorf("FINNHUB_API_KEY isSet = true, want false for an empty env var")
	}
	if got := byKey["TELEGRAM_CHAT_ID"]; got.Value != "12345" {
		t.Errorf("TELEGRAM_CHAT_ID value = %q, want the plain value (not a secret)", got.Value)
	}
	if got := byKey["SEC_USER_AGENT"]; got.Value != "argus me@example.com" {
		t.Errorf("SEC_USER_AGENT value = %q, want the plain value", got.Value)
	}
	// The path variables are excluded on purpose (a bad one is unrecoverable
	// from this page) — a future edit that adds one should fail here.
	for _, banned := range []string{"DB_PATH", "LOG_PATH", "BACKUP_DIR", "PAPER_DB_PATH", "WEB_PASSWORD", "WEB_ADDR", "SJ_PRODUCTION"} {
		if _, ok := byKey[banned]; ok {
			t.Errorf("%s is exposed by /api/settings but must never be", banned)
		}
	}
}

func TestSettingsSave(t *testing.T) {
	t.Run("writes whitelisted keys and restarts", func(t *testing.T) {
		envPath := filepath.Join(t.TempDir(), ".env")
		os.WriteFile(envPath, []byte("# comment\nTELEGRAM_BOT_TOKEN=old\nDB_PATH=data/argus.db\n"), 0o600)
		s, restarted := newSettingsTestServer(t, envPath)
		cookie := loginAndGetCookie(t, s, "secret")

		rec := postSettings(t, s, cookie, map[string]string{
			"TELEGRAM_BOT_TOKEN": "new-token",
			"SEC_USER_AGENT":     "argus me@example.com",
			"DB_PATH":            "/etc/passwd", // not whitelisted: must be ignored
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		waitRestart(t, restarted)

		got, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read env: %v", err)
		}
		want := "# comment\nTELEGRAM_BOT_TOKEN=new-token\nDB_PATH=data/argus.db\nSEC_USER_AGENT=argus me@example.com\n"
		if string(got) != want {
			t.Errorf("env file =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("empty value keeps the current one", func(t *testing.T) {
		envPath := filepath.Join(t.TempDir(), ".env")
		os.WriteFile(envPath, []byte("FINNHUB_API_KEY=keep-me\n"), 0o600)
		s, _ := newSettingsTestServer(t, envPath)
		cookie := loginAndGetCookie(t, s, "secret")

		rec := postSettings(t, s, cookie, map[string]string{"FINNHUB_API_KEY": "", "FINMIND_TOKEN": "  "})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 when every field was left blank", rec.Code)
		}
		got, _ := os.ReadFile(envPath)
		if string(got) != "FINNHUB_API_KEY=keep-me\n" {
			t.Errorf("env file = %q, want it untouched", got)
		}
	})

	t.Run("rejects line breaks", func(t *testing.T) {
		envPath := filepath.Join(t.TempDir(), ".env")
		os.WriteFile(envPath, []byte("FINNHUB_API_KEY=keep-me\n"), 0o600)
		s, restarted := newSettingsTestServer(t, envPath)
		cookie := loginAndGetCookie(t, s, "secret")

		rec := postSettings(t, s, cookie, map[string]string{"FINMIND_TOKEN": "abc\nWEB_PASSWORD=pwned"})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		refuseRestart(t, restarted)
		got, _ := os.ReadFile(envPath)
		if strings.Contains(string(got), "WEB_PASSWORD") {
			t.Errorf("env file = %q, injected line was written", got)
		}
	})

	t.Run("rejects a non-numeric chat id", func(t *testing.T) {
		s, _ := newSettingsTestServer(t, filepath.Join(t.TempDir(), ".env"))
		cookie := loginAndGetCookie(t, s, "secret")
		rec := postSettings(t, s, cookie, map[string]string{"TELEGRAM_CHAT_ID": "@mychannel"})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("needs auth", func(t *testing.T) {
		s, _ := newSettingsTestServer(t, filepath.Join(t.TempDir(), ".env"))
		raw, _ := json.Marshal(map[string]string{"FINNHUB_API_KEY": "x"})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(raw)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 without a session cookie", rec.Code)
		}
	})
}

func TestPatchEnvFile(t *testing.T) {
	tests := []struct {
		name    string
		before  string
		updates map[string]string
		want    string
	}{{
		name:    "preserves comments, blank lines and order",
		before:  "# Telegram\nTELEGRAM_BOT_TOKEN=old\n\n# Data\nFINNHUB_API_KEY=finn\n",
		updates: map[string]string{"TELEGRAM_BOT_TOKEN": "new"},
		want:    "# Telegram\nTELEGRAM_BOT_TOKEN=new\n\n# Data\nFINNHUB_API_KEY=finn\n",
	}, {
		name:    "appends a key the file doesn't have",
		before:  "FINNHUB_API_KEY=finn\n",
		updates: map[string]string{"SHIOAJI_ADDR": "/root/.shioaji/server.sock"},
		want:    "FINNHUB_API_KEY=finn\nSHIOAJI_ADDR=/root/.shioaji/server.sock\n",
	}, {
		name:    "leaves a commented-out variable commented out",
		before:  "# TRAILING_STOP_ATR_MULT=3\n# SHIOAJI_ADDR=/old.sock\n",
		updates: map[string]string{"SHIOAJI_ADDR": "/new.sock"},
		want:    "# TRAILING_STOP_ATR_MULT=3\n# SHIOAJI_ADDR=/old.sock\nSHIOAJI_ADDR=/new.sock\n",
	}, {
		// SEC_USER_AGENT is "identifier email@host" — the one whitelisted
		// value with a space in it. It must survive unquoted; see
		// patchEnvFile's ponytail note.
		name:    "writes a value containing spaces unquoted",
		before:  "SEC_USER_AGENT=old ua@example.com\n",
		updates: map[string]string{"SEC_USER_AGENT": "argus real@example.com"},
		want:    "SEC_USER_AGENT=argus real@example.com\n",
	}, {
		name:    "creates a missing file",
		before:  "",
		updates: map[string]string{"FINMIND_TOKEN": "tok"},
		want:    "FINMIND_TOKEN=tok\n",
	}, {
		name:    "terminates a file that didn't end in a newline",
		before:  "FINNHUB_API_KEY=finn",
		updates: map[string]string{"FINMIND_TOKEN": "tok"},
		want:    "FINNHUB_API_KEY=finn\nFINMIND_TOKEN=tok\n",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if tt.before != "" {
				if err := os.WriteFile(path, []byte(tt.before), 0o600); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			if err := patchEnvFile(path, tt.updates); err != nil {
				t.Fatalf("patchEnvFile() error = %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}

	t.Run("leaves no temp files behind", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		if err := patchEnvFile(path, map[string]string{"FINMIND_TOKEN": "tok"}); err != nil {
			t.Fatalf("patchEnvFile() error = %v", err)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 {
			t.Errorf("directory holds %d files, want just .env", len(entries))
		}
	})
}

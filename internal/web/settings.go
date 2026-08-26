package web

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"argus/internal/logger"
)

// Phase 17 (docs/phase-17-web-settings.md): the dashboard can edit the
// connection/credential env vars and restart the process to apply them.
//
// The whitelist below is the only thing this endpoint can read or write, and
// the rule for what may join it is not "is it a credential" but "if it's
// filled in wrong, does the process still come up?" — saving here always ends
// in os.Exit(1) + a supervisor restart, so a value that breaks boot would
// lock the operator out of the very page that could fix it. That's why the
// path variables (DB_PATH/LOG_PATH/BACKUP_DIR/PAPER_DB_PATH) are excluded
// permanently: a bad path fails before web.New and turns into a crash loop.
// TELEGRAM_* qualify only because Phase 17 PR1 made them non-fatal.
//
// Also deliberately absent: WEB_ADDR/WEB_PASSWORD (this page is unreachable
// without them, so it can't bootstrap itself), JWT_SECRET/API_KEY (Phase 24
// Stage 4 — same family as WEB_PASSWORD: this deployment's own access
// credentials rather than a third-party service connection, and rotating
// either one silently logs out every app and script that already has a
// token), SJ_PRODUCTION (the paper vs. real-money switch for the Shioaji
// daemon — worth the friction of an SSH session), and the strategy-tuning
// numbers, which would pass the boot test but aren't part of "connect a
// service."
type settingKey struct {
	name string
	// group is an id, not a display string (see handlers.go's calendarEvent
	// for the same convention) — the frontend maps it to a section heading
	// from its own dictionary, so adding a key here is the only edit needed
	// to make it appear in the UI.
	group string
	// secret keys never leave the process: GET reports only whether they're
	// set. Not a strong boundary (a logged-in operator could read .env over
	// SSH anyway) — it stops a token from sitting in a browser tab, DOM dump
	// or screenshot.
	secret bool
}

var settingKeys = []settingKey{
	{name: "TELEGRAM_BOT_TOKEN", group: "telegram", secret: true},
	{name: "TELEGRAM_CHAT_ID", group: "telegram"},
	{name: "FINNHUB_API_KEY", group: "data", secret: true},
	{name: "FINMIND_TOKEN", group: "data", secret: true},
	{name: "SEC_USER_AGENT", group: "data"},
	{name: "SHIOAJI_ADDR", group: "sinopac"},
	// SJ_API_KEY/SJ_SEC_KEY are read by the `shioaji server` daemon, not by
	// Argus (deploy/shioaji.service points its EnvironmentFile at the same
	// .env this writes). Saving them therefore restarts Argus but NOT the
	// daemon — the frontend says so, since there's no way to tell from here.
	{name: "SJ_API_KEY", group: "sinopac", secret: true},
	{name: "SJ_SEC_KEY", group: "sinopac", secret: true},
	{name: "SINOPAC_SKIP_TICKERS", group: "sinopac"},
	{name: "SINOPAC_SYNC_LIVE", group: "sinopac"},
}

type settingResponse struct {
	Key    string `json:"key"`
	Group  string `json:"group"`
	Secret bool   `json:"secret"`
	// Value is always "" for a secret key — read IsSet instead. For the rest
	// it's the value this process actually loaded at boot, which is what the
	// operator needs to see to know whether an edit took effect.
	Value string `json:"value"`
	IsSet bool   `json:"isSet"`
}

type settingsResponse struct {
	Settings []settingResponse `json:"settings"`
}

// exitForRestart is the "apply" half of saving settings: there is no hot
// reload (every provider/bot in this codebase is wired once at boot), so the
// process ends itself and lets docker-compose's `restart: unless-stopped` or
// deploy/argus.service's `Restart=on-failure` bring it back. Exit code 1 is
// what makes on-failure fire. A var so settings_test.go can substitute it —
// nothing else should reassign it.
var exitForRestart = func() {
	// Long enough for the 200 to reach the browser, short enough that the
	// user doesn't start clicking again first.
	time.Sleep(500 * time.Millisecond)
	os.Exit(1)
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	out := make([]settingResponse, 0, len(settingKeys))
	for _, k := range settingKeys {
		v := os.Getenv(k.name)
		item := settingResponse{Key: k.name, Group: k.group, Secret: k.secret, IsSet: v != ""}
		if !k.secret {
			item.Value = v
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, settingsResponse{Settings: out})
}

// handleSettingsSave is POST /api/settings — a partial {KEY: value} map.
// An empty value means "leave this one alone" rather than "clear it", so the
// secret fields can render as blank inputs without a stray submit wiping a
// working token. The cost is that this endpoint can't clear a value at all;
// doing that means editing .env by hand, which is rare enough not to deserve
// its own UI.
func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if !decodeJSON(w, r, &body) {
		return
	}

	allowed := make(map[string]bool, len(settingKeys))
	for _, k := range settingKeys {
		allowed[k.name] = true
	}

	updates := make(map[string]string)
	for key, value := range body {
		if !allowed[key] {
			continue // silently ignored, same as an unknown JSON field
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		// A newline would end the KEY=value line and let the rest of the
		// string become further .env entries — the one input here that can
		// change more than the field it was typed into.
		if strings.ContainsAny(value, "\n\r") {
			writeError(w, http.StatusBadRequest, key+" must not contain line breaks")
			return
		}
		// Checked here rather than left for the next boot: an unparseable
		// chat id would restart the process into exactly the same
		// Telegram-disabled state, with the reason buried in the log.
		if key == "TELEGRAM_CHAT_ID" {
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				writeError(w, http.StatusBadRequest, "TELEGRAM_CHAT_ID must be a number")
				return
			}
		}
		updates[key] = value
	}
	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "no settings to update")
		return
	}

	if err := patchEnvFile(s.envPath, updates); err != nil {
		logger.Errorf("web: patch env file %s: %v", s.envPath, err)
		writeError(w, http.StatusInternalServerError, "failed to write .env")
		return
	}
	logger.Infof("web: settings saved (%s), restarting", strings.Join(sortedKeys(updates), ", "))
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
	go exitForRestart()
}

// sortedKeys lists updates' keys in settingKeys order, purely so the log line
// above is stable rather than map-random.
func sortedKeys(updates map[string]string) []string {
	var out []string
	for _, k := range settingKeys {
		if _, ok := updates[k.name]; ok {
			out = append(out, k.name)
		}
	}
	return out
}

// patchEnvFile rewrites only the KEY= lines named in updates, leaving every
// other line — comments included — exactly where it was. godotenv's
// Write/Marshal would have been one call, but they re-serialize the whole
// file from a map: the ordering goes alphabetical and every comment is lost,
// and .env/.env.example here carry paragraphs of explanation per variable.
// Keys not already present are appended at the end.
//
// ponytail: values are written unquoted and unescaped. Every whitelisted
// value is a token, socket path, comma-separated ticker list, "true", or the
// "name email@host" shape SEC wants — none can contain a `#`, which is the
// only character godotenv would misread (it starts a comment). A value with
// spaces is fine: godotenv and systemd's EnvironmentFile both read an
// unquoted value to end-of-line, which settings_test.go pins down. Adding a
// key whose value could contain `#` or a quote means adding escaping here.
func patchEnvFile(path string, updates map[string]string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	remaining := make(map[string]string, len(updates))
	for k, v := range updates {
		remaining[k] = v
	}

	lines := strings.Split(string(existing), "\n")
	// A newline-terminated file leaves a trailing "" here; drop it and
	// re-terminate at the end, so appended keys don't grow a blank line
	// between them and the previous content on every save.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	for i, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// No trimming: a commented-out "# FOO=bar" cuts to key "# FOO" and
		// correctly doesn't match, so re-enabling a variable the user
		// deliberately commented out is impossible by accident.
		if v, found := remaining[key]; found {
			lines[i] = key + "=" + v
			delete(remaining, key)
		}
	}
	for _, k := range settingKeys {
		if v, ok := remaining[k.name]; ok {
			lines = append(lines, k.name+"="+v)
		}
	}

	// Written via a temp file + rename so an interrupted write can't leave a
	// truncated .env behind: that file is the only copy of every credential
	// this deployment has, and it isn't in the SQLite backup.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".env-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once the rename below succeeds
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp's 0600 becomes the file's mode after the rename, replacing
	// whatever .env had — a tightening, and the right mode for a file full of
	// API keys.
	return os.Rename(tmp.Name(), path)
}

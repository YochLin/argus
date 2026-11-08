package web

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"argus/internal/logger"
)

// Phase 17 (docs/phase-17-web-settings.md): the dashboard can edit a fixed
// set of env vars and restart the process to apply them.
//
// The whitelist below is the only thing this endpoint can read or write, and
// the rule for what may join it is "if it's filled in wrong, does the process
// still come up?" — saving here always ends in os.Exit(1) + a supervisor
// restart, so a value that breaks boot would lock the operator out of the very
// page that could fix it. That's why the path variables (DB_PATH/LOG_PATH/
// BACKUP_DIR/PAPER_DB_PATH) are excluded permanently: a bad path fails before
// web.New and turns into a crash loop. TELEGRAM_* qualify only because Phase
// 17 PR1 made them non-fatal; the exit-discipline numbers and model names
// qualify because Load falls back to a default on a bad one — and
// validateSetting rejects a bad one before it's written anyway, since that
// fallback would otherwise be a silent no-op save.
//
// Also deliberately absent: WEB_ADDR/WEB_PASSWORD (this page is unreachable
// without them, so it can't bootstrap itself), JWT_SECRET/API_KEY (Phase 24
// Stage 4 — same family as WEB_PASSWORD: this deployment's own access
// credentials rather than a third-party service connection, and rotating
// either one silently logs out every app and script that already has a
// token), SJ_PRODUCTION (the paper vs. real-money switch for the Shioaji
// daemon — worth the friction of an SSH session), CLAUDE_ACP_COMMAND/
// ANTIGRAVITY_CLI_COMMAND (a command to execute) and the paper account's
// settings, which mean nothing until PAPER_DB_PATH (a path) is set by hand.
type settingKey struct {
	name string
	// group is an id, not a display string (see calendar.go's calendarEvent
	// for the same convention) — the frontend maps it to a section heading
	// from its own dictionary.
	group string
	kind  settingKind
	// max bounds a settingNumber, which is never negative: the % fields
	// can't exceed 100 and an ATR multiple past 20 is a typo.
	max float64
}

type settingKind string

const (
	// settingSecret values never leave the process: GET reports only whether
	// they're set. Not a strong boundary (a logged-in operator could read
	// .env over SSH anyway) — it stops a token from sitting in a browser
	// tab, DOM dump or screenshot.
	settingSecret settingKind = "secret"
	settingText   settingKind = "text"
	settingNumber settingKind = "number"
	settingInt    settingKind = "int"
	// settingToggle is "true"/"false"; Load only ever checks == "true".
	settingToggle settingKind = "toggle"
	// settingModel is one of claudeModels.
	settingModel settingKind = "model"
	// settingDiscount is TW_BROKER_FEE_DISCOUNT: above 0, at most 1.
	settingDiscount settingKind = "discount"
)

// claudeModels are the aliases the model fields accept. A full model ID
// already in .env still loads (Load takes any string); the page just can't
// write one.
var claudeModels = []string{"opus", "sonnet", "haiku"}

var settingKeys = []settingKey{
	{name: "TELEGRAM_BOT_TOKEN", group: "telegram", kind: settingSecret},
	{name: "TELEGRAM_CHAT_ID", group: "telegram", kind: settingInt},
	{name: "FINNHUB_API_KEY", group: "data", kind: settingSecret},
	{name: "FINMIND_TOKEN", group: "data", kind: settingSecret},
	{name: "SEC_USER_AGENT", group: "data", kind: settingText},
	{name: "SHIOAJI_ADDR", group: "sinopac", kind: settingText},
	// SJ_API_KEY/SJ_SEC_KEY are read by the `shioaji server` daemon, not by
	// Argus (deploy/shioaji.service points its EnvironmentFile at the same
	// .env this writes). Saving them therefore restarts Argus but NOT the
	// daemon — the frontend says so, since there's no way to tell from here.
	{name: "SJ_API_KEY", group: "sinopac", kind: settingSecret},
	{name: "SJ_SEC_KEY", group: "sinopac", kind: settingSecret},
	{name: "SINOPAC_SKIP_TICKERS", group: "sinopac", kind: settingText},
	{name: "SINOPAC_SYNC_LIVE", group: "sinopac", kind: settingToggle},
	{name: "STOP_LOSS_PCT", group: "risk", kind: settingNumber, max: 100},
	{name: "STOP_LOSS_PCT_TW", group: "risk", kind: settingNumber, max: 100},
	{name: "TRAILING_STOP_PCT", group: "risk", kind: settingNumber, max: 100},
	{name: "TRAILING_STOP_PCT_TW", group: "risk", kind: settingNumber, max: 100},
	{name: "TRAILING_STOP_ATR_MULT", group: "risk", kind: settingNumber, max: 20},
	{name: "RISK_PCT_PER_TRADE", group: "risk", kind: settingNumber, max: 100},
	{name: "RISK_HEAT_PCT", group: "risk", kind: settingNumber, max: 100},
	{name: "TW_BROKER_FEE_DISCOUNT", group: "risk", kind: settingDiscount},
	{name: "CLAUDE_RECOMMEND_MODEL", group: "ai", kind: settingModel},
	{name: "CLAUDE_CHECK_MODEL", group: "ai", kind: settingModel},
	{name: "CLAUDE_CHAT_MODEL", group: "ai", kind: settingModel},
	{name: "ANTIGRAVITY_ENABLED", group: "ai", kind: settingToggle},
	{name: "ANTIGRAVITY_MODEL", group: "ai", kind: settingText},
}

// secUserAgent is SEC_USER_AGENT's "name email@host" shape (see
// internal/data/sec.go: SEC's edge filter blocks a UA without an email).
var secUserAgent = regexp.MustCompile(`^\S+\s+\S+@\S+\.\S+$`)

// validateSetting checks a non-empty value against its key's kind, so a bad
// value is refused here instead of being written, restarting the process,
// and then silently falling back to the default in Load.
func validateSetting(k settingKey, value string) error {
	switch k.kind {
	case settingInt:
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("%s must be a whole number", k.name)
		}
	case settingNumber:
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || n < 0 || n > k.max {
			return fmt.Errorf("%s must be a number from 0 to %g", k.name, k.max)
		}
	case settingDiscount:
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || n <= 0 || n > 1 {
			return fmt.Errorf("%s must be above 0 and at most 1", k.name)
		}
	case settingToggle:
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be true or false", k.name)
		}
	case settingModel:
		if !slices.Contains(claudeModels, value) {
			return fmt.Errorf("%s must be one of %s", k.name, strings.Join(claudeModels, ", "))
		}
	}
	if k.name == "SEC_USER_AGENT" && !secUserAgent.MatchString(value) {
		return errors.New(`SEC_USER_AGENT must look like "name email@host"`)
	}
	return nil
}

type settingResponse struct {
	Key   string      `json:"key"`
	Group string      `json:"group"`
	Kind  settingKind `json:"kind"`
	// Value is always "" for a secret key — read IsSet instead. For the rest
	// it's the value this process actually loaded at boot, which is what the
	// operator needs to see to know whether an edit took effect.
	Value string `json:"value"`
	IsSet bool   `json:"isSet"`
	// Default is what Load uses while the variable is unset or cleared;
	// empty when it has none.
	Default string `json:"default,omitempty"`
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
		item := settingResponse{Key: k.name, Group: k.group, Kind: k.kind, IsSet: v != "", Default: s.settingDefaults[k.name]}
		if k.kind != settingSecret {
			item.Value = v
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, settingsResponse{Settings: out})
}

// handleSettingsSave is POST /api/settings — a partial {KEY: value} map.
// An empty value means "leave this one alone", so the secret fields can
// render as blank inputs without a stray submit wiping a working token; null
// means "clear it", written as KEY= so the next boot falls back to the
// default (or, for a connection, to that service being off).
func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]*string
	if !decodeJSON(w, r, &body) {
		return
	}

	allowed := make(map[string]settingKey, len(settingKeys))
	for _, k := range settingKeys {
		allowed[k.name] = k
	}

	updates := make(map[string]string)
	for key, raw := range body {
		k, ok := allowed[key]
		if !ok {
			continue // silently ignored, same as an unknown JSON field
		}
		if raw == nil {
			updates[key] = ""
			continue
		}
		value := strings.TrimSpace(*raw)
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
		if err := validateSetting(k, value); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
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
// value is a token, socket path, comma-separated ticker list, number,
// "true"/"false", model name, or the "name email@host" shape SEC wants —
// none can contain a `#`, which is the
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

package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// authCookieName/authCookieTTL/authSalt back Phase 10's single-user write
// auth (see docs/phase-10-web-trade-input.md §4.1): a stateless, HMAC-signed
// session cookie rather than a server-side session table, so a bot restart
// never logs the user out. authSalt is a fixed, non-secret constant folded
// into the HMAC key derivation purely to keep that key distinct from the
// raw WEB_PASSWORD value — it isn't a secret of its own.
const (
	authCookieName = "argus_web_auth"
	authCookieTTL  = 30 * 24 * time.Hour
	authSalt       = "argus-web-auth-v1"
)

func authKey(password string) []byte {
	sum := sha256.Sum256([]byte(password + authSalt))
	return sum[:]
}

// signAuthToken produces a "<expiresUnix>.<hmac-hex>" cookie value —
// self-verifying, no server-side state to look up.
func signAuthToken(password string) string {
	payload := strconv.FormatInt(time.Now().Add(authCookieTTL).Unix(), 10)
	mac := hmac.New(sha256.New, authKey(password))
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func verifyAuthToken(password, token string) bool {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	expires, err := strconv.ParseInt(payload, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	mac := hmac.New(sha256.New, authKey(password))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) == 1
}

// handleLogin is POST /api/login — WEB_PASSWORD presence already gates
// whether this route is even registered (see New), so a request reaching
// here always means write mode is on. Password comparison is
// constant-time; a wrong password gets a deliberate 1s delay instead of any
// lockout bookkeeping — single-user, Tailscale-internal deployment, per
// docs/phase-10-web-trade-input.md §4.1.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.password)) != 1 {
		time.Sleep(time.Second)
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    signAuthToken(s.password),
		Path:     "/",
		MaxAge:   int(authCookieTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// requireWritable 404s a write endpoint entirely when WEB_PASSWORD isn't
// configured — see Config.Password's doc comment. Wrapping every write
// handler with this (rather than conditionally registering routes in New)
// is what makes an unconfigured deployment's write endpoints a real 404,
// not spaHandler's catch-all 200 SPA fallback for an unregistered pattern.
func (s *Server) requireWritable(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.password == "" {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

// requireTrade rejects the write endpoints that go through TradeExecutor
// when Telegram isn't configured (Phase 17 PR1: main.go leaves Trade nil in
// that case, and the dashboard is otherwise fully functional). 409 rather
// than 404/500: the route exists and the request is well-formed, it's the
// server's own configuration that isn't ready — and the frontend needs to
// tell this apart from a plain rejected trade to show the right message.
// Only trade/stop/buy-alert-add need this; watchlist and thesis writes go
// straight to the DB.
func (s *Server) requireTrade(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.trade == nil {
			writeError(w, http.StatusConflict, "Telegram is not configured yet")
			return
		}
		next(w, r)
	}
}

// requireAuth gates a write endpoint on a valid session cookie — the only
// auth check in this package, per docs/phase-10-web-trade-input.md §4.1's
// "auth only locks writes" decision; every read-only /api/* route is
// unwrapped.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookieName)
		if err != nil || !verifyAuthToken(s.password, cookie.Value) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

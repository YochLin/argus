package web

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Phase 24 Stage 4 Step 4.1 (docs/architecture/server-refactor-plan.md): JWT
// + API-key auth for the /api/v1 surface, which exists for a future mobile
// app and for scripts.
//
// This deliberately does NOT replace or absorb auth.go's Phase 10 cookie
// auth. That one guards the dashboard's write endpoints from a browser that
// has a session; this one guards a token-bearing client that has no cookie
// jar and no login page. Folding them together would mean either giving the
// SPA a token to store in JS (worse than an HttpOnly cookie) or giving a
// shell script a cookie (awkward and no better). They share the one thing
// worth sharing — WEB_PASSWORD, the single credential this single-user
// system has — and nothing else.
//
// There is no user table and no `sub` beyond a constant: Argus is single-user
// by design (AGENTS.md), and multi-tenancy is Phase 22's separate question.
const (
	// apiAccessTTL is short because a refresh token exists; apiRefreshTTL
	// matches the dashboard cookie's 30 days, since both represent the same
	// thing — "this operator's device stays logged in".
	apiAccessTTL  = 30 * time.Minute
	apiRefreshTTL = 30 * 24 * time.Hour

	apiTokenSubject = "argus"
	apiTokenAccess  = "access"
	apiTokenRefresh = "refresh"
)

// apiClaims is the JWT payload: registered claims plus a token-type tag, so
// a refresh token can't be replayed as a bearer token on a data endpoint
// (the classic mistake when both are signed with the same key).
type apiClaims struct {
	Type string `json:"typ"`
	jwt.RegisteredClaims
}

func (s *Server) signAPIToken(typ string, ttl time.Duration) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, apiClaims{
		Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   apiTokenSubject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}).SignedString([]byte(s.jwtSecret))
}

// parseAPIToken verifies signature, expiry and token type. jwt.WithValidMethods
// pins HS256: without it a token could name "none" (or an RS256 key confusion)
// and pass verification.
func (s *Server) parseAPIToken(token, wantType string) error {
	var claims apiClaims
	_, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return []byte(s.jwtSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithSubject(apiTokenSubject))
	if err != nil {
		return err
	}
	if claims.Type != wantType {
		return errors.New("wrong token type")
	}
	return nil
}

type apiTokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	// ExpiresIn is the access token's remaining lifetime in seconds, so a
	// client can schedule its refresh without decoding the JWT.
	ExpiresIn int `json:"expiresIn"`
}

// handleAPILogin is POST /api/v1/auth/login — {"password": "..."} checked
// against the same WEB_PASSWORD the dashboard login uses, with the same
// constant-time compare and same deliberate 1s delay on failure (single-user,
// private-network deployment: no lockout bookkeeping, see
// docs/phase-10-web-trade-input.md §4.1).
func (s *Server) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.password)) != 1 {
		time.Sleep(time.Second)
		writeAPIError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	s.writeTokenPair(w)
}

// handleAPIRefresh is POST /api/v1/auth/refresh — {"refreshToken": "..."}.
// It mints a fresh pair rather than only a new access token: the refresh
// token is the thing a long-lived app keeps, and sliding its expiry on use is
// what stops an actively-used app from being logged out every 30 days.
func (s *Server) handleAPIRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return
	}
	if err := s.parseAPIToken(body.RefreshToken, apiTokenRefresh); err != nil {
		writeAPIError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	s.writeTokenPair(w)
}

func (s *Server) writeTokenPair(w http.ResponseWriter) {
	access, err := s.signAPIToken(apiTokenAccess, apiAccessTTL)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to sign token")
		return
	}
	refresh, err := s.signAPIToken(apiTokenRefresh, apiRefreshTTL)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to sign token")
		return
	}
	writeAPIOK(w, apiTokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: int(apiAccessTTL.Seconds())})
}

// requireAPIAuth accepts either an `Authorization: Bearer <jwt>` access token
// or an `X-API-Key: <key>` header. The API key is for personal scripts and
// cron jobs, which have nowhere sensible to run a login/refresh cycle; it's
// only accepted when API_KEY is configured, so an unset one can never be
// matched by an empty header.
func (s *Server) requireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if key := r.Header.Get("X-API-Key"); key != "" && s.apiKey != "" &&
			subtle.ConstantTimeCompare([]byte(key), []byte(s.apiKey)) == 1 {
			next(w, r)
			return
		}
		bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || s.parseAPIToken(strings.TrimSpace(bearer), apiTokenAccess) != nil {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

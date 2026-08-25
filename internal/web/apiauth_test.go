package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testAPIServer() *Server {
	return New(Config{Password: "hunter2", JWTSecret: "test-secret", APIKey: "script-key"})
}

func postJSON(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) apiResponse {
	t.Helper()
	var env apiResponse
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Timestamp == 0 {
		t.Error("envelope has no timestamp")
	}
	return env
}

func loginTokens(t *testing.T, s *Server) apiTokenPair {
	t.Helper()
	rec := postJSON(t, s, "/api/v1/auth/login", `{"password":"hunter2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", rec.Code)
	}
	env := decodeEnvelope(t, rec)
	raw, _ := json.Marshal(env.Data)
	var pair apiTokenPair
	if err := json.Unmarshal(raw, &pair); err != nil {
		t.Fatalf("decode token pair: %v", err)
	}
	return pair
}

func TestAPILoginRejectsWrongPassword(t *testing.T) {
	s := testAPIServer()
	rec := postJSON(t, s, "/api/v1/auth/login", `{"password":"nope"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login with wrong password = %d, want 401", rec.Code)
	}
	if env := decodeEnvelope(t, rec); env.Success || env.Error == "" {
		t.Errorf("failure envelope = %+v, want success=false with an error", env)
	}
}

// TestAPIAuthAcceptedCredentials is the whole auth matrix in one table: what
// requireAPIAuth lets through and what it doesn't. The refresh-token row is
// the important one — both tokens are signed with the same key, so only the
// typ claim stops a refresh token from being used as a bearer.
func TestAPIAuthAcceptedCredentials(t *testing.T) {
	s := testAPIServer()
	pair := loginTokens(t, s)

	tests := []struct {
		name   string
		header [2]string
		want   int
	}{
		{"access token", [2]string{"Authorization", "Bearer " + pair.AccessToken}, http.StatusOK},
		{"api key", [2]string{"X-API-Key", "script-key"}, http.StatusOK},
		{"refresh token as bearer", [2]string{"Authorization", "Bearer " + pair.RefreshToken}, http.StatusUnauthorized},
		{"wrong api key", [2]string{"X-API-Key", "guess"}, http.StatusUnauthorized},
		{"empty api key", [2]string{"X-API-Key", ""}, http.StatusUnauthorized},
		{"garbage bearer", [2]string{"Authorization", "Bearer not.a.jwt"}, http.StatusUnauthorized},
		{"no credentials", [2]string{"X-Nothing", "x"}, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			req.Header.Set(tt.header[0], tt.header[1])
			rec := httptest.NewRecorder()
			s.mux.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("GET /api/v1/auth/me = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestAPIRefreshRejectsAccessToken(t *testing.T) {
	s := testAPIServer()
	pair := loginTokens(t, s)

	rec := postJSON(t, s, "/api/v1/auth/refresh", `{"refreshToken":"`+pair.RefreshToken+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh with a refresh token = %d, want 200", rec.Code)
	}
	// The mirror image of the bearer check: an access token must not be
	// spendable as a refresh token either, or its short TTL would mean
	// nothing.
	rec = postJSON(t, s, "/api/v1/auth/refresh", `{"refreshToken":"`+pair.AccessToken+`"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("refresh with an access token = %d, want 401", rec.Code)
	}
}

// TestAPITokenExpiry covers the one failure mode a hand-rolled check would
// have gotten wrong: an expired but otherwise perfectly signed token.
func TestAPITokenExpiry(t *testing.T) {
	s := testAPIServer()
	expired, err := s.signAPIToken(apiTokenAccess, -time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := s.parseAPIToken(expired, apiTokenAccess); err == nil {
		t.Error("expired token accepted")
	}
}

// TestAPITokenRejectsForeignSecret pins that the signature is actually
// checked against this server's key.
func TestAPITokenRejectsForeignSecret(t *testing.T) {
	other := New(Config{Password: "hunter2", JWTSecret: "different-secret"})
	token, err := other.signAPIToken(apiTokenAccess, time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := testAPIServer().parseAPIToken(token, apiTokenAccess); err == nil {
		t.Error("token signed with another key accepted")
	}
}

// TestAPIV1UnregisteredWithoutSecret: with no JWT_SECRET the routes must not
// exist at all, which spaHandler's catch-all turns into a 200 SPA shell —
// never an unauthenticated JSON response.
func TestAPIV1UnregisteredWithoutSecret(t *testing.T) {
	s := New(Config{Password: "hunter2"})
	rec := postJSON(t, s, "/api/v1/auth/login", `{"password":"hunter2"}`)
	if strings.Contains(rec.Body.String(), "accessToken") {
		t.Errorf("login answered with a token while JWT_SECRET is unset: %s", rec.Body.String())
	}
}

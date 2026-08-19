package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
)

func newNewsSourceTestServer(password string, fake *fakeDB) *Server {
	s := &Server{db: fake, newsSourceDB: fake, lang: i18n.EN, password: password, llmAudit: true}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("GET /api/news-sources/blocked", s.handleBlockedNewsSources)
	s.mux.HandleFunc("POST /api/news-sources/block", s.requireWritable(s.requireAuth(s.handleNewsSourceBlock)))
	s.mux.HandleFunc("POST /api/news-sources/unblock", s.requireWritable(s.requireAuth(s.handleNewsSourceUnblock)))
	return s
}

func TestHandleBlockedNewsSources(t *testing.T) {
	fake := &fakeDB{blockedSources: []db.BlockedNewsSource{{Source: "spam.example.com", CreatedAt: "2026-08-01"}}}
	s := newNewsSourceTestServer("secret", fake)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/news-sources/blocked", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var got blockedSourcesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 1 || got.Sources[0].Source != "spam.example.com" {
		t.Errorf("Sources = %+v, want one spam.example.com entry", got.Sources)
	}
}

func TestHandleNewsSourceBlockUnblock(t *testing.T) {
	fake := &fakeDB{}
	s := newNewsSourceTestServer("secret", fake)

	t.Run("block requires auth", func(t *testing.T) {
		body, _ := json.Marshal(newsSourceRequest{Source: "spam.example.com"})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/news-sources/block", bytes.NewReader(body)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	cookie := loginAndGetCookie(t, s, "secret")

	t.Run("block", func(t *testing.T) {
		body, _ := json.Marshal(newsSourceRequest{Source: "spam.example.com"})
		req := httptest.NewRequest(http.MethodPost, "/api/news-sources/block", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		if len(fake.blockedSources) != 1 || fake.blockedSources[0].Source != "spam.example.com" {
			t.Errorf("blockedSources = %+v, want one spam.example.com entry", fake.blockedSources)
		}
	})

	t.Run("block rejects empty source", func(t *testing.T) {
		body, _ := json.Marshal(newsSourceRequest{Source: "  "})
		req := httptest.NewRequest(http.MethodPost, "/api/news-sources/block", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unblock", func(t *testing.T) {
		body, _ := json.Marshal(newsSourceRequest{Source: "spam.example.com"})
		req := httptest.NewRequest(http.MethodPost, "/api/news-sources/unblock", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		if len(fake.blockedSources) != 0 {
			t.Errorf("blockedSources = %+v, want empty after unblock", fake.blockedSources)
		}
	})
}

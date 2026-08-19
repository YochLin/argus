package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
)

func newLLMAuditTestServer(fake *fakeDB) *Server {
	s := &Server{db: fake, newsSourceDB: fake, llmAudit: true}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /api/llm-runs", s.handleLLMRuns)
	s.mux.HandleFunc("GET /api/llm-runs/{id}", s.handleLLMRunDetail)
	s.mux.HandleFunc("GET /api/news-sources/blocked", s.handleBlockedNewsSources)
	return s
}

func TestHandleLLMRuns(t *testing.T) {
	fake := &fakeDB{llmRuns: []db.LLMRun{
		{ID: 2, Kind: "daily_report", Market: "tw", Model: "sonnet", LatencyMs: 3100, WatchlistCount: 1, CandidateCount: 3, NewsCount: 4},
		{ID: 1, Kind: "recommend", Market: "us", Model: "opus", LatencyMs: 4200, WatchlistCount: 2, CandidateCount: 5, NewsCount: 8},
	}}
	s := newLLMAuditTestServer(fake)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-runs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got llmRunsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Runs) != 2 || got.Runs[0].ID != 2 || got.Runs[1].ID != 1 {
		t.Errorf("Runs = %+v, want newest-first [2, 1]", got.Runs)
	}
}

func TestHandleLLMRunDetail(t *testing.T) {
	fake := &fakeDB{llmRunByID: map[int64]db.LLMRunDetail{
		1: {
			LLMRun:    db.LLMRun{ID: 1, Kind: "recommend", Market: "us", Model: "opus"},
			InputJSON: `{"watchlist":["AAPL"]}`,
			OutputRaw: "[TICKER: AAPL]\nAction: BUY\nReason: strong earnings\n",
		},
	}}
	s := newLLMAuditTestServer(fake)

	t.Run("found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-runs/1", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		var got llmRunDetailResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.OutputRaw == "" || string(got.Input) != `{"watchlist":["AAPL"]}` {
			t.Errorf("detail = %+v, want input/output round-tripped", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-runs/999", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/llm-runs/not-a-number", nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

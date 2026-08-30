package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"argus/internal/market"
)

// fakeRecommender blocks inside RunRecommend until release is closed, which
// is what lets the test hold a run "in flight" while it fires the second
// request at the endpoint.
type fakeRecommender struct {
	started chan struct{}
	release chan struct{}
	market  market.MarketID
	ctxErr  error
}

func (f *fakeRecommender) RunRecommend(ctx context.Context, m market.MarketID) {
	f.market = m
	// Recorded rather than asserted here: a t.Error from this goroutine
	// could outlive the test. Read back after <-started, which the channel
	// send below happens-before.
	f.ctxErr = ctx.Err()
	f.started <- struct{}{}
	<-f.release
}

func authedPost(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("X-API-Key", "script-key")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

// TestAPIRecommendationsTriggerOneAtATime covers the three things this
// endpoint promises beyond "call the seam": it answers before the run
// finishes, it refuses a concurrent second run, and it lets go of that gate
// once the run ends. The middle one is the one worth a test — a broken gate
// means a double-tap burns a second LLM call and races the first run's
// writes into the recommendations table.
func TestAPIRecommendationsTriggerOneAtATime(t *testing.T) {
	rec := &fakeRecommender{started: make(chan struct{}, 1), release: make(chan struct{})}
	s := New(Config{
		Password:  "hunter2",
		JWTSecret: "test-secret-at-least-32-characters-long",
		APIKey:    "script-key",
		Recommend: rec,
	})

	first := authedPost(t, s, "/api/v1/recommendations/trigger?market=tw")
	if first.Code != http.StatusAccepted {
		t.Fatalf("first trigger = %d, want 202", first.Code)
	}
	<-rec.started
	if rec.market != market.TW {
		t.Errorf("market = %q, want tw", rec.market)
	}
	// The handler has already returned by now, so a plain r.Context() would
	// be cancelled and the run would abort at its first ctx check.
	if rec.ctxErr != nil {
		t.Errorf("run ctx = %v, want live", rec.ctxErr)
	}

	if second := authedPost(t, s, "/api/v1/recommendations/trigger"); second.Code != http.StatusConflict {
		t.Fatalf("concurrent trigger = %d, want 409", second.Code)
	}

	close(rec.release)
	// The gate is cleared by the run goroutine's defer, so poll rather than
	// assume it has been scheduled by now.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if authedPost(t, s, "/api/v1/recommendations/trigger").Code == http.StatusAccepted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("trigger still 409 after the run finished: the gate never released")
		}
	}
}

// TestAPIRecommendationsTriggerWithoutRunner: a deployment with no
// Recommender wired must say so with the same 409 the trade routes use, not
// panic on a nil interface call.
func TestAPIRecommendationsTriggerWithoutRunner(t *testing.T) {
	if got := authedPost(t, testAPIServer(), "/api/v1/recommendations/trigger").Code; got != http.StatusConflict {
		t.Errorf("trigger without a runner = %d, want 409", got)
	}
}

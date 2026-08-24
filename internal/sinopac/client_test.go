package sinopac

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestDoRetriesSessionNotEstablished exercises the one non-obvious branch in
// do(): a "SessionNotEstablished" error retries with backoff instead of
// failing immediately (live-verified against the real daemon — its Solace
// session drops for 1-2 minutes after TW market close and self-heals).
func TestDoRetriesSessionNotEstablished(t *testing.T) {
	orig := sessionRetryBackoff
	sessionRetryBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { sessionRetryBackoff = orig }()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(apiError{Message: "Shioaji error ... SubCode(SessionNotEstablished) ..."})
			return
		}
		json.NewEncoder(w).Encode([]Snapshot{{Code: "2330"}})
	}))
	defer srv.Close()

	c := New(srv.Listener.Addr().String())
	var out []Snapshot
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if len(out) != 1 || out[0].Code != "2330" {
		t.Fatalf("unexpected result: %+v", out)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 retries), got %d", calls)
	}
}

// TestDoNonSessionErrorNoRetry ensures an ordinary API error (e.g. bad
// request/permission) fails immediately rather than burning the full
// backoff — SessionNotEstablished is the one retryable case.
func TestDoNonSessionErrorNoRetry(t *testing.T) {
	orig := sessionRetryBackoff
	sessionRetryBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { sessionRetryBackoff = orig }()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apiError{Message: "Token doesn't have permission"})
	}))
	defer srv.Close()

	c := New(srv.Listener.Addr().String())
	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls)
	}
}

// TestSessionRetryBudgetCoversOutage pins the one thing that made a real
// session drop surface as "永豐同步失敗": the backoff steps must add up to
// the daemon's observed 1-2 minute self-heal window, not a few seconds.
func TestSessionRetryBudgetCoversOutage(t *testing.T) {
	var total time.Duration
	for _, d := range sessionRetryBackoff {
		total += d
	}
	if total < 90*time.Second {
		t.Fatalf("retry budget %v is shorter than the daemon's self-heal window", total)
	}
}

// TestDoSessionRetryHonorsContext ensures a cancelled context aborts mid-
// backoff instead of sleeping out the (now ~2 minute) retry budget.
func TestDoSessionRetryHonorsContext(t *testing.T) {
	orig := sessionRetryBackoff
	sessionRetryBackoff = []time.Duration{time.Hour}
	defer func() { sessionRetryBackoff = orig }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(apiError{Message: "SubCode(SessionNotEstablished)"})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- New(srv.Listener.Addr().String()).do(ctx, http.MethodGet, "/x", nil, nil) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("do did not abort on context cancellation")
	}
}

// TestRegulatoryColumnParsing covers the parallel-arrays-to-map conversion
// (the daemon returns a DataFrame.to_dict("list") shape, not a row array —
// see Client's doc comment) for both punish (has end_date) and notice (has
// reason) response shapes.
func TestRegulatoryColumnParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/data/regulatory_punish":
			json.NewEncoder(w).Encode(map[string]any{
				"code":       []string{"2330", "2317"},
				"start_date": []string{"2026-08-10", "2026-08-11"},
				"end_date":   []string{"2026-08-14", "2026-08-17"},
			})
		case "/api/v1/data/regulatory_notice":
			json.NewEncoder(w).Encode(map[string]any{
				"code":   []string{"2454"},
				"reason": []string{"最近六個營業日累積收盤價漲幅過大"},
			})
		}
	}))
	defer srv.Close()
	c := New(srv.Listener.Addr().String())

	punish, err := c.RegulatoryPunish(context.Background())
	if err != nil {
		t.Fatalf("RegulatoryPunish: %v", err)
	}
	if len(punish) != 2 || punish["2330"] == "" || punish["2317"] == "" {
		t.Fatalf("unexpected punish map: %+v", punish)
	}

	notice, err := c.RegulatoryNotice(context.Background())
	if err != nil {
		t.Fatalf("RegulatoryNotice: %v", err)
	}
	if notice["2454"] != "最近六個營業日累積收盤價漲幅過大" {
		t.Fatalf("unexpected notice map: %+v", notice)
	}
}

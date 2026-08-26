package web

import (
	"os"
	"strings"
	"testing"

	"argus/internal/notification"
)

// TestOpenAPICoversEveryRoute is the check that keeps docs/openapi.yaml from
// rotting: a route added to apiV1Handlers without a matching entry in the
// spec fails here. It matches on the path key and the method verb rather
// than parsing YAML — the spec's structure is stable and hand-written, and a
// parser dependency for a substring search would be the more fragile choice.
func TestOpenAPICoversEveryRoute(t *testing.T) {
	spec, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	text := string(spec)

	s := New(Config{Password: "hunter2", JWTSecret: "test-secret", Events: notification.NewWebSocketHub()})
	for pattern := range s.apiV1Handlers() {
		method, path, ok := strings.Cut(pattern, " ")
		if !ok {
			t.Fatalf("unparseable route pattern %q", pattern)
		}
		// Paths are top-level keys under `paths:`, so they sit at exactly two
		// spaces of indentation — enough to distinguish "/api/v1/watchlist:"
		// from a substring of "/api/v1/watchlist/remove:".
		key := "\n  " + path + ":"
		i := strings.Index(text, key)
		if i < 0 {
			t.Errorf("%s is registered but missing from docs/openapi.yaml", path)
			continue
		}
		// Cut the path's own block at the next top-level path key, so a
		// neighbouring entry's verb can't satisfy the check below.
		block := text[i+len(key):]
		if end := strings.Index(block, "\n  /"); end >= 0 {
			block = block[:end]
		}
		if !strings.Contains(block, "\n    "+strings.ToLower(method)+":") {
			t.Errorf("%s %s is registered but the spec has no %s operation for it", method, path, strings.ToLower(method))
		}
	}
}

// TestOpenAPIDescribesNoPhantomRoutes is the other direction: a path
// documented in the spec that no longer exists sends a generated client at a
// 404.
func TestOpenAPIDescribesNoPhantomRoutes(t *testing.T) {
	spec, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	s := New(Config{Password: "hunter2", JWTSecret: "test-secret", Events: notification.NewWebSocketHub()})
	registered := make(map[string]bool)
	for pattern := range s.apiV1Handlers() {
		_, path, _ := strings.Cut(pattern, " ")
		registered[path] = true
	}
	for _, line := range strings.Split(string(spec), "\n") {
		if !strings.HasPrefix(line, "  /api/v1") || !strings.HasSuffix(line, ":") {
			continue
		}
		path := strings.TrimSuffix(strings.TrimSpace(line), ":")
		if !registered[path] {
			t.Errorf("docs/openapi.yaml documents %s, which no route serves", path)
		}
	}
}

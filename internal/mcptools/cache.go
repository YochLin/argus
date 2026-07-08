package mcptools

import (
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ttlCache is a minimal in-memory, per-key TTL cache of tool results. It
// exists so a chat model re-asking about the same ticker seconds apart
// (or restating a question, which the model does more often than a
// deliberate bot-side prefetch loop ever would) doesn't cost a fresh
// provider round-trip every time — see server.go's per-tool TTL constants
// for how long each tool's data is considered fresh.
type ttlCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	result  *mcp.CallToolResult
	expires time.Time
}

func newTTLCache() *ttlCache {
	return &ttlCache{entries: make(map[string]cacheEntry)}
}

func (c *ttlCache) get(key string) (*mcp.CallToolResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		delete(c.entries, key) // expired; drop it so the map doesn't grow unboundedly over a long chat session
		return nil, false
	}
	return e.result, true
}

func (c *ttlCache) set(key string, result *mcp.CallToolResult, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{result: result, expires: time.Now().Add(ttl)}
}

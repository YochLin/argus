package bot

import (
	"sync"
	"time"
)

// ttlCache is a minimal in-memory, per-key TTL cache, same shape as
// mcptools' cache (not shared across packages — mcptools' variant is typed
// to *mcp.CallToolResult and process-scoped to one chat session; this one
// backs fetchStockData's slow-moving fields across the bot's whole
// long-running process, see slowDataCacheTTL). Not persisted, so a bot
// restart just costs one full refetch — acceptable for a personal, mostly
// long-running process.
type ttlCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	value   any
	expires time.Time
}

func newTTLCache() *ttlCache {
	return &ttlCache{entries: make(map[string]cacheEntry)}
}

func (c *ttlCache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		delete(c.entries, key)
		return nil, false
	}
	return e.value, true
}

func (c *ttlCache) set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{value: value, expires: time.Now().Add(ttl)}
}

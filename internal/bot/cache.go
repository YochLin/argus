package bot

import (
	"sync"
	"time"
)

// ttlCache is a minimal in-memory, per-key TTL cache, same shape as
// mcptools' cache (not shared across packages — mcptools' variant is
// process-scoped to one chat session; this one backs fetchStockData's
// slow-moving fields across the bot's whole long-running process, see
// slowDataCacheTTL). Not persisted, so a bot restart just costs one full
// refetch — acceptable for a personal, mostly long-running process.
type ttlCache[T any] struct {
	mu      sync.Mutex
	entries map[string]cacheEntry[T]
}

type cacheEntry[T any] struct {
	value   T
	expires time.Time
}

func newTTLCache[T any]() *ttlCache[T] {
	return &ttlCache[T]{entries: make(map[string]cacheEntry[T])}
}

func (c *ttlCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		var zero T
		return zero, false
	}
	if time.Now().After(e.expires) {
		delete(c.entries, key)
		var zero T
		return zero, false
	}
	return e.value, true
}

func (c *ttlCache[T]) set(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry[T]{value: value, expires: time.Now().Add(ttl)}
}

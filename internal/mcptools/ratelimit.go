package mcptools

import (
	"context"
	"sync"
	"time"
)

// tokenBucket is a minimal token-bucket rate limiter guarding this
// process's outbound provider calls (see server.go's rateLimiterCapacity/
// rateLimiterRefillPerSecond for why: Finnhub's free-tier 60 req/min limit,
// now reachable at whatever cadence the chat model decides to call tools,
// not just the bot's own controlled prefetch paths). Deliberately hand-
// rolled instead of pulling in golang.org/x/time/rate — this package's
// dependency graph is kept narrow on purpose (see server.go's doc comment),
// and the full behavior needed here is ~20 lines.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	last       time.Time
}

func newTokenBucket(capacity, refillRatePerSecond float64) *tokenBucket {
	return &tokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRatePerSecond,
		last:       time.Now(),
	}
}

// tryAcquire refills based on elapsed wall-clock time and then takes one
// token if available, returning false (consuming nothing) otherwise.
func (b *tokenBucket) tryAcquire() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(b.capacity, b.tokens+elapsed*b.refillRate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Wait blocks until a token is available or ctx is cancelled. Polls at a
// short fixed interval rather than computing an exact wake time — simpler,
// and fine at this scale (sub-second waits, single-digit requests/minute).
func (b *tokenBucket) Wait(ctx context.Context) error {
	if b.tryAcquire() {
		return nil
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if b.tryAcquire() {
				return nil
			}
		}
	}
}

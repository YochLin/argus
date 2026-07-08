package mcptools

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucketAllowsBurstUpToCapacity(t *testing.T) {
	b := newTokenBucket(3, 0.001) // capacity 3, effectively no refill during this test
	for i := 0; i < 3; i++ {
		if !b.tryAcquire() {
			t.Fatalf("tryAcquire() #%d should succeed within burst capacity", i+1)
		}
	}
	if b.tryAcquire() {
		t.Error("tryAcquire() beyond burst capacity with no refill should fail")
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	b := newTokenBucket(1, 100) // capacity 1, refills a token every 10ms
	if !b.tryAcquire() {
		t.Fatal("first tryAcquire() should succeed")
	}
	if b.tryAcquire() {
		t.Fatal("second tryAcquire() immediately after should fail (bucket empty)")
	}

	time.Sleep(30 * time.Millisecond)
	if !b.tryAcquire() {
		t.Error("tryAcquire() after waiting past the refill interval should succeed")
	}
}

func TestTokenBucketWaitBlocksThenSucceeds(t *testing.T) {
	b := newTokenBucket(1, 50) // capacity 1, refills a token every 20ms
	if !b.tryAcquire() {
		t.Fatal("first tryAcquire() should succeed")
	}

	start := time.Now()
	if err := b.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Errorf("Wait() returned after %v, expected it to block for a refill", elapsed)
	}
}

func TestTokenBucketWaitRespectsContextCancellation(t *testing.T) {
	b := newTokenBucket(1, 0.001) // capacity 1, effectively no refill
	if !b.tryAcquire() {
		t.Fatal("first tryAcquire() should succeed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := b.Wait(ctx)
	if err == nil {
		t.Fatal("Wait() should return an error once ctx is done and no token is available")
	}
}

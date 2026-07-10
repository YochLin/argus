package mcptools

import (
	"testing"
	"time"
)

func TestTTLCacheHitBeforeExpiry(t *testing.T) {
	c := newTTLCache()
	want := textResult("hello")
	c.set("k", want, time.Hour)

	got, ok := c.get("k")
	if !ok {
		t.Fatal("get() after set() with a long TTL should hit")
	}
	if got != want {
		t.Errorf("get() returned a different *mcp.CallToolResult than was set")
	}
}

func TestTTLCacheMissAfterExpiry(t *testing.T) {
	c := newTTLCache()
	c.set("k", textResult("hello"), 10*time.Millisecond)

	time.Sleep(30 * time.Millisecond)

	if _, ok := c.get("k"); ok {
		t.Error("get() should miss once the TTL has elapsed")
	}
}

func TestTTLCacheMissUnknownKey(t *testing.T) {
	c := newTTLCache()
	if _, ok := c.get("nope"); ok {
		t.Error("get() on a never-set key should miss")
	}
}

func TestTTLCacheDistinctKeys(t *testing.T) {
	c := newTTLCache()
	a := textResult("a")
	b := textResult("b")
	c.set("a", a, time.Hour)
	c.set("b", b, time.Hour)

	gotA, _ := c.get("a")
	gotB, _ := c.get("b")
	if gotA != a || gotB != b {
		t.Error("distinct keys should not clobber each other's cached result")
	}
}

func TestTTLCacheDelete(t *testing.T) {
	c := newTTLCache()
	c.set("k", textResult("hello"), time.Hour)

	c.delete("k")

	if _, ok := c.get("k"); ok {
		t.Error("get() after delete() should miss")
	}
}

func TestTTLCacheDeleteUnknownKeyIsNoop(t *testing.T) {
	c := newTTLCache()
	c.delete("nope") // must not panic
}

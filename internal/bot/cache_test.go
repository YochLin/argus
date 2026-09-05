package bot

import (
	"testing"
	"time"
)

func TestTTLCache(t *testing.T) {
	c := newTTLCache[string]()

	if _, ok := c.get("missing"); ok {
		t.Error("get(missing) = ok, want not found")
	}

	c.set("k", "v", 20*time.Millisecond)
	if v, ok := c.get("k"); !ok || v != "v" {
		t.Errorf("get(k) = %v, %v, want v, true", v, ok)
	}

	time.Sleep(30 * time.Millisecond)
	if _, ok := c.get("k"); ok {
		t.Error("get(k) after expiry = ok, want not found")
	}
}

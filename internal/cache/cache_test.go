package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testCache(t *testing.T) *Cache {
	t.Helper()
	dir := t.TempDir()
	return &Cache{dir: dir, ttl: DefaultTTL}
}

func TestMiss(t *testing.T) {
	c := testCache(t)
	got := c.Get("nonexistent")
	if got != nil {
		t.Fatalf("expected nil, got %s", got)
	}
}

func TestRoundTrip(t *testing.T) {
	c := testCache(t)
	data := []byte(`["wt-1","wt-2"]`)
	if err := c.Set("branches", data); err != nil {
		t.Fatal(err)
	}
	got := c.Get("branches")
	if string(got) != string(data) {
		t.Fatalf("got %s, want %s", got, data)
	}
}

func TestExpiry(t *testing.T) {
	c := testCache(t)
	c.ttl = 1 * time.Millisecond
	data := []byte(`["expired"]`)
	if err := c.Set("branches", data); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	got := c.Get("branches")
	if got != nil {
		t.Fatalf("expected nil (expired), got %s", got)
	}
}

func TestEvict(t *testing.T) {
	c := testCache(t)
	c.Set("branches", []byte(`["data"]`))
	c.Evict("branches")

	if _, err := os.Stat(filepath.Join(c.dir, "branches.json")); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed")
	}
	got := c.Get("branches")
	if got != nil {
		t.Fatalf("expected nil after evict, got %s", got)
	}
}

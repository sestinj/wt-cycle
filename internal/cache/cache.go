package cache

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const DefaultTTL = 5 * time.Minute

// entry is the on-disk cache format.
type entry struct {
	Data      json.RawMessage `json:"data"`
	ExpiresAt time.Time       `json:"expires_at"`
}

// Cache provides a TTL file cache at ~/.cache/wt-cycle/<hash>/.
type Cache struct {
	dir string
	ttl time.Duration
}

// New creates a cache for the given repo root.
func New(repoRoot string) *Cache {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(repoRoot)))
	home, _ := os.UserHomeDir()
	return &Cache{
		dir: filepath.Join(home, ".cache", "wt-cycle", hash),
		ttl: DefaultTTL,
	}
}

// Get retrieves a cached value. Returns nil if missing or expired.
func (c *Cache) Get(key string) []byte {
	path := filepath.Join(c.dir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil
	}
	if time.Now().After(e.ExpiresAt) {
		os.Remove(path)
		return nil
	}
	return e.Data
}

// Set stores a value with TTL.
func (c *Cache) Set(key string, data []byte) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	e := entry{
		Data:      data,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, key+".json"), raw, 0644)
}

// Evict removes a cached key.
func (c *Cache) Evict(key string) {
	os.Remove(filepath.Join(c.dir, key+".json"))
}

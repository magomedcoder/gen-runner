package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

func analyzeCacheKey(req AnalyzeRequest) (string, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type responseCache struct {
	mu   sync.Mutex
	ttl  time.Duration
	max  int
	data map[string]cacheEntry
}

type cacheEntry struct {
	msg     string
	expires time.Time
}

func newResponseCache(cfg Config) *responseCache {
	if cfg.AnalyzeCacheDisable {
		return &responseCache{
			ttl: 0,
			max: 0,
		}
	}

	ttl := cfg.AnalyzeCacheTTL
	maxK := cfg.AnalyzeCacheMaxKeys
	if ttl <= 0 || maxK <= 0 {
		return &responseCache{
			ttl: 0,
			max: 0,
		}
	}

	return &responseCache{
		ttl:  ttl,
		max:  maxK,
		data: make(map[string]cacheEntry),
	}
}

func (c *responseCache) enabled() bool {
	return c != nil && c.ttl > 0 && c.max > 0
}

func (c *responseCache) get(key string) (string, bool) {
	if !c.enabled() {
		return "", false
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.data[key]
	if !ok || now.After(e.expires) {
		if ok {
			delete(c.data, key)
		}
		return "", false
	}

	return e.msg, true
}

func (c *responseCache) set(key, message string) {
	if !c.enabled() || key == "" || message == "" {
		return
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, e := range c.data {
		if now.After(e.expires) {
			delete(c.data, k)
		}
	}

	for len(c.data) >= c.max {
		for k := range c.data {
			delete(c.data, k)
			break
		}
	}

	c.data[key] = cacheEntry{
		msg:     message,
		expires: now.Add(c.ttl),
	}
}

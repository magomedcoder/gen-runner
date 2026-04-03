package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

func historySummaryCacheKey(model, dialogueBody string) string {
	h := sha256.Sum256([]byte(model + "\n\x00\n" + dialogueBody))
	return hex.EncodeToString(h[:])
}

type historySummaryCache struct {
	mu      sync.Mutex
	max     int
	keys    []string
	entries map[string]string
}

func newHistorySummaryCache(maxEntries int) *historySummaryCache {
	if maxEntries <= 0 {
		return nil
	}

	return &historySummaryCache{
		max:     maxEntries,
		entries: make(map[string]string, maxEntries),
	}
}

func normalizeHistorySummaryCacheEntries(n int) int {
	if n < 0 {
		return 0
	}

	if n > 50_000 {
		return 50_000
	}

	return n
}

func (h *historySummaryCache) get(key string) (string, bool) {
	if h == nil {
		return "", false
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.entries[key]

	return v, ok
}

func (h *historySummaryCache) ensureMax(m int) {
	if h == nil || m <= 0 {
		return
	}

	m = normalizeHistorySummaryCacheEntries(m)
	if m <= 0 {
		return
	}

	h.mu.Lock()
	if m > h.max {
		h.max = m
	}

	h.mu.Unlock()
}

func (h *historySummaryCache) put(key, summary string) {
	if h == nil || key == "" || summary == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.entries[key]; exists {
		h.entries[key] = summary
		return
	}

	for len(h.keys) >= h.max {
		old := h.keys[0]
		h.keys = h.keys[1:]
		delete(h.entries, old)
	}

	h.entries[key] = summary
	h.keys = append(h.keys, key)
}

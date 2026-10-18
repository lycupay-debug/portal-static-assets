package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	searchCacheTTL        = 5 * time.Second
	searchCacheMaxEntries = 128
)

type searchCacheEntry struct {
	value     []byte
	expiresAt time.Time
}

// searchResultCache is a bounded, short-lived cache for public search/list
// responses. singleflight coalesces concurrent misses for the same query, so a
// burst of identical requests produces one database query rather than N.
// Generation changes prevent an in-flight pre-write query from repopulating
// stale data after a mutation invalidates the cache.
type searchResultCache struct {
	mu         sync.Mutex
	entries    map[string]searchCacheEntry
	generation uint64
	ttl        time.Duration
	maxEntries int
	group      singleflight.Group
}

func newSearchResultCache() *searchResultCache {
	return &searchResultCache{entries: make(map[string]searchCacheEntry), ttl: searchCacheTTL, maxEntries: searchCacheMaxEntries}
}

func (c *searchResultCache) get(ctx context.Context, key string, load func() ([]byte, error)) ([]byte, error) {
	if c == nil {
		return load()
	}
	c.mu.Lock()
	generation := c.generation
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		value := append([]byte(nil), entry.value...)
		c.mu.Unlock()
		return value, nil
	}
	delete(c.entries, key)
	internalKey := strings.Join([]string{strconv.FormatUint(generation, 10), key}, "\x00")
	c.mu.Unlock()

	result := c.group.DoChan(internalKey, func() (any, error) {
		// Recheck after joining singleflight: another request may have filled it.
		c.mu.Lock()
		if entry, ok := c.entries[key]; ok && time.Now().Before(entry.expiresAt) && c.generation == generation {
			value := append([]byte(nil), entry.value...)
			c.mu.Unlock()
			return value, nil
		}
		c.mu.Unlock()

		value, err := load()
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.generation == generation {
			if len(c.entries) >= c.maxEntries {
				// Small bounded cache: evict expired entries first, then the entry
				// closest to expiry. No background sweeper or timer is required.
				now := time.Now()
				for cacheKey, entry := range c.entries {
					if !now.Before(entry.expiresAt) {
						delete(c.entries, cacheKey)
					}
				}
				if len(c.entries) >= c.maxEntries {
					keys := make([]string, 0, len(c.entries))
					for cacheKey := range c.entries {
						keys = append(keys, cacheKey)
					}
					sort.Slice(keys, func(i, j int) bool { return c.entries[keys[i]].expiresAt.Before(c.entries[keys[j]].expiresAt) })
					if len(keys) > 0 {
						delete(c.entries, keys[0])
					}
				}
			}
			c.entries[key] = searchCacheEntry{value: append([]byte(nil), value...), expiresAt: time.Now().Add(c.ttl)}
		}
		c.mu.Unlock()
		return value, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case shared := <-result:
		if shared.Err != nil {
			return nil, shared.Err
		}
		return append([]byte(nil), shared.Val.([]byte)...), nil
	}
}

func (c *searchResultCache) invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.generation++
	c.entries = make(map[string]searchCacheEntry)
	c.mu.Unlock()
}

func normalizeSearchQuery(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120])
	}
	if len([]rune(value)) < 2 {
		return ""
	}
	return value
}

func encodeSearchItems(items []map[string]any) ([]byte, error) { return json.Marshal(items) }

func writeCachedItems(w http.ResponseWriter, items []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"items":`))
	_, _ = w.Write(items)
	_, _ = w.Write([]byte(`}`))
}

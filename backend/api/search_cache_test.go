package main

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchCacheCoalescesConcurrentMisses(t *testing.T) {
	cache := newSearchResultCache()
	var calls atomic.Int32
	start := make(chan struct{})
	load := func() ([]byte, error) {
		calls.Add(1)
		<-start
		return []byte(`[{"id":"one"}]`), nil
	}
	const workers = 24
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			value, err := cache.get(context.Background(), "public:test", load)
			if err == nil && string(value) != `[{"id":"one"}]` {
				t.Errorf("unexpected cached value %s", value)
			}
			errs <- err
		}()
	}
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader called %d times; want one", got)
	}
}

func TestSearchCacheReturnsCopiesAndInvalidates(t *testing.T) {
	cache := newSearchResultCache()
	calls := 0
	load := func() ([]byte, error) { calls++; return []byte("value"), nil }
	first, err := cache.get(context.Background(), "k", load)
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'X'
	second, err := cache.get(context.Background(), "k", load)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != "value" {
		t.Fatalf("cache exposed internal bytes: %q", second)
	}
	if calls != 1 {
		t.Fatalf("loader calls before invalidation = %d, want 1", calls)
	}
	cache.invalidate()
	if _, err := cache.get(context.Background(), "k", load); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("loader calls after invalidation = %d, want 2", calls)
	}
}

func TestNormalizeSearchQueryBoundsAndNormalizes(t *testing.T) {
	if got := normalizeSearchQuery("  HELLO\n  World  "); got != "HELLO World" {
		t.Fatalf("normalized query = %q", got)
	}
	if got := normalizeSearchQuery("中"); got != "" {
		t.Fatalf("single-character query = %q, want empty", got)
	}
	if got := len([]rune(normalizeSearchQuery(strings.Repeat("中", 150)))); got > 120 {
		t.Fatalf("query was not bounded: %d runes", got)
	}
}

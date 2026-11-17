// internal/store/store_concurrent_test.go
package store

import (
	"strconv"
	"sync"
	"testing"
)

// TestConcurrentDelDeletesEachKeyOnce checks SPEC.md 2b rule 3: concurrent
// commands behave as if they ran one at a time. If every goroutine Dels every
// key, each key can be live for exactly one of those Dels, so the 1s returned
// across all goroutines must add up to exactly the number of keys.
// Without the store's lock this either crashes with "concurrent map writes"
// or over-counts because two goroutines both see the same key as live.
func TestConcurrentDelDeletesEachKeyOnce(t *testing.T) {
	const (
		numKeys       = 10000
		numGoroutines = 8
	)

	s := New()
	keys := make([]string, numKeys)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
		s.Set(keys[i], "v", -1)
	}

	start := make(chan struct{})
	counts := make([]int, numGoroutines)
	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start // release all goroutines together to maximize overlap
			for _, k := range keys {
				counts[g] += s.Del(k)
			}
		}(g)
	}

	close(start)
	wg.Wait()

	total := 0
	for _, c := range counts {
		total += c
	}
	if total != numKeys {
		t.Errorf("expected each of %d keys to be deleted exactly once, got %d successful Dels in total", numKeys, total)
	}
}

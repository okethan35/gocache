// internal/store/ttl_test.go
package store

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// These tests close the gaps found in review of issue #4: the original TTL
// tests in store_test.go pass against a constant (0, false) stub for rules 4,
// 5 and 7, and nothing checked rule 6 (lock held for the full body).

// TestTTLExactBoundaryDeletesEntry checks SPEC Phase 7 rules 4 and 7 together:
// at now == expiresAt the key is expired AND lazily removed from both the data
// map and the TTL index.
func TestTTLExactBoundaryDeletesEntry(t *testing.T) {
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime }

	s.Set("foo", "bar", 5)
	currentTime = currentTime.Add(5 * time.Second)

	if secs, ok := s.TTL("foo"); ok || secs != 0 {
		t.Fatalf("expected (0, false) at the exact boundary, got (%d, %v)", secs, ok)
	}
	if _, exists := s.data["foo"]; exists {
		t.Errorf("expected the entry to be deleted from data at the boundary")
	}
	if _, exists := s.expires["foo"]; exists {
		t.Errorf("expected the entry to be deleted from the TTL index at the boundary")
	}
}

// TestTTLOneNanosecondBeforeBoundaryIsAlive pins the other side of rule 7, so
// the boundary test cannot pass with an always-expired implementation.
func TestTTLOneNanosecondBeforeBoundaryIsAlive(t *testing.T) {
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime }

	s.Set("foo", "bar", 5)
	currentTime = currentTime.Add(5*time.Second - time.Nanosecond)

	if secs, ok := s.TTL("foo"); !ok || secs != 0 {
		t.Errorf("expected (0, true) 1ns before expiry, got (%d, %v)", secs, ok)
	}
	if _, exists := s.data["foo"]; !exists {
		t.Errorf("a live entry must not be deleted by TTL")
	}
}

// TestTTLEmptyKeyDoesNotTouchMap checks rule 5. An entry is planted under the
// empty key (Set refuses to, so only a bug could put one there); TTL must
// ignore it rather than answer from the map or remove it.
func TestTTLEmptyKeyDoesNotTouchMap(t *testing.T) {
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime }

	s.data["expired"] = entry{value: "x", hasExpiry: true, expiresAt: currentTime.Add(-time.Second)}
	s.data[""] = entry{value: "planted", hasExpiry: true, expiresAt: currentTime.Add(-time.Second)}

	if secs, ok := s.TTL(""); ok || secs != 0 {
		t.Errorf("expected (0, false) for the empty key, got (%d, %v)", secs, ok)
	}
	if _, exists := s.data[""]; !exists {
		t.Errorf("TTL(\"\") must not touch the map, but it removed the empty-key entry")
	}
	if _, exists := s.data["expired"]; !exists {
		t.Errorf("TTL(\"\") must not touch the map, but it removed another entry")
	}
}

// TestTTLMissingKeyLeavesStoreUnchanged checks rule 4 for a missing key: the
// answer is (0, false) and no other entry is disturbed or created.
func TestTTLMissingKeyLeavesStoreUnchanged(t *testing.T) {
	s := New()
	s.Set("keep", "v", -1)
	s.Set("keepttl", "v", 100)

	if secs, ok := s.TTL("absent"); ok || secs != 0 {
		t.Errorf("expected (0, false) for a missing key, got (%d, %v)", secs, ok)
	}
	if len(s.data) != 2 || len(s.expires) != 1 {
		t.Errorf("expected data=2 expires=1 after TTL on a missing key, got data=%d expires=%d", len(s.data), len(s.expires))
	}
	if secs, ok := s.TTL("keep"); !ok || secs != -1 {
		t.Errorf("expected (-1, true) for the untouched no-expiry key, got (%d, %v)", secs, ok)
	}
}

// TestConcurrentTTLHoldsLock checks rule 6. Every key below is expired, so each
// TTL call deletes from the map, while other goroutines Set and Get the same
// map. Without mu held for the body this dies with "concurrent map writes" (or
// a race report under -race), so it fails under plain go test too.
func TestConcurrentTTLHoldsLock(t *testing.T) {
	const (
		numKeys       = 5000
		numGoroutines = 16
	)
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime } // never reassigned after start

	keys := make([]string, numKeys)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
		s.Set(keys[i], "v", 1)
	}
	currentTime = currentTime.Add(2 * time.Second) // everything above is now expired

	start := make(chan struct{})
	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start
			for i, k := range keys {
				switch (g + i) % 3 {
				case 0:
					s.TTL(k)
				case 1:
					s.Set("other"+k, "v", -1)
				default:
					s.Get(k)
				}
			}
		}(g)
	}
	close(start)
	wg.Wait()

	for _, k := range keys {
		if _, ok := s.TTL(k); ok {
			t.Fatalf("expected %s to be expired", k)
		}
		if _, exists := s.data[k]; exists {
			t.Fatalf("expected %s to be removed after TTL", k)
		}
	}
}

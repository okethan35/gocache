// internal/store/sharded_test.go
package store

import (
	"hash/fnv"
	"strconv"
	"sync"
	"testing"
	"time"
)

const testShards = 5

// newFrozenSharded returns a sharded store whose shards all share one fake
// clock, set before any goroutine starts (SPEC.md 2b rule 6).
func newFrozenSharded(clock *time.Time) *ShardedStore {
	s := NewSharded(testShards)
	for _, sh := range s.shards {
		sh.now = func() time.Time { return *clock }
	}
	return s
}

// SPEC.md 2b rule 7: the sharded store follows Phase 1 rules 1-10.

func TestShardedGetMissingKey(t *testing.T) {
	s := NewSharded(testShards)
	if v, ok := s.Get("foo"); ok || v != "" {
		t.Errorf("expected (\"\", false) for missing key, got (%q, %v)", v, ok)
	}
}

func TestShardedGetExpiredKey(t *testing.T) {
	clock := time.Now()
	s := newFrozenSharded(&clock)
	s.Set("foo", "bar", 1)
	clock = clock.Add(2 * time.Second)
	if v, ok := s.Get("foo"); ok || v != "" {
		t.Errorf("expected expired key to miss, got (%q, %v)", v, ok)
	}
}

func TestShardedSetImmediatelyExpiringKey(t *testing.T) {
	clock := time.Now()
	s := newFrozenSharded(&clock)
	s.Set("foo", "bar", 0)
	if v, ok := s.Get("foo"); ok || v != "" {
		t.Errorf("expected ttl 0 key to miss immediately, got (%q, %v)", v, ok)
	}
}

func TestShardedNegativeTTLMeansNoExpiry(t *testing.T) {
	clock := time.Now()
	s := newFrozenSharded(&clock)
	s.Set("a", "1", -1)
	s.Set("b", "2", -5) // clamped to -1
	clock = clock.Add(1000 * time.Hour)
	for _, k := range []string{"a", "b"} {
		if _, ok := s.Get(k); !ok {
			t.Errorf("expected %q with negative ttl to never expire", k)
		}
	}
}

func TestShardedSetOverwritesValueAndTTL(t *testing.T) {
	clock := time.Now()
	s := newFrozenSharded(&clock)
	s.Set("foo", "bar", 100)
	s.Set("foo", "baz", 5)
	if v, ok := s.Get("foo"); !ok || v != "baz" {
		t.Fatalf("expected (\"baz\", true) after overwrite, got (%q, %v)", v, ok)
	}
	clock = clock.Add(6 * time.Second) // past the new ttl, before the old one
	if _, ok := s.Get("foo"); ok {
		t.Errorf("expected overwrite to replace ttl 100 with ttl 5")
	}
}

func TestShardedDelReturns(t *testing.T) {
	s := NewSharded(testShards)
	if got := s.Del("foo"); got != 0 {
		t.Errorf("Del on missing key: expected 0, got %d", got)
	}
	s.Set("foo", "bar", -1)
	if got := s.Del("foo"); got != 1 {
		t.Errorf("Del on live key: expected 1, got %d", got)
	}
}

func TestShardedEmptyKey(t *testing.T) {
	s := NewSharded(testShards)
	if got := s.Set("", "bar", 3); got != 0 {
		t.Errorf("Set on empty key: expected 0, got %d", got)
	}
	if got := s.Del(""); got != 0 {
		t.Errorf("Del on empty key: expected 0, got %d", got)
	}
}

func TestShardedSetValidKeyReturnsOne(t *testing.T) {
	s := NewSharded(testShards)
	if got := s.Set("foo", "bar", 10); got != 1 {
		t.Errorf("Set on valid key: expected 1, got %d", got)
	}
}

// SPEC.md 2b rule 3, against the sharded store.
func TestShardedConcurrentDelDeletesEachKeyOnce(t *testing.T) {
	const (
		numKeys       = 10000
		numGoroutines = 8
	)
	s := NewSharded(testShards)
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
			<-start
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

// SPEC.md 2b rule 8: fnv32 is FNV-1a. The inline version must match hash/fnv.
func TestFNV32aMatchesStdlib(t *testing.T) {
	for _, k := range []string{"", "a", "foo", "k9999", "a much longer key with spaces"} {
		h := fnv.New32a()
		h.Write([]byte(k))
		if got, want := fnv32a(k), h.Sum32(); got != want {
			t.Errorf("fnv32a(%q) = %d, want %d", k, got, want)
		}
	}
}

// SPEC.md 2b rule 8: the same key always maps to the same shard, and keys
// actually spread over more than one shard.
func TestShardedKeyToShardMapping(t *testing.T) {
	s := NewSharded(testShards)
	if s.shardFor("foo") != s.shardFor("foo") {
		t.Errorf("expected the same key to always map to the same shard")
	}
	used := map[*Store]bool{}
	for i := 0; i < 10000; i++ {
		used[s.shardFor("k"+strconv.Itoa(i))] = true
	}
	if len(used) < 2 {
		t.Errorf("expected keys to spread over more than one shard, all landed in %d", len(used))
	}
}

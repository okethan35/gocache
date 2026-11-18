// internal/store/sharded_test.go
package store

import (
	"hash/fnv"
	"strconv"
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

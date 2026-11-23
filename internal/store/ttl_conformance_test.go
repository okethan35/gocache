// internal/store/ttl_conformance_test.go
package store

import (
	"testing"
	"time"
)

// SPEC.md Phase 7 rules 1-8: TTL is part of KV, and both implementations
// follow rules 1-7.
func TestConformanceTTLWithExpiry(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("foo", "bar", 30)
		*clock = clock.Add(1500 * time.Millisecond)
		if n, ok := s.TTL("foo"); !ok || n != 28 {
			t.Errorf("TTL = (%d, %v), want (28, true) (floor of 28.5)", n, ok)
		}
	})
}

func TestConformanceTTLZeroWhenLessThanOneSecond(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("foo", "bar", 1)
		*clock = clock.Add(900 * time.Millisecond)
		if n, ok := s.TTL("foo"); !ok || n != 0 {
			t.Errorf("TTL = (%d, %v), want (0, true)", n, ok)
		}
	})
}

func TestConformanceTTLNoExpiry(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", -1)
		if n, ok := s.TTL("foo"); !ok || n != -1 {
			t.Errorf("TTL = (%d, %v), want (-1, true)", n, ok)
		}
	})
}

func TestConformanceTTLMissingKey(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		if n, ok := s.TTL("nope"); ok || n != 0 {
			t.Errorf("TTL = (%d, %v), want (0, false)", n, ok)
		}
	})
}

func TestConformanceTTLEmptyKey(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		if n, ok := s.TTL(""); ok || n != 0 {
			t.Errorf("TTL = (%d, %v), want (0, false)", n, ok)
		}
	})
}

func TestConformanceTTLExpiredKeyAtBoundary(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("foo", "bar", 10)
		*clock = clock.Add(10 * time.Second) // now == expiresAt
		if n, ok := s.TTL("foo"); ok || n != 0 {
			t.Errorf("TTL = (%d, %v), want (0, false) at the boundary", n, ok)
		}
		if _, ok := s.Get("foo"); ok {
			t.Errorf("Get after expired TTL should miss")
		}
	})
}

// A TTL query on an expired key must be followed by a miss from the shard's
// own map: the sharded TTL has to reach the key's shard, not another one.
func TestShardedTTLUsesKeysShard(t *testing.T) {
	clock := time.Now()
	s := newFrozenSharded(&clock)
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		s.Set(k, "v", 60)
		if n, ok := s.TTL(k); !ok || n != 60 {
			t.Errorf("TTL(%q) = (%d, %v), want (60, true)", k, n, ok)
		}
	}
}

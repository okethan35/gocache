// internal/store/expire_conformance_test.go
package store

import (
	"testing"
	"time"
)

// backing returns the Store that holds key, so tests can look at the map and
// the TTL index directly. Both implementations are covered (Phase 8 rule 11).
func backing(s KV, key string) *Store {
	switch v := s.(type) {
	case *Store:
		return v
	case *ShardedStore:
		return v.shardFor(key)
	}
	panic("unknown KV")
}

func inIndex(s KV, key string) bool {
	st := backing(s, key)
	st.mu.Lock()
	defer st.mu.Unlock()
	_, ok := st.expires[key]
	return ok
}

func inMap(s KV, key string) bool {
	st := backing(s, key)
	st.mu.Lock()
	defer st.mu.Unlock()
	_, ok := st.data[key]
	return ok
}

func exportOf(s KV) []Record {
	switch v := s.(type) {
	case *Store:
		return v.Export()
	case *ShardedStore:
		var out []Record
		for _, sh := range v.shards {
			out = append(out, sh.Export()...)
		}
		return out
	}
	return nil
}

// Rules 1, 2, 6.
func TestConformanceExpireAddsExpiry(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", -1)
		if n := s.Expire("foo", 10); n != 1 {
			t.Fatalf("Expire = %d, want 1", n)
		}
		if n, ok := s.TTL("foo"); !ok || n != 10 {
			t.Errorf("TTL = (%d, %v), want (10, true)", n, ok)
		}
		if v, _ := s.Get("foo"); v != "bar" {
			t.Errorf("value = %q, want bar", v)
		}
		if !inIndex(s, "foo") {
			t.Errorf("key not in TTL index after Expire")
		}
	})
}

// Rule 2: replaces an earlier expiry.
func TestConformanceExpireReplacesExpiry(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", 100)
		if n := s.Expire("foo", 5); n != 1 {
			t.Fatalf("Expire = %d, want 1", n)
		}
		if n, ok := s.TTL("foo"); !ok || n != 5 {
			t.Errorf("TTL = (%d, %v), want (5, true)", n, ok)
		}
	})
}

// Rule 3.
func TestConformanceExpireZeroDeletes(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", 100)
		if n := s.Expire("foo", 0); n != 1 {
			t.Fatalf("Expire = %d, want 1", n)
		}
		if _, ok := s.Get("foo"); ok {
			t.Errorf("Get hit after Expire 0")
		}
		if inMap(s, "foo") || inIndex(s, "foo") {
			t.Errorf("key still in map or TTL index after Expire 0")
		}
	})
}

// Rule 4.
func TestConformanceExpireMissingEmptyExpired(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		if n := s.Expire("nope", 10); n != 0 {
			t.Errorf("missing: Expire = %d, want 0", n)
		}
		if n := s.Expire("", 10); n != 0 {
			t.Errorf("empty: Expire = %d, want 0", n)
		}
		s.Set("old", "v", 1)
		*clock = clock.Add(2 * time.Second)
		if n := s.Expire("old", 10); n != 0 {
			t.Errorf("expired: Expire = %d, want 0", n)
		}
		if inMap(s, "old") || inIndex(s, "old") {
			t.Errorf("expired entry not deleted by Expire")
		}
	})
}

// Rule 5.
func TestConformanceExpireOutOfRange(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", 50)
		s.Set("plain", "v", -1)
		for _, secs := range []int{-1, -5, MaxTTLSeconds + 1} {
			if n := s.Expire("foo", secs); n != 0 {
				t.Errorf("Expire(%d) = %d, want 0", secs, n)
			}
			if n := s.Expire("plain", secs); n != 0 {
				t.Errorf("Expire plain (%d) = %d, want 0", secs, n)
			}
		}
		if n, ok := s.TTL("foo"); !ok || n != 50 {
			t.Errorf("TTL foo = (%d, %v), want (50, true)", n, ok)
		}
		if n, ok := s.TTL("plain"); !ok || n != -1 {
			t.Errorf("TTL plain = (%d, %v), want (-1, true)", n, ok)
		}
	})
}

func TestConformanceExpireMaxTTLAccepted(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", -1)
		if n := s.Expire("foo", MaxTTLSeconds); n != 1 {
			t.Errorf("Expire(MaxTTLSeconds) = %d, want 1", n)
		}
	})
}

// Rules 7, 8.
func TestConformancePersistRemovesExpiry(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", 100)
		if n := s.Persist("foo"); n != 1 {
			t.Fatalf("Persist = %d, want 1", n)
		}
		if n, ok := s.TTL("foo"); !ok || n != -1 {
			t.Errorf("TTL = (%d, %v), want (-1, true)", n, ok)
		}
		if v, _ := s.Get("foo"); v != "bar" {
			t.Errorf("value = %q, want bar", v)
		}
		if inIndex(s, "foo") {
			t.Errorf("key still in TTL index after Persist")
		}
	})
}

// Rule 9.
func TestConformancePersistReturnsZero(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("plain", "v", -1)
		if n := s.Persist("plain"); n != 0 {
			t.Errorf("no expiry: Persist = %d, want 0", n)
		}
		if n := s.Persist("nope"); n != 0 {
			t.Errorf("missing: Persist = %d, want 0", n)
		}
		if n := s.Persist(""); n != 0 {
			t.Errorf("empty: Persist = %d, want 0", n)
		}
		s.Set("old", "v", 1)
		*clock = clock.Add(2 * time.Second)
		if n := s.Persist("old"); n != 0 {
			t.Errorf("expired: Persist = %d, want 0", n)
		}
		if inMap(s, "old") {
			t.Errorf("expired entry not deleted by Persist")
		}
		if v, ok := s.Get("plain"); !ok || v != "v" {
			t.Errorf("plain changed")
		}
	})
}

// Rule 12.
func TestConformanceExportAfterExpireAndPersist(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("a", "1", -1)
		s.Set("b", "2", 100)
		s.Expire("a", 10)
		s.Persist("b")
		got := map[string]Record{}
		for _, r := range exportOf(s) {
			got[r.Key] = r
		}
		if r := got["a"]; !r.HasExpiry || !r.ExpiresAt.Equal(clock.Add(10*time.Second)) {
			t.Errorf("a = %+v, want expiry now+10s", r)
		}
		if r := got["b"]; r.Key != "b" || r.HasExpiry {
			t.Errorf("b = %+v, want no expiry", r)
		}
	})
}

// Rule 10/11: sharded delegates to the key's own shard, for many keys.
func TestShardedExpirePersistUseKeysShard(t *testing.T) {
	clock := time.Now()
	s := newFrozenSharded(&clock)
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		s.Set(k, "v", -1)
		if s.Expire(k, 30) != 1 {
			t.Errorf("Expire(%q) != 1", k)
		}
		if !inIndex(s, k) {
			t.Errorf("%q not in its shard's index", k)
		}
		if s.Persist(k) != 1 {
			t.Errorf("Persist(%q) != 1", k)
		}
	}
}

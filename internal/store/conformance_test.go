// internal/store/conformance_test.go
package store

import (
	"testing"
	"time"
)

// storeImpls are the KV implementations every conformance test runs against
// (SPEC.md 2b rule 7: the sharded store follows Phase 1 rules 1-10). Each
// constructor wires the given fake clock in before any goroutine starts
// (2b rule 6).
var storeImpls = []struct {
	name string
	mk   func(clock *time.Time) KV
}{
	{"single", func(clock *time.Time) KV { return newFrozenStore(clock) }},
	{"sharded", func(clock *time.Time) KV { return newFrozenSharded(clock) }},
}

// forEachStore runs fn as a subtest against every implementation, each with
// its own fresh store and clock.
func forEachStore(t *testing.T, fn func(t *testing.T, s KV, clock *time.Time)) {
	t.Helper()
	for _, impl := range storeImpls {
		t.Run(impl.name, func(t *testing.T) {
			clock := time.Now()
			fn(t, impl.mk(&clock), &clock)
		})
	}
}

func TestConformanceGetMissingKey(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		if v, ok := s.Get("foo"); ok || v != "" {
			t.Errorf("expected (\"\", false) for missing key, got (%q, %v)", v, ok)
		}
	})
}

func TestConformanceGetExpiredKey(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("foo", "bar", 1)
		*clock = clock.Add(2 * time.Second)
		if v, ok := s.Get("foo"); ok || v != "" {
			t.Errorf("expected expired key to miss, got (%q, %v)", v, ok)
		}
	})
}

func TestConformanceSetImmediatelyExpiringKey(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		s.Set("foo", "bar", 0)
		if v, ok := s.Get("foo"); ok || v != "" {
			t.Errorf("expected ttl 0 key to miss immediately, got (%q, %v)", v, ok)
		}
	})
}

func TestConformanceNegativeTTLMeansNoExpiry(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("a", "1", -1)
		s.Set("b", "2", -5) // clamped to -1
		*clock = clock.Add(1000 * time.Hour)
		for _, k := range []string{"a", "b"} {
			if _, ok := s.Get(k); !ok {
				t.Errorf("expected %q with negative ttl to never expire", k)
			}
		}
	})
}

func TestConformanceSetOverwritesValueAndTTL(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, clock *time.Time) {
		s.Set("foo", "bar", 100)
		s.Set("foo", "baz", 5)
		if v, ok := s.Get("foo"); !ok || v != "baz" {
			t.Fatalf("expected (\"baz\", true) after overwrite, got (%q, %v)", v, ok)
		}
		*clock = clock.Add(6 * time.Second) // past the new ttl, before the old one
		if _, ok := s.Get("foo"); ok {
			t.Errorf("expected overwrite to replace ttl 100 with ttl 5")
		}
	})
}

func TestConformanceDelReturns(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		if got := s.Del("foo"); got != 0 {
			t.Errorf("Del on missing key: expected 0, got %d", got)
		}
		s.Set("foo", "bar", -1)
		if got := s.Del("foo"); got != 1 {
			t.Errorf("Del on live key: expected 1, got %d", got)
		}
	})
}

func TestConformanceEmptyKey(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		if got := s.Set("", "bar", 3); got != 0 {
			t.Errorf("Set on empty key: expected 0, got %d", got)
		}
		if got := s.Del(""); got != 0 {
			t.Errorf("Del on empty key: expected 0, got %d", got)
		}
	})
}

func TestConformanceSetValidKeyReturnsOne(t *testing.T) {
	forEachStore(t, func(t *testing.T, s KV, _ *time.Time) {
		if got := s.Set("foo", "bar", 10); got != 1 {
			t.Errorf("Set on valid key: expected 1, got %d", got)
		}
	})
}

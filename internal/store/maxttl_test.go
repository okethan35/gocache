// internal/store/maxttl_test.go
package store

import (
	"testing"
	"time"
)

// SPEC.md Phase 1 rule 11: Set accepts a TTL of exactly 100 years, and for
// anything above it stores nothing and returns 0, instead of overflowing
// time.Duration into an expiry in the past.
func TestSetMaxTTL(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)

	if got := s.Set("max", "v", MaxTTLSeconds); got != 1 {
		t.Errorf("expected Set with the max TTL to return 1, got %d", got)
	}
	if v, ok := s.Get("max"); !ok || v != "v" {
		t.Errorf("expected the max-TTL key to be readable, got (%q, %v)", v, ok)
	}

	for _, ttl := range []int{MaxTTLSeconds + 1, 9999999999} {
		if got := s.Set("over", "v", ttl); got != 0 {
			t.Errorf("ttl %d: expected Set to return 0, got %d", ttl, got)
		}
		if _, ok := s.data["over"]; ok {
			t.Errorf("ttl %d: expected nothing to be stored", ttl)
		}
	}

	// An over-limit Set must not replace an existing value either.
	s.Set("keep", "old", -1)
	s.Set("keep", "new", MaxTTLSeconds+1)
	if v, _ := s.Get("keep"); v != "old" {
		t.Errorf("expected an over-limit Set to leave the old value, got %q", v)
	}
}

// internal/store/export_test.go
package store

import (
	"sort"
	"testing"
	"time"
)

func byKey(recs []Record) map[string]Record {
	m := make(map[string]Record, len(recs))
	for _, r := range recs {
		m[r.Key] = r
	}
	return m
}

// SPEC.md Phase 5 rules 5-6: Export returns every live entry with its value
// and absolute expiry, and leaves out entries that are already expired.
func TestExportSkipsExpired(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)
	s.Set("old", "v", 1)
	clock = clock.Add(2 * time.Second) // "old" is now expired
	s.Set("forever", "a", -1)
	s.Set("ttl", "b", 100)
	s.Set("empty", "", -1)

	got := byKey(s.Export())

	if len(got) != 3 {
		keys := make([]string, 0, len(got))
		for k := range got {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("expected 3 live entries, got %d: %v", len(got), keys)
	}
	if r := got["forever"]; r.Value != "a" || r.HasExpiry {
		t.Errorf("forever: expected value %q with no expiry, got %+v", "a", r)
	}
	if r := got["ttl"]; r.Value != "b" || !r.HasExpiry || !r.ExpiresAt.Equal(clock.Add(100*time.Second)) {
		t.Errorf("ttl: expected value %q expiring at %v, got %+v", "b", clock.Add(100*time.Second), r)
	}
	if r, ok := got["empty"]; !ok || r.Value != "" {
		t.Errorf("empty: expected an empty value to be exported, got %+v (present=%v)", r, ok)
	}
}

// SPEC.md Phase 5 rules 6-7: Import adds entries, drops ones that have expired
// by load time, and keeps the TTL index in sync.
func TestImportDropsExpiredAndIndexesTTLs(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)

	s.Import([]Record{
		{Key: "forever", Value: "a"},
		{Key: "ttl", Value: "b", HasExpiry: true, ExpiresAt: clock.Add(time.Minute)},
		{Key: "gone", Value: "c", HasExpiry: true, ExpiresAt: clock.Add(-time.Second)},
	})

	if v, ok := s.Get("forever"); !ok || v != "a" {
		t.Errorf("forever: expected (\"a\", true), got (%q, %v)", v, ok)
	}
	if v, ok := s.Get("ttl"); !ok || v != "b" {
		t.Errorf("ttl: expected (\"b\", true), got (%q, %v)", v, ok)
	}
	if _, ok := s.data["gone"]; ok {
		t.Errorf("gone: expected an entry expired at load time to be dropped")
	}
	if _, ok := s.expires["ttl"]; !ok || len(s.expires) != 1 {
		t.Errorf("expected the TTL index to hold exactly \"ttl\", got %v", s.expires)
	}
	clock = clock.Add(2 * time.Minute)
	if _, ok := s.Get("ttl"); ok {
		t.Errorf("ttl: expected the imported expiry to be honoured")
	}
}

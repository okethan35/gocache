// internal/store/export.go
package store

import "time"

// Record is one entry as saved in a snapshot (SPEC.md Phase 5 rule 5).
// ExpiresAt is meaningful only when HasExpiry is true.
type Record struct {
	Key       string
	Value     string
	HasExpiry bool
	ExpiresAt time.Time
}

// Export returns a copy of every live entry. It holds the lock only while
// copying, so the caller can write the copy out without blocking clients, and
// the copy reflects the store at one instant (SPEC.md Phase 5 rule 11).
// Entries already expired are left out (rule 6).
func (s *Store) Export() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()

	recs := make([]Record, 0, len(s.data))
	for key, e := range s.data {
		if s.isExpired(e) {
			continue
		}
		recs = append(recs, Record{Key: key, Value: e.value, HasExpiry: e.hasExpiry, ExpiresAt: e.expiresAt})
	}
	return recs
}

// Import adds recs to the store, overwriting entries with the same key, and
// keeps the TTL index in sync (SPEC.md Phase 5 rule 7). Records that have
// expired by now are dropped (rule 6), and records with an empty key are
// skipped (rule 7).
func (s *Store) Import(recs []Record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range recs {
		if r.Key == "" {
			continue // the store never holds an empty key (Phase 1 rule 9)
		}
		e := entry{value: r.Value, hasExpiry: r.HasExpiry, expiresAt: r.ExpiresAt}
		if s.isExpired(e) {
			continue
		}
		s.data[r.Key] = e
		if e.hasExpiry {
			s.expires[r.Key] = struct{}{}
		} else {
			delete(s.expires, r.Key)
		}
	}
}

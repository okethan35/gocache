// internal/store/export.go
package store

import "time"

// Record is one entry as saved in a snapshot (SPEC.md Phase 5 rule 5).
type Record struct {
	Key       string
	Value     string
	HasExpiry bool
	ExpiresAt time.Time
}

// CHECKPOINT: stubs so the tests compile.
func (s *Store) Export() []Record { return nil }

func (s *Store) Import(recs []Record) {}

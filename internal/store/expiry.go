// internal/store/expiry.go
package store

import "time"

// CHECKPOINT: stubs so the tests compile; nothing is swept yet.
func (s *Store) sweep() int { return 0 }

func (s *Store) startExpiryLoop(ticks <-chan time.Time) (stop func()) { return func() {} }

func (s *Store) StartExpiry(interval time.Duration) (stop func()) {
	return s.startExpiryLoop(time.NewTicker(interval).C)
}

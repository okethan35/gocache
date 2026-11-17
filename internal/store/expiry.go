// internal/store/expiry.go
package store

import (
	"sync"
	"time"
)

// Active expiry (SPEC.md Phase 3). Lazy expiry in Get/Del stays the source of
// correctness; the sweep only reclaims memory held by expired entries that are
// never read again.
const (
	sweepSampleSize = 20 // entries examined per round (rule 3)
	sweepMaxRounds  = 16 // rounds per tick (rule 3)
)

// sweep runs one tick's worth of rounds and returns how many rounds ran.
// Another round runs only while more than 25% of the last round's examined
// entries were expired (rule 4).
func (s *Store) sweep() int {
	for round := 1; round <= sweepMaxRounds; round++ {
		examined, expired := s.sweepRound()
		if examined == 0 || expired*4 <= examined {
			return round
		}
	}
	return sweepMaxRounds
}

// sweepRound holds the lock for one round only, so clients can run between
// rounds (rule 3). It samples only TTL keys, by ranging over the TTL index
// (rule 5). Ranging over a map starts at a random position, so stopping after
// sweepSampleSize keys is a cheap, non-uniform sample (rule 5a). Deleting the
// current key while ranging is allowed in Go.
func (s *Store) sweepRound() (examined, expired int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key := range s.expires {
		if examined == sweepSampleSize {
			break
		}
		examined++
		if s.isExpired(s.data[key]) {
			s.remove(key)
			expired++
		}
	}
	return examined, expired
}

// StartExpiry starts the active-expiry sweep, running once per interval
// (rule 6). New does not call it. The returned stop function returns only
// after the sweep goroutine has exited, and is safe to call more than once.
func (s *Store) StartExpiry(interval time.Duration) (stop func()) {
	ticker := time.NewTicker(interval)
	stopLoop := s.startExpiryLoop(ticker.C)
	return func() {
		stopLoop()
		ticker.Stop()
	}
}

// startExpiryLoop runs sweep for every value received on ticks. Tests pass
// their own channel instead of a real ticker, so they never need to sleep.
func (s *Store) startExpiryLoop(ticks <-chan time.Time) (stop func()) {
	quit := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		for {
			select {
			case <-quit:
				return
			case <-ticks:
				s.sweep()
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(quit) })
		<-exited
	}
}

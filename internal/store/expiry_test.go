// internal/store/expiry_test.go
package store

import (
	"strconv"
	"testing"
	"time"
)

// newFrozenStore returns a Store whose clock is the value *clock points to.
// The clock is only changed while no sweep goroutine is running.
func newFrozenStore(clock *time.Time) *Store {
	s := New()
	s.now = func() time.Time { return *clock }
	return s
}

// SPEC.md Phase 3 rules 3-4: one sweep removes expired entries and leaves
// no-TTL and unexpired entries alone. With 10 keys, the first round examines
// all of them (3 expired is 30% > 25%), so a second round runs, finds nothing
// expired, and the sweep stops.
func TestSweepRemovesOnlyExpired(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)
	for i := 0; i < 3; i++ {
		s.Set("expired"+strconv.Itoa(i), "v", 1)
	}
	clock = clock.Add(2 * time.Second) // the three above are now expired
	for i := 0; i < 3; i++ {
		s.Set("forever"+strconv.Itoa(i), "v", -1)
	}
	for i := 0; i < 4; i++ {
		s.Set("later"+strconv.Itoa(i), "v", 100)
	}

	rounds := s.sweep()

	if len(s.data) != 7 {
		t.Errorf("expected 7 entries left after sweep, got %d", len(s.data))
	}
	for i := 0; i < 3; i++ {
		if _, ok := s.data["expired"+strconv.Itoa(i)]; ok {
			t.Errorf("expected expired%d to be swept", i)
		}
	}
	if rounds != 2 {
		t.Errorf("expected 2 rounds (30%% expired, then 0%%), got %d", rounds)
	}
}

// SPEC.md Phase 3 rule 4: if no more than 25% of a round is expired, the
// sweep stops after that round.
func TestSweepStopsWhenFewExpired(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)
	for i := 0; i < 100; i++ {
		s.Set("k"+strconv.Itoa(i), "v", -1)
	}

	if rounds := s.sweep(); rounds != 1 {
		t.Errorf("expected 1 round when nothing is expired, got %d", rounds)
	}
	if len(s.data) != 100 {
		t.Errorf("expected all 100 no-TTL entries to survive, got %d", len(s.data))
	}
}

// SPEC.md Phase 3 rule 3: a tick runs at most 16 rounds of at most 20
// entries, so with everything expired one sweep deletes exactly 320.
func TestSweepCapsRounds(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)
	for i := 0; i < 1000; i++ {
		s.Set("k"+strconv.Itoa(i), "v", 0) // ttl 0: expired immediately
	}

	if rounds := s.sweep(); rounds != 16 {
		t.Errorf("expected the 16-round cap, got %d rounds", rounds)
	}
	if want := 1000 - 16*20; len(s.data) != want {
		t.Errorf("expected %d entries left after a capped sweep, got %d", want, len(s.data))
	}
}

// SPEC.md Phase 3 rule 6: the sweep loop runs sweep on each tick, and stop
// waits for the loop to exit. The test owns the tick channel, so no sleeping:
// the unbuffered send returns once the loop has taken the tick, and stop()
// returns only after that sweep finished and the loop exited.
func TestExpiryLoopSweepsOnTick(t *testing.T) {
	s := New()
	for i := 0; i < 5; i++ {
		s.Set("k"+strconv.Itoa(i), "v", 0) // ttl 0: expired immediately
	}

	ticks := make(chan time.Time)
	stop := s.startExpiryLoop(ticks)
	select {
	case ticks <- time.Now():
	case <-time.After(2 * time.Second): // a deadline, so a broken loop fails instead of hanging
		t.Fatal("expiry loop never took the tick")
	}
	stop()

	if len(s.data) != 0 {
		t.Errorf("expected the tick to sweep all 5 expired entries, %d left", len(s.data))
	}
}

// SPEC.md Phase 3 rule 6: New does not start a sweep, and StartExpiry's stop
// returns (the goroutine exits) even if no tick has fired yet.
func TestStartExpiryStopReturns(t *testing.T) {
	s := New()
	s.Set("k", "v", 0)
	stop := s.StartExpiry(time.Hour)
	stop()
	stop() // calling stop twice must be safe
	if len(s.data) != 1 {
		t.Errorf("expected no sweep before the first tick, got %d entries", len(s.data))
	}
}

// SPEC.md Phase 3 rule 6: a zero or negative interval panics.
func TestStartExpiryPanicsOnNonPositiveInterval(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("expected StartExpiry(%v) to panic", d)
				}
			}()
			New().StartExpiry(d)
		}()
	}
}

// SPEC.md Phase 3 rule 5: the sweep samples only TTL keys, so a large number
// of no-TTL keys cannot hide the expired ones. Sampling all entries would
// usually examine mostly no-TTL keys here and stop after one round.
func TestSweepFindsExpiredAmongManyNoTTLKeys(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)
	for i := 0; i < 1000; i++ {
		s.Set("forever"+strconv.Itoa(i), "v", -1)
	}
	for i := 0; i < 20; i++ {
		s.Set("expired"+strconv.Itoa(i), "v", 0) // ttl 0: expired immediately
	}

	s.sweep()

	if len(s.data) != 1000 {
		t.Errorf("expected all 20 expired keys swept and 1000 no-TTL keys kept, got %d entries", len(s.data))
	}
}

// SPEC.md Phase 3 rule 5: the TTL index holds exactly the keys whose entry
// has an expiry, through every way a key can be written or removed.
func TestTTLIndexMatchesEntries(t *testing.T) {
	clock := time.Now()
	s := newFrozenStore(&clock)

	s.Set("ttl", "v", 100)     // added with a ttl
	s.Set("forever", "v", -1)  // never in the index
	s.Set("demoted", "v", 100) // ttl, then overwritten with no expiration
	s.Set("demoted", "v", -1)
	s.Set("promoted", "v", -1) // no expiration, then overwritten with a ttl
	s.Set("promoted", "v", 100)
	s.Set("deleted", "v", 100) // removed by Del
	s.Del("deleted")
	s.Set("lazyget", "v", 1) // removed lazily by Get once expired
	s.Set("lazydel", "v", 1) // removed lazily by Del once expired
	s.Set("swept", "v", 1)   // removed by the sweep once expired
	clock = clock.Add(2 * time.Second)
	s.Get("lazyget")
	s.Del("lazydel")
	s.sweep()

	want := map[string]bool{}
	for k, e := range s.data {
		if e.hasExpiry && !s.isExpired(e) {
			want[k] = true
		}
	}
	if len(s.expires) != len(want) {
		t.Errorf("expected TTL index to hold %d keys %v, got %d: %v", len(want), want, len(s.expires), s.expires)
	}
	for k := range want {
		if _, ok := s.expires[k]; !ok {
			t.Errorf("expected %q in the TTL index", k)
		}
	}
	for k := range s.expires {
		if !want[k] {
			t.Errorf("expected %q not to be in the TTL index", k)
		}
	}
}

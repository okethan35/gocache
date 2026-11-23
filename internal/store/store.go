// internal/store/store.go
package store

import (
	"sync"
	"time"
)

// MaxTTLSeconds is the largest TTL Set accepts: 100 years (SPEC.md Phase 1
// rule 11). It keeps now+TTL far inside the range of time.Duration (about 292
// years) and of the snapshot's Unix-nanosecond expiry (until year 2262).
const MaxTTLSeconds = 3155760000

type Store struct {
	mu   sync.Mutex // guards data; held for the whole body of Get/Set/Del
	data map[string]entry
	// expires indexes the keys in data whose entry has an expiry, so the
	// active-expiry sweep samples only TTL keys (SPEC.md Phase 3 rule 5).
	expires map[string]struct{}
	now     func() time.Time // defaults to time.Now, but swappable in tests
}

func New() *Store {
	s := &Store{}
	s.init()
	return s
}

// init prepares a zero Store in place. It exists so a Store can live inside a
// larger struct (the padded shards) without being copied, since a Store holds
// a mutex.
func (s *Store) init() {
	s.data = make(map[string]entry)
	s.expires = make(map[string]struct{})
	s.now = time.Now
}

type entry struct {
	value     string
	expiresAt time.Time
	hasExpiry bool // false means expiresAt is meaningless/ignored
}

// isExpired reports whether e has an expiry that has been reached or passed
// as of s.now(). A ttl of 0 sets expiresAt == the Set instant, so the boundary
// itself (now == expiresAt) counts as expired, not just strictly-after.
func (s *Store) isExpired(e entry) bool {
	return e.hasExpiry && !s.now().Before(e.expiresAt)
}

// Get returns the value stored at key and true, or ("", false) if the key is
// missing or expired. Expiry is checked lazily here; an expired entry found
// during Get is deleted from the map before reporting the miss.
func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok {
		return "", false
	}
	if s.isExpired(e) {
		s.remove(key)
		return "", false
	}
	return e.value, true
}

// Set stores value at key with the given ttlSeconds, fully replacing any
// existing value and TTL for that key. ttlSeconds == -1 (or anything below
// -1, which gets clamped to -1) means no expiration. Setting on an empty key
// is a no-op. Returns 1 if the key was set, 0 otherwise.
func (s *Store) Set(key, value string, ttlSeconds int) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return 0
	}
	if ttlSeconds > MaxTTLSeconds {
		return 0 // would overflow time.Duration or the snapshot format
	}
	if ttlSeconds < -1 {
		ttlSeconds = -1
	}

	if ttlSeconds == -1 {
		s.data[key] = entry{value: value, hasExpiry: false}
		delete(s.expires, key)
	} else {
		s.data[key] = entry{
			value:     value,
			hasExpiry: true,
			expiresAt: s.now().Add(time.Duration(ttlSeconds) * time.Second),
		}
		s.expires[key] = struct{}{}
	}
	return 1
}

// Del removes key from the store. It returns 1 if a live (non-expired) key
// was deleted, 0 otherwise — a missing key, an empty key, and an expired key
// (already treated as absent) all return 0. An expired entry found here is
// still physically removed from the map as a cleanup side effect.
func (s *Store) Del(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return 0
	}
	e, ok := s.data[key]
	if !ok {
		return 0
	}
	if s.isExpired(e) {
		s.remove(key)
		return 0
	}
	s.remove(key)
	return 1
}

// remove deletes key from data and from the TTL index. Callers hold s.mu.
func (s *Store) remove(key string) {
	delete(s.data, key)
	delete(s.expires, key)
}

// TTL returns the remaining whole seconds (floored) before key expires and
// true, (-1, true) if the key has no expiry, or (0, false) if the key is
// empty, missing or expired. An expired entry found here is removed, as in Get.
func (s *Store) TTL(key string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == "" {
		return 0, false
	}
	e, ok := s.data[key]
	if !ok {
		return 0, false
	}
	if s.isExpired(e) {
		s.remove(key)
		return 0, false
	}
	if !e.hasExpiry {
		return -1, true
	}
	return int64(e.expiresAt.Sub(s.now()) / time.Second), true
}

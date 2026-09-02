// internal/store/store.go
package store

import "time"

type Store struct {
	data map[string]entry
	now  func() time.Time // defaults to time.Now, but swappable in tests
}

func New() *Store {
	return &Store{
		data: make(map[string]entry),
		now:  time.Now,
	}
}

type entry struct {
	value      string
	expiresAt  time.Time
	hasExpiry  bool // false means expiresAt is meaningless/ignored
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
	e, ok := s.data[key]
	if !ok {
		return "", false
	}
	if s.isExpired(e) {
		delete(s.data, key)
		return "", false
	}
	return e.value, true
}

// Set stores value at key with the given ttlSeconds, fully replacing any
// existing value and TTL for that key. ttlSeconds == -1 (or anything below
// -1, which gets clamped to -1) means no expiration. Setting on an empty key
// is a no-op. Returns 1 if the key was set, 0 otherwise.
func (s *Store) Set(key, value string, ttlSeconds int) int {
	if key == "" {
		return 0
	}
	if ttlSeconds < -1 {
		ttlSeconds = -1
	}

	if ttlSeconds == -1 {
		s.data[key] = entry{value: value, hasExpiry: false}
	} else {
		s.data[key] = entry{
			value:     value,
			hasExpiry: true,
			expiresAt: s.now().Add(time.Duration(ttlSeconds) * time.Second),
		}
	}
	return 1
}

// Del removes key from the store. It returns 1 if a live (non-expired) key
// was deleted, 0 otherwise — a missing key, an empty key, and an expired key
// (already treated as absent) all return 0. An expired entry found here is
// still physically removed from the map as a cleanup side effect.
func (s *Store) Del(key string) int {
	if key == "" {
		return 0
	}
	e, ok := s.data[key]
	if !ok {
		return 0
	}
	if s.isExpired(e) {
		delete(s.data, key)
		return 0
	}
	delete(s.data, key)
	return 1
}
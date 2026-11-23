package store

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// runLockTest hammers the map from many goroutines. Without mu held in the
// function under test this dies with "concurrent map writes", so it fails
// under plain go test too.
func runLockTest(t *testing.T, expired bool, op func(s *Store, k string)) {
	t.Helper()
	const (
		numKeys       = 5000
		numGoroutines = 16
	)
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime } // never reassigned after start

	keys := make([]string, numKeys)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
		s.Set(keys[i], "v", 1)
	}
	if expired {
		currentTime = currentTime.Add(2 * time.Second)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start
			for i, k := range keys {
				switch (g + i) % 3 {
				case 0:
					op(s, k)
				case 1:
					s.Set("other"+k, "v", 100)
				default:
					s.Get(k)
				}
			}
		}(g)
	}
	close(start)
	wg.Wait()
}

func TestConcurrentExpireHoldsLock(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		runLockTest(t, true, func(s *Store, k string) { s.Expire(k, 10) })
	})
	t.Run("live", func(t *testing.T) {
		runLockTest(t, false, func(s *Store, k string) { s.Expire(k, 10) })
	})
	t.Run("zero", func(t *testing.T) {
		runLockTest(t, false, func(s *Store, k string) { s.Expire(k, 0) })
	})
}

func TestConcurrentPersistHoldsLock(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		runLockTest(t, true, func(s *Store, k string) { s.Persist(k) })
	})
	t.Run("live", func(t *testing.T) {
		runLockTest(t, false, func(s *Store, k string) { s.Persist(k) })
	})
}

// internal/snapshot/snapshot.go
package snapshot

import (
	"io"
	"os"
	"time"

	"gocache/internal/store"
)

// CHECKPOINT: stubs so the tests compile.
func Write(w io.Writer, recs []store.Record) error { return nil }

func Read(r io.Reader) ([]store.Record, error) { return nil, nil }

func SaveFile(path string, recs []store.Record) error { return nil }

func LoadFile(path string) ([]store.Record, error) { return nil, os.ErrNotExist }

func Start(s *store.Store, path string, interval time.Duration) (stop func()) {
	return startLoop(s, path, time.NewTicker(interval).C)
}

func startLoop(s *store.Store, path string, ticks <-chan time.Time) (stop func()) { return func() {} }

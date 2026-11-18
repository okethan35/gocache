// internal/snapshot/snapshot.go

// Package snapshot saves and loads the store to and from a single file
// (SPEC.md Phase 5). It depends on internal/store, never the reverse.
package snapshot

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gocache/internal/store"
)

// File format, SPEC.md Phase 5 rule 8. All integers are little-endian.
const (
	magic   = "GOCACHE1"
	version = 1

	headerSize   = len(magic) + 4 + 8 // magic, version, count
	checksumSize = 4
)

// ErrCorrupt is returned (wrapped) for any damaged or unrecognised file
// (SPEC.md Phase 5 rule 9).
var ErrCorrupt = errors.New("corrupt")

// Write encodes recs in the snapshot format.
func Write(w io.Writer, recs []store.Record) error {
	b := make([]byte, 0, headerSize+checksumSize)
	b = append(b, magic...)
	b = binary.LittleEndian.AppendUint32(b, version)
	b = binary.LittleEndian.AppendUint64(b, uint64(len(recs)))
	for _, r := range recs {
		b = binary.LittleEndian.AppendUint32(b, uint32(len(r.Key)))
		b = append(b, r.Key...)
		b = binary.LittleEndian.AppendUint32(b, uint32(len(r.Value)))
		b = append(b, r.Value...)
		var expiresAt int64
		if r.HasExpiry {
			b = append(b, 1)
			expiresAt = r.ExpiresAt.UnixNano()
		} else {
			b = append(b, 0)
		}
		b = binary.LittleEndian.AppendUint64(b, uint64(expiresAt))
	}
	b = binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(b))
	_, err := w.Write(b)
	return err
}

// Read decodes a snapshot. Any damage returns an error wrapping ErrCorrupt.
// The checksum is verified before any field is trusted.
func Read(r io.Reader) ([]store.Record, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(b) < headerSize+checksumSize {
		return nil, fmt.Errorf("%w: too short (%d bytes)", ErrCorrupt, len(b))
	}
	body, sum := b[:len(b)-checksumSize], binary.LittleEndian.Uint32(b[len(b)-checksumSize:])
	if crc32.ChecksumIEEE(body) != sum {
		return nil, fmt.Errorf("%w: checksum mismatch", ErrCorrupt)
	}
	if string(body[:len(magic)]) != magic {
		return nil, fmt.Errorf("%w: not a gocache snapshot", ErrCorrupt)
	}
	if v := binary.LittleEndian.Uint32(body[len(magic):]); v != version {
		return nil, fmt.Errorf("%w: unknown version %d", ErrCorrupt, v)
	}
	count := binary.LittleEndian.Uint64(body[len(magic)+4:])

	d := decoder{b: body[headerSize:]}
	var recs []store.Record
	for i := uint64(0); i < count; i++ {
		key, val := d.str(), d.str()
		hasExpiry := d.byte()
		expiresAt := int64(d.uint64())
		if d.err != nil {
			return nil, fmt.Errorf("%w: entry %d: %v", ErrCorrupt, i, d.err)
		}
		if hasExpiry > 1 {
			return nil, fmt.Errorf("%w: entry %d: bad hasExpiry %d", ErrCorrupt, i, hasExpiry)
		}
		if hasExpiry == 0 && expiresAt != 0 {
			return nil, fmt.Errorf("%w: entry %d: expiresAt %d with no expiry", ErrCorrupt, i, expiresAt)
		}
		rec := store.Record{Key: key, Value: val, HasExpiry: hasExpiry == 1}
		if rec.HasExpiry {
			rec.ExpiresAt = time.Unix(0, expiresAt)
		}
		recs = append(recs, rec)
	}
	if len(d.b) != 0 {
		return nil, fmt.Errorf("%w: %d bytes left over after %d entries", ErrCorrupt, len(d.b), count)
	}
	return recs, nil
}

// decoder reads fields from b, recording the first out-of-bounds read in err
// instead of panicking, so a bogus length or count cannot crash the server.
type decoder struct {
	b   []byte
	err error
}

func (d *decoder) take(n uint64) []byte {
	if d.err != nil {
		return nil
	}
	if n > uint64(len(d.b)) {
		d.err = errors.New("truncated")
		return nil
	}
	out := d.b[:n]
	d.b = d.b[n:]
	return out
}

func (d *decoder) byte() byte {
	if b := d.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (d *decoder) uint64() uint64 {
	if b := d.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}

func (d *decoder) str() string {
	n := d.take(4)
	if n == nil {
		return ""
	}
	return string(d.take(uint64(binary.LittleEndian.Uint32(n))))
}

// SaveFile writes recs to path atomically (SPEC.md Phase 5 rule 10): write a
// temporary file in the same directory, flush it to disk, rename it over
// path, then flush the directory so the rename itself survives a crash.
// If any step fails, the previous snapshot is unchanged and the temporary
// file is removed (rule 15).
func SaveFile(path string, recs []store.Record) error {
	return saveFile(osFS{}, path, recs)
}

func saveFile(fsys fileSystem, path string, recs []store.Record) (err error) {
	dir := filepath.Dir(path)
	tmp, err := fsys.CreateTemp(dir, ".gocache-*.tmp")
	if err != nil {
		return fmt.Errorf("snapshot %s: %w", path, err)
	}
	renamed := false
	defer func() {
		if err != nil {
			if !renamed {
				tmp.Close() // may already be closed; the error is irrelevant here
				fsys.Remove(tmp.Name())
			}
			err = fmt.Errorf("snapshot %s: %w", path, err)
		}
	}()

	w := bufio.NewWriter(tmp)
	if err = Write(w, recs); err != nil {
		return err
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = fsys.Rename(tmp.Name(), path); err != nil {
		return err
	}
	renamed = true
	// The new snapshot is in place; flushing the directory makes the rename
	// itself durable. If this fails the save is still reported as failed.
	return fsys.SyncDir(dir)
}

// fileSystem is the handful of file operations saveFile needs. Production
// uses osFS; tests substitute a fake that can fail any single step and
// records the order of operations.
type fileSystem interface {
	CreateTemp(dir, pattern string) (tempFile, error)
	Rename(oldpath, newpath string) error
	Remove(name string) error
	SyncDir(dir string) error
}

type tempFile interface {
	io.Writer
	Sync() error
	Close() error
	Name() string
}

type osFS struct{}

func (osFS) CreateTemp(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
func (osFS) Rename(oldpath, newpath string) error             { return os.Rename(oldpath, newpath) }
func (osFS) Remove(name string) error                         { return os.Remove(name) }

func (osFS) SyncDir(dir string) error { return syncDirFor(runtime.GOOS, dir) }

// syncDirFor CHECKPOINT: ignores goos.
func syncDirFor(goos, dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// LoadFile reads the snapshot at path. A missing file returns an error
// satisfying errors.Is(err, fs.ErrNotExist) (SPEC.md Phase 5 rule 12); any
// other error names the file (rule 13).
func LoadFile(path string) ([]store.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err // *PathError already names the file
	}
	defer f.Close()
	recs, err := Read(bufio.NewReader(f))
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", path, err)
	}
	return recs, nil
}

// Start saves s to path once per interval (SPEC.md Phase 5 rule 2). The
// returned stop function returns only after the goroutine has exited and is
// safe to call more than once. A non-positive interval panics, as
// time.NewTicker does.
func Start(s *store.Store, path string, interval time.Duration) (stop func()) {
	ticker := time.NewTicker(interval)
	stopLoop := startLoop(s, path, ticker.C)
	return func() {
		stopLoop()
		ticker.Stop()
	}
}

// startLoop saves on every value received from ticks. Tests pass their own
// channel instead of a real ticker. A failed periodic save is logged and the
// loop keeps going; the next tick tries again.
func startLoop(s *store.Store, path string, ticks <-chan time.Time) (stop func()) {
	return startLoopWith(s, path, ticks, SaveFile)
}

// startLoopWith is startLoop with the save function injected, so tests can
// make a save fail (SPEC.md Phase 5 rule 14).
func startLoopWith(s *store.Store, path string, ticks <-chan time.Time, save func(string, []store.Record) error) (stop func()) {
	quit := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		for {
			select {
			case <-quit:
				return
			case <-ticks:
				if err := save(path, s.Export()); err != nil {
					log.Printf("periodic snapshot failed: %v", err)
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(quit) })
		<-exited
	}
}

// CleanTemp CHECKPOINT: stub.
func CleanTemp(path string) (int, error) { return 0, nil }

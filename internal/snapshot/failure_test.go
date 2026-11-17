// internal/snapshot/failure_test.go
package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gocache/internal/store"
)

// fakeFS is an in-memory fileSystem that can fail exactly one named step and
// records every operation in order.
type fakeFS struct {
	files  map[string][]byte
	failAt string // "create", "write", "sync", "close", "rename", "syncdir", or ""
	ops    []string
	n      int
}

var errInjected = errors.New("injected failure")

func newFakeFS(failAt string) *fakeFS {
	return &fakeFS{files: map[string][]byte{}, failAt: failAt}
}

func (f *fakeFS) op(name string) error {
	if len(f.ops) == 0 || f.ops[len(f.ops)-1] != name || name != "write" {
		f.ops = append(f.ops, name) // consecutive writes are recorded once
	}
	if f.failAt == name {
		return errInjected
	}
	return nil
}

func (f *fakeFS) CreateTemp(dir, pattern string) (tempFile, error) {
	if err := f.op("create"); err != nil {
		return nil, err
	}
	f.n++
	name := filepath.Join(dir, strings.Replace(pattern, "*", fmt.Sprint(f.n), 1))
	f.files[name] = nil
	return &fakeFile{fs: f, name: name}, nil
}

func (f *fakeFS) Rename(oldpath, newpath string) error {
	if err := f.op("rename"); err != nil {
		return err
	}
	f.files[newpath] = f.files[oldpath]
	delete(f.files, oldpath)
	return nil
}

func (f *fakeFS) Remove(name string) error {
	delete(f.files, name)
	return nil
}

func (f *fakeFS) SyncDir(dir string) error { return f.op("syncdir") }

func (f *fakeFS) names() []string {
	var out []string
	for n := range f.files {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

type fakeFile struct {
	fs     *fakeFS
	name   string
	closed bool
}

func (w *fakeFile) Write(p []byte) (int, error) {
	if err := w.fs.op("write"); err != nil {
		return 0, err
	}
	w.fs.files[w.name] = append(w.fs.files[w.name], p...)
	return len(p), nil
}

func (w *fakeFile) Sync() error { return w.fs.op("sync") }

func (w *fakeFile) Close() error {
	if w.closed {
		return errors.New("already closed")
	}
	w.closed = true
	return w.fs.op("close")
}

func (w *fakeFile) Name() string { return w.name }

// SPEC.md Phase 5 rule 10: a successful save happens in exactly this order:
// write the temp file, flush it to disk, close it, rename it into place, then
// flush the directory. This is the in-process stand-in for crash durability.
func TestSaveFileStepOrder(t *testing.T) {
	fsys := newFakeFS("")
	if err := saveFile(fsys, "/d/dump.gocache", sampleRecords()); err != nil {
		t.Fatalf("saveFile failed: %v", err)
	}
	want := []string{"create", "write", "sync", "close", "rename", "syncdir"}
	if strings.Join(fsys.ops, ",") != strings.Join(want, ",") {
		t.Errorf("expected steps %v, got %v", want, fsys.ops)
	}
	if got := fsys.names(); len(got) != 1 || got[0] != "/d/dump.gocache" {
		t.Errorf("expected only the snapshot to remain, got %v", got)
	}
}

// SPEC.md Phase 5 rule 15: whichever step fails, the save reports an error
// naming the path, and no temporary file is left. Before the rename the old
// snapshot is unchanged; a failed directory flush leaves the new one in place.
func TestSaveFileFailureAtEachStep(t *testing.T) {
	const path = "/d/dump.gocache"
	old := encode(t, []store.Record{{Key: "old", Value: "x"}})
	newBytes := encode(t, sampleRecords())

	for _, step := range []string{"create", "write", "sync", "close", "rename", "syncdir"} {
		t.Run(step, func(t *testing.T) {
			fsys := newFakeFS(step)
			fsys.files[path] = old

			err := saveFile(fsys, path, sampleRecords())
			if !errors.Is(err, errInjected) {
				t.Fatalf("expected the injected error, got %v", err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected the error to name %s, got %q", path, err)
			}
			if got := fsys.names(); len(got) != 1 || got[0] != path {
				t.Errorf("expected only %s to remain (no temp file), got %v", path, got)
			}
			want := old
			if step == "syncdir" {
				want = newBytes // already renamed into place
			}
			if !bytes.Equal(fsys.files[path], want) {
				t.Errorf("expected %s to hold the %s snapshot", path, map[bool]string{true: "new", false: "old"}[step == "syncdir"])
			}
		})
	}
}

// SPEC.md Phase 5 rule 14: a failed periodic save does not stop the loop; the
// next tick saves again.
func TestSnapshotLoopSurvivesFailedSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gocache")
	s := store.New()
	s.Set("foo", "bar", -1)

	calls := 0
	save := func(p string, recs []store.Record) error {
		calls++
		if calls == 1 {
			return errInjected
		}
		return SaveFile(p, recs)
	}

	ticks := make(chan time.Time)
	stop := startLoopWith(s, path, ticks, save)
	for i := 0; i < 2; i++ {
		select {
		case ticks <- time.Now():
		case <-time.After(2 * time.Second):
			t.Fatalf("snapshot loop stopped taking ticks after %d", i)
		}
	}
	stop()

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("expected the second tick to save, LoadFile failed: %v", err)
	}
	sameRecords(t, got, []store.Record{{Key: "foo", Value: "bar"}})
}

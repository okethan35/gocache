// internal/snapshot/followups_test.go
package snapshot

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gocache/internal/store"
)

// SPEC.md Phase 5 rule 10: Windows cannot flush a directory, so the step is
// skipped there; elsewhere it really runs (and fails for a missing directory).
func TestSyncDirSkippedOnWindows(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := syncDirFor("windows", missing); err != nil {
		t.Errorf("windows: expected the directory flush to be skipped, got %v", err)
	}
	if err := syncDirFor("linux", missing); err == nil {
		t.Errorf("linux: expected the directory flush to run and fail for a missing directory")
	}
}

// SPEC.md Phase 5 rule 9: an empty key makes the file corrupt.
func TestReadRejectsEmptyKey(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, []store.Record{{Key: "", Value: "v"}}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if _, err := Read(&buf); err == nil {
		t.Errorf("expected an empty key to make the snapshot corrupt")
	}
}

// SPEC.md Phase 5 rule 16: CleanTemp removes only leftover ".gocache-*.tmp"
// files in the snapshot's directory and reports how many.
func TestCleanTempRemovesOnlyLeftovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.gocache")
	for _, name := range []string{"dump.gocache", ".gocache-123.tmp", ".gocache-abc.tmp", "other.tmp", ".gocache-1.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	n, err := CleanTemp(path)
	if err != nil {
		t.Fatalf("CleanTemp failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 leftovers removed, got %d", n)
	}
	entries, _ := os.ReadDir(dir)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	sort.Strings(left)
	want := []string{".gocache-1.txt", "dump.gocache", "other.tmp"}
	if len(left) != len(want) {
		t.Fatalf("expected %v to remain, got %v", want, left)
	}
	for i := range want {
		if left[i] != want[i] {
			t.Errorf("expected %v to remain, got %v", want, left)
			break
		}
	}
}

// cmd/server/save_failure_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SPEC.md Phase 5 rules 2 and 15, on the real filesystem with no fakes: if
// the final save at shutdown fails, run returns an error naming the snapshot
// (so main exits 1), and what was at the path is left alone. A non-empty
// directory at the snapshot path makes the rename fail, even as root.
func TestFinalSaveFailureIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gocache")
	c, r, stop := startPersistent(t, path)
	roundTrip(t, c, r, "SET foo $bar$", "1")

	if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	err := stop()
	if err == nil {
		t.Fatalf("expected run to return the final save error, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("expected the error to name %s, got %q", path, err)
	}
	if fi, statErr := os.Stat(filepath.Join(path, "blocker")); statErr != nil || !fi.IsDir() {
		t.Errorf("expected the directory at the snapshot path to be untouched")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("expected no temp file left, found %s", e.Name())
		}
	}
}

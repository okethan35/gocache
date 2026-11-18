// cmd/server/tempclean_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// SPEC.md Phase 5 rule 16: a temp file left by a crash during a save is
// removed when the server starts.
func TestRunRemovesLeftoverTempFiles(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, ".gocache-999.tmp")
	if err := os.WriteFile(stale, []byte("half a snapshot"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	c, r, stop := startPersistent(t, filepath.Join(dir, "dump.gocache"))
	roundTrip(t, c, r, "GET foo", "False $nil$") // the server is up, so startup has run
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed at startup, stat err=%v", stale, err)
	}
	if err := stop(); err != nil {
		t.Fatalf("run returned %v", err)
	}
}

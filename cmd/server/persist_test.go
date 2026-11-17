// cmd/server/persist_test.go
package main

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startPersistent runs runPersistent on a fresh local listener and returns a
// connected client plus a stop func that cancels and returns run's error.
func startPersistent(t *testing.T, path string) (net.Conn, *bufio.Reader, func() error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runPersistent(ctx, ln, time.Hour, time.Hour, path) }()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	c.SetDeadline(time.Now().Add(2 * time.Second))
	stop := func() error {
		cancel()
		defer c.Close()
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			t.Fatalf("runPersistent did not return within 2s of cancel")
			return nil
		}
	}
	return c, bufio.NewReader(c), stop
}

func roundTrip(t *testing.T, c net.Conn, r *bufio.Reader, line, want string) {
	t.Helper()
	if _, err := c.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	got, err := r.ReadString('\n')
	if err != nil || got != want+"\n" {
		t.Fatalf("%s: expected %q, got %q (err=%v)", line, want, got, err)
	}
}

// SPEC.md Phase 5 rules 2-3 and 12: with no snapshot the server starts empty;
// data written before a graceful shutdown is there after the next start.
func TestDataSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gocache")

	c, r, stop := startPersistent(t, path)
	roundTrip(t, c, r, "GET foo", "False $nil$") // first run: no snapshot yet
	roundTrip(t, c, r, "SET foo $bar$", "1")
	roundTrip(t, c, r, "SET ttl $soon$ 100", "1")
	if err := stop(); err != nil {
		t.Fatalf("first run returned %v", err)
	}

	c, r, stop = startPersistent(t, path)
	roundTrip(t, c, r, "GET foo", "True $bar$")
	roundTrip(t, c, r, "GET ttl", "True $soon$")
	if err := stop(); err != nil {
		t.Fatalf("second run returned %v", err)
	}
}

// SPEC.md Phase 5 rule 13: a corrupt snapshot stops startup with an error
// naming the file, instead of serving an empty store.
func TestCorruptSnapshotStopsStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gocache")
	if err := os.WriteFile(path, []byte("definitely not a snapshot"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	done := make(chan error, 1)
	go func() { done <- runPersistent(context.Background(), ln, time.Hour, time.Hour, path) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected an error for a corrupt snapshot, got nil")
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("expected the error to name %s, got %q", path, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runPersistent kept running with a corrupt snapshot")
	}
}

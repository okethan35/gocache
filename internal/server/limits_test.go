// internal/server/limits_test.go
package server

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"gocache/internal/store"
)

// These tests use real network deadlines, so they run on the real clock with
// short timeouts. None of them sleeps; every wait is bounded by a deadline.

// startServerConfig runs ServeConfig on a fresh local listener and stops it
// when the test ends.
func startServerConfig(t *testing.T, cfg Config) (addr string, cancel context.CancelFunc) {
	t.Helper()
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeConfig(ctx, ln, store.New(), cfg) }()
	t.Cleanup(func() {
		cancel()
		ln.Close()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			t.Errorf("ServeConfig did not return within %v of cancel", shutdownTimeout)
		}
	})
	return ln.Addr().String(), cancel
}

// SPEC.md Phase 6 rules 1-3: connections over the limit get
// "max clients reached" and are closed; a slot frees up when a client leaves.
func TestMaxClientsRejectsExtra(t *testing.T) {
	addr, _ := startServerConfig(t, Config{MaxClients: 2})

	a, ra := dial(t, addr)
	writeLine(t, a, "SET foo $bar$")
	expectLine(t, ra, "1")
	b, rb := dial(t, addr)
	writeLine(t, b, "GET foo")
	expectLine(t, rb, "True $bar$")

	_, rc := dial(t, addr)
	expectLine(t, rc, "max clients reached")
	if _, err := rc.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Errorf("expected the rejected connection to be closed, got err=%v", err)
	}

	a.Close()
	// The server notices A is gone asynchronously, so retry until its slot is
	// free, bounded by a deadline.
	deadline := time.Now().Add(2 * time.Second)
	for {
		d, rd := dial(t, addr)
		writeLine(t, d, "GET foo")
		got, err := rd.ReadString('\n')
		d.Close()
		if err == nil && got == "True $bar$\n" {
			return
		}
		if err == nil && got != "max clients reached\n" {
			t.Fatalf("unexpected reply %q", got)
		}
		if time.Now().After(deadline) {
			t.Fatalf("no slot freed up within 2s of a client leaving (last reply %q, err %v)", got, err)
		}
	}
}

// SPEC.md Phase 6 rule 4: a client that sends nothing is closed after the
// idle timeout.
func TestIdleClientIsClosed(t *testing.T) {
	addr, _ := startServerConfig(t, Config{IdleTimeout: 100 * time.Millisecond})

	a, ra := dial(t, addr)
	writeLine(t, a, "SET foo $bar$")
	expectLine(t, ra, "1")

	if _, err := ra.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Errorf("expected the idle client to be closed (EOF), got err=%v", err)
	}
}

// SPEC.md Phase 6 rule 5: the timeout restarts before each line, so a client
// that keeps sending is never closed, even well past one timeout.
func TestActiveClientIsNotClosed(t *testing.T) {
	addr, _ := startServerConfig(t, Config{IdleTimeout: 100 * time.Millisecond})

	a, ra := dial(t, addr)
	start := time.Now()
	for n := 0; time.Since(start) < 300*time.Millisecond; n++ {
		writeLine(t, a, "SET foo $bar$")
		expectLine(t, ra, "1")
	}
}

// SPEC.md Phase 6 rule 5: the timeout covers a whole line, so dripping one
// byte at a time does not keep a connection alive.
func TestSlowLineIsClosed(t *testing.T) {
	addr, _ := startServerConfig(t, Config{IdleTimeout: 100 * time.Millisecond})

	a, ra := dial(t, addr)
	closed := make(chan error, 1)
	go func() {
		_, err := ra.ReadString('\n')
		closed <- err
	}()

	// A ticker (not a sleep) drips one byte every 20ms, well inside the
	// timeout, without ever finishing the line.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	giveUp := time.After(2 * time.Second)
	for {
		select {
		case err := <-closed:
			// The server closes while holding unread bytes (the drip), so the
			// OS may report the close as a reset instead of EOF. Both mean
			// the server closed the connection.
			if !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
				t.Errorf("expected the slow client to be closed (EOF or reset), got err=%v", err)
			}
			return
		case <-ticker.C:
			a.Write([]byte("S")) // errors once the server has closed; ignored
		case <-giveUp:
			t.Fatalf("a client dripping bytes was never closed")
		}
	}
}

// SPEC.md Phase 6 rule 6: restarting the idle timeout after shutdown has
// started must not push the shutdown deadline back. The idle timeout here is
// an hour, so if it won, shutdown would wait an hour.
func TestShutdownBeatsIdleTimeout(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	// Swapped before the server starts and restored after it has returned.
	dispatch = func(s KV, line string) string {
		if line == "SET slow $v$" {
			close(entered)
			<-release
		}
		return Dispatch(s, line)
	}
	t.Cleanup(func() { dispatch = Dispatch })

	addr, cancel := startServerConfig(t, Config{IdleTimeout: time.Hour})
	c, rc := dial(t, addr)
	writeLine(t, c, "SET slow $v$")
	<-entered

	cancel()
	// Wait until shutdown has been applied to the open connections, which the
	// server does before it closes the listener: once dialing is refused, the
	// shutdown deadline is already set.
	refusedBy := time.Now().Add(2 * time.Second)
	for {
		probe, err := net.Dial("tcp", addr)
		if err != nil {
			break
		}
		probe.Close()
		if time.Now().After(refusedBy) {
			t.Fatalf("listener still accepting 2s after cancel")
		}
	}
	close(release) // the command finishes; the idle timeout is restarted next

	expectLine(t, rc, "1")
	if _, err := rc.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Errorf("expected the connection to close at shutdown, got err=%v", err)
	}
}

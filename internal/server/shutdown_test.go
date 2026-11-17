// internal/server/shutdown_test.go
package server

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"gocache/internal/store"
)

// shutdownTimeout bounds how long a test waits for ServeContext to return
// after cancel, so a server that ignores ctx fails instead of hanging.
const shutdownTimeout = 2 * time.Second

// startServerContext runs ServeContext on a random local port. The returned
// wait func blocks until ServeContext returns (or fails the test after
// shutdownTimeout) and reports its error.
func startServerContext(t *testing.T, ln net.Listener) (cancel context.CancelFunc, wait func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeContext(ctx, ln, store.New()) }()
	var once sync.Once
	var result error
	wait = func() error {
		once.Do(func() {
			select {
			case result = <-done:
			case <-time.After(shutdownTimeout):
				t.Fatalf("ServeContext did not return within %v of cancel", shutdownTimeout)
			}
		})
		return result
	}
	t.Cleanup(func() {
		cancel()
		ln.Close() // lets a broken ServeContext stop accepting
	})
	return cancel, wait
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	return ln
}

// SPEC.md Phase 4 rules 3, 4 and 8: cancelling closes idle connections,
// stops accepting, and ServeContext returns nil.
func TestShutdownClosesIdleConnections(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	cancel, wait := startServerContext(t, ln)

	a, ra := dial(t, addr)
	writeLine(t, a, "SET foo $bar$") // one round trip so A is surely accepted
	expectLine(t, ra, "1")

	cancel()
	if err := wait(); err != nil {
		t.Errorf("expected ServeContext to return nil after cancel, got %v", err)
	}
	if _, err := ra.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Errorf("expected idle connection to be closed (EOF), got err=%v", err)
	}
	if c, err := net.Dial("tcp", addr); err == nil {
		c.Close()
		t.Errorf("expected new connections to be refused after shutdown")
	}
}

// SPEC.md Phase 4 rule 5: a command already running when shutdown starts
// finishes, its response is delivered, and then the connection closes.
func TestShutdownFinishesRunningCommand(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	// Swapped before the server starts and restored after it has returned,
	// so no connection goroutine reads dispatch while it changes.
	dispatch = func(s KV, line string) string {
		if line == "SET slow $v$" {
			close(entered)
			<-release
		}
		return Dispatch(s, line)
	}
	t.Cleanup(func() { dispatch = Dispatch })

	ln := listen(t)
	cancel, wait := startServerContext(t, ln)

	c, rc := dial(t, ln.Addr().String())
	writeLine(t, c, "SET slow $v$")
	<-entered // the command is now running inside the server
	cancel()
	close(release)

	expectLine(t, rc, "1")
	if _, err := rc.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Errorf("expected connection to close after the running command, got err=%v", err)
	}
	if err := wait(); err != nil {
		t.Errorf("expected ServeContext to return nil, got %v", err)
	}
}

// failingListener always fails Accept with a non-closed error until Close,
// so ServeContext keeps backing off. It signals once backoff has grown long.
type failingListener struct {
	net.Listener
	mu       sync.Mutex
	fails    int
	closed   chan struct{}
	longWait chan struct{}
}

func (l *failingListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	default:
	}
	l.mu.Lock()
	l.fails++
	if l.fails == 10 { // 5ms doubled 9 times is capped at 1s
		close(l.longWait)
	}
	l.mu.Unlock()
	return nil, errors.New("injected accept error")
}

func (l *failingListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

// SPEC.md Phase 4 rule 10: shutdown during an accept-error backoff does not
// wait for the backoff. The backoff is at its 1s cap here, so a ServeContext
// that sleeps through it misses the 300ms deadline.
func TestShutdownInterruptsAcceptBackoff(t *testing.T) {
	ln := &failingListener{closed: make(chan struct{}), longWait: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeContext(ctx, ln, store.New()) }()

	<-ln.longWait
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Errorf("ServeContext kept sleeping through its backoff after cancel")
		ln.Close() // force a broken ServeContext to stop, so the test can end
		<-done
	}
}

// SPEC.md Phase 4 rule 5: lines a client already sent but the server has not
// started executing are discarded at shutdown, not executed.
func TestShutdownDiscardsQueuedCommands(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var seen []string
	// Swapped before the server starts and restored after it has returned.
	dispatch = func(s KV, line string) string {
		mu.Lock()
		seen = append(seen, line)
		mu.Unlock()
		if line == "SET slow $v$" {
			close(entered)
			<-release
		}
		return Dispatch(s, line)
	}
	t.Cleanup(func() { dispatch = Dispatch })

	ln := listen(t)
	cancel, wait := startServerContext(t, ln)

	c, rc := dial(t, ln.Addr().String())
	// One write, so the second line is normally already buffered by the server
	// while the first one runs.
	if _, err := c.Write([]byte("SET slow $v$\nSET queued $x$\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	<-entered
	cancel()
	close(release)

	expectLine(t, rc, "1")
	if _, err := rc.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Errorf("expected connection to close with no reply for the queued line, got err=%v", err)
	}
	if err := wait(); err != nil {
		t.Errorf("expected ServeContext to return nil, got %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, line := range seen {
		if line == "SET queued $x$" {
			t.Errorf("expected the queued line to be discarded, but it was executed")
		}
	}
}

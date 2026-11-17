// internal/server/serve_test.go
package server

import (
	"bufio"
	"net"
	"testing"
	"time"

	"gocache/internal/store"
)

// readTimeout bounds how long a client waits for a response, so a server that
// fails to serve a connection makes the test fail instead of hang.
const readTimeout = 2 * time.Second

// startServer runs Serve on a random local port and stops it when the test ends.
func startServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	s := store.New()
	done := make(chan error, 1)
	go func() { done <- Serve(ln, s) }()
	t.Cleanup(func() {
		ln.Close()
		if err := <-done; err != nil {
			t.Errorf("Serve returned %v after listener closed, want nil", err)
		}
	})
	return ln.Addr().String()
}

func dial(t *testing.T, addr string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		t.Fatalf("set deadline failed: %v", err)
	}
	return conn, bufio.NewReader(conn)
}

// SPEC.md 2b rule 1: an idle client must not block other clients.
func TestServeIdleClientDoesNotBlockOthers(t *testing.T) {
	addr := startServer(t)

	dial(t, addr) // client A connects and never sends anything

	b, rb := dial(t, addr)
	writeLine(t, b, "SET foo $bar$")
	expectLine(t, rb, "1")
}

// SPEC.md 2b rule 2: the store is shared between connections, checked in
// both directions while both clients stay connected.
// Depends on rule 1: if TestServeIdleClientDoesNotBlockOthers also fails,
// look there first, since B can't be served while A blocks it.
func TestServeConnectionsShareStore(t *testing.T) {
	addr := startServer(t)

	a, ra := dial(t, addr)
	b, rb := dial(t, addr)

	writeLine(t, a, "SET foo $bar$")
	expectLine(t, ra, "1")
	writeLine(t, b, "GET foo")
	expectLine(t, rb, "True $bar$")

	writeLine(t, b, "SET baz $qux$")
	expectLine(t, rb, "1")
	writeLine(t, a, "GET baz")
	expectLine(t, ra, "True $qux$")
}

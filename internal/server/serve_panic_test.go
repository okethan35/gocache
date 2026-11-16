// internal/server/serve_panic_test.go
package server

import (
	"errors"
	"io"
	"testing"

	"gocache/internal/store"
)

// SPEC.md 2b rule 16: a panic while serving one connection closes only that
// connection; the server keeps accepting and other clients keep working.
func TestServePanicClosesOnlyThatConnection(t *testing.T) {
	// Swap dispatch before the server starts, so no goroutine reads it
	// while it changes. "BOOM" panics; everything else behaves normally.
	dispatch = func(s *store.Store, line string) string {
		if line == "BOOM" {
			panic("injected test panic")
		}
		return Dispatch(s, line)
	}
	t.Cleanup(func() { dispatch = Dispatch })

	addr := startServer(t)

	a, ra := dial(t, addr)
	writeLine(t, a, "BOOM")
	if _, err := ra.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Fatalf("expected panicking connection to be closed (EOF), got err=%v", err)
	}

	b, rb := dial(t, addr)
	writeLine(t, b, "SET foo $bar$")
	expectLine(t, rb, "1")
}

// internal/server/server_test.go
package server

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"gocache/internal/store"
)

func TestDispatchSetSuccess(t *testing.T) {
	s := store.New()

	resp := Dispatch(s, "SET foo $bar$ 30")

	if resp != "1\n" {
		t.Errorf("expected %q, got %q", "1\n", resp)
	}
}

func TestDispatchGetFound(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", -1)

	resp := Dispatch(s, "GET foo")

	if resp != "True $bar$\n" {
		t.Errorf("expected %q, got %q", "True $bar$\n", resp)
	}
}

func TestDispatchGetMissing(t *testing.T) {
	s := store.New()

	resp := Dispatch(s, "GET foo")

	if resp != "False $nil$\n" {
		t.Errorf("expected %q, got %q", "False $nil$\n", resp)
	}
}

func TestDispatchDelFound(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", -1)

	resp := Dispatch(s, "DEL foo")

	if resp != "1\n" {
		t.Errorf("expected %q, got %q", "1\n", resp)
	}
}

func TestDispatchDelMissing(t *testing.T) {
	s := store.New()

	resp := Dispatch(s, "DEL foo")

	if resp != "0\n" {
		t.Errorf("expected %q, got %q", "0\n", resp)
	}
}

func TestDispatchInvalidCommand(t *testing.T) {
	s := store.New()

	resp := Dispatch(s, "CREATE foo $bar$ 30")

	if resp != "invalid command\n" {
		t.Errorf("expected %q, got %q", "invalid command\n", resp)
	}
}

func TestHandleConnMultipleCommands(t *testing.T) {
	client, srv := net.Pipe()
	s := store.New()
	r := bufio.NewReader(client)

	go HandleConn(srv, s)

	writeLine(t, client, "SET foo $bar$ 30")
	expectLine(t, r, "1")

	writeLine(t, client, "GET foo")
	expectLine(t, r, "True $bar$")

	writeLine(t, client, "DEL foo")
	expectLine(t, r, "1")

	writeLine(t, client, "GET foo")
	expectLine(t, r, "False $nil$")

	client.Close()
}

func TestHandleConnKeepsGoingAfterInvalidCommand(t *testing.T) {
	client, srv := net.Pipe()
	s := store.New()
	r := bufio.NewReader(client)

	go HandleConn(srv, s)

	writeLine(t, client, "NOPE")
	expectLine(t, r, "invalid command")

	writeLine(t, client, "SET foo $bar$")
	expectLine(t, r, "1")

	client.Close()
}

func TestHandleConnMaxLineLength(t *testing.T) {
	client, srv := net.Pipe()
	s := store.New()
	r := bufio.NewReader(client)

	go HandleConn(srv, s)

	overlong := "SET foo $" + strings.Repeat("a", MaxLineLength) + "$"
	writeLine(t, client, overlong)
	expectLine(t, r, "max line length exceeded")

	// Connection should remain open and usable afterwards.
	writeLine(t, client, "GET foo")
	expectLine(t, r, "False $nil$")

	client.Close()
}

func TestHandleConnClosesOnInterruptedInput(t *testing.T) {
	client, srv := net.Pipe()
	s := store.New()

	done := make(chan struct{})
	go func() {
		HandleConn(srv, s)
		close(done)
	}()

	// Write a partial command with no trailing newline, then hang up.
	if _, err := client.Write([]byte("SET foo $bar")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	client.Close()

	<-done // HandleConn must return (closing srv) instead of hanging.
}

func writeLine(t *testing.T, conn net.Conn, line string) {
	t.Helper()
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func expectLine(t *testing.T, r *bufio.Reader, want string) {
	t.Helper()
	got, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	got = strings.TrimSuffix(got, "\n")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

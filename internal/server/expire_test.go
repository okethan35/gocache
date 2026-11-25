package server

import (
	"bufio"
	"net"
	"testing"

	"gocache/internal/store"
)

// SPEC.md Phase 8 rules 13-16.
func TestDispatchExpireThenTTL(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", -1)
	if resp := Dispatch(s, "EXPIRE foo 30"); resp != "1\n" {
		t.Fatalf("EXPIRE = %q, want 1", resp)
	}
	if resp := Dispatch(s, "TTL foo"); resp != "True 30\n" && resp != "True 29\n" {
		t.Errorf("TTL = %q, want True 30 (or 29 if a second ticked)", resp)
	}
}

func TestDispatchExpireMissing(t *testing.T) {
	s := store.New()
	if resp := Dispatch(s, "EXPIRE missing 30"); resp != "0\n" {
		t.Errorf("got %q, want 0", resp)
	}
}

func TestDispatchExpireZeroDeletes(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", -1)
	if resp := Dispatch(s, "EXPIRE foo 0"); resp != "1\n" {
		t.Fatalf("EXPIRE = %q, want 1", resp)
	}
	if resp := Dispatch(s, "GET foo"); resp != "False $nil$\n" {
		t.Errorf("GET = %q, want False $nil$", resp)
	}
}

func TestDispatchPersist(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", 30)
	if resp := Dispatch(s, "PERSIST foo"); resp != "1\n" {
		t.Fatalf("PERSIST = %q, want 1", resp)
	}
	if resp := Dispatch(s, "TTL foo"); resp != "True -1\n" {
		t.Errorf("TTL = %q, want True -1", resp)
	}
	if resp := Dispatch(s, "PERSIST foo"); resp != "0\n" {
		t.Errorf("second PERSIST = %q, want 0", resp)
	}
	if resp := Dispatch(s, "PERSIST nope"); resp != "0\n" {
		t.Errorf("PERSIST missing = %q, want 0", resp)
	}
}

func TestDispatchExpirePersistInvalid(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", -1)
	for _, line := range []string{
		"EXPIRE foo -1", "EXPIRE foo abc", "EXPIRE foo 3155760001",
		"EXPIRE foo", "EXPIRE", "PERSIST", "PERSIST foo bar",
		"EXPIRE foo 30 extra", "EXPIRE $foo$ 30", "PERSIST $foo$",
		"expire foo 30", "persist foo",
	} {
		if resp := Dispatch(s, line); resp != "invalid command\n" {
			t.Errorf("Dispatch(%q) = %q, want invalid command", line, resp)
		}
	}
	if n, ok := s.TTL("foo"); !ok || n != -1 {
		t.Errorf("TTL = (%d, %v), want (-1, true): invalid commands must not change the store", n, ok)
	}
}

func TestHandleConnExpirePersistInvalidKeepsConnectionOpen(t *testing.T) {
	client, srv := net.Pipe()
	s := store.New()
	r := bufio.NewReader(client)

	go HandleConn(srv, s)

	writeLine(t, client, "EXPIRE foo")
	expectLine(t, r, "invalid command")
	writeLine(t, client, "PERSIST")
	expectLine(t, r, "invalid command")
	writeLine(t, client, "EXPIRE foo -1")
	expectLine(t, r, "invalid command")

	writeLine(t, client, "SET foo $bar$")
	expectLine(t, r, "1")
	writeLine(t, client, "EXPIRE foo 30")
	expectLine(t, r, "1")
	writeLine(t, client, "PERSIST foo")
	expectLine(t, r, "1")
	writeLine(t, client, "GET foo")
	expectLine(t, r, "True $bar$")

	client.Close()
}

package server

import (
	"bufio"
	"net"
	"testing"

	"gocache/internal/store"
)

// SPEC.md Phase 7 rules 10-12.
func TestDispatchTTLWithExpiry(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", 30)
	if resp := Dispatch(s, "TTL foo"); resp != "True 30\n" && resp != "True 29\n" {
		t.Errorf("got %q, want True 30 (or 29 if a second ticked)", resp)
	}
}

func TestDispatchTTLNoExpiry(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", -1)
	if resp := Dispatch(s, "TTL foo"); resp != "True -1\n" {
		t.Errorf("got %q, want %q", resp, "True -1\n")
	}
}

func TestDispatchTTLMissing(t *testing.T) {
	s := store.New()
	if resp := Dispatch(s, "TTL foo"); resp != "False $nil$\n" {
		t.Errorf("got %q, want %q", resp, "False $nil$\n")
	}
}

func TestDispatchTTLExpiredIsMissing(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", 0) // expires immediately
	if resp := Dispatch(s, "TTL foo"); resp != "False $nil$\n" {
		t.Errorf("got %q, want %q", resp, "False $nil$\n")
	}
}

func TestDispatchTTLInvalid(t *testing.T) {
	s := store.New()
	for _, line := range []string{"TTL", "TTL foo bar", "TTL $foo$", "TTL fo$o", "ttl foo"} {
		if resp := Dispatch(s, line); resp != "invalid command\n" {
			t.Errorf("Dispatch(%q) = %q, want invalid command", line, resp)
		}
	}
}

// Rule 14: TTL does not change a live key.
func TestDispatchTTLDoesNotChangeStore(t *testing.T) {
	s := store.New()
	s.Set("foo", "bar", -1)
	Dispatch(s, "TTL foo")
	if v, ok := s.Get("foo"); !ok || v != "bar" {
		t.Errorf("Get = (%q, %v), want (bar, true)", v, ok)
	}
	if n, ok := s.TTL("foo"); !ok || n != -1 {
		t.Errorf("TTL = (%d, %v), want (-1, true)", n, ok)
	}
}

// Rule 13: invalid TTL lines keep the connection open.
func TestHandleConnTTLInvalidKeepsConnectionOpen(t *testing.T) {
	client, srv := net.Pipe()
	s := store.New()
	r := bufio.NewReader(client)

	go HandleConn(srv, s)

	writeLine(t, client, "TTL")
	expectLine(t, r, "invalid command")
	writeLine(t, client, "TTL a b")
	expectLine(t, r, "invalid command")
	writeLine(t, client, "TTL $a$")
	expectLine(t, r, "invalid command")

	writeLine(t, client, "SET foo $bar$")
	expectLine(t, r, "1")
	writeLine(t, client, "TTL foo")
	expectLine(t, r, "True -1")
	writeLine(t, client, "TTL nope")
	expectLine(t, r, "False $nil$")

	client.Close()
}

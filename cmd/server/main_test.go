// cmd/server/main_test.go
package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// SPEC.md Phase 4 rules 8-9: run serves until ctx is cancelled (what a signal
// does in main), then closes open connections, stops the expiry sweep and
// returns nil. No real signal is sent; cancelling ctx stands in for it.
func TestRunStopsOnCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, ln, time.Hour) }()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	r := bufio.NewReader(c)
	if _, err := c.Write([]byte("SET foo $bar$\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got, err := r.ReadString('\n'); err != nil || got != "1\n" {
		t.Fatalf("expected %q, got %q (err=%v)", "1\n", got, err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected run to return nil after cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("run did not return within 2s of cancel")
	}
	if _, err := r.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Errorf("expected the open connection to be closed, got err=%v", err)
	}
}

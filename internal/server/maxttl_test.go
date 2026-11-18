// internal/server/maxttl_test.go
package server

import (
	"testing"

	"gocache/internal/protocol"
	"gocache/internal/store"
)

// The parser cannot import the store (CLAUDE.md), so each keeps its own copy
// of the 100-year limit. This pins them together.
func TestMaxTTLMatchesBetweenProtocolAndStore(t *testing.T) {
	if protocol.MaxTTLSeconds != store.MaxTTLSeconds {
		t.Errorf("protocol.MaxTTLSeconds = %d, store.MaxTTLSeconds = %d; they must match",
			protocol.MaxTTLSeconds, store.MaxTTLSeconds)
	}
}

// SPEC.md 2a rule 16a, on the wire: an over-limit TTL is an invalid command,
// not a key that silently expires at once.
func TestDispatchRejectsOverlongTTL(t *testing.T) {
	s := store.New()
	if resp := Dispatch(s, "SET foo $bar$ 9999999999"); resp != "invalid command\n" {
		t.Errorf("expected %q, got %q", "invalid command\n", resp)
	}
	if resp := Dispatch(s, "GET foo"); resp != "False $nil$\n" {
		t.Errorf("expected nothing stored, got %q", resp)
	}
}

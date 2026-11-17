// cmd/server/main.go
package main

import (
	"log"
	"net"
	"time"

	"gocache/internal/server"
	"gocache/internal/store"
)

const (
	addr = ":6380"

	// expiryInterval is how often the active-expiry sweep runs (SPEC.md Phase 3 rule 7).
	expiryInterval = 100 * time.Millisecond
)

func main() {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}
	log.Printf("gocache listening on %s", addr)

	s := store.New()
	stopExpiry := s.StartExpiry(expiryInterval)
	defer stopExpiry()

	if err := server.Serve(ln, s); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

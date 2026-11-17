// cmd/server/main.go
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
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
	// SIGINT or SIGTERM starts a graceful shutdown (SPEC.md Phase 4 rule 1).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Once the first signal arrives, restore default signal handling, so a
	// second SIGINT or SIGTERM kills the process immediately (rule 2).
	context.AfterFunc(ctx, stop)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}
	if err := run(ctx, ln, expiryInterval); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// run serves on ln with the active-expiry sweep running, until ctx is
// cancelled. Then it shuts down in the order SPEC.md Phase 4 rule 9 gives:
// log "shutting down", stop accepting and drain connections, stop the sweep,
// log "stopped".
func run(ctx context.Context, ln net.Listener, interval time.Duration) error {
	log.Printf("gocache listening on %s", ln.Addr())

	s := store.New()
	stopExpiry := s.StartExpiry(interval)

	logged := make(chan struct{})
	stopLog := context.AfterFunc(ctx, func() {
		log.Print("shutting down")
		close(logged)
	})

	err := server.ServeContext(ctx, ln, s)
	if !stopLog() { // the shutdown log ran (or is running): keep it before "stopped"
		<-logged
	}
	stopExpiry()
	log.Print("stopped")
	return err
}

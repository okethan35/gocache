// cmd/server/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gocache/internal/server"
	"gocache/internal/snapshot"
	"gocache/internal/store"
)

const (
	addr = ":6380"

	// expiryInterval is how often the active-expiry sweep runs (SPEC.md Phase 3 rule 7).
	expiryInterval = 100 * time.Millisecond

	// snapshotPath and snapshotInterval: where and how often the store is
	// saved (SPEC.md Phase 5 rules 2 and 4).
	snapshotPath     = "dump.gocache"
	snapshotInterval = 60 * time.Second
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
	if err := run(ctx, ln, expiryInterval, snapshotInterval, snapshotPath); err != nil {
		log.Fatal(err) // the error says what failed (e.g. loading or saving the snapshot)
	}
}

// run loads the snapshot at path (SPEC.md Phase 5 rules 3, 12, 13),
// then serves on ln with the active-expiry sweep and periodic snapshots
// running, until ctx is cancelled. Then it shuts down in order: log "shutting
// down", stop accepting and drain connections, stop the sweep, stop periodic
// snapshots, save a final snapshot, log "stopped" (Phase 4 rule 9, Phase 5
// rule 2). A failed final save is returned, so main exits with status 1.
func run(ctx context.Context, ln net.Listener, expiryInterval, snapshotInterval time.Duration, path string) error {
	s := store.New()
	recs, err := snapshot.LoadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		log.Printf("no snapshot at %s, starting empty", path)
	case err != nil:
		return fmt.Errorf("loading %w", err) // names the file
	default:
		s.Import(recs)
		log.Printf("loaded %d entries from %s", len(recs), path)
	}

	log.Printf("gocache listening on %s", ln.Addr())
	stopExpiry := s.StartExpiry(expiryInterval)
	stopSnapshots := snapshot.Start(s, path, snapshotInterval)

	logged := make(chan struct{})
	stopLog := context.AfterFunc(ctx, func() {
		log.Print("shutting down")
		close(logged)
	})

	err = server.ServeConfig(ctx, ln, s, server.Config{
		MaxClients:   server.DefaultMaxClients,
		IdleTimeout:  server.DefaultIdleTimeout,
		WriteTimeout: server.DefaultWriteTimeout,
	})
	if !stopLog() { // the shutdown log ran (or is running): keep it before "stopped"
		<-logged
	}
	stopExpiry()
	stopSnapshots()
	recs = s.Export()
	if serr := snapshot.SaveFile(path, recs); serr != nil {
		log.Printf("final snapshot failed: %v", serr)
		if err == nil {
			err = serr
		}
	} else {
		log.Printf("saved %d entries to %s", len(recs), path)
	}
	log.Print("stopped")
	return err
}

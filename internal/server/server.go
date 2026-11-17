// internal/server/server.go
package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"gocache/internal/protocol"
	"gocache/internal/store"
)

// MaxLineLength is the longest command line (excluding the trailing
// newline) the server will accept, per SPEC.md rule 23.
const MaxLineLength = 256

// Accept-error backoff bounds (SPEC.md 2b rule 15).
const (
	minAcceptBackoff = 5 * time.Millisecond
	maxAcceptBackoff = time.Second
)

// shutdownWriteGrace is how long each connection may keep writing once
// shutdown starts (SPEC.md Phase 4 rule 6).
const shutdownWriteGrace = time.Second

// Serve accepts connections on ln and serves each one in its own goroutine
// (SPEC.md 2b rule 1), all sharing s. Once ln is closed it stops accepting,
// waits for every open connection to end, and returns nil. It is ServeContext
// with a context that is never cancelled (SPEC.md Phase 4 rule 8).
func Serve(ln net.Listener, s *store.Store) error {
	return ServeContext(context.Background(), ln, s)
}

// ServeContext serves like Serve, and cancelling ctx starts a graceful
// shutdown (SPEC.md Phase 4): stop accepting (rule 3), wake every connection
// blocked on a read so it closes (rule 4) while letting a command already
// running finish and write its response (rule 5), and give each connection
// shutdownWriteGrace to finish writing (rule 6). It returns nil once every
// connection has ended, so nothing it started outlives it. Accept errors other
// than a closed listener are logged and retried with exponential backoff that
// resets after the next successful accept (2b rule 15); a shutdown interrupts
// the backoff (Phase 4 rule 10).
func ServeContext(ctx context.Context, ln net.Listener, s *store.Store) error {
	tracker := &connTracker{open: make(map[net.Conn]struct{})}

	quit := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			ln.Close()
			tracker.shutdown()
		case <-quit:
		}
	}()
	defer func() {
		close(quit)
		<-watcherDone
	}()

	var conns sync.WaitGroup
	backoff := time.Duration(0)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				conns.Wait()
				return nil
			}
			if backoff == 0 {
				backoff = minAcceptBackoff
			} else {
				backoff = min(backoff*2, maxAcceptBackoff)
			}
			log.Printf("accept error: %v; retrying in %v", err, backoff)
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done(): // the watcher closes ln, so the next Accept returns ErrClosed
				timer.Stop()
			}
			continue
		}
		backoff = 0
		tracker.add(conn)
		conns.Go(func() {
			defer tracker.remove(conn)
			// Shutdown has started the moment ctx is cancelled; ctx.Err is set
			// synchronously by cancel, before the watcher goroutine runs.
			serveConn(conn, s, func() bool { return ctx.Err() != nil })
		})
	}
}

// Config limits connections (SPEC.md Phase 6). CHECKPOINT: ignored by the stub.
type Config struct {
	MaxClients  int
	IdleTimeout time.Duration
}

// ServeConfig CHECKPOINT: stub that ignores cfg.
func ServeConfig(ctx context.Context, ln net.Listener, s *store.Store, cfg Config) error {
	return ServeContext(ctx, ln, s)
}

// connTracker remembers open connections so shutdown can reach them.
type connTracker struct {
	mu           sync.Mutex
	open         map[net.Conn]struct{}
	shuttingDown bool
}

// add registers conn. A connection accepted just as shutdown starts gets the
// shutdown deadlines immediately.
func (t *connTracker) add(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.open[conn] = struct{}{}
	if t.shuttingDown {
		setShutdownDeadlines(conn)
	}
}

func (t *connTracker) remove(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.open, conn)
}

// shutdown applies the shutdown deadlines to every open connection.
func (t *connTracker) shutdown() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shuttingDown = true
	for conn := range t.open {
		setShutdownDeadlines(conn)
	}
}

// setShutdownDeadlines makes any read on conn fail now, which ends HandleConn
// through its normal read-error path, while a response being written still has
// shutdownWriteGrace to finish.
func setShutdownDeadlines(conn net.Conn) {
	now := time.Now()
	conn.SetReadDeadline(now)
	conn.SetWriteDeadline(now.Add(shutdownWriteGrace))
}

// serveConn runs HandleConn for one connection in the goroutine Serve starts.
// A panic is recovered and logged with its stack trace, so it closes only this
// connection (HandleConn's deferred Close still runs while unwinding) and the
// server keeps running (SPEC.md 2b rule 16). Fatal runtime errors cannot be
// recovered (rule 17).
func serveConn(conn net.Conn, s *store.Store, stopping func() bool) {
	remote := conn.RemoteAddr()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic serving %v: %v\n%s", remote, r, debug.Stack())
		}
	}()
	handleConn(conn, s, stopping)
}

// dispatch is what HandleConn calls to execute a line. It is a variable only
// so tests can inject a panic; like Store.now, it is set before any goroutine
// starts and never reassigned while the server runs.
var dispatch = Dispatch

// HandleConn serves commands on a single connection until the client
// disconnects, a read is interrupted, or a write fails. It never stops for a
// shutdown; ServeContext runs each connection through handleConn instead, in
// its own goroutine. Neither starts goroutines itself.
func HandleConn(conn net.Conn, s *store.Store) {
	handleConn(conn, s, func() bool { return false })
}

// handleConn is HandleConn with a shutdown check: once stopping reports true,
// no further line is executed, even one already received and buffered, and
// the connection closes (SPEC.md Phase 4 rule 5). A line whose dispatch has
// already begun runs to completion and its response is written.
func handleConn(conn net.Conn, s *store.Store, stopping func() bool) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		line, tooLong, err := readLine(reader)
		if err != nil {
			// Connection closed or interrupted mid-command: discard
			// whatever was read and close (SPEC.md rule 22).
			return
		}

		if stopping() {
			return // discard this line and anything after it, unexecuted
		}

		if tooLong {
			if _, werr := io.WriteString(conn, "max line length exceeded\n"); werr != nil {
				return
			}
			continue
		}

		if _, werr := io.WriteString(conn, dispatch(s, line)); werr != nil {
			return
		}
	}
}

// Dispatch parses a single command line and executes it against s,
// returning the exact wire-format response (including trailing "\n") per
// SPEC.md's Responses section.
func Dispatch(s *store.Store, line string) string {
	cmd, err := protocol.ParseCommand(line)
	if err != nil {
		return "invalid command\n"
	}

	switch cmd.Op {
	case "SET":
		return fmt.Sprintf("%d\n", s.Set(cmd.Key, cmd.Value, cmd.TTL))
	case "GET":
		if value, ok := s.Get(cmd.Key); ok {
			return fmt.Sprintf("True $%s$\n", value)
		}
		return "False $nil$\n"
	case "DEL":
		return fmt.Sprintf("%d\n", s.Del(cmd.Key))
	default:
		return "invalid command\n"
	}
}

// readLine reads one '\n'-terminated line, not including the newline.
// If the line exceeds MaxLineLength characters, it is fully drained from
// the reader (so parsing can resume with the next line) and tooLong is
// reported true instead of returning an error. err is non-nil only when
// the underlying read failed or hit EOF before a newline was found.
func readLine(r *bufio.Reader) (line string, tooLong bool, err error) {
	buf := make([]byte, 0, MaxLineLength)
	count := 0

	for {
		b, rerr := r.ReadByte()
		if rerr != nil {
			return "", false, rerr
		}
		if b == '\n' {
			break
		}
		count++
		if count <= MaxLineLength {
			buf = append(buf, b)
		}
	}

	if count > MaxLineLength {
		return "", true, nil
	}
	return string(buf), false, nil
}

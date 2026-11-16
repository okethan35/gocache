// internal/server/server.go
package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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

// Serve accepts connections on ln and serves each one in its own goroutine
// (SPEC.md 2b rule 1), all sharing s. It returns nil once ln is closed. Any
// other accept error is logged and retried with exponential backoff, which
// resets after the next successful accept (SPEC.md 2b rule 15).
func Serve(ln net.Listener, s *store.Store) error {
	backoff := time.Duration(0)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			if backoff == 0 {
				backoff = minAcceptBackoff
			} else {
				backoff = min(backoff*2, maxAcceptBackoff)
			}
			log.Printf("accept error: %v; retrying in %v", err, backoff)
			time.Sleep(backoff)
			continue
		}
		backoff = 0
		go serveConn(conn, s)
	}
}

// serveConn runs HandleConn for one connection in the goroutine Serve starts.
// CHECKPOINT: no panic recovery yet (SPEC.md 2b rule 16).
func serveConn(conn net.Conn, s *store.Store) {
	HandleConn(conn, s)
}

// dispatch is what HandleConn calls to execute a line. It is a variable only
// so tests can inject a panic; like Store.now, it is set before any goroutine
// starts and never reassigned while the server runs.
var dispatch = Dispatch

// HandleConn serves commands on a single connection until the client
// disconnects, a read is interrupted, or a write fails. Serve runs each
// connection's HandleConn in its own goroutine; HandleConn itself never
// starts goroutines.
func HandleConn(conn net.Conn, s *store.Store) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		line, tooLong, err := readLine(reader)
		if err != nil {
			// Connection closed or interrupted mid-command: discard
			// whatever was read and close (SPEC.md rule 22).
			return
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

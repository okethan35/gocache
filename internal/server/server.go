// internal/server/server.go
package server

import (
	"bufio"
	"fmt"
	"io"
	"net"

	"gocache/internal/protocol"
	"gocache/internal/store"
)

// MaxLineLength is the longest command line (excluding the trailing
// newline) the server will accept, per SPEC.md rule 23.
const MaxLineLength = 256

// HandleConn serves commands on a single connection until the client
// disconnects, a read is interrupted, or a write fails. It never spawns
// goroutines: Phase 2a is single-connection, no concurrency.
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

		if _, werr := io.WriteString(conn, Dispatch(s, line)); werr != nil {
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

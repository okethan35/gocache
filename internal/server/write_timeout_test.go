// internal/server/write_timeout_test.go
package server

import (
	"net"
	"strings"
	"testing"
	"time"
)

// SPEC.md Phase 6 rule 7: a client that sends commands but never reads the
// replies is closed once a reply misses the write timeout, freeing its slot.
// With MaxClients 1, a second client can only get in after that happens.
func TestSlowReaderIsClosed(t *testing.T) {
	addr, _ := startServerConfig(t, Config{MaxClients: 1, WriteTimeout: 100 * time.Millisecond})

	slow, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { slow.Close() })
	// A small receive buffer fills sooner. Each GET line is 6 bytes and its
	// reply about 240, so the replies quickly back up into the server.
	slow.(*net.TCPConn).SetReadBuffer(4096)
	value := strings.Repeat("x", 230)
	if _, err := slow.Write([]byte("SET k $" + value + "$\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	flood := []byte(strings.Repeat("GET k\n", 10000))
	go func() {
		slow.SetWriteDeadline(time.Now().Add(3 * time.Second))
		for {
			if _, err := slow.Write(flood); err != nil {
				return // the server closed us (or the deadline passed)
			}
		}
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.SetDeadline(time.Now().Add(500 * time.Millisecond))
			c.Write([]byte("GET k\n"))
			buf := make([]byte, 512)
			n, _ := c.Read(buf)
			c.Close()
			if got := string(buf[:n]); strings.HasPrefix(got, "True $") {
				return // the slow reader's slot was freed
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("slow reader still holds the only slot 3s later")
		}
	}
}

// deadlineConn records write deadlines set on it.
type deadlineConn struct {
	net.Conn
	writeDeadlines []time.Time
}

func (c *deadlineConn) SetWriteDeadline(d time.Time) error {
	c.writeDeadlines = append(c.writeDeadlines, d)
	return nil
}

func (c *deadlineConn) SetReadDeadline(time.Time) error { return nil }

// SPEC.md Phase 6 rule 8: once shutdown has started, arming the write
// timeout must not replace the shutdown write deadline.
func TestArmWriteAfterShutdownKeepsShutdownDeadline(t *testing.T) {
	conn := &deadlineConn{}
	tracker := &connTracker{open: make(map[net.Conn]struct{})}
	tracker.tryAdd(conn, 1)

	tracker.armWrite(conn, time.Hour) // before shutdown: sets a deadline
	if len(conn.writeDeadlines) != 1 {
		t.Fatalf("expected armWrite to set a write deadline before shutdown, got %d calls", len(conn.writeDeadlines))
	}

	tracker.shutdown() // sets the shutdown write deadline
	if len(conn.writeDeadlines) != 2 {
		t.Fatalf("expected shutdown to set a write deadline, got %d calls", len(conn.writeDeadlines))
	}
	shutdownDeadline := conn.writeDeadlines[1]

	tracker.armWrite(conn, time.Hour) // after shutdown: must do nothing
	if len(conn.writeDeadlines) != 2 {
		t.Errorf("expected armWrite after shutdown to leave the shutdown deadline %v alone, but it set %v",
			shutdownDeadline, conn.writeDeadlines[len(conn.writeDeadlines)-1])
	}
}

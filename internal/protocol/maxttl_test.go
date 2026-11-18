// internal/protocol/maxttl_test.go
package protocol

import (
	"errors"
	"testing"
)

// SPEC.md 2a rule 16a: a TTL up to 100 years parses; anything above it is an
// invalid command, including values large enough to overflow time.Duration.
func TestParseSetMaxTTL(t *testing.T) {
	cmd, err := ParseCommand("SET foo $bar$ 3155760000")
	if err != nil || cmd.TTL != MaxTTLSeconds {
		t.Errorf("expected a TTL of exactly %d to parse, got cmd=%+v err=%v", MaxTTLSeconds, cmd, err)
	}
	for _, line := range []string{
		"SET foo $bar$ 3155760001",          // one second over
		"SET foo $bar$ 9999999999",          // ~317 years: used to overflow and expire at once
		"SET foo $bar$ 9223372036854775807", // max int64
	} {
		if _, err := ParseCommand(line); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%q: expected ErrInvalidCommand, got %v", line, err)
		}
	}
}

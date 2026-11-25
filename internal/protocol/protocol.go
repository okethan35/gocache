// Package protocol parses the gocache wire format into commands.
// It holds wire-format parsing only: it contains no store logic and no
// networking code.
package protocol

import (
	"errors"
	"strconv"
	"strings"
)

type Command struct {
	Op    string
	Key   string
	Value string // only meaningful for SET
	TTL   int    // SET's TTL (-1 if not provided) or EXPIRE's seconds
}

// MaxTTLSeconds is the largest TTL a SET may ask for: 100 years (SPEC.md 2a
// rule 16a). Anything larger is an invalid command. It must equal
// store.MaxTTLSeconds; internal/protocol cannot import the store, so a test in
// internal/server keeps the two in step.
const MaxTTLSeconds = 3155760000

var ErrInvalidCommand = errors.New("invalid command")

// ParseCommand parses a single wire-protocol line (without the trailing
// newline) per SPEC.md's "2a - Protocol" section: SET/GET/DEL/TTL only, command
// word case-sensitive and fully capital, key is a single word, SET values
// wrapped in exactly one pair of '$', GET/DEL/TTL forbid '$' entirely.
func ParseCommand(line string) (Command, error) {
	if line == "" {
		return Command{}, ErrInvalidCommand
	}

	op, rest, hasRest := strings.Cut(line, " ")
	switch op {
	case "SET", "GET", "DEL", "TTL", "EXPIRE", "PERSIST":
	default:
		return Command{}, ErrInvalidCommand
	}
	if !hasRest || rest == "" {
		return Command{}, ErrInvalidCommand
	}

	switch op {
	case "SET":
		return parseSet(rest)
	case "EXPIRE":
		return parseExpire(rest)
	}
	return parseGetDel(op, rest)
}

func parseGetDel(op, rest string) (Command, error) {
	fields := strings.Fields(rest)
	if len(fields) != 1 {
		return Command{}, ErrInvalidCommand
	}
	key := fields[0]
	if strings.Contains(key, "$") {
		return Command{}, ErrInvalidCommand
	}
	return Command{Op: op, Key: key, TTL: -1}, nil
}

// parseExpire parses "<key> <seconds>" (SPEC.md Phase 8 rules 13, 14, 16).
func parseExpire(rest string) (Command, error) {
	fields := strings.Fields(rest)
	if len(fields) != 2 || strings.Contains(rest, "$") {
		return Command{}, ErrInvalidCommand
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || n < 0 || n > MaxTTLSeconds {
		return Command{}, ErrInvalidCommand
	}
	return Command{Op: "EXPIRE", Key: fields[0], TTL: n}, nil
}

func parseSet(rest string) (Command, error) {
	key, remainder, hasRemainder := strings.Cut(rest, " ")
	if key == "" || strings.Contains(key, "$") {
		return Command{}, ErrInvalidCommand
	}
	if !hasRemainder || remainder == "" || remainder[0] != '$' {
		return Command{}, ErrInvalidCommand
	}

	closeOffset := strings.IndexByte(remainder[1:], '$')
	if closeOffset == -1 {
		return Command{}, ErrInvalidCommand
	}
	closeIdx := closeOffset + 1

	if strings.Count(remainder, "$") != 2 {
		return Command{}, ErrInvalidCommand
	}

	value := remainder[1:closeIdx]
	after := strings.TrimSpace(remainder[closeIdx+1:])

	ttl := -1
	if after != "" {
		fields := strings.Fields(after)
		if len(fields) != 1 {
			return Command{}, ErrInvalidCommand
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			return Command{}, ErrInvalidCommand
		}
		if n > MaxTTLSeconds {
			return Command{}, ErrInvalidCommand
		}
		ttl = n
	}

	return Command{Op: "SET", Key: key, Value: value, TTL: ttl}, nil
}

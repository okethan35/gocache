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
	TTL   int    // only meaningful for SET, -1 if not provided
}

var ErrInvalidCommand = errors.New("invalid command")

// ParseCommand parses a single wire-protocol line (without the trailing
// newline) per SPEC.md's "2a - Protocol" section: SET/GET/DEL only, command
// word case-sensitive and fully capital, key is a single word, SET values
// wrapped in exactly one pair of '$', GET/DEL forbid '$' entirely.
func ParseCommand(line string) (Command, error) {
	if line == "" {
		return Command{}, ErrInvalidCommand
	}

	op, rest, hasRest := strings.Cut(line, " ")
	if op != "SET" && op != "GET" && op != "DEL" {
		return Command{}, ErrInvalidCommand
	}
	if !hasRest || rest == "" {
		return Command{}, ErrInvalidCommand
	}

	if op == "SET" {
		return parseSet(rest)
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
		ttl = n
	}

	return Command{Op: "SET", Key: key, Value: value, TTL: ttl}, nil
}

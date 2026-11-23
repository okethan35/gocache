package protocol

import "testing"

// SPEC.md Phase 7 rules 9 and 13.
func TestParseTTLCommand(t *testing.T) {
	cmd, err := ParseCommand("TTL foo")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cmd.Op != "TTL" || cmd.Key != "foo" {
		t.Errorf("parsed command doesn't match expected, got %v", cmd)
	}
}

func TestParseTTLInvalid(t *testing.T) {
	for _, line := range []string{
		"TTL",
		"TTL ",
		"TTL foo bar",
		"TTL $foo$",
		"TTL fo$o",
		"ttl foo",
		"Ttl foo",
	} {
		if _, err := ParseCommand(line); err != ErrInvalidCommand {
			t.Errorf("ParseCommand(%q) err = %v, want ErrInvalidCommand", line, err)
		}
	}
}

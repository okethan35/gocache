package protocol

import "testing"

// SPEC.md Phase 8 rules 13, 14 and 16.
func TestParseExpireCommand(t *testing.T) {
	for line, want := range map[string]int{
		"EXPIRE foo 30":         30,
		"EXPIRE foo 0":          0,
		"EXPIRE foo 3155760000": 3155760000,
	} {
		cmd, err := ParseCommand(line)
		if err != nil {
			t.Fatalf("ParseCommand(%q) err = %v", line, err)
		}
		if cmd.Op != "EXPIRE" || cmd.Key != "foo" || cmd.TTL != want {
			t.Errorf("ParseCommand(%q) = %+v, want EXPIRE foo ttl %d", line, cmd, want)
		}
	}
}

func TestParseExpireInvalid(t *testing.T) {
	for _, line := range []string{
		"EXPIRE",
		"EXPIRE ",
		"EXPIRE foo",
		"EXPIRE foo ",
		"EXPIRE foo -1",
		"EXPIRE foo abc",
		"EXPIRE foo 1.5",
		"EXPIRE foo 3155760001",
		"EXPIRE foo 99999999999999999999",
		"EXPIRE foo 30 extra",
		"EXPIRE $foo$ 30",
		"EXPIRE fo$o 30",
		"EXPIRE foo $30$",
		"expire foo 30",
		"Expire foo 30",
	} {
		if _, err := ParseCommand(line); err != ErrInvalidCommand {
			t.Errorf("ParseCommand(%q) err = %v, want ErrInvalidCommand", line, err)
		}
	}
}

func TestParsePersistCommand(t *testing.T) {
	cmd, err := ParseCommand("PERSIST foo")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cmd.Op != "PERSIST" || cmd.Key != "foo" {
		t.Errorf("parsed command doesn't match expected, got %v", cmd)
	}
}

func TestParsePersistInvalid(t *testing.T) {
	for _, line := range []string{
		"PERSIST",
		"PERSIST ",
		"PERSIST foo bar",
		"PERSIST $foo$",
		"PERSIST fo$o",
		"persist foo",
		"Persist foo",
	} {
		if _, err := ParseCommand(line); err != ErrInvalidCommand {
			t.Errorf("ParseCommand(%q) err = %v, want ErrInvalidCommand", line, err)
		}
	}
}

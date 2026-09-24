package protocol

import "testing"

func TestParseSetCommand(t *testing.T) {
	cmd, err := ParseCommand("SET foo $bar$ 30")

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cmd.Op != "SET" || cmd.Key != "foo" || cmd.Value != "bar" || cmd.TTL != 30 {
		t.Errorf("parsed command doesn't match expected, got %v", cmd)
	}
}

func TestParseGetCommand(t *testing.T) {
	cmd, err := ParseCommand("GET foo")

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cmd.Op != "GET" || cmd.Key != "foo" {
		t.Errorf("parsed command doesn't match expected, got %v", cmd)
	}
}

func TestParseSetNoTTL(t *testing.T) {
	cmd, err := ParseCommand("SET foo $bar$")

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cmd.TTL != -1 {
		t.Errorf("parsed command doesn't match expected, got %v", cmd)
	}
}

func TestLowercaseCommand(t *testing.T) {
	_, err := ParseCommand("set foo $bar$ 30")

	if err == nil {
		t.Errorf("expected an error but received none")
	}
}

func TestUppercaseKeyValue(t *testing.T) {
	cmd, err := ParseCommand("SET FOO $BAR$ 30")

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cmd.Key != "FOO" || cmd.Value != "BAR" {
		t.Errorf("parsed command doesn't match expected, got %v", cmd)
	}
}

func TestMultipleWordKey(t *testing.T) {
	_, err := ParseCommand("SET foo bar $bar$ 30")

	if err == nil {
		t.Errorf("expected an error, but received none")
	}
}

func TestNoWrapperValue(t *testing.T) {
	_, err := ParseCommand("SET foo bar 30")

	if err == nil {
		t.Errorf("expected an error, but received none")
	}
}

func TestNoKey(t *testing.T) {
	_, err := ParseCommand("GET")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestSetNoValue(t *testing.T) {
	_, err := ParseCommand("SET foo 30")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestNonIntTTL(t *testing.T) {
	_, err := ParseCommand("SET foo $bar$ bar")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestUnknownCommand(t *testing.T) {
	_, err := ParseCommand("CREATE foo $bar$ 30")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestEmptyLine(t *testing.T) {
	_, err := ParseCommand("")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestSetExtraWrapper(t *testing.T) {
	_, err := ParseCommand("SET fo$o $bar$ 30")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestGetExtraWrapper(t *testing.T) {
	_, err := ParseCommand("GET foo$")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestDelExtraWrapper(t *testing.T) {
	_, err := ParseCommand("DEL foo$")

	if err == nil{
		t.Errorf("expected an error, but received none")
	}
}

func TestSetEmptyValue(t *testing.T) {
	cmd, err := ParseCommand("SET foo $$ 30")

	if err != nil{
		t.Errorf("expected no error, got %v", err)
	}

	if cmd.Value != "" {
		t.Errorf("parsed command doesn't match expected, got %v", cmd)
	}
}


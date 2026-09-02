// internal/store/store_test.go
package store

import (
	"testing"
	"time"
)

func TestGetMissingKey(t *testing.T) {
	s := New() 

	value, ok := s.Get("foo") // try to GET key that doesn't exist

	if ok {
		t.Errorf("expected to get ok=false when fetching missing key, but got ok=true")
	}
	if value != "" {
		t.Errorf("expected empty value on miss, got %q", value)
	}
}

func TestGetExpiredKey(t *testing.T) {
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime } // freeze time

	s.Set("foo", "bar", 1) // expires 1 second after currentTime

	currentTime = currentTime.Add(2 * time.Second) // jump forward instantly

	value, ok := s.Get("foo")
	if ok {
		t.Errorf("expected key to be expired, but got ok=true")
	}
	if value != "" {
		t.Errorf("expected empty value on expired key, got %q", value)
	}
}

func TestSetImmediatelyExpiringKey(t *testing.T) {
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime } // freeze time

	s.Set("foo", "bar", 0) // expires immediately

	value, ok := s.Get("foo")
	if ok {
		t.Errorf("expected key to be expired, but got ok=true")
	}
	if value != "" {
		t.Errorf("expected empty value on expired key, got %q", value)
	}
}

func TestNegativeValueExpiry(t *testing.T) {
	s := New()
	s.Set("foo", "bar", -2) // should get clamped to -1

	e := s.data["foo"]

	if e.hasExpiry {
		t.Errorf("expected hasExpiry=false for no-expiration, got true (expiresAt=%v)", e)
	}
}

func TestNegativeOneExpiry(t *testing.T) {
	s := New()
	s.Set("foo", "bar", -1) 

	e := s.data["foo"]

	if e.hasExpiry {
		t.Errorf("expected hasExpiry=false for no-expiration, got true (expiresAt=%v)", e.expiresAt)
	}
}

func TestSetOverwritesExistingKey(t *testing.T) {
	currentTime := time.Now()
	s := New()
	s.now = func() time.Time { return currentTime } // freeze time

	s.Set("foo", "bar", 100) // first write: value "bar", ttl 100s
	s.Set("foo", "baz", 5)   // second write: same key, new value + new ttl

	value, ok := s.Get("foo")

	if !ok {
		t.Errorf("expected key to exist after overwrite, but got ok=false")
	}

	if value != "baz" {
		t.Errorf("expected value to be fully replaced to %q, got %q", "baz", value)
	}

	e := s.data["foo"]
	expectedExpiry := currentTime.Add(5 * time.Second)

	if !e.expiresAt.Equal(expectedExpiry) {
		t.Errorf("expected expiresAt to be replaced to %v (from the second Set), got %v", expectedExpiry, e.expiresAt)
	}
}

func TestDelMissingKey(t *testing.T) {
	s := New()

	res := s.Del("foo")

	if res != 0 {
		t.Errorf("expected response to be 0, but got %v", res)
	}
}

func TestSetEmptyKey(t *testing.T){
	s := New()
	res := s.Set("", "bar", 3)

	if res != 0{
		t.Errorf("expected response ot be 0, but got %v", res)
	}
}

func TestDelEmptyKey(t *testing.T){
	s := New()
	res := s.Del("")

	if res != 0{
		t.Errorf("expected response ot be 0, but got %v", res)
	}
}

func TestSetValidKeyReturnsOne(t *testing.T) {
	s := New()

	res := s.Set("foo", "bar", 10)

	if res != 1 {
		t.Errorf("expected Set on a valid key to return 1, got %v", res)
	}
}
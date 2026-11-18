// internal/store/import_emptykey_test.go
package store

import "testing"

// SPEC.md Phase 5 rule 7 and Phase 1 rule 9: Import never stores an empty key.
func TestImportSkipsEmptyKey(t *testing.T) {
	s := New()
	s.Import([]Record{{Key: "", Value: "ghost"}, {Key: "real", Value: "v"}})
	if _, ok := s.data[""]; ok {
		t.Errorf("expected an empty key not to be imported")
	}
	if v, ok := s.Get("real"); !ok || v != "v" {
		t.Errorf("expected the other record to load, got (%q, %v)", v, ok)
	}
}

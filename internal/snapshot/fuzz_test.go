// internal/snapshot/fuzz_test.go
package snapshot

import (
	"bytes"
	"testing"
	"time"

	"gocache/internal/store"
)

// FuzzRead feeds Read arbitrary bytes (SPEC.md Phase 5 rules 9 and 13): it
// must never panic, and anything it accepts must survive Write then Read
// unchanged. Plain `go test` runs only the seed corpus; run the fuzzer with
//
//	go test -run '^$' -fuzz FuzzRead -fuzztime 30s ./internal/snapshot
func FuzzRead(f *testing.F) {
	for _, recs := range [][]store.Record{
		nil,
		{{Key: "k", Value: "v"}},
		{{Key: "a", Value: ""}, {Key: "b", Value: "x", HasExpiry: true, ExpiresAt: time.Unix(0, 42)}},
	} {
		var buf bytes.Buffer
		if err := Write(&buf, recs); err != nil {
			f.Fatalf("seed Write failed: %v", err)
		}
		f.Add(buf.Bytes())
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		recs, err := Read(bytes.NewReader(b))
		if err != nil {
			return // rejected input is fine; panicking is not
		}
		var buf bytes.Buffer
		if err := Write(&buf, recs); err != nil {
			t.Fatalf("Write of accepted records failed: %v", err)
		}
		again, err := Read(&buf)
		if err != nil {
			t.Fatalf("re-reading accepted records failed: %v", err)
		}
		if len(again) != len(recs) {
			t.Fatalf("round trip changed record count: %d -> %d", len(recs), len(again))
		}
		for i := range recs {
			g, w := again[i], recs[i]
			if g.Key != w.Key || g.Value != w.Value || g.HasExpiry != w.HasExpiry || !g.ExpiresAt.Equal(w.ExpiresAt) {
				t.Fatalf("round trip changed record %d: %+v -> %+v", i, w, g)
			}
		}
	})
}

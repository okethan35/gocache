// internal/snapshot/snapshot_test.go
package snapshot

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gocache/internal/store"
)

func sampleRecords() []store.Record {
	return []store.Record{
		{Key: "forever", Value: "a"},
		{Key: "empty", Value: ""},
		{Key: "ttl", Value: "b", HasExpiry: true, ExpiresAt: time.Unix(0, 1_900_000_000_123_456_789)},
	}
}

func encode(t *testing.T, recs []store.Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, recs); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	return buf.Bytes()
}

func sameRecords(t *testing.T, got, want []store.Record) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d records, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Key != w.Key || g.Value != w.Value || g.HasExpiry != w.HasExpiry || !g.ExpiresAt.Equal(w.ExpiresAt) {
			t.Errorf("record %d: expected %+v, got %+v", i, w, g)
		}
	}
}

// SPEC.md Phase 5 rules 5 and 8: what Write encodes, Read decodes exactly,
// including empty values, entries with no expiry, and nanosecond expiries.
func TestRoundTrip(t *testing.T) {
	want := sampleRecords()
	got, err := Read(bytes.NewReader(encode(t, want)))
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	sameRecords(t, got, want)
}

func TestRoundTripEmpty(t *testing.T) {
	got, err := Read(bytes.NewReader(encode(t, nil)))
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no records, got %+v", got)
	}
}

// SPEC.md Phase 5 rule 8: the exact layout, so the format is pinned down.
func TestLayout(t *testing.T) {
	b := encode(t, []store.Record{{Key: "k", Value: "vv", HasExpiry: true, ExpiresAt: time.Unix(0, 7)}})
	want := []byte("GOCACHE1")
	want = binary.LittleEndian.AppendUint32(want, 1) // version
	want = binary.LittleEndian.AppendUint64(want, 1) // count
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = append(want, 'k')
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = append(want, 'v', 'v')
	want = append(want, 1)
	want = binary.LittleEndian.AppendUint64(want, 7)
	want = binary.LittleEndian.AppendUint32(want, crc32.ChecksumIEEE(want))
	if !bytes.Equal(b, want) {
		t.Errorf("layout mismatch:\n got  %x\n want %x", b, want)
	}
}

// withChecksum replaces the trailing checksum so a test can change a field
// and reach the check for that field, not the checksum check.
func withChecksum(b []byte) []byte {
	body := b[:len(b)-4]
	return binary.LittleEndian.AppendUint32(append([]byte{}, body...), crc32.ChecksumIEEE(body))
}

// SPEC.md Phase 5 rule 9: every kind of damage is reported as an error.
func TestReadRejectsCorruption(t *testing.T) {
	good := encode(t, sampleRecords())

	flipped := append([]byte{}, good...)
	flipped[20] ^= 0xFF

	badMagic := append([]byte{}, good...)
	copy(badMagic, "NOTCACHE")
	badMagic = withChecksum(badMagic)

	badVersion := append([]byte{}, good...)
	binary.LittleEndian.PutUint32(badVersion[8:], 2)
	badVersion = withChecksum(badVersion)

	extra := withChecksum(append(append([]byte{}, good[:len(good)-4]...), 0xAB, 0, 0, 0, 0))

	cases := map[string][]byte{
		"flipped byte":   flipped,
		"truncated":      good[:len(good)-7],
		"bad magic":      badMagic,
		"bad version":    badVersion,
		"leftover bytes": extra,
		"empty file":     {},
	}
	for name, b := range cases {
		if _, err := Read(bytes.NewReader(b)); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}

// SPEC.md Phase 5 rule 10: SaveFile then LoadFile round-trips, replaces an
// existing snapshot, and leaves no temporary file behind.
func TestSaveLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.gocache")

	if err := SaveFile(path, []store.Record{{Key: "old", Value: "x"}}); err != nil {
		t.Fatalf("first SaveFile failed: %v", err)
	}
	want := sampleRecords()
	if err := SaveFile(path, want); err != nil {
		t.Fatalf("second SaveFile failed: %v", err)
	}

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	sameRecords(t, got, want)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only the snapshot in the directory, got %v", names)
	}
}

// SPEC.md Phase 5 rule 12: a missing file is reported as fs.ErrNotExist, so
// the caller can tell "first run" apart from "corrupt".
func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "nope.gocache"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected an fs.ErrNotExist error, got %v", err)
	}
}

// SPEC.md Phase 5 rule 2: the snapshot loop saves on each tick, and stop waits
// for it. The test owns the tick channel, so it never sleeps.
func TestSnapshotLoopSavesOnTick(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.gocache")
	s := store.New()
	s.Set("foo", "bar", -1)

	ticks := make(chan time.Time)
	stop := startLoop(s, path, ticks)
	select {
	case ticks <- time.Now():
	case <-time.After(2 * time.Second): // a deadline, so a broken loop fails instead of hanging
		t.Fatal("snapshot loop never took the tick")
	}
	stop()

	got, err := LoadFile(path)
	if err != nil {
		t.Fatalf("expected a snapshot after one tick, LoadFile failed: %v", err)
	}
	sameRecords(t, got, []store.Record{{Key: "foo", Value: "bar"}})
}

// SPEC.md Phase 5 rule 9: an entry with hasExpiry 0 must have expiresAt 0,
// and hasExpiry must be 0 or 1.
func TestReadRejectsBadExpiryFields(t *testing.T) {
	good := encode(t, []store.Record{{Key: "k", Value: "v"}})
	// Entry layout after the 20-byte header: keyLen(4) 'k' valLen(4) 'v' hasExpiry(1) expiresAt(8).
	const hasExpiryAt = 20 + 4 + 1 + 4 + 1
	const expiresAtAt = hasExpiryAt + 1

	nonzero := append([]byte{}, good...)
	binary.LittleEndian.PutUint64(nonzero[expiresAtAt:], 99)
	nonzero = withChecksum(nonzero)

	badFlag := append([]byte{}, good...)
	badFlag[hasExpiryAt] = 2
	badFlag = withChecksum(badFlag)

	for name, b := range map[string][]byte{"nonzero expiresAt with no expiry": nonzero, "hasExpiry 2": badFlag} {
		if _, err := Read(bytes.NewReader(b)); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}

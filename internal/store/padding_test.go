// internal/store/padding_test.go
package store

import (
	"sort"
	"testing"
	"unsafe"
)

// SPEC.md 2b rule 13a: each shard's mutex is on its own cache line. Many CPUs
// fetch cache lines in adjacent pairs, so this checks the stronger property:
// between the end of one shard's Store and the start of the next there are at
// least 128 bytes, so no two shards ever share a 64-byte line or a 128-byte pair.
func TestShardsDoNotShareCacheLines(t *testing.T) {
	s := NewSharded(5)
	addrs := make([]uintptr, len(s.shards))
	for i, sh := range s.shards {
		addrs[i] = uintptr(unsafe.Pointer(sh))
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })

	size := unsafe.Sizeof(Store{})
	for i := 1; i < len(addrs); i++ {
		gap := addrs[i] - (addrs[i-1] + size)
		if addrs[i] < addrs[i-1]+size || gap < 128 {
			t.Errorf("shards %d and %d: only %d bytes between them (Store is %d bytes), want at least 128",
				i-1, i, int(addrs[i])-int(addrs[i-1]+size), size)
		}
	}
}

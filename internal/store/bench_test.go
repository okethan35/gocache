// internal/store/bench_test.go
package store

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"sync/atomic"
	"testing"
)

const (
	benchKeys = 10000
	benchVal  = "0123456789abcdef" // fixed 16 bytes (SPEC.md 2b rule 11)
)

// benchParallelism is goroutines per GOMAXPROCS (SPEC.md 2b rule 10).
var benchParallelism = []int{1, 4, 16}

// BenchmarkStores compares the single-lock and sharded stores on a 70/20/10
// GET/SET/DEL mix over 10,000 uniform keys (SPEC.md 2b rules 10-11).
func BenchmarkStores(b *testing.B) {
	impls := []struct {
		name string
		mk   func() KV
	}{
		{"single", func() KV { return New() }},
		{"sharded", func() KV { return NewSharded(5) }},
	}
	keys := make([]string, benchKeys)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}

	for _, impl := range impls {
		for _, p := range benchParallelism {
			b.Run(fmt.Sprintf("%s/p=%d", impl.name, p), func(b *testing.B) {
				s := impl.mk()
				for _, k := range keys { // preload before the timer starts
					s.Set(k, benchVal, -1)
				}
				var seed atomic.Uint64
				b.SetParallelism(p)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					// Each goroutine has its own random source, so no shared
					// RNG lock adds contention.
					r := rand.New(rand.NewPCG(seed.Add(1), 0))
					for pb.Next() {
						k := keys[r.IntN(benchKeys)]
						switch op := r.IntN(10); {
						case op < 7:
							s.Get(k)
						case op < 9:
							s.Set(k, benchVal, -1)
						default:
							s.Del(k)
						}
					}
				})
			})
		}
	}
}

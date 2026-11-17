// internal/server/bench_test.go
package server

import (
	"bufio"
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"strconv"
	"sync/atomic"
	"testing"

	"gocache/internal/store"
)

// BenchmarkServer is Stage B of the padded-sharded-store REVISIT (see
// BENCHMARKS.md): the same 70/20/10 GET/SET/DEL mix over 10,000 uniform keys
// as BenchmarkStores, but each operation is a full request/reply round trip
// through the real server over loopback, one connection per goroutine.
// ns/op is wall time per round trip across all clients.
func BenchmarkServer(b *testing.B) {
	const numKeys = 10000
	const value = "0123456789abcdef" // fixed 16 bytes, as in BenchmarkStores
	impls := []struct {
		name string
		mk   func() KV
	}{
		{"single", func() KV { return store.New() }},
		{"sharded", func() KV { return store.NewSharded(5) }},
	}
	keys := make([]string, numKeys)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}
	lines := [3][]string{} // GET, SET, DEL lines per key, built once
	for _, k := range keys {
		lines[0] = append(lines[0], "GET "+k+"\n")
		lines[1] = append(lines[1], "SET "+k+" $"+value+"$\n")
		lines[2] = append(lines[2], "DEL "+k+"\n")
	}

	for _, impl := range impls {
		for _, p := range []int{1, 4, 16} {
			b.Run(fmt.Sprintf("%s/p=%d", impl.name, p), func(b *testing.B) {
				s := impl.mk()
				for _, k := range keys { // preload before the timer starts
					s.Set(k, value, -1)
				}
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					b.Fatalf("listen failed: %v", err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan error, 1)
				go func() { done <- ServeConfig(ctx, ln, s, Config{}) }()
				defer func() {
					cancel()
					<-done
				}()

				var seed atomic.Uint64
				b.SetParallelism(p)
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					conn, err := net.Dial("tcp", ln.Addr().String())
					if err != nil {
						b.Errorf("dial failed: %v", err)
						return
					}
					defer conn.Close()
					r := bufio.NewReader(conn)
					rng := rand.New(rand.NewPCG(seed.Add(1), 0))
					for pb.Next() {
						i := rng.IntN(numKeys)
						op := 0 // GET
						switch n := rng.IntN(10); {
						case n >= 9:
							op = 2 // DEL
						case n >= 7:
							op = 1 // SET
						}
						if _, err := conn.Write([]byte(lines[op][i])); err != nil {
							b.Errorf("write failed: %v", err)
							return
						}
						if _, err := r.ReadString('\n'); err != nil {
							b.Errorf("read failed: %v", err)
							return
						}
					}
				})
				b.StopTimer()
			})
		}
	}
}

# gocache - project conventions

## What this is
A Redis-style in-memory key-value cache server in Go. Phase 1 (store), Phase 2a (protocol + single-connection TCP server) and Phase 2b (concurrency) and Phase 3 (active expiry) are done. Phase 4 (graceful shutdown) is in progress. See SPEC.md for behavior contracts.

## Testing philosophy
- Tests in the *_test.go files are the source of truth, written before implementation.
- Do not modify existing tests to make them pass. If a test seems wrong, flag it and ask - don't silently "fix" it
- All new logic needs a corresponding test before it's considered done.
- Time-dependent tests use the injectable `now func() time.Time` field on Store, never time.Sleep.
- Every test must pass under both plain `go test` and `go test -race`. A concurrency test must still fail under plain `go test` when the protection it checks is missing.

## Code conventions
- internal/store holds all store logic, no networking code goes here.
- internal/store may start goroutines for store maintenance (the active-expiry sweep), but every goroutine it starts must have a stop function that waits for it to exit.
- internal/protocol holds wire-format parsing only, no store or networking code.
- internal/server holds connection handling and command dispatch.
- entry struct uses an explicit hasExpiry bool, not a zero-value time.Time sentinel, for representing "no expiration".
- Follow SPEC.md exactly for edge case behavior (empty keys, negative TTLs, overwrite semantics, wire format). Don't infer or improvise on ambiguous cases, ask.

## Phase 2b decisions (done)
- Store uses a plain sync.Mutex (mu field on Store), locked for the whole body of Get/Set/Del and released with defer. Revisit only if a benchmark shows a meaningful gap at a realistic read-heavy mix.
- `now` is set only at construction (or by a test before any goroutine starts) and never reassigned, so it needs no lock.
- A sharded store is built alongside the single-lock store for benchmarking: shard = fnv32(key) % numShards, each shard with its own map and mutex, starting at 5 shards. Result: sharded was only ~10% faster (ratio 0.90), so the single lock is kept; see BENCHMARKS.md. Padding shards onto separate cache lines reached 0.56; switching the server to it is deferred. Benchmark spec: b.RunParallel at SetParallelism 1/4/16, 70/20/10 GET/SET/DEL mix, 10,000 uniform keys, comparing ns/op. If sharded is <20% faster at the highest parallelism: verify go test -race and per-shard mutexes, check key spread, try another hash, re-run higher. If still <20% or slower, keep the single lock and log why.
- Goroutine model: one goroutine per connection; the store is the only shared state. The accept loop moves from cmd/server into internal/server so the goroutine model can be tested.
- The concurrent store test must fail under plain `go test` (no -race) when the lock is missing. (Settled in Phase 4: all tests must also pass under `-race`; see Testing philosophy.)
- Panic recovery: a deferred recover in the goroutine Serve starts per connection; a panic closes only that connection and is logged with its stack trace. Fatal runtime errors are not recoverable.
- Serve waits for all open connections to end before returning after the listener closes, so nothing it started outlives it. Full graceful shutdown (closing open connections) is out of scope for 2b.

## Phase 3 decisions (done)
- Active expiry never changes command results; lazy expiry stays the source of correctness, the sweep only reclaims memory.
- Sweep: every 100ms (server), up to 16 rounds per tick, each round locks, samples up to 20 keys, deletes expired ones, unlocks. Another round runs only if >25% of the round was expired.
- The sweep samples only TTL keys, from an `expires` index on Store kept in sync by Set and by a single remove(key) helper used for every deletion. Sampling ranges over the index (random start, not uniform).
- StartExpiry(interval) returns a stop func that waits for the goroutine to exit; New() does not start it; a non-positive interval panics.
- Tests drive the sweep loop through an injected tick channel, never a real ticker or sleep.

## Carried into the next phase
- REVISIT: padded sharded store (BENCHMARKS.md) vs the single lock for the server.
- REVISIT: main never calls the expiry stop function or closes the listener, since there is no shutdown path yet.

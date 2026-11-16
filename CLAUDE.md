# gocache - project conventions

## What this is
A Redis-style in-memory key-value cache server in Go. Phase 1 (store) and Phase 2a (protocol + single-connection TCP server) are done; Phase 2b (concurrency) is in progress. See SPEC.md for behavior contracts.

## Testing philosophy
- Tests in the *_test.go files are the source of truth, written before implementation.
- Do not modify existing tests to make them pass. If a test seems wrong, flag it and ask - don't silently "fix" it
- All new logic needs a corresponding test before it's considered done.
- Time-dependent tests use the injectable `now func() time.Time` field on Store, never time.Sleep.

## Code conventions
- internal/store holds all store logic, no networking code goes here.
- internal/protocol holds wire-format parsing only, no store or networking code.
- internal/server holds connection handling and command dispatch.
- entry struct uses an explicit hasExpiry bool, not a zero-value time.Time sentinel, for representing "no expiration".
- Follow SPEC.md exactly for edge case behavior (empty keys, negative TTLs, overwrite semantics, wire format). Don't infer or improvise on ambiguous cases, ask.

## Current phase
Phase 2b: concurrency. Decisions so far:
- Store uses a plain sync.Mutex (mu field on Store), locked for the whole body of Get/Set/Del and released with defer. Revisit only if a benchmark shows a meaningful gap at a realistic read-heavy mix.
- `now` is set only at construction (or by a test before any goroutine starts) and never reassigned, so it needs no lock.
- A sharded store is built alongside the single-lock store for benchmarking: shard = fnv32(key) % numShards, each shard with its own map and mutex, starting at 5 shards. Benchmark with b.RunParallel at SetParallelism 1/4/16, 70/20/10 GET/SET/DEL mix, 10,000 uniform keys, comparing ns/op. If sharded is <20% faster at the highest parallelism: verify go test -race and per-shard mutexes, check key spread, try another hash, re-run higher. If still <20% or slower, keep the single lock and log why.
- Goroutine model: one goroutine per connection; the store is the only shared state. The accept loop moves from cmd/server into internal/server so the goroutine model can be tested.
- The concurrent store test must fail under plain `go test` (no -race) when the lock is missing. REVISIT LATER: whether it should instead (or also) rely on `go test -race`.
- Still open: panic recovery per connection.

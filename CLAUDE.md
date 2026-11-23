# gocache - project conventions

## What this is
A Redis-style in-memory key-value cache server in Go. Phase 1 (store), Phase 2a (protocol + single-connection TCP server) and Phase 2b (concurrency) Phase 3 (active expiry), Phase 4 (graceful shutdown) Phase 5 (persistence) and Phase 6 (connection limits) are done. Phase 7 (TTL query) is specified in SPEC.md and in progress: Store.TTL is issue #4, the wire command is issue #5. See SPEC.md for behavior contracts.

## Testing philosophy
- Tests in the *_test.go files are the source of truth, written before implementation.
- Do not modify existing tests to make them pass. If a test seems wrong, flag it and ask - don't silently "fix" it
- All new logic needs a corresponding test before it's considered done.
- Time-dependent tests use the injectable `now func() time.Time` field on Store, never time.Sleep.
- Network deadlines (idle timeout, shutdown) cannot use the fake clock, so those server tests run on the real clock with short timeouts. They still never call time.Sleep, and every wait is bounded by a deadline so a broken server fails the test instead of hanging it.
- Every test must pass under both plain `go test` and `go test -race`. A concurrency test must still fail under plain `go test` when the protection it checks is missing.

## Code conventions
- internal/store holds all store logic, no networking code goes here.
- internal/store may start goroutines for store maintenance (the active-expiry sweep), but every goroutine it starts must have a stop function that waits for it to exit.
- internal/protocol holds wire-format parsing only, no store or networking code.
- internal/snapshot holds the snapshot file format and atomic file writing; it depends on internal/store, never the reverse.
- internal/server holds connection handling and command dispatch.
- entry struct uses an explicit hasExpiry bool, not a zero-value time.Time sentinel, for representing "no expiration".
- Follow SPEC.md exactly for edge case behavior (empty keys, negative TTLs, overwrite semantics, wire format). Don't infer or improvise on ambiguous cases, ask.

## Phase 2b decisions (done)
- Store uses a plain sync.Mutex (mu field on Store), locked for the whole body of Get/Set/Del and released with defer. Revisit only if a benchmark shows a meaningful gap at a realistic read-heavy mix.
- `now` is set only at construction (or by a test before any goroutine starts) and never reassigned, so it needs no lock.
- A sharded store is built alongside the single-lock store for benchmarking: shard = fnv32(key) % numShards, each shard with its own map and mutex, starting at 5 shards. Result: sharded was only ~10% faster (ratio 0.90), so the single lock is kept; see BENCHMARKS.md. Padded shards (128 bytes each, TestShardsDoNotShareCacheLines) reach 0.61 at store level but make no measurable difference through the server (ratio 1.02, BenchmarkServer), so the server keeps the single lock; see BENCHMARKS.md. The server takes a KV interface (Get/Set/Del), so either store can be benchmarked through it. Benchmark spec: b.RunParallel at SetParallelism 1/4/16, 70/20/10 GET/SET/DEL mix, 10,000 uniform keys, comparing ns/op. If sharded is <20% faster at the highest parallelism: verify go test -race and per-shard mutexes, check key spread, try another hash, re-run higher. If still <20% or slower, keep the single lock and log why.
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

## Phase 4 decisions (done)
- SIGINT/SIGTERM start a graceful shutdown; a second signal kills the process (default handling restored after the first).
- server.ServeContext(ctx, ln, s): cancelling ctx closes the listener, sets every open connection's read deadline to now and write deadline to now+1s, and returns nil once all connections end. Serve(ln, s) is ServeContext with a never-cancelled context, so its tests are unchanged.
- Shutdown starts the instant ctx is cancelled: connections check ctx.Err() before executing each line, so lines already received but not started are discarded unexecuted. Only the command already executing finishes and gets its reply.
- Client contract: a reply means the command ran; no reply means it may or may not have run and is safe to retry.
- main's body lives in run(ctx, ...) so it can be tested by cancelling ctx: log "shutting down", drain, stop the expiry sweep, log "stopped", exit 0. (Phase 5 added snapshot steps; see below.)

## Phase 5 decisions (done)
- Persistence is by snapshot to dump.gocache (always on): loaded at startup before serving, saved every 60s and once more at shutdown after draining and stopping the sweep. A failed final save exits 1.
- Store.Export copies live entries under the lock (expired ones skipped); the file is written with the lock released. Store.Import drops entries expired by load time and keeps the TTL index in sync.
- internal/snapshot owns the binary format (SPEC.md Phase 5 rule 8: magic, version, count, entries, CRC-32). Read verifies the checksum before trusting any field and bounds-checks every read, so damaged input returns an error, never a panic. FuzzRead (run with -fuzz) checks this.
- SaveFile is atomic: temp file in the same directory, fsync, rename, fsync the directory.
- Missing snapshot: start empty. Corrupt snapshot: refuse to start, naming the file.
- TTLs are stored as absolute wall-clock Unix nanoseconds; a clock jump between save and load shifts expiries (known limitation).
- TTLs above 100 years (MaxTTLSeconds, in both internal/protocol and internal/store, pinned equal by a test in internal/server) are rejected: invalid command on the wire, Set returns 0. This keeps expiries inside time.Duration and the snapshot's Unix-nanosecond range.
- An empty key makes a snapshot corrupt, and Import skips empty keys as a backstop. Leftover .gocache-*.tmp files are removed at startup (one server per snapshot directory). The directory fsync is skipped on Windows.
- run(ctx, ln, expiryInterval, snapshotInterval, path) is main's testable body; tests pass a t.TempDir path.
- Save failures: saveFile goes through an internal fileSystem interface so tests can fail any single step (fakeFS) and check step order; one end-to-end test uses a real non-empty directory at the snapshot path to make the rename fail. A failed periodic save is logged and retried next tick; a failed save leaves the old snapshot and no temp file (except a failed directory fsync, after which the new file is in place but the save still reports an error).

## Phase 6 decisions (done)
- server.ServeConfig(ctx, ln, s, Config{MaxClients, IdleTimeout, WriteTimeout}); zero fields mean the defaults (1000, 5m, 10s). ServeContext and Serve use the defaults.
- Over the limit: accept, send "max clients reached\n" with a 1s write deadline in its own goroutine (tracked, so Serve waits for it), close, log. Rejected connections never count.
- Idle timeout: the read deadline is reset before each line, so a whole line must arrive in time (stops slow-loris drips). Closed without a message.
- Write timeout: the write deadline is set before each reply (after dispatch, so it covers only the write). A client that never reads is closed, freeing its slot.
- Shutdown always wins: armIdle and armWrite take the tracker lock and do nothing once shutdown has started. The shutdown watcher applies shutdown deadlines before closing the listener.
- Network-deadline tests run on the real clock with short timeouts and bounded waits, never time.Sleep (see Testing philosophy).
- Negative Config fields are an error from ServeConfig. Per-connection behavior (shutdown check, read and write deadlines) goes through a connPolicy: noPolicy for HandleConn, servePolicy for ServeConfig.

## Phase 7 decisions (in progress)
- Store.TTL returns three states as (int64, bool): (-1, true) no expiry, (N, true) N whole seconds left (floor, may be 0), (0, false) missing/expired/empty key.
- TTL lazily deletes an expired entry, like Get and Del, and holds mu for its whole body.
- The expiry boundary matches Phase 1 rule 4: now == expiresAt is expired.
- TTL is added to the KV interface; ShardedStore delegates to the key's shard.
- Wire reply: `True <N>` (plain integer, no $), `True -1`, or `False $nil$`, same as a GET miss.

## Agent workflow
- Issues labeled `ralph-loop` are implemented by a scheduled writer agent, reviewed by a scheduled reviewer agent (`needs-review`), and handed to the owner (`human-review`).
- Issues declare dependencies with a `**Blocked by:** #N` or `**Blocked by:** none` line.
- Specs are written by the owner before issues are queued; agents implement, never write spec.

## Carried into the next phase
- Crash durability can only be checked in-process as step order (write, fsync file, close, rename, fsync dir); a real power-loss test would need a VM or fault-injecting block device.
- Snapshot peak memory is about 3x the store (data, Export copy, encoded file in memory); streaming Write would remove the third copy.

# gocache - project conventions

## What this is
A Redis-style in-memory key-value cache server in Go. Phases 1-7 are done. Phase 8 (EXPIRE and PERSIST) is in progress: Store.Expire/Persist (#23) is merged, the wire commands are #24. SPEC.md holds the behavior contracts; DECISIONS.md holds the history and rationale behind the invariants below (read it only when you need the why).

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
- Follow SPEC.md exactly for edge case behavior (empty keys, negative TTLs, overwrite semantics, wire format). Don't infer or improvise on ambiguous cases, ask.

## Invariants
Implementation rules that SPEC.md does not state. Breaking one is a bug even if the tests pass.

### Store (internal/store)
- One plain sync.Mutex (`mu`), locked for the whole body of every method that touches the map (Get, Set, Del, TTL, Expire, Persist, Export, Import) and released with defer. No sharding or RWMutex in Store: benchmarks showed no gain through the server (DECISIONS.md, BENCHMARKS.md).
- `now` is set only at construction (or by a test before any goroutine starts) and never reassigned, so it needs no lock.
- entry uses an explicit hasExpiry bool, not a zero-value time.Time sentinel, for "no expiration".
- Every key with an expiry is in the `expires` index. Anything that adds or removes an expiry updates the index, and every deletion goes through the single remove(key) helper.
- Lazy expiry is the source of correctness: every method that finds an expired entry deletes it and treats the key as missing. The active-expiry sweep only reclaims memory and never changes a command result.
- The sweep runs only after StartExpiry(interval), which returns a stop func that waits for the goroutine to exit. Tests drive it through an injected tick channel, never a real ticker or sleep.
- MaxTTLSeconds exists in both internal/protocol and internal/store; a test in internal/server pins them equal. Change both or neither.
- The server takes the store.KV interface (internal/store/kv.go). Every new store method goes on KV, and both Store and ShardedStore implement it (ShardedStore delegates to the key's shard).

### Server (internal/server)
- One goroutine per connection; the store is the only shared state. Each connection goroutine recovers panics, logs the stack trace and closes only that connection.
- Serve, ServeContext and ServeConfig return only after every goroutine they started has ended (connections and over-limit rejections alike).
- Shutdown always wins: once ctx is cancelled, nothing re-arms a deadline (armIdle and armWrite take the tracker lock and do nothing after shutdown starts), and the watcher applies shutdown deadlines before closing the listener.
- A connection checks ctx.Err() before executing each line, so after shutdown starts only the command already running finishes.
- Client contract: a reply means the command ran; no reply means it may or may not have run and is safe to retry.
- The read deadline is reset before each line; the write deadline is set after dispatch, before each reply.
- Per-connection behavior goes through connPolicy (noPolicy for HandleConn, servePolicy for ServeConfig), not flags on the connection loop.

### Snapshot (internal/snapshot) and main (cmd/server)
- Store.Export copies under the lock; encoding and file writing happen with the lock released.
- Read verifies the CRC before trusting any field and bounds-checks every read: damaged input returns an error, never a panic (FuzzRead).
- Saves are atomic: temp file in the same directory, fsync, close, rename, fsync the directory (skipped on Windows). File operations go through the fileSystem interface so tests can fail any single step.
- main's body is run(ctx, ln, expiryInterval, snapshotInterval, path), testable by cancelling ctx. Shutdown order: log "shutting down", drain connections, stop the sweep, save the final snapshot, log "stopped". Exit 1 if the final save failed, else 0.

## Known limitations
- TTLs are stored as absolute wall-clock Unix nanoseconds; a clock jump between save and load shifts expiries.
- Crash durability can only be checked in-process as step order (write, fsync file, close, rename, fsync dir); a real power-loss test would need a VM or fault-injecting block device.
- Snapshot peak memory is about 3x the store (data, Export copy, encoded file in memory); streaming Write would remove the third copy.

## Agent workflow
The Ralph Loop's contract, config and setup live in `.claude/ralph/` (SPEC.md, config.json, SETUP.md). Agents follow `.claude/ralph/SPEC.md`; this file holds project conventions only.

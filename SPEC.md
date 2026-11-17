# Phase 1 spec — gocache store

Commands supported: SET key value [ttl_seconds], GET key, DEL key

1. GET on a missing key returns: (empty value, False) value plus "bad" flag, either nil or "" depending on value
2. GET on an expired key returns: (empty value, False) value plus "bad" flag, either nil or "" depending on value
3. Expiry checking strategy: lazy, get at GET time, plus a periodic active sweep (see Phase 3)
4. SET with ttl_seconds = 0 means: key expires immediately — a GET issued 
   at the exact same instant as SET, with zero real time elapsed, is 
   already treated as a miss.
5. SET with ttl_seconds < -1 means: ttl_seconds get compressed to -1
6. SET with ttl_seconds = -1 (or not provided by client) means: explicitly not set, no expiration
7. SET on an existing key: does it fully replace the old value and TTL? yes
8. DEL on a missing key: error, or silently do nothing? return 0 (number of deleted keys)
9. SET or DEL on empty key: return 0 (number of keys updated)
10. SET on valid key: return 1


# Phase 2 spec

## 2a - Protocol + single connection, no concurrency

### Wire format
1. Commands follow the format: SET foo $bar$ 30\n (newline-terminated)
   values must be wrapped in dollar signs. Dollar signs are forbidden input.
2. commands are case sensitive. first word must be a fully capital command. everything else is case insensitive. 
3. Keys are one word only.
4. Values are wrapped in $.
5. GET and DEL will only take one argument, the variable. e.g. GET foo, DEL foo

### Responses
6. A successful SET response on the wire looks like: 1\n
7. A successful GET response on the wire looks like: True $bar$\n
8. A DEL response will return either 0 or 1
9. An unsuccessful SET response on the wire looks like: 0\n
10. An unsuccessful GET response on the wire looks like: False $nil$\n

### Parsing Algorithm
11. After command word, the next word is the key. 
12. For SET, next look for a $. anything contained between first and second $ is the argument. Everything after the closing $ is the ttl. If there is none, ttl is -1.
13. For GET and DEL, any other trailing input returns: invalid command\n.

### Malformed input
14. Command with no key: invalid command\n
15. SET with no value: invalid command\n
16. SET with non-integer ttls return: invalid command\n
17. Unknown command: invalid command\n
18. Empty line: invalid command\n
19. If more dollar signs are in input than allowed for command (0 for GET and DEL, 2 for SET), then stop and return: invalid command\n
20. On any malformed input, keep connection open and keep parsing for the next commands

### connection lifecycle
21. One connection can send multiple commands
22. If connection is interrupted mid command, discard input and close connection.
23. Only support line lengths of 256 characters. If it's exceeded, return "max line length exceeded\n". Keep connection open.

### Server startup
24. Server listens on port 6380.

### Misc
25. Empty values ($$) will be accepted. Empty values may be stored.

## 2b - Concurrency

### Goroutine model
1. Each accepted connection is served in its own goroutine. An idle or slow client must not block other clients.
2. The store is the only state shared between connections.
3. Each GET, SET and DEL is atomic: concurrent commands behave as if they ran one at a time in some order. Commands on one connection run in the order sent. There is no ordering guarantee across connections.

### Store concurrency
4. Access to the store from multiple goroutines is serialized with a plain sync.Mutex. Revisit only if a benchmark shows a meaningful gap at a realistic read-heavy mix.
5. Locks live inside each method to ensure safety when multiple methods are called concurrently. The lock protects the data field on Store. It is a sync.Mutex field on Store (mu), which Get, Set, and Del take for their whole body and release with defer.
6. The now field on Store is set only at construction, or by a test before any goroutine starts, and is never reassigned. It is not protected by the lock.

### Sharded store (benchmark only)
7. Sharded locking is implemented alongside the single-lock store so the two can be benchmarked against each other. The goal is to learn what the performance difference is. The sharded store follows Phase 1 rules 1-10 exactly. It is used only by benchmarks, not by the server.
8. A key's shard is fnv32(key) % numShards, so the same key always maps to the same shard. fnv32 is FNV-1a, computed inline (no per-operation allocation). Each shard has its own map and its own sync.Mutex.
9. The starting shard count is 5, an arbitrary baseline to be tuned by benchmark.

### Benchmark
10. Benchmark both stores with b.RunParallel at SetParallelism 1, 4 and 16. SetParallelism is goroutines per GOMAXPROCS, not total goroutines; record GOMAXPROCS with every result.
11. Workload: GET/SET/DEL mix of 70/20/10, keys drawn uniformly from a fixed pool of 10,000. All 10,000 keys are loaded before the timer starts. Values are a fixed 16 bytes. SETs have no TTL. Each goroutine uses its own random source.
12. Compare ns/op. "At least 20% faster" means sharded ns/op <= 0.80 x single-lock ns/op.
13. If sharded is less than 20% faster at the highest parallelism level:
    a. Verify correctness: go test -race passes, and each shard really has its own mutex, on its own cache line (no false sharing between shards).
    b. Check that keys spread evenly: every shard holds between 15% and 25% of the 10,000 keys.
    c. Try a different hash function (hash/maphash).
    d. Re-run at SetParallelism 64.
    e. If it is still less than 20% faster, or it is slower, keep the single lock and log why.
14. Results are logged in BENCHMARKS.md at the repo root: raw go test -bench output, GOMAXPROCS, the decision, and the reason.

### Server lifecycle
15. Closing the listener stops accepting new connections. The server stops once all open connections have ended. Other accept errors are logged and retried with backoff.
16. A panic while serving a connection closes that connection only. The server and other connections keep running, and the store remains usable. The panic and its stack trace are logged.
17. Fatal runtime errors (for example, concurrent map writes or out of memory) cannot be recovered and stop the server.


# Phase 3 spec

## Active expiry

### Contract
1. Active expiry never changes the result of any command. It only reclaims memory. Lazy expiry (Phase 1 rule 3) stays exactly as is, so correctness never depends on the sweep having run.
2. Active expiry applies to the single-lock Store only. The sharded store is unchanged. No new wire commands.

### Sweep
3. Each tick runs up to 16 rounds. A round takes the store lock, examines up to 20 entries, deletes the expired ones, and releases the lock, so clients can run between rounds.
4. Another round runs only if more than 25% of the entries examined in the current round were expired.
5. The sweep samples only keys that have a TTL. The store keeps an index of exactly the keys whose entry has an expiry: Set with a ttl adds the key, Set with no expiration removes it, and every deletion (Del, a lazy delete in Get or Del, the sweep) removes it. The 25% threshold in rule 4 is measured over these TTL keys.
5a. Keys are sampled by ranging over the TTL index and stopping after 20. Go starts map iteration at a random position, so this is a cheap approximation of random sampling, not a uniform one.

### Lifecycle
6. The sweep is started with Store.StartExpiry(interval), which returns a stop function. New() does not start it. stop() returns only after the sweep goroutine has exited. StartExpiry panics if interval is zero or negative.
7. The server starts the sweep with an interval of 100ms.
8. SPEC.md 2b rule 6 still applies: now is set before StartExpiry is called and is never reassigned while the sweep runs.

### Visibility
9. No metrics or logging for active expiry yet.


# Phase 4 spec

## Graceful shutdown

### Trigger
1. SIGINT or SIGTERM starts a graceful shutdown.
2. A second SIGINT or SIGTERM during shutdown exits the process immediately.

### Clients
3. When shutdown starts, the server stops accepting new connections.
4. Idle connections are closed straight away.
5. A command already executing when shutdown starts runs to completion and its response is written; then the connection closes. Any other input is discarded without being executed, including complete lines the server has received but not yet started: a partially received line (2a rule 22), and further lines already sent on the same connection.
5a. A response means the command ran. A command with no response may or may not have run (only the one executing at shutdown can be in that state) and is safe for the client to retry.
6. Once shutdown starts, each connection has 1 second to finish writing; a client that does not read its response is closed after that.
7. No goodbye message is sent; the connection just closes.

### API and order
8. server.ServeContext(ctx, ln, s) serves like Serve; cancelling ctx starts shutdown. It returns nil once every connection has ended. Serve(ln, s) is ServeContext with a context that is never cancelled.
9. Shutdown order: signal, log "shutting down", stop accepting and drain connections, stop the expiry sweep, log "stopped", exit 0.
10. A shutdown during an accept-error backoff does not wait for the backoff to finish.


# Phase 5 spec

## Persistence

### Model
1. Persistence is by snapshot: the whole store is written to one file. Writes made after the last snapshot are lost on a crash.
2. A snapshot is taken every 60 seconds while running, and once more during graceful shutdown after connections have drained (after Phase 4 rule 9's drain and expiry-sweep stop, before "stopped"). If that final save fails, the error is logged and the process exits with status 1.
3. At startup the snapshot is loaded into the empty store before any connection is served.
4. The snapshot file is dump.gocache in the working directory. An empty path disables persistence (no load, no snapshots).

### Contents
5. Each live entry is saved with its key, value, and expiry. A TTL is saved as an absolute wall-clock time (Unix nanoseconds). Known limitation: if the system clock jumps between save and load, expiries shift with it.
6. Entries already expired when the snapshot is taken are not saved. Entries that have expired by the time a snapshot is loaded are dropped.
7. Loading adds entries to the store and keeps the TTL index (Phase 3 rule 5) in sync.

### File format
8. All integers little-endian:
   - magic: 8 bytes "GOCACHE1"
   - version: uint32, currently 1
   - count: uint64, number of entries
   - each entry: key length uint32, key bytes, value length uint32, value bytes, hasExpiry uint8 (0 or 1), expiresAt int64 Unix nanoseconds (0 when hasExpiry is 0)
   - checksum: uint32 CRC-32 (IEEE) of every byte before it
9. Wrong magic, an unknown version, a checksum mismatch, a truncated file, or bytes left over after the entries make the file corrupt.

### Writing
10. A snapshot is written to a temporary file in the same directory, flushed to disk, renamed over the old snapshot, and then the directory is flushed. A crash at any point leaves either the complete old snapshot or the complete new one.
11. Taking a snapshot holds the store lock only while copying the entries; the file is written with the lock released. Each snapshot reflects the store at one instant.

### Loading problems
12. A missing snapshot file is normal (first run): log it and start empty.
13. A corrupt snapshot file stops startup with an error naming the file, exit status 1. The server never starts empty over a corrupt file.

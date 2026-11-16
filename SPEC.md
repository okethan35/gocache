# Phase 1 spec — gocache store

Commands supported: SET key value [ttl_seconds], GET key, DEL key

1. GET on a missing key returns: (empty value, False) value plus "bad" flag, either nil or "" depending on value
2. GET on an expired key returns: (empty value, False) value plus "bad" flag, either nil or "" depending on value
3. Expiry checking strategy: lazy, get at GET time
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
    a. Verify correctness: go test -race passes, and each shard really has its own mutex.
    b. Check that keys spread evenly: every shard holds between 15% and 25% of the 10,000 keys.
    c. Try a different hash function (hash/maphash).
    d. Re-run at SetParallelism 64.
    e. If it is still less than 20% faster, or it is slower, keep the single lock and log why.
14. Results are logged in BENCHMARKS.md at the repo root: raw go test -bench output, GOMAXPROCS, the decision, and the reason.

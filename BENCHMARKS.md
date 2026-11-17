# Benchmarks

## 2b: single-lock Store vs ShardedStore (SPEC.md 2b rules 10-14)

### Prediction (recorded before any real run)
Sharded/single ns/op ratio at SetParallelism 16: about **0.25**.
Reasoning: 5 shards should split the work somewhat evenly, though likely not completely equally.

### Environment
- goos linux, goarch amd64, cpu Intel(R) Xeon(R) Processor @ 2.30GHz (shared cloud VM)
- GOMAXPROCS 4, so SetParallelism 1/4/16/64 = 4/16/64/256 goroutines
- go test -run '^$' -bench BenchmarkStores -benchtime=1s -count=6 ./internal/store
- Compared with benchstat (medians, n=6). 0 allocs/op for every benchmark.

### Result (sharded ns/op / single ns/op; rule 12 bar is <= 0.80)

| SetParallelism | single | sharded | ratio |
|---|---|---|---|
| 1  | 205.6 ns ± 15% | 224.8 ns ± 9% | 1.09 (no significant difference, p=0.132) |
| 4  | 343.1 ns ± 8%  | 292.7 ns ± 5% | 0.85 |
| 16 | 333.9 ns ± 12% | 301.1 ns ± 4% | **0.90** |

Sharded is less than 20% faster at the highest level, so rule 13 applies.

### Rule 13 follow-up
- a. Correctness: `go test -race` passes; the 5 shards have 5 distinct mutexes. Pass.
- b. Key spread over the 10,000 benchmark keys: 2012 / 1991 / 1997 / 1988 / 2012 (19.9%-20.1% each). Pass.
- c. hash/maphash instead of FNV-1a: p=16 323.6 ns vs 283.6 ns, ratio 0.88. Still under 20% faster.
- d. SetParallelism 64: 315.4 ns vs 300.0 ns, ratio 0.95. Still under 20% faster.
- e. Decision below.

### Decision
**Keep the single-lock Store.** The sharded store as built is only ~10% faster at
SetParallelism 16, and neither a different hash nor more parallelism closes the gap.

### Why it fell short: false sharing (lead for later)
Each shard is a 24-byte Store and the five were allocated next to each other, so
their mutexes sit 24 bytes apart, inside the same 64-byte cache lines. Cores
locking different shards still contended on the same cache line, so the shards
were independent in code but not in hardware. Rule 13a as written ("each shard
really has its own mutex") passed and did not catch this; it has been tightened.

An extra experiment (not part of rule 13, run on a scratch copy, not committed)
padded each shard to 128 extra bytes:

| SetParallelism | single | sharded (padded) | ratio |
|---|---|---|---|
| 1  | 225.2 ns ± 6% | 182.1 ns ± 9%  | 0.81 |
| 4  | 327.4 ns ± 6% | 205.4 ns ± 5%  | 0.63 |
| 16 | 328.3 ns ± 9% | 183.7 ns ± 16% | **0.56** |

Padded sharding would clear the bar. Reopening decision 1 (switching the server
to a padded sharded store) is deferred to a later phase.

### Prediction vs actual
Predicted 0.25; actual 0.90 as built, 0.56 with padding. Reasons the gap stays
well above 0.25 even with padding: with 4 CPUs at most 4 operations run at once
however many shards exist, and much of each operation (random key choice,
hashing, map lookup and its cache misses) happens outside the lock, so sharding
cannot speed it up.

### Raw output (official run)
```
goos: linux
goarch: amd64
pkg: gocache/internal/store
cpu: Intel(R) Xeon(R) Processor @ 2.30GHz
BenchmarkStores/single/p=1-4         	 5184468	       209.4 ns/op
BenchmarkStores/single/p=1-4         	 5963007	       211.1 ns/op
BenchmarkStores/single/p=1-4         	 6845533	       189.5 ns/op
BenchmarkStores/single/p=1-4         	 5380022	       235.9 ns/op
BenchmarkStores/single/p=1-4         	 6104350	       189.9 ns/op
BenchmarkStores/single/p=1-4         	 5506794	       201.8 ns/op
BenchmarkStores/single/p=4-4         	 3943741	       372.0 ns/op
BenchmarkStores/single/p=4-4         	 3664370	       356.7 ns/op
BenchmarkStores/single/p=4-4         	 3408403	       315.1 ns/op
BenchmarkStores/single/p=4-4         	 3544990	       347.6 ns/op
BenchmarkStores/single/p=4-4         	 3490503	       338.6 ns/op
BenchmarkStores/single/p=4-4         	 3506647	       333.9 ns/op
BenchmarkStores/single/p=16-4        	 4838805	       333.7 ns/op
BenchmarkStores/single/p=16-4        	 3281344	       328.6 ns/op
BenchmarkStores/single/p=16-4        	 3437682	       334.2 ns/op
BenchmarkStores/single/p=16-4        	 3436720	       316.8 ns/op
BenchmarkStores/single/p=16-4        	 3687153	       345.2 ns/op
BenchmarkStores/single/p=16-4        	 3324679	       373.2 ns/op
BenchmarkStores/sharded/p=1-4        	 5749041	       226.1 ns/op
BenchmarkStores/sharded/p=1-4        	 4963170	       230.1 ns/op
BenchmarkStores/sharded/p=1-4        	 5466663	       204.5 ns/op
BenchmarkStores/sharded/p=1-4        	 5462812	       221.7 ns/op
BenchmarkStores/sharded/p=1-4        	 5440905	       223.5 ns/op
BenchmarkStores/sharded/p=1-4        	 4942102	       233.5 ns/op
BenchmarkStores/sharded/p=4-4        	 4361242	       279.3 ns/op
BenchmarkStores/sharded/p=4-4        	 4102342	       289.5 ns/op
BenchmarkStores/sharded/p=4-4        	 4070695	       297.3 ns/op
BenchmarkStores/sharded/p=4-4        	 4024657	       289.8 ns/op
BenchmarkStores/sharded/p=4-4        	 4042221	       295.6 ns/op
BenchmarkStores/sharded/p=4-4        	 4073514	       304.1 ns/op
BenchmarkStores/sharded/p=16-4       	 3889417	       309.1 ns/op
BenchmarkStores/sharded/p=16-4       	 3874864	       307.3 ns/op
BenchmarkStores/sharded/p=16-4       	 3891123	       289.9 ns/op
BenchmarkStores/sharded/p=16-4       	 4560514	       291.4 ns/op
BenchmarkStores/sharded/p=16-4       	 4004902	       305.0 ns/op
BenchmarkStores/sharded/p=16-4       	 4245966	       297.2 ns/op
PASS
ok  	gocache/internal/store	57.333s
```

## Phase 3: TTL index cost (single-lock Store)
Adding the TTL index (SPEC.md Phase 3 rule 5) puts one extra map operation in
every Set. Same benchmark, same machine, before vs after (benchstat, n=6): no
statistically significant change at any SetParallelism (p=0.39 to 0.59).

| SetParallelism | before | after |
|---|---|---|
| 1  | 205.6 ns ± 15% | 192.6 ns ± 19% |
| 4  | 343.1 ns ± 8%  | 322.1 ns ± 25% |
| 16 | 333.9 ns ± 12% | 324.4 ns ± 17% |

The benchmark's SETs have no TTL, so this measures the delete-from-index path only.

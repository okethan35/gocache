# Benchmarks

## 2b: single-lock Store vs ShardedStore (SPEC.md 2b rules 10-14)

### Prediction (recorded before any real run)
Sharded/single ns/op ratio at SetParallelism 16: about **0.25**.
Reasoning: 5 shards should split the work somewhat evenly, though likely not completely equally.

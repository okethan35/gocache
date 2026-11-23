// internal/store/sharded.go
package store

// ShardedStore splits keys across several independent Stores, each with its
// own map and its own mutex (SPEC.md 2b rules 7-9). It exists only so it can
// be benchmarked against the single-lock Store; the server does not use it.
// Each shard is a full Store, so Phase 1 rules 1-10 hold by construction.
type ShardedStore struct {
	shards []*Store      // shards[i] points into padded[i]
	padded []paddedShard // keeps the shards allocated together, 128 bytes apart
}

// paddedShard puts 128 bytes after each Store, so no two shards' mutexes ever
// share a 64-byte cache line, or the 128-byte pair many CPUs fetch together.
// Without it, cores locking different shards still fight over the same cache
// line (false sharing); see BENCHMARKS.md and SPEC.md 2b rule 13a.
type paddedShard struct {
	Store
	_ [128]byte
}

// NewSharded returns a ShardedStore with n shards. n must be at least 1.
func NewSharded(n int) *ShardedStore {
	if n < 1 {
		panic("store: NewSharded needs at least 1 shard")
	}
	padded := make([]paddedShard, n)
	shards := make([]*Store, n)
	for i := range padded {
		padded[i].Store.init()
		shards[i] = &padded[i].Store
	}
	return &ShardedStore{shards: shards, padded: padded}
}

// shardFor maps key to its shard: fnv32a(key) % numShards, so the same key
// always maps to the same shard (SPEC.md 2b rule 8).
func (s *ShardedStore) shardFor(key string) *Store {
	return s.shards[fnv32a(key)%uint32(len(s.shards))]
}

// fnv32a is 32-bit FNV-1a, computed inline so hashing a key does not allocate
// (hash/fnv's New32a returns an interface value).
func fnv32a(key string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= prime32
	}
	return h
}

func (s *ShardedStore) Get(key string) (string, bool) { return s.shardFor(key).Get(key) }

func (s *ShardedStore) Set(key, value string, ttlSeconds int) int {
	return s.shardFor(key).Set(key, value, ttlSeconds)
}

func (s *ShardedStore) Del(key string) int { return s.shardFor(key).Del(key) }

// TTL delegates to the key's shard (SPEC.md Phase 7 rule 8).
func (s *ShardedStore) TTL(key string) (int64, bool) { return s.shardFor(key).TTL(key) }

// internal/store/kv.go
package store

// KV is the API shared by the single-lock Store and the ShardedStore. The
// server serves any KV, and the store benchmarks and conformance tests drive
// both implementations through it.
type KV interface {
	Get(key string) (string, bool)
	Set(key, value string, ttlSeconds int) int
	Del(key string) int
	TTL(key string) (int64, bool)
}

// Both stores must keep satisfying KV.
var (
	_ KV = (*Store)(nil)
	_ KV = (*ShardedStore)(nil)
)

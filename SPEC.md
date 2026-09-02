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


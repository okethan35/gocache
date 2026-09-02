# gocache - project conventions

## What this is
An in-memory key-value cache server, phase 1 (in progress): single-threaded core store, no networking yet. See spec.md for behavior contracts

## Testing philosophy
- Tests in store_test.go are the source of truth, written before implementation.
- Do not modify exisiting tests to make them pass. If a test seems wrong, flag it and ask - don't silently "fix" it
- All new logic needs a corresponding test before it's considered done.
- Time-dependent tests use the injectable `now func() time.Time` field on Store, never time.Sleep.

## Code conventions
- internal/store hold all store logic, no networking code goes here.
- entry struct uses an explicit hasExpiry bool, not a zero-value time.Time sentinel, for representing "no expiration".
- Follow spec.md exactly for edge case behavior (empty keys, negative TTLs, overwrite semantics). Don't infer or improvise on ambigous cases, ask. 

## Current phase
Phase 1 only: Get, Set, Del on a single-threaded map. No goroutines, no locks, no networking yet, to be done in Phase 2.
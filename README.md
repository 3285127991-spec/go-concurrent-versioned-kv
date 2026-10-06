# versionedkv

A concurrent, versioned, in-memory key-value store written in Go. Standard
library only: backend library, a minimal CLI demo, and automated tests.
No TTL, persistence, transactions, networking, or any other extras.

## Features

- Keys are non-empty strings; each key maps to a string value and a `uint64` version.
- First `Put` of a key gets version 1; every successful write to that key gets
  a strictly larger version.
- `Delete` keeps a tombstone with the last version, so re-creating a key never
  restarts at 1 and never reuses an old version.
- `CompareAndSwap` checks the expected version and updates atomically under a
  single lock: two goroutines cannot both win with the same stale version.
- `Get` and `Snapshot` return copies; `Snapshot` lists live keys only, sorted
  lexicographically by key.
- Safe for concurrent use from multiple goroutines (`sync.RWMutex`).

## Layout

- `kvstore/` - the library (`Store`, `Entry`, `CASResult`, `ErrEmptyKey`).
- `kvstore/kvstore_test.go` - 11 tests: Put/Get, version increments, CAS
  success/mismatch/not-found, delete-then-reput monotonicity, snapshot order
  and isolation, empty-key rejection, `Len`, and a concurrent CAS race.
- `cmd/kvcli/` - minimal demo: Put, Get, one successful and one failed CAS,
  Delete, and re-Put, using built-in data only.

## API

```go
store := kvstore.New()
version, err := store.Put("k", "v")                    // 1 on first write
entry, found, err := store.Get("k")                    // Entry{Key, Value, Version}
result, version, err := store.CompareAndSwap("k", 1, "v2")
deleted, err := store.Delete("k")
entries := store.Snapshot()                            // sorted by key, live only
n := store.Len()
```

`CompareAndSwap` returns `CASSuccess`, `CASVersionMismatch`, or
`CASKeyNotFound`. Empty keys are rejected with `ErrEmptyKey` by all
operations.

## Run

```sh
go run ./cmd/kvcli      # minimal demo
go test -race ./...     # tests with the race detector
```

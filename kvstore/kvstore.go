// Package kvstore implements a concurrent, versioned, in-memory key-value
// store. Each key maps to a string value and a uint64 version that increases
// monotonically on every successful write, including re-creation after
// deletion. It depends only on the Go standard library.
package kvstore

import (
	"errors"
	"sort"
	"sync"
)

// ErrEmptyKey is returned by every mutating or reading operation when the
// provided key is an empty string.
var ErrEmptyKey = errors.New("kvstore: key must not be empty")

// Entry is a snapshot of a single key's state.
type Entry struct {
	Key     string
	Value   string
	Version uint64
}

// CASResult describes the outcome of a CompareAndSwap call.
type CASResult int

const (
	// CASSuccess means the expected version matched and the value was updated.
	CASSuccess CASResult = iota
	// CASKeyNotFound means the key does not currently exist in the store.
	CASKeyNotFound
	// CASVersionMismatch means the key exists but its current version differs
	// from the expected version.
	CASVersionMismatch
)

// String returns a human-readable description of the CAS outcome.
func (r CASResult) String() string {
	switch r {
	case CASSuccess:
		return "success"
	case CASKeyNotFound:
		return "key not found"
	case CASVersionMismatch:
		return "version mismatch"
	default:
		return "unknown"
	}
}

// record is the internal per-key state. Deleted keys stay in the map as
// tombstones so their last version is never reused.
type record struct {
	value   string
	version uint64
	deleted bool
}

// Store is a goroutine-safe, versioned, in-memory key-value store.
// The zero value is not usable; create one with New.
type Store struct {
	mu      sync.RWMutex
	records map[string]record
	live    int
}

// New returns an empty Store ready for use.
func New() *Store {
	return &Store{records: make(map[string]record)}
}

// Put sets key to value and returns the newly assigned version. The first
// write to a key gets version 1; every later write to the same key gets a
// strictly larger version, even if the key was deleted in between.
func (s *Store) Put(key, value string) (uint64, error) {
	if key == "" {
		return 0, ErrEmptyKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[key]
	if !ok || rec.deleted {
		s.live++
	}
	rec.value = value
	rec.version++
	rec.deleted = false
	s.records[key] = rec
	return rec.version, nil
}

// Get returns a copy of the entry for key. found is false when the key does
// not exist (never written, or deleted).
func (s *Store) Get(key string) (entry Entry, found bool, err error) {
	if key == "" {
		return Entry{}, false, ErrEmptyKey
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[key]
	if !ok || rec.deleted {
		return Entry{}, false, nil
	}
	return Entry{Key: key, Value: rec.value, Version: rec.version}, true, nil
}

// CompareAndSwap atomically replaces the value of key with newValue when the
// key exists and its current version equals expectedVersion. The check and
// the update happen under a single lock, so two callers cannot both succeed
// with the same expected version.
//
// It returns the outcome and the key's version after the call: the new
// version on success, the current version on mismatch, and 0 when the key is
// absent.
func (s *Store) CompareAndSwap(key string, expectedVersion uint64, newValue string) (CASResult, uint64, error) {
	if key == "" {
		return CASKeyNotFound, 0, ErrEmptyKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[key]
	if !ok || rec.deleted {
		return CASKeyNotFound, 0, nil
	}
	if rec.version != expectedVersion {
		return CASVersionMismatch, rec.version, nil
	}
	rec.value = newValue
	rec.version++
	s.records[key] = rec
	return CASSuccess, rec.version, nil
}

// Delete removes the value of key but keeps its last version, so a later Put
// continues with a strictly larger version instead of restarting at 1.
// It reports whether the key existed.
func (s *Store) Delete(key string) (bool, error) {
	if key == "" {
		return false, ErrEmptyKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[key]
	if !ok || rec.deleted {
		return false, nil
	}
	rec.value = ""
	rec.deleted = true
	s.records[key] = rec
	s.live--
	return true, nil
}

// Snapshot returns a copy of all live entries sorted by key in lexicographic
// order. Mutating the returned slice or its entries does not affect the
// store.
func (s *Store) Snapshot() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]Entry, 0, s.live)
	for key, rec := range s.records {
		if rec.deleted {
			continue
		}
		entries = append(entries, Entry{Key: key, Value: rec.value, Version: rec.version})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}

// Len returns the number of live (not deleted) keys in the store.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.live
}

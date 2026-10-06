// Package versionedkv implements a concurrent, versioned, in-memory
// key-value store. Every key maps to a string value and a uint64
// version that grows monotonically with each successful modification
// of that key, even across deletes.
package versionedkv

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	// ErrEmptyKey is returned when an operation is attempted with an empty key.
	ErrEmptyKey = errors.New("versionedkv: key must not be empty")
	// ErrKeyNotFound is returned by CompareAndSwap when the key does not exist.
	ErrKeyNotFound = errors.New("versionedkv: key not found")
	// ErrVersionMismatch is returned by CompareAndSwap when the expected
	// version does not match the key's current version.
	ErrVersionMismatch = errors.New("versionedkv: version mismatch")
)

// Item is a single versioned key-value pair, as returned by Snapshot.
type Item struct {
	Key     string
	Value   string
	Version uint64
}

// entry is the internal record for a key. Deleted keys are kept as
// tombstones (deleted == true) so their versions are never reused.
type entry struct {
	value   string
	version uint64
	deleted bool
}

// Store is a concurrent, versioned, in-memory key-value store.
// Use New to create one. It is safe for concurrent use by multiple
// goroutines.
type Store struct {
	mu      sync.RWMutex
	entries map[string]entry
	live    int // number of keys currently present (not deleted)
}

// New returns an empty Store ready for use.
func New() *Store {
	return &Store{entries: make(map[string]entry)}
}

// Put sets key to value and returns the new version.
//
// The first Put of a key yields version 1. Every later successful
// modification of the key (Put, CompareAndSwap, Delete) yields a
// strictly larger version, and versions are never reused, even after
// the key is deleted and created again.
func (s *Store) Put(key, value string) (uint64, error) {
	if key == "" {
		return 0, ErrEmptyKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || e.deleted {
		s.live++
	}
	e.value = value
	e.version++
	e.deleted = false
	s.entries[key] = e
	return e.version, nil
}

// Get returns the value and version currently stored for key.
// ok is false when the key is empty, missing, or deleted.
func (s *Store) Get(key string) (value string, version uint64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key]
	if !ok || e.deleted {
		return "", 0, false
	}
	return e.value, e.version, true
}

// CompareAndSwap atomically sets key to newValue, but only when the
// key exists and its current version equals expectedVersion. On
// success it returns the new version. It fails with ErrKeyNotFound
// when the key does not exist (deleted keys count as missing) and
// with ErrVersionMismatch when the version does not match; a failed
// call leaves the store unchanged.
func (s *Store) CompareAndSwap(key string, expectedVersion uint64, newValue string) (uint64, error) {
	if key == "" {
		return 0, ErrEmptyKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || e.deleted {
		return 0, ErrKeyNotFound
	}
	if e.version != expectedVersion {
		return 0, fmt.Errorf("%w: expected %d, current %d", ErrVersionMismatch, expectedVersion, e.version)
	}
	e.value = newValue
	e.version++
	s.entries[key] = e
	return e.version, nil
}

// Delete removes key and reports whether it existed. The key's
// version keeps advancing on delete and is retained as a tombstone,
// so a later Put continues with a larger version instead of
// restarting at 1. Deleting a missing or already deleted key is not
// an error and reports false.
func (s *Store) Delete(key string) (bool, error) {
	if key == "" {
		return false, ErrEmptyKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || e.deleted {
		return false, nil
	}
	e.value = ""
	e.version++
	e.deleted = true
	s.entries[key] = e
	s.live--
	return true, nil
}

// Snapshot returns a copy of all currently existing keys as Items
// sorted by key in lexicographic order. The result is independent of
// later mutations of the store, and mutating it does not affect the
// store.
func (s *Store) Snapshot() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Item, 0, s.live)
	for key, e := range s.entries {
		if e.deleted {
			continue
		}
		items = append(items, Item{Key: key, Value: e.value, Version: e.version})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items
}

// Len returns the number of keys currently present in the store.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.live
}

package versionedkv_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"versionedkv"
)

func TestPutGet(t *testing.T) {
	s := versionedkv.New()

	version, err := s.Put("name", "alice")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if version != 1 {
		t.Fatalf("first Put version = %d, want 1", version)
	}

	value, gotVersion, ok := s.Get("name")
	if !ok {
		t.Fatal("Get: key not found")
	}
	if value != "alice" || gotVersion != 1 {
		t.Fatalf("Get = (%q, %d), want (%q, 1)", value, gotVersion, "alice")
	}

	if _, _, ok := s.Get("missing"); ok {
		t.Fatal("Get on missing key: ok = true, want false")
	}
}

func TestVersionIncrements(t *testing.T) {
	s := versionedkv.New()
	for want := uint64(1); want <= 3; want++ {
		got, err := s.Put("k", "v")
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if got != want {
			t.Fatalf("Put version = %d, want %d", got, want)
		}
	}

	// Versions are independent per key.
	got, err := s.Put("other", "v")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got != 1 {
		t.Fatalf("first version of a new key = %d, want 1", got)
	}
}

func TestCompareAndSwapSuccess(t *testing.T) {
	s := versionedkv.New()
	if _, err := s.Put("k", "old"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	version, err := s.CompareAndSwap("k", 1, "new")
	if err != nil {
		t.Fatalf("CompareAndSwap: %v", err)
	}
	if version != 2 {
		t.Fatalf("CAS new version = %d, want 2", version)
	}

	value, gotVersion, ok := s.Get("k")
	if !ok || value != "new" || gotVersion != 2 {
		t.Fatalf("Get = (%q, %d, %v), want (%q, 2, true)", value, gotVersion, ok, "new")
	}
}

func TestCompareAndSwapFailures(t *testing.T) {
	s := versionedkv.New()

	if _, err := s.CompareAndSwap("absent", 1, "v"); !errors.Is(err, versionedkv.ErrKeyNotFound) {
		t.Fatalf("CAS on absent key err = %v, want ErrKeyNotFound", err)
	}

	if _, err := s.Put("k", "v"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.CompareAndSwap("k", 7, "v2"); !errors.Is(err, versionedkv.ErrVersionMismatch) {
		t.Fatalf("CAS with wrong version err = %v, want ErrVersionMismatch", err)
	}
	// A failed CAS must not change anything.
	if value, version, _ := s.Get("k"); value != "v" || version != 1 {
		t.Fatalf("after failed CAS Get = (%q, %d), want (%q, 1)", value, version, "v")
	}

	// A deleted key counts as not found.
	if _, err := s.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.CompareAndSwap("k", 1, "v2"); !errors.Is(err, versionedkv.ErrKeyNotFound) {
		t.Fatalf("CAS on deleted key err = %v, want ErrKeyNotFound", err)
	}
}

func TestDeleteAndRePutKeepsVersionMonotonic(t *testing.T) {
	s := versionedkv.New()
	if _, err := s.Put("k", "a"); err != nil { // version 1
		t.Fatalf("Put: %v", err)
	}

	deleted, err := s.Delete("k") // tombstone at version 2
	if err != nil || !deleted {
		t.Fatalf("Delete = (%v, %v), want (true, nil)", deleted, err)
	}
	if _, _, ok := s.Get("k"); ok {
		t.Fatal("Get after Delete: ok = true, want false")
	}

	version, err := s.Put("k", "b")
	if err != nil {
		t.Fatalf("Put after Delete: %v", err)
	}
	if version != 3 {
		t.Fatalf("re-Put version = %d, want 3 (versions must keep increasing)", version)
	}

	// Deleting a missing key reports false and no error.
	if deleted, err := s.Delete("missing"); deleted || err != nil {
		t.Fatalf("Delete(missing) = (%v, %v), want (false, nil)", deleted, err)
	}
}

func TestSnapshotSortedAndIsolated(t *testing.T) {
	s := versionedkv.New()
	for _, kv := range [][2]string{{"b", "2"}, {"a", "1"}, {"c", "3"}} {
		if _, err := s.Put(kv[0], kv[1]); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if _, err := s.Delete("b"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	snap := s.Snapshot()
	if len(snap) != 2 || snap[0].Key != "a" || snap[1].Key != "c" {
		t.Fatalf("Snapshot = %v, want keys [a c]", snap)
	}

	// Later mutations must not affect the snapshot.
	if _, err := s.Put("a", "changed"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Delete("c"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(snap) != 2 || snap[0].Value != "1" || snap[1].Value != "3" {
		t.Fatalf("Snapshot changed after store mutation: %v", snap)
	}

	// Mutating the snapshot must not affect the store.
	snap[0].Value = "hacked"
	if value, _, _ := s.Get("a"); value != "changed" {
		t.Fatalf("store value = %q, want %q", value, "changed")
	}
}

func TestInvalidKey(t *testing.T) {
	s := versionedkv.New()

	if _, err := s.Put("", "v"); !errors.Is(err, versionedkv.ErrEmptyKey) {
		t.Fatalf("Put with empty key err = %v, want ErrEmptyKey", err)
	}
	if _, err := s.CompareAndSwap("", 1, "v"); !errors.Is(err, versionedkv.ErrEmptyKey) {
		t.Fatalf("CAS with empty key err = %v, want ErrEmptyKey", err)
	}
	if _, err := s.Delete(""); !errors.Is(err, versionedkv.ErrEmptyKey) {
		t.Fatalf("Delete with empty key err = %v, want ErrEmptyKey", err)
	}
	if _, _, ok := s.Get(""); ok {
		t.Fatal("Get with empty key: ok = true, want false")
	}
	if n := s.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0", n)
	}
}

func TestLen(t *testing.T) {
	s := versionedkv.New()
	if n := s.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0", n)
	}

	mustPut(t, s, "a", "1")
	mustPut(t, s, "b", "2")
	mustPut(t, s, "a", "3") // update existing key
	if n := s.Len(); n != 2 {
		t.Fatalf("Len = %d, want 2", n)
	}

	if _, err := s.Delete("a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n := s.Len(); n != 1 {
		t.Fatalf("Len after Delete = %d, want 1", n)
	}

	mustPut(t, s, "a", "4") // resurrect deleted key
	if n := s.Len(); n != 2 {
		t.Fatalf("Len after re-Put = %d, want 2", n)
	}
}

func TestConcurrentCompareAndSwap(t *testing.T) {
	s := versionedkv.New()
	if _, err := s.Put("k", "v0"); err != nil { // version 1
		t.Fatalf("Put: %v", err)
	}

	// Many goroutines race to CAS the same key from the same expected
	// version; exactly one of them may win.
	const goroutines = 16
	var successes atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if _, err := s.CompareAndSwap("k", 1, "v1"); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("successful CAS count = %d, want exactly 1", got)
	}
	if _, version, _ := s.Get("k"); version != 2 {
		t.Fatalf("final version = %d, want 2", version)
	}
}

func mustPut(t *testing.T, s *versionedkv.Store, key, value string) {
	t.Helper()
	if _, err := s.Put(key, value); err != nil {
		t.Fatalf("Put(%q): %v", key, err)
	}
}

package kvstore_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"versionedkv/kvstore"
)

func TestPutAndGet(t *testing.T) {
	store := kvstore.New()
	version, err := store.Put("a", "hello")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if version != 1 {
		t.Fatalf("first Put version = %d, want 1", version)
	}
	entry, found, err := store.Get("a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get: key not found")
	}
	if entry.Value != "hello" || entry.Version != 1 {
		t.Fatalf("Get = %+v, want value=hello version=1", entry)
	}
}

func TestVersionIncrementsOnUpdate(t *testing.T) {
	store := kvstore.New()
	for want := uint64(1); want <= 3; want++ {
		got, err := store.Put("k", fmt.Sprintf("v%d", want))
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if got != want {
			t.Fatalf("Put version = %d, want %d", got, want)
		}
	}
	entry, found, _ := store.Get("k")
	if !found || entry.Value != "v3" || entry.Version != 3 {
		t.Fatalf("Get = %+v found=%v, want value=v3 version=3", entry, found)
	}
}

func TestCompareAndSwapSuccess(t *testing.T) {
	store := kvstore.New()
	version, _ := store.Put("k", "old")
	result, newVersion, err := store.CompareAndSwap("k", version, "new")
	if err != nil {
		t.Fatalf("CompareAndSwap: %v", err)
	}
	if result != kvstore.CASSuccess {
		t.Fatalf("result = %v, want CASSuccess", result)
	}
	if newVersion != version+1 {
		t.Fatalf("new version = %d, want %d", newVersion, version+1)
	}
	entry, _, _ := store.Get("k")
	if entry.Value != "new" || entry.Version != version+1 {
		t.Fatalf("Get = %+v, want value=new version=%d", entry, version+1)
	}
}

func TestCompareAndSwapVersionMismatch(t *testing.T) {
	store := kvstore.New()
	store.Put("k", "v")
	result, current, err := store.CompareAndSwap("k", 99, "x")
	if err != nil {
		t.Fatalf("CompareAndSwap: %v", err)
	}
	if result != kvstore.CASVersionMismatch {
		t.Fatalf("result = %v, want CASVersionMismatch", result)
	}
	if current != 1 {
		t.Fatalf("reported current version = %d, want 1", current)
	}
	entry, _, _ := store.Get("k")
	if entry.Value != "v" || entry.Version != 1 {
		t.Fatalf("entry changed after failed CAS: %+v", entry)
	}
}

func TestCompareAndSwapKeyNotFound(t *testing.T) {
	store := kvstore.New()
	result, _, err := store.CompareAndSwap("missing", 1, "x")
	if err != nil {
		t.Fatalf("CompareAndSwap: %v", err)
	}
	if result != kvstore.CASKeyNotFound {
		t.Fatalf("result = %v, want CASKeyNotFound", result)
	}
	store.Put("gone", "v")
	store.Delete("gone")
	result, _, _ = store.CompareAndSwap("gone", 1, "x")
	if result != kvstore.CASKeyNotFound {
		t.Fatalf("CAS on deleted key = %v, want CASKeyNotFound", result)
	}
}

func TestDeleteThenRePutKeepsVersionMonotonic(t *testing.T) {
	store := kvstore.New()
	v1, _ := store.Put("k", "a")
	deleted, err := store.Delete("k")
	if err != nil || !deleted {
		t.Fatalf("Delete = %v, %v, want true, nil", deleted, err)
	}
	if _, found, _ := store.Get("k"); found {
		t.Fatal("Get after Delete: key still found")
	}
	if deleted, _ = store.Delete("k"); deleted {
		t.Fatal("second Delete = true, want false")
	}
	v2, _ := store.Put("k", "b")
	if v2 <= v1 {
		t.Fatalf("re-Put version = %d, want > %d (no restart, no reuse)", v2, v1)
	}
}

func TestSnapshotSortedAndLiveOnly(t *testing.T) {
	store := kvstore.New()
	store.Put("delta", "d")
	store.Put("alpha", "a")
	store.Put("charlie", "c")
	store.Put("bravo", "b")
	store.Delete("charlie")
	snap := store.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("Snapshot len = %d, want 3 (deleted key excluded)", len(snap))
	}
	wantKeys := []string{"alpha", "bravo", "delta"}
	for i, want := range wantKeys {
		if snap[i].Key != want {
			t.Fatalf("Snapshot[%d].Key = %q, want %q (lexicographic order)", i, snap[i].Key, want)
		}
	}
}

func TestSnapshotIsolation(t *testing.T) {
	store := kvstore.New()
	store.Put("a", "1")
	store.Put("b", "2")
	snap := store.Snapshot()
	snap[1].Value = "mutated"
	store.Put("a", "changed")
	entry, _, _ := store.Get("b")
	if entry.Value != "2" {
		t.Fatal("mutating the snapshot affected the store")
	}
	if snap[0].Value != "1" {
		t.Fatalf("snapshot entry changed by later Put: %q", snap[0].Value)
	}
	fresh := store.Snapshot()
	if fresh[0].Value != "changed" {
		t.Fatalf("fresh snapshot = %q, want updated value", fresh[0].Value)
	}
}

func TestEmptyKeyRejected(t *testing.T) {
	store := kvstore.New()
	if _, err := store.Put("", "v"); !errors.Is(err, kvstore.ErrEmptyKey) {
		t.Fatalf("Put empty key err = %v, want ErrEmptyKey", err)
	}
	if _, _, err := store.Get(""); !errors.Is(err, kvstore.ErrEmptyKey) {
		t.Fatalf("Get empty key err = %v, want ErrEmptyKey", err)
	}
	if _, _, err := store.CompareAndSwap("", 1, "v"); !errors.Is(err, kvstore.ErrEmptyKey) {
		t.Fatalf("CompareAndSwap empty key err = %v, want ErrEmptyKey", err)
	}
	if _, err := store.Delete(""); !errors.Is(err, kvstore.ErrEmptyKey) {
		t.Fatalf("Delete empty key err = %v, want ErrEmptyKey", err)
	}
	if store.Len() != 0 {
		t.Fatalf("Len = %d after rejected writes, want 0", store.Len())
	}
}

func TestLen(t *testing.T) {
	store := kvstore.New()
	if store.Len() != 0 {
		t.Fatalf("Len = %d, want 0", store.Len())
	}
	store.Put("a", "1")
	store.Put("b", "2")
	if store.Len() != 2 {
		t.Fatalf("Len = %d, want 2", store.Len())
	}
	store.Put("a", "updated")
	if store.Len() != 2 {
		t.Fatalf("Len = %d after overwrite, want 2", store.Len())
	}
	store.Delete("a")
	if store.Len() != 1 {
		t.Fatalf("Len = %d after Delete, want 1", store.Len())
	}
	store.Put("a", "back")
	if store.Len() != 2 {
		t.Fatalf("Len = %d after re-Put, want 2", store.Len())
	}
}

func TestConcurrentCompareAndSwap(t *testing.T) {
	store := kvstore.New()
	version, _ := store.Put("k", "v0")
	const workers = 8
	var won int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, _, err := store.CompareAndSwap("k", version, fmt.Sprintf("w%d", i))
			if err == nil && result == kvstore.CASSuccess {
				atomic.AddInt32(&won, 1)
			}
		}(i)
	}
	wg.Wait()
	if got := atomic.LoadInt32(&won); got != 1 {
		t.Fatalf("%d goroutines won the CAS, want exactly 1", got)
	}
	entry, _, _ := store.Get("k")
	if entry.Version != version+1 {
		t.Fatalf("final version = %d, want %d", entry.Version, version+1)
	}
}

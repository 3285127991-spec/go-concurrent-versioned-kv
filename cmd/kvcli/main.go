// Command kvcli is a minimal, non-interactive demo of the kvstore library.
// It uses only built-in data: no flags, no files, no input loops.
package main

import (
	"fmt"
	"log"

	"versionedkv/kvstore"
)

func main() {
	store := kvstore.New()

	v1, err := store.Put("alpha", "one")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Put alpha=one            -> version %d\n", v1)
	if _, err := store.Put("beta", "two"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Put beta=two             -> version 1")

	entry, found, err := store.Get("alpha")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Get alpha                -> %q (version %d, found=%t)\n", entry.Value, entry.Version, found)

	result, version, err := store.CompareAndSwap("alpha", v1, "ONE")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("CAS alpha expect v%d      -> %s (now version %d)\n", v1, result, version)

	result, version, err = store.CompareAndSwap("alpha", v1, "uno")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("CAS alpha expect v%d again -> %s (current version %d, stale rejected)\n", v1, result, version)

	deleted, err := store.Delete("alpha")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Delete alpha             -> existed=%t\n", deleted)

	v3, err := store.Put("alpha", "reborn")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Put alpha=reborn         -> version %d (monotonic, never restarts at 1)\n", v3)

	fmt.Printf("Len                      -> %d live keys\n", store.Len())
}

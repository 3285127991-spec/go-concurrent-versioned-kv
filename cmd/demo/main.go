// Command demo shows the basic versionedkv operations with a few
// built-in values: Put, Get, a successful and a failed
// CompareAndSwap, Delete, and re-Put after Delete.
package main

import (
	"errors"
	"fmt"

	"versionedkv"
)

func main() {
	store := versionedkv.New()

	v1, _ := store.Put("color", "red")
	fmt.Printf("Put(color, red)        -> version=%d\n", v1)

	if value, version, ok := store.Get("color"); ok {
		fmt.Printf("Get(color)             -> value=%q version=%d\n", value, version)
	}

	v2, err := store.CompareAndSwap("color", v1, "blue")
	fmt.Printf("CAS(color, v%d, blue)   -> version=%d err=%v\n", v1, v2, err)

	if _, err := store.CompareAndSwap("color", v1, "green"); errors.Is(err, versionedkv.ErrVersionMismatch) {
		fmt.Printf("CAS(color, v%d, green)  -> failed as expected: %v\n", v1, err)
	}

	deleted, _ := store.Delete("color")
	fmt.Printf("Delete(color)          -> deleted=%v\n", deleted)

	if _, _, ok := store.Get("color"); !ok {
		fmt.Println("Get(color)             -> not found (deleted)")
	}

	v3, _ := store.Put("color", "yellow")
	fmt.Printf("Put(color, yellow)     -> version=%d (still increasing after delete)\n", v3)
}

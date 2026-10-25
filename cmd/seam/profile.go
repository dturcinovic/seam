package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
)

// createHeapProfile creates the file of the heap profile, unless path is
// empty: a path that cannot be written fails before the session rather than
// after it, with the profile it was to hold lost.
func createHeapProfile(path string) error {
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		return fmt.Errorf("heap profile: %w", err)
	}
	return nil
}

// writeHeapProfile writes the heap profile to path, unless path is empty.
// It collects first: the profile shows the heap as of the last collection,
// which may be long past when a session ends.
func writeHeapProfile(path string) error {
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("heap profile: %w", err)
	}
	runtime.GC()
	err = pprof.WriteHeapProfile(f)
	if err := errors.Join(err, f.Close()); err != nil {
		return fmt.Errorf("heap profile: %w", err)
	}
	return nil
}

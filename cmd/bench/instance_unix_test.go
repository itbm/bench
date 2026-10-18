//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestBenchRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.github.itbm.bench.lock")
	check := func(want bool) {
		t.Helper()
		if got, err := benchRunning(path); got != want || err != nil {
			t.Errorf("benchRunning() = %v, %v; want %v", got, err, want)
		}
	}
	check(false) // No lock file: Bench has never run, or quit.
	if _, err := os.Stat(path); err == nil {
		t.Error("benchRunning made the lock file")
	}

	// Bench running: Wails holds an exclusive lock on the file.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	check(true)
	check(true) // Checking didn't take the lock away.

	// A crashed Bench: the file is there, but nobody holds the lock.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	check(false)
	// Checking let the lock go again, so Bench can start.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Errorf("Bench couldn't take its lock after benchRunning: %v", err)
	}
}

//go:build unix

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// benchRunning reports whether Bench is running: whether the flock(2) lock
// that Wails' single-instance check takes on lockPath is held. It takes a
// shared lock for a moment to find out. It never creates the file, which
// Wails removes when Bench quits; one that a Bench that crashed left isn't
// locked.
func benchRunning(lockPath string) (bool, error) {
	f, err := os.Open(lockPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking whether Bench is running: %w", err)
	}
	defer f.Close()
	switch err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); {
	case err == nil:
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return false, nil
	case errors.Is(err, syscall.EWOULDBLOCK):
		return true, nil
	default:
		return false, fmt.Errorf("checking whether Bench is running: %w", err)
	}
}

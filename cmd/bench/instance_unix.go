//go:build unix

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// lockInstance takes the flock(2) lock that Wails' single-instance check
// takes on lockPath, and holds it until unlock is called, so that Bench
// can't start meanwhile: Wails would take it for a second Bench, which only
// brings the first forward and exits. If another process holds the lock,
// which means Bench is running, it returns errLocked. It leaves the file
// when it lets the lock go: Wails creates it if it's missing, and a file
// that nobody holds a lock on doesn't count as Bench running.
func lockInstance(lockPath string) (unlock func(), err error) {
	for {
		f, err := flockFile(lockPath, nil)
		if err != nil {
			if errors.Is(err, errLocked) {
				return nil, err
			}
			return nil, fmt.Errorf("checking whether Bench is running: %w", err)
		}
		// A Bench that quits lets the lock go, then removes the file. If
		// that happened between opening the file and locking it, the lock
		// is on a file that a Bench starting now wouldn't see, so it
		// tries again.
		if stillAt(f, lockPath) {
			return func() { _ = f.Close() }, nil
		}
		_ = f.Close()
	}
}

// stillAt reports whether the open file f is still the file at path, and
// hasn't been removed or replaced.
func stillAt(f *os.File, path string) bool {
	open, err := f.Stat()
	if err != nil {
		return false
	}
	now, err := os.Stat(path)
	return err == nil && os.SameFile(open, now)
}

// lockDir takes the lock on lockFile in the support folder dir, which every
// process that acts on its parked.json holds while it does, and holds it
// until unlock is called. If another process holds it, it returns
// errLocked, or, if wait isn't nil, calls wait and waits for the lock. A
// folder that doesn't exist holds nothing to guard, so then it does nothing.
func lockDir(dir string, wait func()) (unlock func(), err error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return func() {}, nil
	}
	f, err := flockFile(filepath.Join(dir, lockFile), wait)
	if err != nil {
		if errors.Is(err, errLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("locking the support folder: %w", err)
	}
	return func() { _ = f.Close() }, nil
}

// flockFile opens path, creating it if needed, and takes an exclusive
// flock(2) lock on it, which lasts until the file is closed (or this process
// ends, however it ends). Go opens files close-on-exec, so child processes,
// such as terminals' shells, never keep the lock after this process ends.
// If another process holds the lock, it returns errLocked, or, if wait isn't
// nil, calls wait and waits for the lock.
func flockFile(path string, wait func()) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	err = flock(f, syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		err = errLocked
		if wait != nil {
			wait()
			err = flock(f, syscall.LOCK_EX)
		}
	}
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

// flock is flock(2), tried again if a signal interrupts it.
func flock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

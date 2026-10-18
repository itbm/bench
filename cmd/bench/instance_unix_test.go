//go:build unix

package main

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/itbm/bench/internal/windows"
)

// wailsLock takes the lock as Wails' single-instance check does on macOS,
// returning the file, or nil if another process holds the lock.
func wailsLock(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			t.Fatal(err)
		}
		return nil
	}
	return f
}

// TestLockInstance checks that bench arrange holds Wails' single-instance
// lock for as long as it runs, so that the app can't start meanwhile, and
// that it can't start while the app (or another bench arrange) holds it.
func TestLockInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.github.itbm.bench.lock")

	// Bench isn't running (it never has, or it crashed and left the file).
	unlock, err := lockInstance(path)
	if err != nil {
		t.Fatalf("lockInstance() = %v, want the lock", err)
	}
	if f := wailsLock(t, path); f != nil {
		f.Close()
		t.Error("Bench could start while bench arrange ran")
	}
	if _, err := lockInstance(path); !errors.Is(err, errLocked) {
		t.Errorf("a second lockInstance() = %v, want errLocked", err)
	}
	unlock()

	// Bench is running: Wails holds the lock.
	f := wailsLock(t, path)
	if f == nil {
		t.Fatal("Bench couldn't take its lock after bench arrange")
	}
	defer f.Close()
	if _, err := lockInstance(path); !errors.Is(err, errLocked) {
		t.Errorf("lockInstance() while Bench runs = %v, want errLocked", err)
	}
}

// TestStillAt checks how bench arrange tells that the single-instance lock
// it took is on a file that a Bench quitting has just removed, which a
// Bench starting next wouldn't see.
func TestStillAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.github.itbm.bench.lock")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !stillAt(f, path) {
		t.Error("stillAt() = false for the file at the path")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if stillAt(f, path) {
		t.Error("stillAt() = true once the file was removed")
	}
	g, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if stillAt(f, path) {
		t.Error("stillAt() = true for a removed file once another took its place")
	}
}

// TestLockDir checks the support folder's lock: a process that waits for
// it gets it once its holder lets it go.
func TestLockDir(t *testing.T) {
	dir := t.TempDir()
	unlock, err := lockDir(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, lockFile)); err != nil {
		t.Errorf("there's no lock file: %v", err)
	}
	if _, err := lockDir(dir, nil); !errors.Is(err, errLocked) {
		t.Errorf("lockDir() while it's held = %v, want errLocked", err)
	}
	got := make(chan error, 1)
	go func() {
		unlock, err := lockDir(dir, func() {})
		if err == nil {
			unlock()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("lockDir() with wait returned %v while the lock was held", err)
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	select {
	case err := <-got:
		if err != nil {
			t.Errorf("lockDir() with wait = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lockDir() with wait didn't get the lock once it was free")
	}
}

// TestLockSupportDir checks that the app's lock creates the support folder,
// which doesn't exist on first run, so that it guards it from the start.
func TestLockSupportDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Bench")
	var stderr strings.Builder
	unlock, err := lockSupportDir(&stderr, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockDir(dir, nil); !errors.Is(err, errLocked) {
		t.Errorf("lockDir() while the app holds the lock = %v, want errLocked", err)
	}
	unlock()
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing, as nothing else held the lock", stderr.String())
	}
}

// TestRestoreRefusesWhileTheFolderIsLocked checks that bench restore leaves
// alone a support folder that a running Bench or bench arrange holds: it
// would put back the windows they have parked, and delete their records.
func TestRestoreRefusesWhileTheFolderIsLocked(t *testing.T) {
	dir := t.TempDir()
	writeParked(t, dir)
	unlock, err := lockDir(dir, nil) // Bench is running.
	if err != nil {
		t.Fatal(err)
	}
	put := 0
	count := func(string) (windows.Restored, error) {
		put++
		return windows.Restored{Windows: 1}, nil
	}
	if _, err := restoreNow(dir, count); !errors.Is(err, errFolderBusy) {
		t.Errorf("restoreNow() while Bench runs = %v, want errFolderBusy", err)
	}
	if put != 0 {
		t.Error("restoreNow put windows back while Bench runs")
	}

	unlock() // Bench has crashed.
	if got, err := restoreNow(dir, count); err != nil || got.Windows != 1 || put != 1 {
		t.Errorf("restoreNow() after Bench = %+v, %v (put %d times); want the window put back", got, err, put)
	}
	// And it let the lock go again, so that Bench can start.
	unlock, err = lockDir(dir, nil)
	if err != nil {
		t.Fatalf("lockDir after restoreNow = %v", err)
	}
	unlock()

	// A folder that doesn't exist has nothing to put back.
	if got, err := restoreNow(filepath.Join(dir, "missing"), count); err != nil || got != (windows.Restored{}) {
		t.Errorf("restoreNow(a missing folder) = %+v, %v; want nothing", got, err)
	}
}

// TestWatchdogWaitsForTheLock checks that the watchdog of a Bench that has
// gone doesn't touch parked.json while another process holds the support
// folder's lock, such as a Bench that has started again and parked windows
// of its own, and that it finishes once the lock is free.
func TestWatchdogWaitsForTheLock(t *testing.T) {
	dir := t.TempDir()
	writeParked(t, dir)
	unlock, err := lockDir(dir, nil) // The new Bench.
	if err != nil {
		t.Fatal(err)
	}
	var put atomic.Int32
	waiting := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- waitAndRestore(strings.NewReader(""), dir, func(string) (windows.Restored, error) {
			put.Add(1)
			return windows.Restored{}, nil
		}, func() { close(waiting) })
	}()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the watchdog didn't wait for the lock")
	}
	time.Sleep(50 * time.Millisecond)
	if put.Load() != 0 {
		t.Fatal("the watchdog put windows back while another Bench held the lock")
	}
	unlock() // The new Bench quits.
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("waitAndRestore() = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watchdog didn't finish once the lock was free")
	}
	if put.Load() != 1 {
		t.Errorf("the watchdog put windows back %d times, want once", put.Load())
	}
}

// TestWatchdogRestoresWithoutTheLock checks that the watchdog still puts
// windows back from a folder whose lock it can't take at all, as it's the
// last thing that can, and reports the problem.
func TestWatchdogRestoresWithoutTheLock(t *testing.T) {
	dir := t.TempDir()
	writeParked(t, dir)
	// A folder in the lock file's place can't be opened for writing.
	if err := os.Mkdir(filepath.Join(dir, lockFile), 0o755); err != nil {
		t.Fatal(err)
	}
	put := 0
	err := waitAndRestore(strings.NewReader(""), dir, func(string) (windows.Restored, error) {
		put++
		return windows.Restored{Windows: 1}, nil
	}, func() {})
	if err == nil {
		t.Error("waitAndRestore() = nil without the lock, want the error")
	}
	if put != 1 {
		t.Errorf("the watchdog put windows back %d times without the lock, want once", put)
	}
}

// TestQuitSignals checks that bench arrange puts everything back when its
// terminal closes, unless it was started to outlive it (nohup).
func TestQuitSignals(t *testing.T) {
	if signal.Ignored(syscall.SIGHUP) {
		t.Skip("SIGHUP is ignored, as under nohup")
	}
	if got := quitSignals(); !slices.Contains(got, os.Signal(syscall.SIGHUP)) ||
		!slices.Contains(got, os.Interrupt) || !slices.Contains(got, os.Signal(syscall.SIGTERM)) {
		t.Errorf("quitSignals() = %v, want Control-C, SIGTERM, and SIGHUP", got)
	}
	signal.Ignore(syscall.SIGHUP)
	defer signal.Reset(syscall.SIGHUP)
	if slices.Contains(quitSignals(), os.Signal(syscall.SIGHUP)) {
		t.Error("quitSignals() includes SIGHUP while it's ignored")
	}
}

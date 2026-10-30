//go:build (darwin || server) && unix

package ui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestReopenTarget(t *testing.T) {
	for exe, want := range map[string]string{
		"/Applications/Bench.app/Contents/MacOS/Bench":         "/Applications/Bench.app",
		"/Users/sam/Apps/My Tools.app/Contents/MacOS/Bench":    "/Users/sam/Apps/My Tools.app",
		"/Users/sam/code/bench/bin/Bench.app/Contents/MacOS/x": "/Users/sam/code/bench/bin/Bench.app",
		"/Users/sam/code/bench/bin/bench":                      "/Users/sam/code/bench/bin/bench",
	} {
		if got := reopenTarget(exe); got != want {
			t.Errorf("reopenTarget(%q) = %q, want %q", exe, got, want)
		}
	}
}

// TestReopenCommand checks that the command opens Bench again only once
// the process that started it has ended.
func TestReopenCommand(t *testing.T) {
	dir := t.TempDir()
	opened := filepath.Join(dir, "opened")
	target := filepath.Join(dir, "bench")
	if err := os.WriteFile(target, []byte("#!/bin/sh\ntouch '"+opened+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	bench := exec.Command("sleep", "0.3")
	if err := bench.Start(); err != nil {
		t.Fatal(err)
	}
	reopen := reopenCommand(bench.Process.Pid, target)
	if err := reopen.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(opened); err == nil {
		t.Error("Bench opened again before it had quit")
	}
	_ = bench.Wait()
	if err := reopen.Wait(); err != nil {
		t.Fatalf("the command that opens Bench again failed: %v", err)
	}
	if _, err := os.Stat(opened); err != nil {
		t.Error("Bench didn't open again once it had quit")
	}
}

// TestRelaunch checks that the shell quits only once it has arranged for
// Bench to open again, and otherwise says so.
func TestRelaunch(t *testing.T) {
	f := newFakeEngine()
	s, quits := quitShell(f)
	reopens := 0
	s.openAgain = func() error { reopens++; return nil }
	s.relaunch()
	waitFor(t, "the quit", func() bool { return quits.Load() == 1 })
	if reopens != 1 || !s.shouldQuit() {
		t.Errorf("arranged to open again %d times, and the quit went through %v; want once, and true", reopens, s.shouldQuit())
	}

	s, quits = quitShell(newFakeEngine())
	var toasts []string
	s.toast = func(message string) { toasts = append(toasts, message) }
	s.openAgain = func() error { return errors.New("no /bin/sh") }
	s.relaunch()
	time.Sleep(20 * time.Millisecond)
	if quits.Load() != 0 {
		t.Error("Bench quit though it couldn't open again")
	}
	if !slices.Equal(toasts, []string{cantReopenText}) {
		t.Errorf("toasts = %q, want one that says Bench hasn't quit", toasts)
	}
}

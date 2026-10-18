//go:build darwin || server

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWatchdogArgs(t *testing.T) {
	got := watchdogArgs("/Users/me/Library/Application Support/Bench")
	want := []string{"watchdog", "--dir", "/Users/me/Library/Application Support/Bench"}
	if !slices.Equal(got, want) {
		t.Errorf("watchdogArgs = %q, want %q", got, want)
	}
}

// TestPipedProcessWaitsForThePipe checks that the child runs until the pipe
// closes, which is how the watchdog knows Bench has gone.
func TestPipedProcessWaitsForThePipe(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	marker := filepath.Join(t.TempDir(), "done")
	p, err := startPiped(sh, []string{"-c", `cat >/dev/null; echo done > "$0"`, marker})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the child finished before the pipe closed")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the child didn't finish once the pipe closed: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("a second Close() = %v, want nil", err)
	}
}

func TestStartPipedFails(t *testing.T) {
	if _, err := startPiped(filepath.Join(t.TempDir(), "missing"), nil); err == nil {
		t.Error("startPiped with a missing program succeeded")
	}
}

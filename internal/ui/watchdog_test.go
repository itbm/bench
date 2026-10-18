//go:build darwin || server

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

// TestStartWatchdogFails checks that a watchdog that can't start comes back
// as a nil io.Closer, not as a nil *pipedProcess inside one, which quitting
// would close and crash on.
func TestStartWatchdogFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	saved := executable
	executable = func() (string, error) { return missing, nil }
	t.Cleanup(func() { executable = saved })
	wd, err := StartWatchdog(t.TempDir())
	if err == nil {
		t.Fatal("StartWatchdog with a missing executable succeeded")
	}
	if wd != nil {
		t.Errorf("StartWatchdog = %#v with its error, want nil", wd)
	}
}

// TestPipedProcessCloseReportsAProcessStillRunning checks that closing says
// when the process hasn't exited, as bench arrange then keeps the folder
// that the watchdog works in.
func TestPipedProcessCloseReportsAProcessStillRunning(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	saved := watchdogWait
	watchdogWait = 100 * time.Millisecond
	t.Cleanup(func() { watchdogWait = saved })
	// The child doesn't read its pipe, so closing the pipe doesn't end it.
	p, err := startPiped(sh, []string{"-c", "exec sleep 10"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if proc, err := os.FindProcess(p.pid); err == nil {
			_ = proc.Kill()
		}
		<-p.done
	})
	if err := p.Close(); !errors.Is(err, errStillRunning) {
		t.Errorf("Close() = %v, want errStillRunning", err)
	}
}

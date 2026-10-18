//go:build (darwin || server) && unix

package ui

import (
	"os/exec"
	"syscall"
	"testing"
)

// TestPipedProcessHasItsOwnGroup checks that the watchdog is out of reach of
// the signals a terminal sends Bench's process group: Control-\ would end
// both, and then nothing would put the windows back.
func TestPipedProcessHasItsOwnGroup(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	p, err := startPiped(sh, []string{"-c", "cat >/dev/null"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	group, err := syscall.Getpgid(p.pid)
	if err != nil {
		t.Fatal(err)
	}
	if group != p.pid || group == syscall.Getpgrp() {
		t.Errorf("the child is in process group %d, want its own (%d), not Bench's (%d)", group, p.pid, syscall.Getpgrp())
	}
}

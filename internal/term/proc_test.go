//go:build darwin || linux

package term

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test process is a known process to look up, so these run against the
// real kernel interfaces: /proc on Linux, sysctl and proc_info on macOS.

func TestProcessNameOfSelf(t *testing.T) {
	name, ok := processName(os.Getpid())
	if !ok || name == "" {
		t.Fatalf("processName(self) = %q, %v; want a name", name, ok)
	}
	// The kernel keeps the start of the program's file name.
	if exe := filepath.Base(os.Args[0]); !strings.HasPrefix(exe, name) {
		t.Errorf("processName(self) = %q, want the start of %q", name, exe)
	}
}

func TestProcessDirOfSelf(t *testing.T) {
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if want, err = filepath.EvalSymlinks(want); err != nil {
		t.Fatal(err)
	}
	got, ok := processDir(os.Getpid())
	if !ok {
		t.Fatal("processDir(self) found no folder")
	}
	if got, err = filepath.EvalSymlinks(got); err != nil {
		t.Fatalf("processDir(self) = %q, which doesn't exist: %v", got, err)
	}
	if got != want {
		t.Errorf("processDir(self) = %q, want %q", got, want)
	}
}

func TestProcessLookupsRejectBadIDs(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if name, ok := processName(pid); ok {
			t.Errorf("processName(%d) = %q, want nothing", pid, name)
		}
		if dir, ok := processDir(pid); ok {
			t.Errorf("processDir(%d) = %q, want nothing", pid, dir)
		}
	}
}

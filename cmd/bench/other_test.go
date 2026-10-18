//go:build !server && !(darwin && cgo)

package main

import (
	"strings"
	"testing"
)

func TestAppNeedsMacOS(t *testing.T) {
	status, stdout, stderr := runBench(t)
	if status != exitError || stdout != "" || !strings.Contains(stderr, previewCommand) {
		t.Errorf("bench = %d %q %q; want 1 and how to run the preview", status, stdout, stderr)
	}
}

func TestMacOSCommands(t *testing.T) {
	for _, args := range [][]string{
		{"arrange", "Code"},
		{"restore"},
		{"watchdog", "--dir", t.TempDir()},
	} {
		status, _, stderr := runBench(t, args...)
		if status != exitError || !strings.Contains(stderr, "needs macOS") {
			t.Errorf("bench %v = %d %q; want 1 and that it needs macOS", args, status, stderr)
		}
	}
}

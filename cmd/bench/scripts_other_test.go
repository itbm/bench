//go:build unix && !(darwin && cgo && !server)

package main

import (
	"strings"
	"testing"
)

// TestScriptCommandsNeedTheMacApp checks that outside the macOS app the
// script subcommands say that they control only it, and how to list the
// preview's tabs instead.
func TestScriptCommandsNeedTheMacApp(t *testing.T) {
	t.Setenv("BENCH_HOME", t.TempDir())
	for _, args := range [][]string{{"tab", "Code"}, {"next"}, {"list"}} {
		status, stdout, stderr := runBench(t, args...)
		if status != exitError || stdout != "" || !strings.Contains(stderr, "this command controls Bench's macOS app") ||
			!strings.Contains(stderr, "bench list --dir") {
			t.Errorf("bench %q = %d %q %q", args, status, stdout, stderr)
		}
	}
}

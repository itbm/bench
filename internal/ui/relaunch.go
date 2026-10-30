//go:build darwin || server

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// This file opens Bench again once it has quit, for Quit and Reopen Bench
// (workspace.Engine.QuitAndReopen): macOS applies Screen Recording
// permission only as Bench opens. Bench can't open itself while it's still
// running, as a second Bench just brings the first forward and exits
// (Wails' single-instance lock), so a small shell script waits for this
// process to end, then opens Bench.

// reopenScript waits for the process $1 to end, for up to a minute, then
// opens $2: Bench's app bundle, with open(1), or else its executable. If
// Bench is still running after a minute, it gives up.
const reopenScript = `i=0
while kill -0 "$1" 2>/dev/null; do
	i=$((i + 1))
	[ "$i" -gt 600 ] && exit 1
	sleep 0.1
done
if [ -d "$2" ]; then
	exec /usr/bin/open "$2"
fi
exec "$2"`

// reopenTarget returns what opens Bench again, given its executable: the
// app bundle that holds it, if it's in one, or else the executable itself,
// as when it's run from a build folder.
func reopenTarget(exe string) string {
	const inBundle = ".app/Contents/MacOS/"
	if i := strings.LastIndex(exe, inBundle); i >= 0 {
		return exe[:i+len(".app")]
	}
	return exe
}

// reopenCommand returns the command that opens target once the process pid
// has ended. It runs in a process group of its own, so that it outlives
// Bench.
func reopenCommand(pid int, target string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", reopenScript, "bench-reopen", strconv.Itoa(pid), target)
	cmd.SysProcAttr = ownProcessGroup()
	return cmd
}

// reopenAfterQuit starts what opens Bench again once this process has
// ended.
func reopenAfterQuit() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding Bench: %w", err)
	}
	cmd := reopenCommand(os.Getpid(), reopenTarget(exe))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting what opens Bench again: %w", err)
	}
	// Bench doesn't wait for it: once Bench has quit, launchd does.
	return cmd.Process.Release()
}

//go:build darwin || linux

package term

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// A process keeps the signals that were ignored when it started, and passes
// them on to the programs it runs. Bench started with nohup has SIGHUP
// ignored, and keeps it so (see quitOnHangup in internal/ui), so a shell
// started from it directly would ignore SIGHUP too, and so would everything
// run in it: closing a session, or quitting Bench, would leave them running
// with no terminal. In that case, Bench starts the shell through its own
// binary, run as a small helper that puts SIGHUP back to its default and
// then becomes the shell.

// hangUpHelperArg, as the first argument to Bench's binary, makes it the
// helper. The next argument is the program to run, and the rest are that
// program's arguments, starting with its name.
const hangUpHelperArg = "-bench-term-exec-with-hangup"

// init runs the helper, when this process is one, before anything else in
// the binary starts.
func init() {
	if len(os.Args) >= 4 && os.Args[1] == hangUpHelperArg {
		execWithHangUp(os.Args[2], os.Args[3:])
	}
}

// execWithHangUp replaces this process with the program at path, with SIGHUP
// at its default action. If path can't run, it exits with status 127, as a
// shell does for a command it can't run.
func execWithHangUp(path string, argv []string) {
	// Catching SIGHUP, even with nowhere for it to go, ends its being
	// ignored. Exec then resets a caught signal to its default action.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGHUP)
	err := syscall.Exec(path, argv, os.Environ())
	// Standard error is the terminal, so the user sees why.
	fmt.Fprintf(os.Stderr, "bench: can't run %s: %v\r\n", path, err)
	os.Exit(127)
}

// shellCommand returns the command that runs shell with args in a session,
// and whether it starts through the helper.
func shellCommand(shell string, args []string) (*exec.Cmd, bool) {
	if signal.Ignored(syscall.SIGHUP) {
		if self, err := os.Executable(); err == nil {
			helperArgs := append([]string{hangUpHelperArg, shell, shell}, args...)
			return exec.Command(self, helperArgs...), true
		}
	}
	return exec.Command(shell, args...), false
}

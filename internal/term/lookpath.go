//go:build darwin || linux

package term

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// lookWait is the longest LookPath waits for the user's shell to say where
// a program is.
const lookWait = 3 * time.Second

// plainName matches a program's name that a shell runs as it's typed, with
// nothing to quote: letters, digits, and a few marks, not starting with a
// mark.
var plainName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._+@-]*$`)

// findInShell returns the path of the program that the user's shell (as
// chooseShell picks it from envShell) runs for the command name: an
// interactive login shell, as terminals run, so that the folders that the
// shell's startup files add to PATH, such as Homebrew's or a version
// manager's, count. It returns "" if name isn't a plain program name, the
// shell finds no program by that name (only an alias, a function, or a
// builtin), or it takes longer than wait.
func findInShell(name, envShell string, wait time.Duration) string {
	if !plainName.MatchString(name) {
		return ""
	}
	shell, err := chooseShell("", envShell)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	home := fallbackDir(homeDir())
	// A plain name needs no quoting, in any shell.
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", "command -v "+name)
	cmd.Dir = home
	cmd.Env = sessionEnv(os.Environ(), home, nil)
	// The shell runs in a session of its own, without a controlling
	// terminal, as terminals' sessions run in their own. Otherwise, when
	// Bench is started from a terminal, as with go run, the interactive
	// shell would take that terminal, or stop until the wait runs out when
	// it tries, and its startup files could write to it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// The startup files may start programs that keep the output open, or
	// wait for input: there is none (nil is /dev/null), and the wait ends
	// with the shell, and whatever it started, which is in its session.
	cmd.Stdin = nil
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = wait
	out, _ := cmd.Output()
	// Startup files may print things of their own first: the answer is
	// the last line.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	path := strings.TrimSpace(lines[len(lines)-1])
	if !filepath.IsAbs(path) || !isExecutable(path) {
		return ""
	}
	return filepath.Clean(path)
}

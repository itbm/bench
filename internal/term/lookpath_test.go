//go:build darwin || linux

package term

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeProgram writes an executable script to dir, and returns its path.
func writeProgram(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindInShellFindsProgramsOnThePath(t *testing.T) {
	sh := firstExisting(t, []string{"/bin/sh"})
	dir := t.TempDir()
	tool := writeProgram(t, dir, "bench-probe-tool", "exit 0\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())

	if got := findInShell("bench-probe-tool", sh, 5*time.Second); got != tool {
		t.Errorf("findInShell(bench-probe-tool) = %q, want %q", got, tool)
	}
	// What isn't on the PATH, or isn't a program, isn't found.
	if got := findInShell("bench-no-such-tool", sh, 5*time.Second); got != "" {
		t.Errorf("findInShell(bench-no-such-tool) = %q, want none", got)
	}
	if got := findInShell("cd", sh, 5*time.Second); got != "" {
		t.Errorf("findInShell(cd), a builtin, = %q, want none", got)
	}
}

func TestFindInShellAsksOnlyForPlainNames(t *testing.T) {
	dir := t.TempDir()
	// A shell that would answer anything, to show that it isn't asked.
	shell := writeProgram(t, dir, "shell", "echo /bin/sh\n")
	for _, name := range []string{"", "my tool", "tool;rm", "$(id)", "-v", "a'b", "a/b"} {
		if got := findInShell(name, shell, 5*time.Second); got != "" {
			t.Errorf("findInShell(%q) = %q, want none", name, got)
		}
	}
	if got := findInShell("node-22.1_x+y@z", shell, 5*time.Second); got != "/bin/sh" {
		t.Errorf("findInShell of a plain name = %q, want the shell's answer", got)
	}
}

func TestFindInShellTakesTheLastLine(t *testing.T) {
	dir := t.TempDir()
	tool := writeProgram(t, dir, "tool", "exit 0\n")
	// Startup files may print things of their own before the answer.
	shell := writeProgram(t, dir, "shell", "echo 'Welcome!'\necho /not/the/answer\necho '"+tool+"'\n")
	if got := findInShell("tool", shell, 5*time.Second); got != tool {
		t.Errorf("findInShell = %q, want the last line, %q", got, tool)
	}
	// An answer that isn't a program's path, such as an alias's, is none.
	alias := writeProgram(t, dir, "alias-shell", "echo \"alias tool='ls -l'\"\n")
	if got := findInShell("tool", alias, 5*time.Second); got != "" {
		t.Errorf("findInShell of an alias = %q, want none", got)
	}
}

func TestFindInShellGivesUpOnASlowShell(t *testing.T) {
	dir := t.TempDir()
	// Its startup starts a program that keeps the output open, and says
	// which.
	started := filepath.Join(dir, "started")
	shell := writeProgram(t, dir, "shell", "sleep 30 &\necho $! > '"+started+"'\nsleep 30\necho /bin/sh\n")
	start := time.Now()
	if got := findInShell("tool", shell, 200*time.Millisecond); got != "" {
		t.Errorf("findInShell = %q, want none from a shell that doesn't answer in time", got)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("findInShell took %v, want it to give up after its wait", took)
	}
	// What the shell started goes with it.
	b, err := os.ReadFile(started)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("/proc"); err == nil {
		waitFor(t, 5*time.Second, "the shell's program to end", func() bool { return !processAlive(pid) })
	}
}

func TestFindInShellRunsTheShellInASessionOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	tool := writeProgram(t, dir, "tool", "exit 0\n")
	// A shell that answers only if it leads its own process group, as a
	// session's first process does, rather than Bench's.
	shell := writeProgram(t, dir, "shell", "if [ \"$(ps -o pgid= -p $$ | tr -d ' ')\" = \"$$\" ]; then echo '"+tool+"'; fi\n")
	if got := findInShell("tool", shell, 5*time.Second); got != tool {
		t.Errorf("findInShell = %q, want %q from a shell in a session of its own", got, tool)
	}
}

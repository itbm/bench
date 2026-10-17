package term

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Shells tried, in order, when neither Options.Shell nor $SHELL names one
// that exists.
var fallbackShells = []string{"/bin/zsh", "/bin/sh"}

// chooseShell returns the shell to run. An explicit shell must exist. Without
// one, it's $SHELL if that exists, then /bin/zsh, then /bin/sh.
func chooseShell(explicit, envShell string) (string, error) {
	if explicit != "" {
		path, err := findShell(explicit)
		if err != nil {
			return "", fmt.Errorf("can't use %s as the shell: %w", explicit, err)
		}
		return path, nil
	}
	candidates := append([]string{envShell}, fallbackShells...)
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if path, err := findShell(c); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("can't find a shell: tried $SHELL and %s", strings.Join(fallbackShells, ", "))
}

// findShell resolves a shell given as a path or as a name in PATH.
func findShell(shell string) (string, error) {
	if !strings.ContainsRune(shell, filepath.Separator) {
		return exec.LookPath(shell)
	}
	if !isExecutable(shell) {
		return "", fmt.Errorf("%s isn't a program that can run", shell)
	}
	return shell, nil
}

// shellArgs returns the arguments for a login shell, or for a login shell
// running command.
func shellArgs(command string) []string {
	if command == "" {
		return []string{"-l"}
	}
	return []string{"-l", "-c", command}
}

// resolveDir returns the folder to start in: dir with "~" expanded, or home
// if dir is empty. If the folder doesn't exist, it returns home instead, with
// a notice to show in the terminal.
func resolveDir(dir, home string) (resolved, notice string) {
	if dir == "" || dir == "~" {
		return fallbackDir(home), ""
	}
	path := expandHome(dir, home)
	if !filepath.IsAbs(path) && home != "" {
		path = filepath.Join(home, path)
	}
	path = filepath.Clean(path)
	if isDir(path) {
		return path, ""
	}
	start := fallbackDir(home)
	return start, fmt.Sprintf("bench: %s isn't a folder, so this terminal starts in %s.\r\n",
		abbreviate(path, home), abbreviate(start, home))
}

// fallbackDir is where a terminal starts when there's no usable folder.
func fallbackDir(home string) string {
	if home != "" && isDir(home) {
		return home
	}
	return "/"
}

// sessionEnv returns the environment for a session that starts in dir: base,
// with the terminal variables Bench sets, PWD, LANG if base doesn't set it,
// and then extra. Later values replace earlier ones with the same key.
//
// PWD is set because base is Bench's own environment, whose PWD is Bench's
// folder; os/exec only sets PWD from Cmd.Dir when Cmd.Env is nil.
func sessionEnv(base []string, dir string, extra []string) []string {
	env := newEnvList(base)
	env.set("TERM=xterm-256color")
	env.set("COLORTERM=truecolor")
	env.set("TERM_PROGRAM=Bench")
	env.set("PWD=" + dir)
	if v, ok := env.get("LANG"); !ok || v == "" {
		env.set("LANG=en_GB.UTF-8")
	}
	for _, kv := range extra {
		env.set(kv)
	}
	return env.list
}

// envList is an environment that keeps each key once, in first-seen order.
type envList struct {
	list  []string
	index map[string]int
}

func newEnvList(base []string) *envList {
	e := &envList{index: make(map[string]int, len(base)+8)}
	for _, kv := range base {
		e.set(kv)
	}
	return e
}

func (e *envList) set(kv string) {
	key, _, ok := strings.Cut(kv, "=")
	if !ok || key == "" {
		return
	}
	if i, seen := e.index[key]; seen {
		e.list[i] = kv
		return
	}
	e.index[key] = len(e.list)
	e.list = append(e.list, kv)
}

func (e *envList) get(key string) (string, bool) {
	i, ok := e.index[key]
	if !ok {
		return "", false
	}
	_, v, _ := strings.Cut(e.list[i], "=")
	return v, true
}

// environ is os.Environ, replaceable in tests.
var environ = os.Environ

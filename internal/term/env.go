package term

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// startDir returns the folder a session starts in, and a notice to show in
// the terminal if that isn't the folder asked for.
//
// A shell whose folder isn't one starts in fallback instead, if that is one
// (see Options.Fallback), or else in home. A command doesn't, and startDir
// returns an error: it was written for its folder, and anywhere else its
// relative paths would name other files.
func startDir(dir, fallback, home, command string) (resolved, notice string, err error) {
	resolved, missing := resolveDir(dir, home)
	switch {
	case missing == "":
		return resolved, "", nil
	case command != "":
		return "", "", fmt.Errorf("%s isn't a folder", abbreviate(missing, home))
	}
	if fallback != "" {
		if path, gone := resolveDir(fallback, home); gone == "" {
			resolved = path
		}
	}
	return resolved, fmt.Sprintf("bench: %s isn't a folder, so this terminal starts in %s.\r\n",
		abbreviate(missing, home), abbreviate(resolved, home)), nil
}

// withoutInput returns startDir's notice for a shell that has Options.Input
// to type, saying that the shell doesn't type it: like a command, it was
// written for its folder.
func withoutInput(notice string) string {
	return strings.TrimSuffix(notice, ".\r\n") + ", and doesn't run its command.\r\n"
}

// resolveDir returns the folder to start in: dir with "~" expanded, or home
// if dir is empty. If that folder doesn't exist, it returns home instead, and
// missing is the folder that was asked for.
func resolveDir(dir, home string) (resolved, missing string) {
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
	return fallbackDir(home), path
}

// fallbackDir is where a terminal starts when there's no usable folder.
func fallbackDir(home string) string {
	if home != "" && isDir(home) {
		return home
	}
	return "/"
}

// sessionEnv returns the environment for a session that starts in dir: base,
// without the variables that describe another terminal's session, with the
// terminal variables Bench sets, PWD, LANG if base doesn't set it, and then
// extra. Later values replace earlier ones with the same key.
//
// PWD is set because base is Bench's own environment, whose PWD is Bench's
// folder; os/exec only sets PWD from Cmd.Dir when Cmd.Env is nil.
func sessionEnv(base []string, dir string, extra []string) []string {
	env := newEnvList(slices.DeleteFunc(slices.Clone(base), fromOtherTerminal))
	env.set("TERM=xterm-256color")
	env.set("COLORTERM=truecolor")
	env.set("TERM_PROGRAM=Bench")
	if Version != "" {
		env.set("TERM_PROGRAM_VERSION=" + Version)
	}
	env.set("PWD=" + dir)
	if v, ok := env.get("LANG"); !ok || v == "" {
		env.set("LANG=" + defaultLang())
	}
	for _, kv := range extra {
		env.set(kv)
	}
	return env.list
}

// otherTerminalVars are variables that terminal apps and multiplexers set
// for the sessions they run, and otherTerminalPrefixes start more of them.
// Bench inherits them when it's started from one of those sessions, but they
// describe that session, not Bench's: with TMUX set, for example, tmux
// refuses to start in a Bench terminal, and its commands control the other
// terminal's tmux instead.
var (
	otherTerminalVars = []string{
		"TERM_PROGRAM_VERSION", "TERM_SESSION_ID", // Terminal, iTerm2, and others.
		"COLUMNS", "LINES", "COLORFGBG", // The other terminal's size and colours.
		"TMUX", "TMUX_PANE", "STY", "WINDOW", "ZELLIJ", // tmux, GNU Screen, and Zellij.
		"LC_TERMINAL", "LC_TERMINAL_VERSION", // iTerm2.
		"VSCODE_IPC_HOOK_CLI", "VSCODE_INJECTION", "VSCODE_NONCE", // VS Code's terminal.
		"TERMINAL_EMULATOR", // JetBrains IDEs.
		"ZED_TERM",          // Zed's terminal.
		"SHLVL", "OLDPWD",   // The shell that started Bench.
	}
	otherTerminalPrefixes = []string{"ITERM_", "KITTY_", "WEZTERM_", "ALACRITTY_", "GHOSTTY_", "WARP_", "ZELLIJ_"}
)

// fromOtherTerminal reports whether the variable kv, as KEY=value, is one of
// otherTerminalVars or starts with one of otherTerminalPrefixes.
func fromOtherTerminal(kv string) bool {
	key, _, _ := strings.Cut(kv, "=")
	if slices.Contains(otherTerminalVars, key) {
		return true
	}
	return slices.ContainsFunc(otherTerminalPrefixes, func(p string) bool { return strings.HasPrefix(key, p) })
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

// Package term runs terminal sessions in pseudo-terminals, for Built-in
// terminal items. The frontend draws them with xterm.js.
package term

// This file fixes the package's exported API so that other packages can build
// against it. Delete it once everything is implemented in its own file.

// Options start a session.
type Options struct {
	// Dir is the starting folder. "~" and "~/..." are expanded; empty
	// means the home folder.
	Dir string
	// Command runs instead of the shell, through the shell ($SHELL -l -c).
	Command string
	// Shell is the shell to run; empty means $SHELL, or /bin/zsh.
	Shell string
	// Cols and Rows are the starting size; zero means 80 by 24.
	Cols, Rows int
	// Env is added to the environment, as KEY=value.
	Env []string
}

// Host runs sessions by ID. Manager is the real one; Echo is a stand-in for
// the browser preview.
type Host interface {
	// Start starts a session, or does nothing if it's already running.
	Start(id string, opts Options) error
	Running(id string) bool
	// Write sends input (keystrokes) to the session.
	Write(id string, data []byte) error
	// Resize sets the terminal's size, in character cells.
	Resize(id string, cols, rows int) error
	// Title is the sub-tab title: the title the shell set with an escape
	// sequence if it set one, otherwise "<foreground command> — <folder>",
	// with the home folder shown as ~.
	Title(id string) string
	// Snapshot returns recent output (up to 256 KB), so a new view can
	// replay it.
	Snapshot(id string) []byte
	// Close ends a session with SIGHUP.
	Close(id string)
	// CloseAll ends every session with SIGHUP. Bench calls it on quit.
	CloseAll()
}

// Callbacks report what sessions do. They're called from the sessions' own
// goroutines and must not block.
type Callbacks struct {
	// Data is output, in order. The slice isn't reused.
	Data func(id string, data []byte)
	// Title is called when a session's title changes.
	Title func(id, title string)
	// Exit is called once when a session's process ends.
	Exit func(id string, err error)
}

// Manager runs real sessions with creack/pty.
type Manager struct{}

// NewManager returns a manager.
func NewManager(cb Callbacks) *Manager { panic("not implemented") }

// Echo is a pretend session host for the browser preview: it never runs a
// process. It prints a prompt, echoes what's typed, and answers a few
// commands (help, ls, pwd, echo, clear) with canned output.
type Echo struct{}

// NewEcho returns an Echo host.
func NewEcho(cb Callbacks) *Echo { panic("not implemented") }

// Package term runs terminal sessions in pseudo-terminals, for Built-in
// terminal items. The frontend draws them with xterm.js.
//
// Manager runs real sessions with creack/pty. Echo is a pretend host for the
// browser preview, which never runs a process.
package term

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// Version is Bench's version, which sessions see as TERM_PROGRAM_VERSION.
// The app sets it as it starts; while it's empty, sessions have no
// TERM_PROGRAM_VERSION.
var Version string

// SnapshotLimit is the most output a session keeps for Snapshot, in bytes.
const SnapshotLimit = 256 * 1024

// Default terminal size, in character cells, when Options leaves it out.
const (
	DefaultCols = 80
	DefaultRows = 24
)

// Errors returned by Host methods. Test for them with errors.Is.
var (
	// ErrNoSession means there's no session with the given ID.
	ErrNoSession = errors.New("no such terminal")
	// ErrEnded means the session's process has ended, or it's closing.
	ErrEnded = errors.New("the terminal has ended")
	// ErrBusy means the session isn't reading its input, and too much is
	// waiting to be sent to it.
	ErrBusy = errors.New("the terminal isn't reading what's typed")
)

// Options start a session.
type Options struct {
	// Dir is the starting folder. "~" and "~/..." are expanded; empty
	// means the home folder.
	Dir string
	// Command runs instead of the shell, through the shell ($SHELL -l -c).
	Command string
	// Shell is the shell to run; empty means $SHELL, or /bin/zsh, or
	// /bin/sh, whichever exists first.
	Shell string
	// Cols and Rows are the starting size; zero means 80 by 24.
	Cols, Rows int
	// Env is added to the environment, as KEY=value.
	Env []string
}

// Host runs sessions by ID. Manager is the real one; Echo is a stand-in for
// the browser preview.
type Host interface {
	// Start starts a session, or does nothing if it's already running. It
	// replaces a session with the same ID that has ended or been closed.
	Start(id string, opts Options) error
	// Running reports whether the session is running: started, and not
	// ended or closed.
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
	// Close ends a session with SIGHUP and forgets it. Exit follows, with
	// a nil error, once the process has ended.
	Close(id string)
	// CloseAll ends every session with SIGHUP. Bench calls it on quit.
	CloseAll()
}

// Callbacks report what sessions do. They're called from the sessions' own
// goroutines, never from inside a Host method, and must not block. Each
// session's callbacks arrive one at a time, in order.
//
// Once a later Start with the same ID has replaced a session that ended or
// was closed, nothing more is reported for the old one, not even its Exit,
// so that everything reported for an ID is about its current session.
type Callbacks struct {
	// Data is output, in order. The slice isn't reused.
	Data func(id string, data []byte)
	// Title is called when a session's title changes. The starting title
	// isn't reported: Host.Title returns it as soon as Start does.
	Title func(id, title string)
	// Exit is called once when a session's process ends, and nothing is
	// reported for the session after it. err is nil if Close ended the
	// session; otherwise it says how the process ended (errors.As finds an
	// *exec.ExitError for a non-zero exit status).
	Exit func(id string, err error)
}

// withDefaults returns cb with every nil callback replaced by one that does
// nothing, so callers never need to check.
func (cb Callbacks) withDefaults() Callbacks {
	if cb.Data == nil {
		cb.Data = func(string, []byte) {}
	}
	if cb.Title == nil {
		cb.Title = func(string, string) {}
	}
	if cb.Exit == nil {
		cb.Exit = func(string, error) {}
	}
	return cb
}

// size returns the starting size from opts, with defaults for zero values.
func (o Options) size() (cols, rows int) {
	cols, rows = o.Cols, o.Rows
	if cols <= 0 {
		cols = DefaultCols
	}
	if rows <= 0 {
		rows = DefaultRows
	}
	return clampCells(cols), clampCells(rows)
}

// checkSize reports whether cols and rows are a usable terminal size.
func checkSize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("terminal size %d by %d isn't positive", cols, rows)
	}
	return nil
}

// clampCells keeps a cell count within what a pty's window size can hold.
func clampCells(n int) int {
	return min(max(n, 1), 0xffff)
}

// titleSeparator goes between the command and the folder in a title.
const titleSeparator = " — "

// formatTitle returns "<command> — <folder>".
func formatTitle(command, folder string) string {
	return command + titleSeparator + folder
}

// generations tells sessions when a later Start with the same ID has replaced
// them, so that their last callbacks (such as Exit after Close) don't reach
// whoever is listening for the new session. It isn't safe for concurrent use;
// hosts guard it with their own mutex.
type generations map[string]*atomic.Bool

// replace marks the previous session with id as replaced, and records flag
// as the new session's.
func (g generations) replace(id string, flag *atomic.Bool) {
	if old := g[id]; old != nil {
		old.Store(true)
	}
	g[id] = flag
}

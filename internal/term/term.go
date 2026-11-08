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

// flowWindow is how far a session's views may fall behind its output. A
// session reads no more of its program's output while they're this many
// bytes or more behind, so that a program that prints faster than Bench can
// draw waits for Bench, rather than flooding the UI. Views say how far
// they've drawn with Host.Ack. If none does for a second (Manager's
// flowWait), as for a terminal that no view shows, the session carries on as
// if they had, and doesn't wait again until a view next does.
const flowWindow = 256 * 1024

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
	// Fallback is where a shell starts if Dir isn't a folder, before the
	// home folder: a terminal's own folder, when Dir is the one its shell
	// was last in, which may have gone since. A Command never starts
	// anywhere but Dir.
	Fallback string
	// Command runs instead of the shell, through the shell ($SHELL -l -c).
	Command string
	// Input is typed into the shell once it's ready for it, as if the user
	// had typed it, as Terminal's "Run command" and iTerm2's "Send text at
	// start" do: a launcher's command, ending with "\r" to run it. The
	// shell is interactive, so the command finds the programs, aliases,
	// and functions it would in the user's own terminal, and the shell
	// carries on at its prompt once the command ends. It's for a shell
	// started without Command.
	Input string
	// Shell is the shell to run; empty means $SHELL, or /bin/zsh, or
	// /bin/sh, whichever exists first.
	Shell string
	// Cols and Rows are the starting size; zero means 80 by 24.
	Cols, Rows int
	// Env is added to the environment, as KEY=value.
	Env []string
	// Preface is output to show before the session's own, as if its
	// program had written it: the output of a terminal from before Bench
	// last quit, made safe to draw with Restorable. It counts as output,
	// so snapshots and offsets include it.
	Preface []byte
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
	// Busy reports whether the session is running something that ending
	// the session would stop, and what, in a few words: a session that
	// runs a command is busy for as long as it runs, and its command is
	// what; a shell is busy while a program it started, such as npm, has
	// the terminal's foreground, and that program's name is what. A shell
	// waiting at its prompt isn't busy, nor is a session that has ended.
	Busy(id string) (what string, busy bool)
	// Write sends input (keystrokes) to the session.
	Write(id string, data []byte) error
	// Resize sets the terminal's size, in character cells.
	Resize(id string, cols, rows int) error
	// Title is the sub-tab title: the title the shell set with an escape
	// sequence if it set one, otherwise "<foreground command> — <folder>",
	// with the home folder shown as ~.
	Title(id string) string
	// Snapshot returns recent output (up to 256 KB), so a new view can
	// replay it, and where it ends in the session's output.
	Snapshot(id string) Snapshot
	// Attach records that a view draws the session from now on, and
	// returns the snapshot it starts from, as Snapshot does. view says
	// which, such as ViewMain; attaching a view again changes nothing.
	// A view answers its program's queries, such as for primary device
	// attributes (DA1), but not those in the snapshot it replays. So the
	// host answers DA1 itself in output that no view draws, and leaves it
	// to the view after its snapshot. Each read of output is either in
	// the snapshot or after it, so each query is answered once. With no
	// session, Attach only records the view, and returns nothing.
	Attach(id, view string) Snapshot
	// Detach records that a view no longer draws the session.
	Detach(id, view string)
	// DetachView records that a view draws no session at all, as when the
	// window that holds it loads its page again.
	DetachView(view string)
	// SetAgentStatus turns agent status on or off; it's on to start with.
	// While it's on, sessions answer their programs' probe for the
	// Program Status Protocol (OSC 7501), and pass on what they report
	// to Callbacks.Status. While it's off, they do neither, and programs
	// carry on as in a terminal that doesn't know the protocol.
	SetAgentStatus(on bool)
	// Clear forgets the session's recent output, as a view's Clear clears
	// its scrollback, so that Snapshot has none of it. The modes it set
	// still count, and offsets carry on from where they were.
	Clear(id string)
	// Close ends a session with SIGHUP and forgets it. Exit follows, with
	// a nil error, once the process has ended.
	Close(id string)
	// CloseAll ends every session with SIGHUP. Bench calls it on quit.
	CloseAll()
	// Ack tells the session that a view has drawn its output up to offset.
	// A session whose views fall far behind waits for them before it
	// reads more of its program's output.
	Ack(id string, offset int64)
	// LookPath returns the path of the program that a terminal's shell
	// runs for the command name, as the user's PATH finds it, or "" if
	// it finds none. It may wait a few seconds for the user's shell.
	LookPath(name string) string
}

// View keys for Host.Attach: the windows that draw terminals. Any other
// string works as well.
const (
	// ViewMain is Bench's main window.
	ViewMain = "main"
	// ViewOwn is a terminal's own window: one popped out, or the
	// drop-down.
	ViewOwn = "own"
)

// Snapshot is a session's recent output, for a new view to replay.
type Snapshot struct {
	// Data is the output, up to SnapshotLimit bytes, and a few more to set
	// the terminal modes the program set in output that has been dropped.
	Data []byte
	// Offset is where Data ends in the session's output: the number of
	// bytes it has output in all. Data callbacks with output from before
	// it are already in Data.
	Offset int64
}

// Callbacks report what sessions do. They're called from the sessions' own
// goroutines, never from inside a Host method, and must not block. Each
// session's callbacks arrive one at a time, in order.
//
// Once a later Start with the same ID has replaced a session that ended or
// was closed, nothing more is reported for the old one, not even its Exit,
// so that everything reported for an ID is about its current session.
type Callbacks struct {
	// Data is output, in order. offset is where data starts in the
	// session's output: the number of bytes output before it. Output
	// carries on from the offset where a session that Start replaced left
	// off, so offsets only grow for one ID until Close. The slice isn't
	// reused.
	Data func(id string, offset int64, data []byte)
	// Title is called when a session's title changes. The starting title
	// isn't reported: Host.Title returns it as soon as Start does. byShell
	// is set when the title is one the shell, or a program in it, set with
	// an escape sequence, rather than "<command> — <folder>".
	Title func(id, title string, byShell bool)
	// Alert is called when the session's program rings the bell, or asks
	// for a desktop notification (see Alert), after the Data that holds
	// it.
	Alert func(id string, a Alert)
	// Folder is called when the session's shell changes folder, with the
	// folder it's in now, as a full path: the shell process's current
	// folder, or if that can't be read, the one the shell last reported
	// with OSC 7. The starting folder isn't reported.
	Folder func(id, folder string)
	// Status is called when the session's program reports its own state
	// with OSC 7501, after the Data that holds the report, and before any
	// Alert from the same output, so that a bell or notification that
	// goes with the report comes after it. Only what the program reports
	// about itself as a whole (its root record) is passed on. Status is
	// also called with StateClear for a full reset (RIS), which forgets
	// every report, and with StateEnded once the program that reported
	// working, blocked, or idle in a shell has gone, and the shell has the
	// terminal again. Nothing is reported while agent status is off (see
	// Host.SetAgentStatus).
	Status func(id string, s Status)
	// Exit is called once when a session's process ends, and nothing is
	// reported for the session after it. err is nil if Close ended the
	// session; otherwise it says how the process ended (errors.As finds an
	// *exec.ExitError for a non-zero exit status).
	Exit func(id string, err error)
}

// Alert is something a session's program wants the user to notice: the bell
// (BEL), or a desktop notification, which a program sends with OSC 9, as
// iTerm2 has it, or with OSC 777, as rxvt does. Claude Code and Codex, set
// up to, send one when they finish, or need an answer.
type Alert struct {
	// Bell is set for the bell, which has no text.
	Bell bool
	// Title and Body are a notification's text, cleaned to show: OSC 777
	// has both, and OSC 9 only a body.
	Title, Body string
}

// withDefaults returns cb with every nil callback replaced by one that does
// nothing, so callers never need to check.
func (cb Callbacks) withDefaults() Callbacks {
	if cb.Data == nil {
		cb.Data = func(string, int64, []byte) {}
	}
	if cb.Title == nil {
		cb.Title = func(string, string, bool) {}
	}
	if cb.Alert == nil {
		cb.Alert = func(string, Alert) {}
	}
	if cb.Folder == nil {
		cb.Folder = func(string, string) {}
	}
	if cb.Status == nil {
		cb.Status = func(string, Status) {}
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

//go:build darwin || server

package ui

import (
	"encoding/json"
	"os"
	"sync/atomic"

	"github.com/itbm/bench/internal/workspace"
)

// Event names, as docs/architecture.md lists them.
const (
	eventState    = "bench:state"
	eventToast    = "bench:toast"
	eventCommand  = "bench:command"
	eventTermData = "term:data"
)

// eventSink sends events to the frontend. Wails' app.Event satisfies it.
type eventSink interface {
	Emit(name string, data ...any) bool
}

// shell is what the emitter asks of the app around the frontend: the window,
// terminal windows, and the menu bar item.
type shell interface {
	// bringForward shows Bench's window and activates the app.
	bringForward()
	// popOutTerminal gives a terminal item a window of its own, and
	// closeTerminalWindow closes it.
	popOutTerminal(itemID, title string) error
	closeTerminalWindow(itemID string)
	// setCurrentTab updates the menu bar item.
	setCurrentTab(name string, tabs []string)
}

// windowCommands are the bench:command names that need Bench's window in
// front: each opens the palette, the layout editor, or the permissions guide.
// focus-terminal isn't one: the engine has already brought Bench forward,
// with Platform.FocusUI, by the time it's sent, and doing it again here could
// take the keyboard from a pane focused since.
var windowCommands = map[string]bool{
	"open-palette":   true,
	"toggle-palette": true,
	"new-tab":        true,
	"edit-tab":       true,
	"first-run":      true,
}

// emitter is the engine's workspace.Emitter: it sends events through Wails
// and asks the shell for the rest. Once closed, it drops everything, so that
// nothing waits for the main thread while Bench quits.
type emitter struct {
	sink   eventSink
	shell  shell
	closed atomic.Bool
	// viewFile, when set (BENCH_DEBUG), gets each view as JSON, so scripts
	// can find what's on screen.
	viewFile string
}

var _ workspace.Emitter = (*emitter)(nil)

// newEmitter returns an emitter that sends events to sink and asks sh for
// the rest.
func newEmitter(sink eventSink, sh shell) *emitter {
	return &emitter{sink: sink, shell: sh}
}

// close stops the emitter sending events.
func (em *emitter) close() { em.closed.Store(true) }

// emit sends an event to the frontend, unless the emitter is closed.
func (em *emitter) emit(name string, data any) {
	if em.closed.Load() {
		return
	}
	em.sink.Emit(name, data)
}

// State sends a bench:state event with the view.
func (em *emitter) State(v workspace.View) {
	em.emit(eventState, v)
	if em.viewFile != "" {
		writeViewFile(em.viewFile, v)
	}
}

// writeViewFile writes a view as JSON, atomically, ignoring errors: it's
// only for debugging.
func writeViewFile(path string, v workspace.View) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// Toast sends a bench:toast event with the message.
func (em *emitter) Toast(message string) { em.emit(eventToast, message) }

// Command brings Bench's window forward if the command needs it, then sends
// a bench:command event.
func (em *emitter) Command(c workspace.Command) {
	if em.closed.Load() {
		return
	}
	if windowCommands[c.Name] {
		em.shell.bringForward()
	}
	em.emit(eventCommand, c)
}

// TermData sends a term:data event.
func (em *emitter) TermData(d workspace.TermData) { em.emit(eventTermData, d) }

// PopOutTerminal opens a window showing just the terminal.
func (em *emitter) PopOutTerminal(itemID, title string) error {
	if em.closed.Load() {
		return nil
	}
	return em.shell.popOutTerminal(itemID, title)
}

// CloseTerminalWindow closes a terminal's own window. While Bench quits, it
// leaves that to Wails, which closes every window.
func (em *emitter) CloseTerminalWindow(itemID string) {
	if em.closed.Load() {
		return
	}
	em.shell.closeTerminalWindow(itemID)
}

// CurrentTab shows the current tab's name in the menu bar item, with the tabs
// in its menu.
func (em *emitter) CurrentTab(name string, tabs []string) {
	if em.closed.Load() {
		return
	}
	em.shell.setCurrentTab(name, tabs)
}

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
	// setMenuBar updates the menu bar item.
	setMenuBar(m workspace.MenuBar)
	// shortcutsChanged registers the global shortcuts again, as the
	// config's [shortcuts] have changed. It mustn't wait for the main
	// thread.
	shortcutsChanged()
	// fillScreen zooms Bench's window to fill its screen, unless it fills
	// it already. It mustn't wait for the main thread.
	fillScreen()
}

// windowCommands are the bench:command names that need Bench's window in
// front: each opens the palette, Tab properties, Settings, or the
// permissions guide.
// focus-terminal isn't one: the engine has already brought Bench forward,
// with Platform.FocusUI, by the time it's sent, and doing it again here could
// take the keyboard from a pane focused since.
var windowCommands = map[string]bool{
	"open-palette":   true,
	"toggle-palette": true,
	"new-tab":        true,
	"edit-tab":       true,
	"settings":       true,
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

// MenuBar updates the menu bar item: the current tab's name, unless the
// menu_bar setting says otherwise, and the tabs in its menu.
func (em *emitter) MenuBar(m workspace.MenuBar) {
	if em.closed.Load() {
		return
	}
	em.shell.setMenuBar(m)
}

// BringForward asks the shell to show Bench's window and activate Bench, as
// a tab shortcut does while Bench is hidden.
func (em *emitter) BringForward() {
	if em.closed.Load() {
		return
	}
	em.shell.bringForward()
}

// ShortcutsChanged asks the shell to register the global shortcuts again,
// once a reload of config.toml has changed them.
func (em *emitter) ShortcutsChanged() {
	if em.closed.Load() {
		return
	}
	em.shell.shortcutsChanged()
}

// FillScreen asks the shell to fill the screen with Bench's window, as
// View ▸ Fill the Screen does.
func (em *emitter) FillScreen() {
	if em.closed.Load() {
		return
	}
	em.shell.fillScreen()
}

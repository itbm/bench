//go:build darwin || server

package ui

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
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
	// setMenuBar updates the menu bar item and the app menu.
	setMenuBar(m workspace.MenuBar)
	// shortcutsChanged registers the global shortcuts again, as the
	// config's [shortcuts] have changed. It mustn't wait for the main
	// thread.
	shortcutsChanged()
	// fillScreen zooms Bench's window to fill its screen, unless it fills
	// it already. It mustn't wait for the main thread.
	fillScreen()
	// quit quits Bench without asking about terminals that are running
	// something, as the user has answered that already. It mustn't wait
	// for the main thread.
	quit()
	// relaunch quits Bench as quit does, and opens it again once it has
	// quit. It mustn't wait for the main thread.
	relaunch()
	// setAppearance gives Bench's windows the appearance setting: system,
	// light, or dark. It mustn't wait for the main thread.
	setAppearance(appearance string)
}

// windowCommands are the bench:command names that need Bench's window in
// front: each opens the palette, the overview of every tab (show-all-tabs
// and toggle-all-tabs, and hold-tabs for the hold after last_tab's
// shortcut), Tab properties, the field that renames a
// tab, the add dialog, Settings, the keyboard shortcuts, the permissions
// guide, the recent messages, the question before quitting, the question
// before closing a tab or a window, or a web page's find bar.
// copy-page-link isn't one: it only puts a page's link on the clipboard.
// focus-terminal isn't one: the engine has already brought Bench forward,
// with Platform.FocusUI, by the time it's sent, and doing it again here could
// take the keyboard from a pane focused since.
var windowCommands = map[string]bool{
	"open-palette":         true,
	"toggle-palette":       true,
	"show-all-tabs":        true,
	"toggle-all-tabs":      true,
	"hold-tabs":            true,
	"new-tab":              true,
	"new-tab-from-windows": true,
	"edit-tab":             true,
	"rename-tab":           true,
	"add-to-pane":          true,
	"settings":             true,
	"keyboard-settings":    true,
	"keyboard-shortcuts":   true,
	"first-run":            true,
	"recent-messages":      true,
	"confirm-quit":         true,
	"close-tab":            true,
	"close-window":         true,
	"find-in-page":         true,
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
	// keys are the global shortcuts, which Settings ▸ Keyboard checks and
	// pauses. Set before the app runs.
	keys *shortcutSet
	// log gets each notice as it first appears, so that the log says when
	// each problem began. Set before the app runs.
	log *slog.Logger
	// noticesMu guards notices, the notices of the last view.
	noticesMu sync.Mutex
	notices   map[string]bool
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

// State sends a bench:state event with the view, and logs its new
// notices.
func (em *emitter) State(v workspace.View) {
	em.emit(eventState, v)
	if em.viewFile != "" {
		writeViewFile(em.viewFile, v)
	}
	em.logNotices(v.Notices)
}

// logNotices logs, as warnings, the notices that the last view didn't
// have.
func (em *emitter) logNotices(notices []workspace.NoticeView) {
	em.noticesMu.Lock()
	defer em.noticesMu.Unlock()
	seen := make(map[string]bool, len(notices))
	for _, n := range notices {
		seen[n.Text] = true
		if !em.notices[n.Text] && em.log != nil {
			em.log.Warn("notice", "text", n.Text)
		}
	}
	em.notices = seen
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

// ActionToast sends a bench:toast event with a message and a button, such
// as Undo.
func (em *emitter) ActionToast(t workspace.Toast) { em.emit(eventToast, t) }

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
// menu_bar setting says otherwise, and the tabs in its menu. It also
// updates the app menu: its tabs, its layout mode, and the focused pane.
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
// once a reload of config.toml or Settings ▸ Keyboard has changed them, or
// the keys that macOS keeps for itself may have.
func (em *emitter) ShortcutsChanged() {
	if em.closed.Load() {
		return
	}
	em.shell.shortcutsChanged()
}

// CheckShortcuts says whether each field of a [shortcuts] table would work,
// for Settings ▸ Keyboard as the user records shortcuts.
func (em *emitter) CheckShortcuts(fields map[string]string) []workspace.ShortcutView {
	if em.keys == nil || em.closed.Load() {
		return nil
	}
	return em.keys.check(fields)
}

// PauseShortcuts releases the global shortcuts while Settings ▸ Keyboard
// records one, or registers them again. It waits for the main thread, so
// the engine calls it without its lock.
func (em *emitter) PauseShortcuts(paused bool) {
	if em.keys == nil || em.closed.Load() {
		return
	}
	em.keys.pause(paused)
}

// FillScreen asks the shell to fill the screen with Bench's window, as
// View ▸ Fill the Screen does.
func (em *emitter) FillScreen() {
	if em.closed.Load() {
		return
	}
	em.shell.fillScreen()
}

// Quit asks the shell to quit Bench without asking about its terminals
// again, as the user has answered the question before quitting.
func (em *emitter) Quit() {
	if em.closed.Load() {
		return
	}
	em.shell.quit()
}

// Relaunch asks the shell to quit Bench, as Quit does, and to open it
// again once it has quit, for macOS to apply Screen Recording permission.
func (em *emitter) Relaunch() {
	if em.closed.Load() {
		return
	}
	em.shell.relaunch()
}

// Appearance gives Bench's windows the appearance setting, which their
// pages' colours follow.
func (em *emitter) Appearance(appearance string) {
	if em.closed.Load() {
		return
	}
	em.shell.setAppearance(appearance)
}

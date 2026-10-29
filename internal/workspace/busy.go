package workspace

import (
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// This file asks before Bench ends terminals that are running something:
// as Bench quits, and as the user closes a popped-out terminal's window
// (confirm_quit_busy_terminals). The shell asks the engine first, and the
// engine asks the user, in Bench's own UI, without waiting for the answer.

// BusyTerminal is a terminal that's running something, which quitting
// Bench, or closing the terminal's own window, would end.
type BusyTerminal struct {
	ID string `json:"id"`
	// Title is the terminal's sub-tab title.
	Title string `json:"title"`
	// Tab is the name of the tab the terminal is in, or "" for one in a
	// window of its own.
	Tab string `json:"tab,omitempty"`
	// Running is what it runs, in a few words: the program in the
	// terminal's foreground, such as npm, or the terminal's command.
	Running string `json:"running,omitempty"`
}

// The bench:command names that ask before ending terminals.
const (
	confirmQuitCommand  = "confirm-quit"
	confirmCloseCommand = "confirm-close-terminal"
)

// stopAskingText is the toast after the user ticks Don't ask again.
const stopAskingText = "Bench won't ask again before ending a terminal that's running something. Settings can turn this back on."

// AskToQuit is the shell's question as Bench is about to quit, however the
// user asked: from the Bench menu, the menu bar item, or the Dock (Wails'
// ShouldQuit). It reports whether Bench may quit now: it may unless
// confirm_quit_busy_terminals is on and a terminal is running something,
// and it always may while the user is logging out, restarting, or shutting
// down. Otherwise it asks the user with the confirm-quit command, which
// brings Bench forward and names the terminals, and returns false: the
// question's Quit calls Quit.
func (e *Engine) AskToQuit() bool {
	e.lock()
	defer e.unlock()
	return e.askToQuitLocked(false)
}

// AskToQuitNow is AskToQuit for the main thread, as macOS asks whether
// Bench may quit there, and it mustn't wait for the engine, which may be
// waiting for the main thread. It answers only if it can take the engine's
// lock at once (answered); otherwise the shell asks AskToQuit on another
// goroutine.
func (e *Engine) AskToQuitNow() (quit, answered bool) {
	if !e.mu.TryLock() {
		return false, false
	}
	defer e.unlock()
	return e.askToQuitLocked(true), true
}

// askToQuitLocked is AskToQuit with the lock held. On the main thread
// (onMain), it sends the command on a goroutine of its own, since sending
// it brings Bench's window forward, which is the main thread's to do once
// it has answered.
func (e *Engine) askToQuitLocked(onMain bool) bool {
	if e.closed || !e.cfg.Settings.ConfirmQuitBusyTerminals || e.loggingOutLocked() {
		return true
	}
	busy := e.busyTerminalsLocked()
	if len(busy) == 0 {
		return true
	}
	e.log.Debug("asking before quitting", "terminals", len(busy))
	em := e.em
	c := Command{Name: confirmQuitCommand, Terminals: busy}
	e.later(func() {
		if onMain {
			go em.Command(c)
		} else {
			em.Command(c)
		}
	})
	return false
}

// AskToCloseTerminal is the shell's question as the user closes a
// popped-out terminal's window, which ends the terminal
// (TerminalWindowClosed). It reports whether the window may close: it may
// unless confirm_quit_busy_terminals is on and the terminal is running
// something. Otherwise it asks the user in the terminal's window, with the
// confirm-close-terminal command, and returns false: the question's Close
// calls CloseTerminal.
func (e *Engine) AskToCloseTerminal(itemID string) bool {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	if e.closed || it == nil || it.Kind != KindBuiltin || !e.cfg.Settings.ConfirmQuitBusyTerminals {
		return true
	}
	busy, ok := e.busyTerminalLocked(it)
	if !ok {
		return true
	}
	em := e.em
	e.later(func() {
		em.Command(Command{Name: confirmCloseCommand, ItemID: itemID, Terminals: []BusyTerminal{busy}})
	})
	return false
}

// Quit is the quit question's Quit: it quits Bench, ending its terminals,
// without asking again. With dontAsk, it first turns
// confirm_quit_busy_terminals off and saves config.toml, as Settings does,
// so that Bench doesn't ask next time; Bench quits even if it can't save
// it.
func (e *Engine) Quit(dontAsk bool) error {
	if dontAsk {
		if notSaved, err := e.stopAsking(); err != nil || notSaved != "" {
			e.log.Warn("couldn't save confirm_quit_busy_terminals before quitting", "error", err, "reason", notSaved)
		}
	}
	e.em.Quit()
	return nil
}

// CloseTerminal is the closing question's Close: it ends a terminal, as
// closing its own window does when Bench doesn't ask. Its session ends, it
// goes from every tab, and its own window, if it has one, closes. With
// dontAsk, it also turns confirm_quit_busy_terminals off and saves
// config.toml, and a toast says so.
func (e *Engine) CloseTerminal(itemID string, dontAsk bool) error {
	e.lock()
	it := e.model.Items[itemID]
	if e.closed || it == nil || it.Kind != KindBuiltin {
		e.unlock()
		return errGone
	}
	ti, _ := e.model.Where(itemID)
	e.removeItemLocked(it)
	if ti == e.model.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.unlock()
	if !dontAsk {
		return nil
	}
	notSaved, err := e.stopAsking()
	switch {
	case err != nil:
		e.em.Toast(err.Error())
	case notSaved != "":
		e.em.Toast(appliedText(notSaved))
	default:
		e.em.Toast(stopAskingText)
	}
	return nil
}

// stopAsking turns confirm_quit_busy_terminals off, for the questions'
// Don't ask again, and saves config.toml, as SaveSettings does. It returns
// why config.toml wasn't saved, if it wasn't, as saveEdit does.
func (e *Engine) stopAsking() (string, error) {
	return e.turnOff(func(s *config.Settings) *bool { return &s.ConfirmQuitBusyTerminals })
}

// turnOff turns off one of the settings that make Bench ask first, which
// setting points to, for a question's Don't ask again, and saves
// config.toml, as SaveSettings does. It returns why config.toml wasn't
// saved, if it wasn't, as saveEdit does.
func (e *Engine) turnOff(setting func(*config.Settings) *bool) (string, error) {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	if e.closed || !*setting(&e.cfg.Settings) {
		e.unlock()
		return "", nil
	}
	cfg := e.cfg.Clone()
	*setting(&cfg.Settings) = false
	e.cfg = cfg
	e.touch()
	writable := e.writable
	saved := cfg.Clone()
	e.unlock()
	return e.saveEdit(saved, writable)
}

// loggingOutLocked reports whether the user is logging out, restarting, or
// shutting down: macOS has said so, through a LoggingOut event or the
// platform.
func (e *Engine) loggingOutLocked() bool {
	return e.clk.Now().Before(e.loggingOut) || e.p.LoggingOut()
}

// busyTerminalsLocked returns the terminals that are running something, in
// the order of the tabs and their panes, then those in windows of their
// own, each once.
func (e *Engine) busyTerminalsLocked() []BusyTerminal {
	var out []BusyTerminal
	seen := map[string]bool{}
	add := func(id string) {
		it := e.model.Items[id]
		if it == nil || it.Kind != KindBuiltin || seen[id] {
			return
		}
		seen[id] = true
		if busy, ok := e.busyTerminalLocked(it); ok {
			out = append(out, busy)
		}
	}
	for _, t := range e.model.Tabs {
		for _, id := range layout.Items(t.Root) {
			add(id)
		}
	}
	for _, id := range e.model.Floating {
		add(id)
	}
	return out
}

// busyTerminalLocked describes a terminal if it's running something: by
// its sub-tab title, and its tab, which is the current tab for one that's
// in several tabs and the current one (Model.Where).
func (e *Engine) busyTerminalLocked(it *Item) (BusyTerminal, bool) {
	running, busy := e.terms.Busy(it.ID)
	if !busy {
		return BusyTerminal{}, false
	}
	title := it.shownTitle()
	if title == "" {
		title = it.App
	}
	b := BusyTerminal{ID: it.ID, Title: title, Running: running}
	if ti, _ := e.model.Where(it.ID); ti >= 0 {
		b.Tab = e.model.Tabs[ti].Name
	}
	return b, true
}

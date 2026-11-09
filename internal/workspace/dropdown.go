package workspace

import (
	"errors"
	"fmt"
	"time"

	"github.com/itbm/bench/internal/apps"
)

// This file has the drop-down terminal (decision 72): one terminal, out of
// the tabs, that slides down from the top of the screen over any app when
// the dropdown shortcut is pressed, and slides back up when it's pressed
// again, or on Escape at the shell's prompt. Its session goes on while it's
// out of sight. It's in Floating, with a window of its own, which the
// Emitter shows and hides (ShowDropdown, HideDropdown), and Model.Dropdown
// names it, so that state.json keeps it.

// escapeKey is what the Escape key types in a terminal.
const escapeKey = "\x1b"

// escapeWait is how long an Escape alone in the drop-down waits for the
// next key before it hides the drop-down. A key within it is typed after
// the Escape, as one of the shell's Escape keys, such as Escape B, which
// moves back a word: readline's keyseq-timeout and vim's ttimeoutlen tell
// an Escape alone from such keys the same way.
const escapeWait = 400 * time.Millisecond

// dropdownSubtitle is the drop-down's notifications' subtitle, where a
// terminal in a tab has its tab's name.
const dropdownSubtitle = "Drop-down"

var errNotTerminal = errors.New("Only a terminal can be the drop-down.")

// dropdownLocked returns the drop-down terminal, or nil if there's none. A
// drop-down that has come back into a tab (see bringBackLocked), or gone,
// is none.
func (e *Engine) dropdownLocked() *Item {
	m := e.model
	it := m.Items[m.Dropdown]
	if it == nil || it.Kind != KindBuiltin || !m.IsFloating(it.ID) {
		return nil
	}
	return it
}

// dropShownLocked reports whether the drop-down shows now.
func (e *Engine) dropShownLocked() bool {
	return e.dropShown && e.dropdownLocked() != nil
}

// isDropdownLocked reports whether an item is the drop-down.
func (e *Engine) isDropdownLocked(it *Item) bool {
	d := e.dropdownLocked()
	return d != nil && d == it
}

// dropdownViewLocked returns the drop-down for the View, or nil.
func (e *Engine) dropdownViewLocked() *DropdownView {
	it := e.dropdownLocked()
	if it == nil {
		return nil
	}
	return &DropdownView{ID: it.ID, Shown: e.dropShown, InPage: e.opts.Demo}
}

// UseAsDropdown makes a terminal the drop-down, from its ⋯ menu: it leaves
// its pane, as Pop Out of Bench takes a window out, or its own window
// closes if it was popped out, and it shows at the top of the screen. The
// terminal that was the drop-down until now goes back into Bench, in the
// pane that the new one leaves, or in the current tab's focused pane. A
// toast says how to show the drop-down again.
func (e *Engine) UseAsDropdown(itemID string) error {
	e.lock()
	defer e.unlock()
	m := e.model
	it := m.Items[itemID]
	switch {
	case it == nil:
		return errGone
	case it.Kind != KindBuiltin:
		return errNotTerminal
	case e.opts.Headless:
		return nil
	case e.isDropdownLocked(it):
		e.showDropdownLocked()
		return nil
	}
	old := e.dropdownLocked()
	if old != nil {
		p := m.PaneIn(m.Current, itemID)
		if p == nil {
			p = m.CurrentTab().FocusedPane()
		}
		if p == nil {
			return errNoPane
		}
		// Into the pane before the new one leaves it, so that the pane
		// stays.
		m.Take(old.ID)
		insertAt(p, old.ID, -1)
		e.bringBackLocked(old)
	}
	if it.OwnWindow {
		// Its window closes; the drop-down's own opens.
		em := e.em
		e.later(func() { em.CloseTerminalWindow(itemID) })
	}
	if e.keyPopOut == itemID {
		e.keyPopOut = ""
	}
	m.Take(itemID)
	if old != nil {
		m.Show(old.ID)
	}
	m.Floating = append(m.Floating, itemID)
	m.Dropdown = itemID
	it.OwnWindow = true
	e.setDropShownLocked("")
	e.toastLocked(e.dropdownToastLocked(it))
	e.showDropdownLocked()
	e.arrangeLocked(arrangeOpts{})
	e.changed()
	return nil
}

// dropdownToastLocked says that a terminal is the drop-down now, and how to
// show it again: with the dropdown shortcut, if it's on, or else with View
// ▸ Show Drop-down, or from the menu bar item.
func (e *Engine) dropdownToastLocked(it *Item) string {
	label := e.labelOfLocked(it)
	if keys := keysOf(e.cfg.Shortcuts).Dropdown; keys != "" {
		return label + " is the drop-down now. Press " + keys + " to show or hide it."
	}
	return label + " is the drop-down now. Show it again with View ▸ Show Drop-down, or set its shortcut in Settings ▸ Keyboard."
}

// ToggleDropdown shows the drop-down, or hides it if it shows: the dropdown
// shortcut, View ▸ Show Drop-down, and the menu bar item's Show Drop-down.
// It doesn't bring Bench forward. With no drop-down yet, a new terminal,
// in the home folder, becomes the drop-down.
func (e *Engine) ToggleDropdown() error {
	e.lock()
	defer e.unlock()
	if e.opts.Headless {
		return nil
	}
	switch {
	case e.dropdownLocked() == nil:
		e.newDropdownLocked()
	case e.dropShown:
		e.hideDropdownLocked()
	default:
		e.showDropdownLocked()
	}
	return nil
}

// HideDropdown hides the drop-down, if it shows: its Hide button.
func (e *Engine) HideDropdown() error {
	e.lock()
	defer e.unlock()
	if e.dropShownLocked() {
		e.hideDropdownLocked()
	}
	return nil
}

// newDropdownLocked makes a new terminal, in the home folder, and shows it
// as the drop-down.
func (e *Engine) newDropdownLocked() {
	m := e.model
	it := &Item{ID: m.NewID(itemPrefix), Kind: KindBuiltin, App: terminalApp, Mono: apps.Mono(terminalApp),
		Title: terminalTitle("", defaultFolder), Folder: defaultFolder, Status: StatusReady, OwnWindow: true}
	m.Float(it)
	m.Dropdown = it.ID
	e.setDropShownLocked("")
	e.showDropdownLocked()
}

// showDropdownLocked shows the drop-down, starting its session if it isn't
// running. Showing it takes its mark off (seenLocked). It takes the
// keyboard, so a focus put off until now, such as the jump palette's
// giving it back to the window that had it (refocusAfterJumpLocked), no
// longer applies. If the shell can't show it, a toast says why. The shell
// puts it at the top of the display that Bench's window is on, which it
// notes (dropScreen): after Bench goes back to the display that the user
// chose (decision 70), the next show is on that display.
func (e *Engine) showDropdownLocked() {
	it := e.dropdownLocked()
	if it == nil {
		return
	}
	e.startTerminalLocked(it)
	e.setDropShownLocked(it.ID)
	e.dropScreen = e.p.ScreenOfWindow().ID
	e.newFocusLocked()
	e.jump = jumpNote{}
	em, id, title, height := e.em, it.ID, e.labelOfLocked(it), e.cfg.Settings.DropdownHeight
	e.later(func() {
		if err := em.ShowDropdown(id, title, height); err != nil {
			e.lock()
			defer e.unlock()
			if e.model.Dropdown == id {
				e.setDropShownLocked("")
				e.changed()
			}
			e.toastLocked(fmt.Sprintf("Bench couldn't show the drop-down: %v.", err))
		}
	})
	e.changed()
}

// dropdownScreensLocked runs once the screens have settled after the
// displays changed. A drop-down that shows on a display that has gone,
// which macOS has moved onto another display at the size it had there,
// shows again at the top of the display that Bench's window is on now, as
// wide as it. One that's hidden goes there the next time it shows anyway,
// and one whose display is still connected stays where it is, even if
// Bench's window went to another display meanwhile, as when Bench goes back
// to the display that the user chose: it doesn't move while the user types
// in it, and shows on Bench's display the next time.
func (e *Engine) dropdownScreensLocked() {
	if !e.dropShownLocked() || e.dropScreen == "" {
		return
	}
	for _, s := range e.p.Screens() {
		if s.ID == e.dropScreen {
			return
		}
	}
	e.log.Debug("the drop-down's display has gone; showing it on Bench's display", "display", e.dropScreen)
	e.showDropdownLocked()
}

// hideDropdownLocked hides the drop-down. Its session goes on.
func (e *Engine) hideDropdownLocked() {
	it := e.dropdownLocked()
	e.setDropShownLocked("")
	if it == nil {
		return
	}
	em, id := e.em, it.ID
	e.later(func() { em.HideDropdown(id) })
	e.changed()
}

// escapeDropdown handles input typed in a terminal for the drop-down's
// Escape, and returns what to type, and whether there's nothing to type
// now. An Escape alone, in the drop-down while it shows and its shell waits
// at the prompt, hides the drop-down and isn't typed. While a program runs
// in it, such as vim, less, or a coding agent, Escape is the program's.
// TermInput sends each piece of input in turn, and the page sends an
// Escape alone on its own, so an Escape that comes with more text, as the
// arrows and Option keys with Use Option as Meta key send, is typed.
//
// The Escape waits for the next key, for up to escapeWait: a key for the
// same terminal within it is typed after it, as one piece, and the
// drop-down stays, but a second Escape hides it at once, with neither
// typed. With no key, it hides once the time is up (escapeTimedOutLocked).
// None of this takes the engine's lock, but the hiding, so that an Escape
// for a program, and the keys after an Escape, never wait for an arrange:
// dropKeys says which terminal is the drop-down that shows, and escMu
// guards the Escape that waits.
func (e *Engine) escapeDropdown(itemID, data string) (string, bool) {
	e.escMu.Lock()
	held := itemID != "" && e.escHeld == itemID
	if held {
		e.forgetEscapeLocked()
	}
	e.escMu.Unlock()
	shown, _ := e.dropKeys.Load().(string)
	switch {
	case held && data == escapeKey && shown == itemID:
		e.lock()
		defer e.unlock()
		if it := e.dropdownLocked(); it != nil && it.ID == itemID && e.dropShown {
			e.hideDropdownLocked()
		}
		return "", true
	case held:
		return escapeKey + data, false
	case data != escapeKey || shown == "" || shown != itemID:
		return data, false
	}
	if _, busy := e.terms.Busy(itemID); busy {
		return data, false
	}
	e.escMu.Lock()
	defer e.escMu.Unlock()
	e.escSeq++
	seq := e.escSeq
	e.escHeld = itemID
	e.escTimer = e.after(escapeWait, func() { e.escapeTimedOutLocked(itemID, seq) })
	return "", true
}

// escapeTimedOutLocked hides the drop-down for an Escape that no key has
// followed (escapeDropdown), unless it's no longer the drop-down that
// shows.
func (e *Engine) escapeTimedOutLocked(itemID string, seq uint64) {
	e.escMu.Lock()
	held := e.escHeld == itemID && e.escSeq == seq
	if held {
		e.forgetEscapeLocked()
	}
	e.escMu.Unlock()
	if it := e.dropdownLocked(); held && it != nil && it.ID == itemID && e.dropShown {
		e.hideDropdownLocked()
	}
}

// forgetEscapeLocked forgets the Escape that waits for the next key in the
// drop-down, if any. escMu is held.
func (e *Engine) forgetEscapeLocked() {
	e.escHeld = ""
	if e.escTimer != nil {
		e.escTimer.Stop()
		e.escTimer = nil
	}
}

// dropdownFromStateLocked starts the drop-down's session as Bench opens,
// so that it runs while out of sight, as it did when Bench quit. Its
// window opens the first time it shows.
func (e *Engine) dropdownFromStateLocked(it *Item) {
	e.startTerminalLocked(it)
	e.setDropShownLocked("")
}

// setDropShownLocked notes that the drop-down with item ID id shows now, or
// that none does, for "". An Escape that waits in it (escapeDropdown) is
// forgotten, as it shows or hides some other way.
func (e *Engine) setDropShownLocked(id string) {
	e.dropShown = id != ""
	e.dropKeys.Store(id)
	e.escMu.Lock()
	e.forgetEscapeLocked()
	e.escMu.Unlock()
}

// leaveDropdownLocked notes that an item is no longer the drop-down, as it
// comes back into a tab (bringBackLocked), or ends (endItemLocked), so that
// Escape in it no longer stops to look for the drop-down.
func (e *Engine) leaveDropdownLocked(it *Item) {
	if e.model.Dropdown == it.ID {
		e.model.Dropdown = ""
	}
	if keys, _ := e.dropKeys.Load().(string); keys == it.ID {
		e.setDropShownLocked("")
	}
}

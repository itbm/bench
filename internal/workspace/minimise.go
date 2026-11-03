package workspace

import (
	"strings"

	"github.com/itbm/bench/internal/platform"
)

// This file respects the user minimising a docked window, with Command-M,
// the yellow button, or the app's Window menu. Its sub-tab says it's
// minimised, and its pane shows another of its windows in its place, or,
// with none, says so, with Show again and Remove from this tab. Arranging
// leaves the window in the Dock until the user shows it again: by clicking
// its sub-tab, with Show again, or from Jump to… (ShowItem, which sets the
// item's restore, see showAgain), or by bringing it back from the Dock
// (WindowRestored), which docks it in its pane once more. A window that
// Bench minimised itself, as it does one it has nowhere to park clear of
// other displays (windows.Manager.Minimised), isn't the user's doing: it
// comes back whenever it's needed, as before.
//
// Bench hears of the minimise in whichever of three ways comes first:
// WindowMinimised (onWindowMinimised); the focus of another of the app's
// windows, which macOS gives the keyboard to as the minimise starts, before
// it says the window is minimised (noticeMinimisedLocked); or arranging,
// where the windows manager leaves a minimised window that it didn't
// minimise in the Dock (windows.StatusMinimised, see applyResultLocked).
//
// Nothing is saved: a window that's still minimised when Bench next opens
// is marked again by the scan (markMinimisedWindowsLocked), since Bench puts
// back what it minimised itself as it quits. A window that an app opens
// already minimised, as apps restoring their windows after a login can, is
// marked as Bench adopts it.

// onWindowMinimised marks the item of a window that the user minimised, if
// it's docked in a tab, and shows another of its pane's windows in its
// place. If the current tab's focused pane showed it, the pane gets the
// keyboard back: another of its windows, or Bench, whose message there
// has Show again.
func (e *Engine) onWindowMinimised(ev platform.Event) {
	it := e.model.ItemByWindow(ev.Window.ID)
	if !e.minimisableLocked(it) || it.minimised || e.win.Minimised(it.WindowID) {
		return
	}
	// The event can come after Bench has restored the window, as when it
	// unparks one it minimised itself: only a window that's still minimised
	// counts.
	if !e.stillMinimisedLocked(it) {
		return
	}
	focused := e.userMinimisedLocked(it)
	if ti, _ := e.model.Where(it.ID); ti == e.model.Current {
		e.arrangeLocked(arrangeOpts{})
		if focused {
			e.focusShownLocked()
		}
	}
	e.changed()
}

// userMinimisedLocked marks the item of a window that the user has just
// minimised, however Bench heard of it, and notes its app: macOS gives the
// keyboard to the app's next window, which can be one of another tab's,
// parked out of sight. That isn't the user choosing it, so for a moment
// that focus isn't followed there (justMinimisedLocked). It reports whether
// the current tab's focused pane showed the item.
func (e *Engine) userMinimisedLocked(it *Item) bool {
	e.log.Debug("the user minimised a docked window", "window", it.WindowID, "item", it.ID)
	focused := false
	if g, ok := e.displayLocked().focusedGeom(); ok {
		focused = g.node.Active == it.ID
	}
	e.markMinimisedLocked(it)
	key := strings.ToLower(it.BundleID)
	if until := e.clk.Now().Add(minimiseFocus); until.After(e.minimised[key]) {
		e.minimised[key] = until
	}
	return focused
}

// justMinimisedLocked reports whether the user has just minimised a docked
// window of an app, so that macOS's focus of another of its windows isn't
// the user's doing (see userMinimisedLocked).
func (e *Engine) justMinimisedLocked(bundleID string) bool {
	key := strings.ToLower(bundleID)
	until, ok := e.minimised[key]
	if ok && !e.clk.Now().Before(until) {
		delete(e.minimised, key)
		return false
	}
	return ok
}

// noticeMinimisedLocked looks, as window w gets the keyboard, for another
// window of its app that a pane of the current tab shows and that's now
// minimised, but not by Bench: macOS gives the keyboard to the app's next
// window as the user's minimise starts, and reports the minimise only once
// the window is in the Dock. It marks each one it finds (userMinimisedLocked),
// and reports whether it found any. Only the panes' shown windows are read:
// the others are out of sight, behind them or parked.
func (e *Engine) noticeMinimisedLocked(w platform.Window) bool {
	found := false
	for _, g := range e.displayLocked().panes {
		it := e.model.Items[g.node.Active]
		if !dockable(it) || it.WindowID == w.ID || !sameApp(it.BundleID, w.BundleID) || e.win.Minimised(it.WindowID) {
			continue
		}
		if e.stillMinimisedLocked(it) {
			e.userMinimisedLocked(it)
			found = true
		}
	}
	return found
}

// onWindowRestored docks a window that the user minimised again once it's
// back from the Dock.
func (e *Engine) onWindowRestored(ev platform.Event) {
	it := e.model.ItemByWindow(ev.Window.ID)
	// The event can come after the user has minimised it again.
	if it == nil || !it.minimised || e.stillMinimisedLocked(it) {
		return
	}
	e.restoredLocked(it)
}

// minimisableLocked reports whether an item can be marked minimised: an App
// item with a window, docked in a tab. A popped-out window, or one that the
// user dragged out of its pane, isn't docked anyway.
func (e *Engine) minimisableLocked(it *Item) bool {
	return it != nil && it.Kind == KindApp && it.WindowID != 0 && !it.Undocked &&
		!e.model.IsFloating(it.ID) && len(e.model.TabsOf(it.ID)) > 0
}

// stillMinimisedLocked reports whether an item's window is minimised now.
// A window that can't be read doesn't count.
func (e *Engine) stillMinimisedLocked(it *Item) bool {
	w, err := e.p.Window(it.WindowID)
	return err == nil && w.Minimised
}

// markMinimisedLocked marks an item minimised, and shows another of its
// pane's windows where it was shown, in each tab it's in: the one used most
// recently, as macOS brings the window behind it forward. A pane with no
// other window ready to show keeps showing it, with its message, and so
// does the "tabs" view.
func (e *Engine) markMinimisedLocked(it *Item) {
	it.minimised, it.restore = true, false
	m := e.model
	for _, ti := range m.TabsOf(it.ID) {
		t := m.Tabs[ti]
		if p := m.PaneIn(ti, it.ID); p != nil && p.Active == it.ID {
			if next := e.insteadOfMinimisedLocked(p.Items); next != "" {
				p.Active = next
			}
		}
		if t.CompactShown == it.ID {
			if next := e.insteadOfMinimisedLocked(t.ItemIDs()); next != "" {
				t.CompactShown = next
			}
		}
	}
}

// insteadOfMinimisedLocked returns the most recently used of a list of items
// that's shown and not minimised, or "".
func (e *Engine) insteadOfMinimisedLocked(ids []string) string {
	var ready []string
	for _, id := range e.shownItemsLocked(ids) {
		if !e.model.Items[id].minimised {
			ready = append(ready, id)
		}
	}
	return e.recentOfLocked(ready)
}

// showAgain notes that the user has asked to see an item, as by clicking its
// sub-tab or moving it into a pane of its own: if it's an App item whose
// window is minimised, marked or not yet, the next arrangement that docks
// it brings it back from the Dock.
func showAgain(it *Item) {
	it.minimised = false
	it.restore = it.Kind == KindApp && it.WindowID != 0
}

// restoredLocked docks a window that the user minimised again, now that
// they've brought it back: it's shown in its pane. One in another tab is
// shown in its pane there: macOS gives it the keyboard as it comes back,
// and that focus is followed as follow_focus says.
func (e *Engine) restoredLocked(it *Item) {
	it.minimised = false
	m := e.model
	ti, _ := m.Where(it.ID)
	if ti != m.Current {
		m.ShowInPane(it.ID, ti)
		e.changed()
		return
	}
	e.newFocusLocked()
	m.Show(it.ID)
	e.arrangeLocked(arrangeOpts{})
	e.changed()
}

// markMinimisedWindowsLocked marks the items of windows that are minimised,
// other than those that Bench minimised itself: the user minimised them,
// before Bench last quit or while it wasn't running, or their app opened
// them minimised, as it restored them. It's for the windows that the scan
// finds and that Bench adopts.
func (e *Engine) markMinimisedWindowsLocked(ws []platform.Window) {
	for _, w := range ws {
		it := e.model.ItemByWindow(w.ID)
		if w.Minimised && e.minimisableLocked(it) && !it.minimised && !e.win.Minimised(w.ID) {
			e.markMinimisedLocked(it)
		}
	}
}

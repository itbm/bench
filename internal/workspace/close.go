package workspace

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// This file closes what Bench shows: a sub-tab's window, page, or terminal
// (CloseItem), and a tab with everything in it (CloseTab). Web pages and
// terminals close at once. An App window is asked to close, as its close
// button would (Platform.PressClose), so that its app can ask about unsaved
// changes first, or refuse. Bench asks only once its own window is lowered
// (SetOverlay false), so that a save sheet doesn't open under it. After
// closeGrace, Bench looks for the windows it asked: those still open are
// shown, with a toast that their apps may be asking about unsaved changes.
//
// A closed tab's App windows are "Not in a tab" while they close, so their
// apps are shown, as for any window there, before Bench asks: an app hidden
// with its window could ignore the request, and a window that its app hides
// rather than close would look as if it were still open (see
// checkClosedLocked). A window that Bench has parked stays parked until
// Bench sees that it's still open, which puts it back on screen. One that
// its app hid stays parked too, for RestoreAll to put back.

// closeGrace is how long Bench waits, after asking App windows to close,
// before it looks for those still open. A window that closes is reported a
// second after it closes (WindowDestroyed), so this is a little longer.
const closeGrace = 1500 * time.Millisecond

// CloseQuestion says what CloseTab, or CloseItem, closes, for the question
// that it asks first. Ask is false once it has closed them, without asking.
type CloseQuestion struct {
	// Ask is set when Bench asks first: nothing has closed. The question's
	// Close calls again, sure.
	Ask bool `json:"ask"`
	// Tab is the tab's name, for CloseTab.
	Tab string `json:"tab,omitempty"`
	// Only is set for the only tab, which stays, empty: its entry reads
	// Close all windows.
	Only bool `json:"only,omitempty"`
	// Windows, Pages, and Terminals count the App windows, web pages, and
	// terminals that close.
	Windows   int `json:"windows,omitempty"`
	Pages     int `json:"pages,omitempty"`
	Terminals int `json:"terminals,omitempty"`
	// Stay counts the tab's windows, pages, and terminals that are in other
	// tabs too, which stay there.
	Stay int `json:"stay,omitempty"`
	// Listed counts those that close which the tab's entry in config.toml
	// lists: Bench won't open them again when it next opens.
	Listed int `json:"listed,omitempty"`
	// Rules counts the rules that name the tab, which go with it.
	Rules int `json:"rules,omitempty"`
	// Busy are the terminals that are running something, which closing
	// ends.
	Busy []BusyTerminal `json:"busy,omitempty"`
	// BusyOnly is set when only Busy has CloseTab ask, as
	// confirm_quit_busy_terminals has it, with confirm_close off: Don't ask
	// again then turns confirm_quit_busy_terminals off, not confirm_close.
	BusyOnly bool `json:"busyOnly,omitempty"`
}

// closes counts what a question closes.
func (q CloseQuestion) closes() int { return q.Windows + q.Pages + q.Terminals }

// closeAskingText is the toast after the user ticks Don't ask again in the
// question before closing a tab.
const closeAskingText = "Bench won't ask again before closing a tab's windows. Settings can turn this back on."

// askedBy returns the setting that has CloseTab ask q, which Don't ask again
// turns off, and what the toast says once it's off: confirm_close, or,
// for a question that only a busy terminal asks, confirm_quit_busy_terminals.
func (q CloseQuestion) askedBy() (func(*config.Settings) *bool, string) {
	if q.BusyOnly {
		return func(s *config.Settings) *bool { return &s.ConfirmQuitBusyTerminals }, stopAskingText
	}
	return func(s *config.Settings) *bool { return &s.ConfirmClose }, closeAskingText
}

// tabClose is what closing a tab does to its items, other than those in
// other tabs too.
type tabClose struct {
	q CloseQuestion
	// apps are the App items with windows, which are asked to close.
	apps []*Item
	// ends are the Web items and terminals, which close at once.
	ends []*Item
	// places are the App items without windows, which are forgotten.
	places []*Item
}

// tabCloseLocked works out what closing tab index does.
func (e *Engine) tabCloseLocked(index int) tabClose {
	m := e.model
	t := m.Tabs[index]
	c := tabClose{q: CloseQuestion{Tab: t.Name, Only: len(m.Tabs) == 1}}
	listed := map[string]bool{}
	if index < len(e.cfg.Tabs) {
		for _, k := range entryKeys(e.cfg.Tabs[index]) {
			listed[k] = true
		}
	}
	for _, id := range t.ItemIDs() {
		it := m.Items[id]
		switch {
		case it == nil:
			continue
		case len(m.TabsOf(id)) > 1:
			c.q.Stay++
			continue
		case it.Kind == KindApp && it.WindowID == 0:
			c.places = append(c.places, it)
			continue
		case it.Kind == KindApp:
			c.q.Windows++
			c.apps = append(c.apps, it)
		case it.Kind == KindBuiltin:
			c.q.Terminals++
			c.ends = append(c.ends, it)
			if busy, ok := e.busyTerminalLocked(it); ok {
				c.q.Busy = append(c.q.Busy, busy)
			}
		default:
			c.q.Pages++
			c.ends = append(c.ends, it)
		}
		if slices.ContainsFunc(itemKeys(it), func(k string) bool { return listed[k] }) {
			c.q.Listed++
		}
	}
	if !c.q.Only {
		for _, r := range e.cfg.Rules {
			if strings.EqualFold(r.Tab, t.Name) {
				c.q.Rules++
			}
		}
	}
	return c
}

// CloseTab is the tab menu's Close Tab…: it closes the tab's App windows,
// web pages, and terminals, other than those in other tabs too, and deletes
// the tab, as DeleteTab does, with the rules that name it. Windows that
// don't close at once, as their apps ask about unsaved changes, are "Not in
// a tab", and after closeGrace, Bench shows those still open. The only tab
// stays, empty: its entry reads Close all windows. The toast's Undo brings
// back the tab, or the only tab's layout, with what's still open, but not
// what has closed (see undo.go).
//
// Unless sure, with confirm_close on, it asks first when it would close two
// or more things, or a terminal that's running something, and with
// confirm_close off, still for a terminal that's running something, with
// confirm_quit_busy_terminals on, as closing the terminal alone would: it
// closes nothing, and returns the question (Ask). With dontAsk (Don't ask
// again), it turns off the setting that asked (askedBy), in the same save
// as the deleted tab.
func (e *Engine) CloseTab(index int, sure, dontAsk bool) (CloseQuestion, error) {
	e.cfgMu.Lock()
	e.lock()
	m := e.model
	if e.closed || m.Tab(index) == nil {
		e.unlock()
		e.cfgMu.Unlock()
		return CloseQuestion{}, errNoTab(index)
	}
	c := e.tabCloseLocked(index)
	set, busy := e.cfg.Settings, len(c.q.Busy) > 0
	c.q.BusyOnly = busy && !set.ConfirmClose && set.ConfirmQuitBusyTerminals
	if !sure && ((set.ConfirmClose && (c.q.closes() >= 2 || busy)) || c.q.BusyOnly) {
		c.q.Ask = true
		e.unlock()
		e.cfgMu.Unlock()
		return c.q, nil
	}
	if c.q.Only {
		// Forgetting the places of apps that have quit changes the tab
		// too, so Undo can bring them back.
		undo := 0
		if c.q.closes() > 0 || len(c.places) > 0 {
			step := e.beginUndoLocked()
			e.closeOnlyTabLocked(c)
			undo = e.keepUndoLocked(step, "Put back the layout of "+c.q.Tab+", with what's still open.")
		} else {
			e.closeOnlyTabLocked(c)
		}
		e.unlock()
		e.cfgMu.Unlock()
		msg := "Closed everything in " + c.q.Tab + "."
		if undo == 0 {
			msg = "There's nothing to close in " + c.q.Tab + "."
		}
		if dontAsk {
			msg += " " + e.stopAskingClose(c.q)
		}
		if undo == 0 {
			e.em.Toast(msg)
		} else {
			e.em.ActionToast(undoToast(msg, undo))
		}
		return c.q, nil
	}
	defer e.cfgMu.Unlock()

	// The tab's pages and terminals close, and its App windows go to "Not
	// in a tab", as they are, until they have closed or Bench shows them.
	step := e.beginDeleteLocked(index)
	for _, it := range c.ends {
		e.endItemLocked(it)
	}
	for _, it := range c.apps {
		it.closing = true
	}
	keep := func(it *Item) bool { return it.closing }
	setting, asking := c.q.askedBy()
	var edit func(*config.Config)
	if dontAsk {
		edit = func(cfg *config.Config) { *setting(&cfg.Settings) = false }
	}
	saved, writable, err := e.deleteTabLocked(index, keep, func(*Item) {}, edit)
	if err != nil {
		for _, it := range c.apps {
			it.closing = false
		}
		e.unlock()
		return CloseQuestion{}, err
	}
	e.pressCloseSoonLocked(c.apps)
	undo := e.keepUndoLocked(step, "Brought back "+c.q.Tab+", with what's still open.")
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return c.q, err
	}
	msg := "Closed " + c.q.Tab + "."
	if c.q.Stay > 0 {
		msg += " What's also in other tabs is still there."
	}
	if dontAsk && notSaved == "" {
		msg += " " + asking
	}
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	if text := closeUndoText(c.q); text != "" {
		msg += " " + text
	}
	e.em.ActionToast(undoToast(msg, undo))
	return c.q, nil
}

// closeUndoText says, after Close tab, what the toast's Undo can't bring
// back, as the question's Details do (closeTabDetails in close.js): "Undo
// brings back the tab, but not the windows and the terminal that closed.",
// or "" if nothing closed.
func closeUndoText(q CloseQuestion) string {
	var kinds []string
	for _, k := range []struct {
		n    int
		what string
	}{{q.Windows, "window"}, {q.Pages, "page"}, {q.Terminals, "terminal"}} {
		switch {
		case k.n == 1:
			kinds = append(kinds, "the "+k.what)
		case k.n > 1:
			kinds = append(kinds, "the "+k.what+"s")
		}
	}
	if len(kinds) == 0 {
		return ""
	}
	return "Undo brings back the tab, but not " + joinNames(kinds) + " that closed."
}

// closeOnlyTabLocked closes everything in the only tab, which stays, with
// its App windows until they close.
func (e *Engine) closeOnlyTabLocked(c tabClose) {
	for _, it := range c.ends {
		e.removeItemLocked(it)
	}
	for _, it := range c.places {
		e.model.Remove(it.ID)
	}
	for _, it := range c.apps {
		it.closing = true
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.pressCloseSoonLocked(c.apps)
	e.changed()
}

// stopAskingClose turns off the setting that asked q, for Don't ask again,
// and returns what the toast says about it.
func (e *Engine) stopAskingClose(q CloseQuestion) string {
	setting, asking := q.askedBy()
	notSaved, err := e.turnOff(setting)
	switch {
	case err != nil:
		return err.Error()
	case notSaved != "":
		return appliedText(notSaved)
	}
	return asking
}

// CloseItem is a sub-tab's Close Window, Close Page, or Close Terminal: it
// closes the item, from every tab it's in. An App window is asked to close,
// and its item goes once it has; after closeGrace, Bench shows it if it's
// still open. A page or a terminal closes at once, and so does an App item
// without a window, which Bench forgets.
//
// Unless sure, with confirm_quit_busy_terminals on, a terminal that's
// running something asks first: it closes nothing, and returns the
// question (Ask). With dontAsk (Don't ask again), it turns
// confirm_quit_busy_terminals off, as the question before quitting does.
func (e *Engine) CloseItem(itemID string, sure, dontAsk bool) (CloseQuestion, error) {
	e.lock()
	m := e.model
	it := m.Items[itemID]
	if e.closed || it == nil {
		e.unlock()
		return CloseQuestion{}, errGone
	}
	var q CloseQuestion
	switch {
	case it.Kind == KindApp && it.WindowID != 0:
		q.Windows = 1
	case it.Kind == KindBuiltin:
		q.Terminals = 1
		if busy, ok := e.busyTerminalLocked(it); ok {
			q.Busy = []BusyTerminal{busy}
		}
	case it.Kind == KindWeb:
		q.Pages = 1
	}
	if !sure && len(q.Busy) > 0 && e.cfg.Settings.ConfirmQuitBusyTerminals {
		q.Ask = true
		e.unlock()
		return q, nil
	}
	here := m.PaneIn(m.Current, itemID) != nil
	switch {
	case q.Windows > 0 && it.closing:
		// Bench has asked already, as when the × is clicked twice, and
		// its app may be asking about unsaved changes, with the close
		// button off meanwhile. Once checkClosedLocked finds it still
		// open, Close asks again.
	case q.Windows > 0:
		it.closing = true
		e.pressCloseSoonLocked([]*Item{it})
	default:
		e.removeItemLocked(it)
		if here {
			e.arrangeLocked(arrangeOpts{})
			e.focusShownLocked()
		}
	}
	e.unlock()
	if dontAsk {
		notSaved, err := e.stopAsking()
		switch {
		case err != nil:
			e.em.Toast(err.Error())
		case notSaved != "":
			e.em.Toast(appliedText(notSaved))
		default:
			e.em.Toast(stopAskingText)
		}
	}
	return q, nil
}

// pressCloseSoonLocked asks App items' windows to close, now, or while Bench
// is raised, once it's lowered (pressCloseLaterLocked): a save sheet would
// open under Bench's raised window.
func (e *Engine) pressCloseSoonLocked(items []*Item) {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		if !slices.Contains(e.closePress, it.ID) {
			ids = append(ids, it.ID)
		}
	}
	if e.overlay {
		e.closePress = append(e.closePress, ids...)
		return
	}
	e.pressCloseLocked(ids)
}

// pressCloseLaterLocked asks the windows that waited for Bench to be
// lowered to close.
func (e *Engine) pressCloseLaterLocked() {
	if len(e.closePress) == 0 {
		return
	}
	ids := e.closePress
	e.closePress = nil
	e.pressCloseLocked(ids)
}

// closingWindow is an App window that Bench has asked to close.
type closingWindow struct {
	itemID string
	wid    platform.WindowID
	// away is set for a window that was on another desktop, as far as
	// Bench could tell, when Bench asked: one that isn't on this desktop
	// afterwards hasn't been hidden by its app.
	away bool
}

// pressCloseLocked asks the windows of App items that are closing to close,
// and looks for them again after closeGrace. A window that has gone already
// has closed: its item goes, and its pane shows what's next, as when
// WindowDestroyed comes first. One that Bench couldn't ask is left open,
// and shown, with a toast that says so.
func (e *Engine) pressCloseLocked(ids []string) {
	var asked []closingWindow
	var failed []string
	var released, removed, here bool
	for _, id := range ids {
		it := e.model.Items[id]
		if it == nil || !it.closing || it.Kind != KindApp || it.WindowID == 0 {
			continue
		}
		err := e.p.PressClose(windowOf(it))
		switch {
		case err == nil:
			asked = append(asked, closingWindow{itemID: id, wid: it.WindowID, away: it.Status == StatusUnavailable})
		case errors.Is(err, platform.ErrNoWindow):
			here = here || e.model.PaneIn(e.model.Current, id) != nil
			e.closedLocked(it)
			removed = true
		default:
			e.log.Warn("couldn't ask a window to close", "window", it.WindowID, "item", it.ID, "error", err)
			failed = append(failed, closeFailedText(it, e.labelOfLocked(it), err))
			it.closing = false
			if e.model.IsFloating(it.ID) {
				e.releaseWindowLocked(it)
				released = true
			}
		}
	}
	if len(asked) > 0 {
		e.after(closeGrace, func() { e.checkClosedLocked(asked) })
	}
	if len(failed) == 0 && !released && !removed {
		return
	}
	e.arrangeLocked(arrangeOpts{})
	if here {
		e.focusShownLocked()
	}
	if len(failed) > 0 {
		msg := strings.Join(failed, " ")
		if released {
			msg += " " + notInTabText
		}
		e.toastLocked(msg)
	}
	e.changed()
}

// notInTabText says where the windows of a closed tab that haven't closed
// are.
const notInTabText = "Windows that are still open are in Jump to… under “Not in a tab”."

// closeFailedText says that Bench couldn't ask an item's window to close,
// naming the item by its label (labelOfLocked).
func closeFailedText(it *Item, label string, err error) string {
	switch {
	case errors.Is(err, platform.ErrOtherDesktop):
		return label + " is on another desktop, so Bench couldn't close it."
	case errors.Is(err, platform.ErrNotTrusted):
		return "Bench needs Accessibility permission to close " + label + "."
	}
	return "Bench couldn't close " + label + ". Close it in " + it.App + "."
}

// closedLocked forgets an App item whose window has closed.
func (e *Engine) closedLocked(it *Item) {
	e.win.Forget(it.WindowID)
	delete(e.redocks, it.WindowID)
	e.removeItemLocked(it)
}

// hidOnCloseLocked forgets an App item whose app hid its window rather than
// close it. The window is let go as one that leaves Bench is
// (releaseWindowLocked), not forgotten where it is: one that Bench had
// parked, which can't be put back while it's hidden, stays parked, so that
// RestoreAll puts it back on screen as Bench quits, rather than the app
// showing it again off screen.
func (e *Engine) hidOnCloseLocked(it *Item) {
	e.releaseWindowLocked(it)
	delete(e.redocks, it.WindowID)
	e.removeItemLocked(it)
}

// checkClosedLocked looks for the windows that Bench asked to close, after
// closeGrace. A window that has gone has closed, though WindowDestroyed may
// not have said so yet. So has one that its app has hidden rather than
// close, as Slack and Music do: the app no longer lists it, which looks
// like another desktop (ErrOtherDesktop). The rest are still open, as their
// apps ask about unsaved changes, say: Bench shows them, and says so.
func (e *Engine) checkClosedLocked(asked []closingWindow) {
	var open []*Item
	for _, c := range asked {
		it := e.model.Items[c.itemID]
		if it == nil || it.WindowID != c.wid || !it.closing {
			continue
		}
		_, err := e.p.Window(c.wid)
		switch {
		case errors.Is(err, platform.ErrNoWindow):
			e.closedLocked(it)
		case errors.Is(err, platform.ErrOtherDesktop) && !c.away:
			e.hidOnCloseLocked(it)
		default:
			it.closing = false
			open = append(open, it)
		}
	}
	if len(open) == 0 {
		e.arrangeLocked(arrangeOpts{})
		return
	}
	e.showStillOpenLocked(open)
}

// showStillOpenLocked shows windows that Bench asked to close that are
// still open, so that their apps' questions can be seen: one from a closed
// tab is put back on screen if Bench parked it; one in the current tab is
// shown in its pane. The first gets the keyboard, unless Bench is raised.
func (e *Engine) showStillOpenLocked(open []*Item) {
	m := e.model
	var apps []string
	floating, shown := false, false
	for _, it := range open {
		if !slices.Contains(apps, it.App) {
			apps = append(apps, it.App)
		}
		switch {
		case m.IsFloating(it.ID):
			floating = true
			e.releaseWindowLocked(it)
		case m.PaneIn(m.Current, it.ID) != nil:
			// A window that the user minimised comes back from the Dock,
			// for its app's question to be seen.
			showAgain(it)
			if !shown {
				m.ShowIn(it.ID, m.Current)
				shown = true
			} else {
				m.ShowInPane(it.ID, m.Current)
			}
		}
	}
	e.arrangeLocked(arrangeOpts{})
	if !e.overlay {
		first := open[0]
		switch {
		case m.IsFloating(first.ID):
			if err := e.p.Focus(windowOf(first)); err == nil {
				e.ownFocus[first.WindowID] = e.clk.Now()
			}
		case shown:
			e.focusShownLocked()
		}
	}
	msg := joinNames(apps) + " may be asking about unsaved changes."
	if floating {
		msg += " " + notInTabText
	}
	e.toastLocked(msg)
	e.changed()
}

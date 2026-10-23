package workspace

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/itbm/bench/internal/layout"
)

// This file keeps the sub-tabs of windows that have closed out of sight. A
// sub-tab is an open window, web page, or terminal: an App item without a
// window, because its app has quit, say, is hidden, unless Bench is opening
// its app for it right now. It keeps its place, out of sight, in its pane,
// which stays, so that the app's window goes back there when it opens
// again, shown if it was showing. A pane with nothing to show lists the
// apps it's waiting for, with Open (OpenItem) and Remove from this pane
// (RemoveRemembered).

// hiddenLocked reports whether an item is out of sight: an App item without
// a window, unless Bench is opening its app for it (see openingForLocked).
// Without Accessibility permission, Bench can't tell which windows are open,
// so it hides nothing.
// One whose window Bench has just failed to find still shows, for the
// moment until Bench checks again (see checkGoneLocked): the window may
// still be there, and hiding it would put another of the pane's windows in
// its place, and back.
func (e *Engine) hiddenLocked(it *Item) bool {
	if it == nil || it.Kind != KindApp || it.WindowID != 0 || !e.trusted {
		return false
	}
	return !e.openingForLocked(it)
}

// arrangeWhenOpeningEndsLocked arranges the current tab again at until,
// when Bench stops opening an app for an item (see openingForLocked), if
// the item is hidden then: its pane shows another of its items in its
// place. Otherwise it sends the view again, which no longer says that the
// app is opening.
func (e *Engine) arrangeWhenOpeningEndsLocked(itemID string, until time.Time) {
	e.after(max(until.Sub(e.clk.Now()), 0), func() { e.openingEndedLocked(itemID) })
}

// openingEndedLocked arranges the current tab again if an item that Bench
// was opening its app for in it is hidden now, or else sends the view again.
func (e *Engine) openingEndedLocked(itemID string) {
	if it := e.model.Items[itemID]; e.hiddenLocked(it) && e.model.PaneIn(e.model.Current, itemID) != nil {
		e.arrangeLocked(arrangeOpts{})
		return
	}
	e.touch()
}

// openingForLocked reports whether Bench is opening an App item's app for
// it right now, so that it shows, saying so: a tab switch or Open waits for
// its window while the app opens, or a placement is to give it the app's
// next window.
func (e *Engine) openingForLocked(it *Item) bool {
	if it.WindowID != 0 {
		return false
	}
	if e.wait != nil && e.wait.opened[it.ID] && e.launchingNow(it.BundleID) {
		return true
	}
	now := e.clk.Now()
	return slices.ContainsFunc(e.pending, func(pl pendingPlace) bool { return pl.item == it.ID && now.Before(pl.until) })
}

// shownItemsLocked returns the items of a list that aren't hidden.
func (e *Engine) shownItemsLocked(ids []string) []string {
	var out []string
	for _, id := range ids {
		if it := e.model.Items[id]; it != nil && !e.hiddenLocked(it) {
			out = append(out, id)
		}
	}
	return out
}

// shownTreeLocked returns a copy of a tab's tree holding only the items
// that aren't hidden. A pane whose active item is hidden shows the most
// recently used of its others, without changing the model, so that the
// item shows again when its window comes back.
func (e *Engine) shownTreeLocked(t *Tab) *layout.Node {
	root := layout.Clone(t.Root)
	for _, p := range layout.Leaves(root) {
		p.Items = e.shownItemsLocked(p.Items)
		if !slices.Contains(p.Items, p.Active) {
			p.Active = e.recentOfLocked(p.Items)
		}
	}
	return root
}

// recentOfLocked returns the most recently used of a list of items, or the
// first if none has been used, or "" for none.
func (e *Engine) recentOfLocked(ids []string) string {
	best, used := "", -1
	for _, id := range ids {
		if it := e.model.Items[id]; it != nil && it.Used > used {
			best, used = id, it.Used
		}
	}
	return best
}

// shownCountLocked returns how many of a pane's items aren't hidden.
func (e *Engine) shownCountLocked(p *layout.Node) int {
	return len(e.shownItemsLocked(p.Items))
}

// rememberedLocked returns what a displayed pane is waiting for: one entry
// for each app with hidden items in it (in the whole tab, for the "tabs"
// view's pane), in reading order. Its item is the one Open fills: the
// pane's active item (the item the "tabs" view's pane last showed) if it's
// the app's, or else the app's most recently used.
func (e *Engine) rememberedLocked(t *Tab, paneID string) []RememberedApp {
	ids, active := t.ItemIDs(), t.CompactShown
	if p := t.Pane(paneID); p != nil {
		ids, active = p.Items, p.Active
	} else if paneID != layout.CompactPaneID {
		return nil
	}
	out := []RememberedApp{}
	byApp := map[string][]string{}
	var order []string
	for _, id := range ids {
		it := e.model.Items[id]
		if !e.hiddenLocked(it) {
			continue
		}
		key := strings.ToLower(it.BundleID)
		if byApp[key] == nil {
			order = append(order, key)
		}
		byApp[key] = append(byApp[key], id)
	}
	for _, key := range order {
		id := e.recentOfLocked(byApp[key])
		if slices.Contains(byApp[key], active) {
			id = active
		}
		it := e.model.Items[id]
		_, text := e.statusOf(it)
		out = append(out, RememberedApp{ItemID: id, BundleID: it.BundleID, App: it.App, Mono: it.Mono,
			Text: text, CanOpen: !e.notFound[key]})
	}
	return out
}

// keepShownLocked runs take, which takes a hidden item out of its pane in
// tab t, and keeps the pane showing what it showed: if the item was the
// pane's active one, the pane showed its most recently used other item in
// its place (see shownTreeLocked), which becomes active, rather than the
// item next to it.
func (e *Engine) keepShownLocked(t *Tab, id string, take func()) {
	p := layout.PaneOf(t.Root, id)
	was := p != nil && p.Active == id
	take()
	if !was {
		return
	}
	if q := t.Pane(p.ID); q != nil {
		if s := e.recentOfLocked(e.shownItemsLocked(q.Items)); s != "" {
			q.Active = s
		}
	}
}

// RemoveRemembered forgets the places of an app in a pane of the current
// tab that its windows would go back to: its App items there without a
// window. An item that's in other tabs too stays there. As when the user
// pops out a pane's last window, a pane left empty closes up. In the "tabs"
// view, the pane is the whole tab.
func (e *Engine) RemoveRemembered(paneID, bundleID string) error {
	e.lock()
	defer e.unlock()
	m := e.model
	t := m.CurrentTab()
	ids := t.ItemIDs()
	if paneID != layout.CompactPaneID || !e.isTabsView(t) {
		p := t.Pane(paneID)
		if p == nil {
			return errNoPane
		}
		ids = slices.Clone(p.Items)
	}
	name := e.appName(bundleID)
	removed := 0
	for _, id := range ids {
		it := m.Items[id]
		if !isAppOf(it, bundleID) || !e.hiddenLocked(it) {
			continue
		}
		e.keepShownLocked(t, id, func() {
			if len(m.TabsOf(id)) > 1 {
				m.takeFrom(id, m.Current)
			} else {
				m.Remove(id)
			}
		})
		removed++
	}
	if removed == 0 {
		return fmt.Errorf("This pane isn't waiting for %s.", name)
	}
	e.toastLocked("Removed " + name + " from this pane.")
	e.arrangeLocked(arrangeOpts{})
	e.changed()
	return nil
}

// ClosePane closes a pane of the current tab that shows nothing, forgetting
// the places of apps it's waiting for (an item in other tabs too stays
// there), and the rest of the tab closes up. A tab's last pane stays, and
// panes can't be closed in the "tabs" view, which shows one.
func (e *Engine) ClosePane(paneID string) error {
	e.lock()
	defer e.unlock()
	m := e.model
	t := m.CurrentTab()
	if e.isTabsView(t) {
		return errSplitsHidden
	}
	p := t.Pane(paneID)
	switch {
	case p == nil:
		return errNoPane
	case len(t.Leaves()) == 1:
		return errLastPane
	case e.shownCountLocked(p) > 0:
		return errPaneNotEmpty
	}
	for _, id := range slices.Clone(p.Items) {
		m.DropFrom(id, m.Current)
	}
	root, err := layout.RemovePane(t.Root, paneID)
	if err != nil {
		return errNoPane
	}
	t.Root = root
	t.markCustom()
	t.fixFocus()
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	return nil
}

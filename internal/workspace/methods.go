package workspace

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// This file has the bound methods for switching, focusing, showing, and
// moving. Each returns an error with a sentence for the user when it refuses.

// SetPaneArea records the pane area (between the title bar and the status
// bar, in content coordinates) and docks the current tab's windows in it.
func (e *Engine) SetPaneArea(area layout.Rect) error {
	e.lockFor(reasonPaneArea)
	defer e.unlock()
	if e.area == area {
		return nil
	}
	e.area = area
	e.arrangeLocked(arrangeOpts{})
	return nil
}

// SwitchTab switches to tab index (counting from 0). Switching to the
// current tab arranges it again.
func (e *Engine) SwitchTab(index int) error {
	e.lockFor(reasonTab)
	defer e.unlock()
	return e.switchTabLocked(index)
}

// switchTabLocked is SwitchTab with the lock held.
func (e *Engine) switchTabLocked(index int) error {
	if e.model.Tab(index) == nil {
		return errNoTab(index)
	}
	e.switchToLocked(index)
	return nil
}

// switchToLocked switches tabs and focuses the focused pane, or rearranges
// the current tab.
func (e *Engine) switchToLocked(index int) {
	if index == e.model.Current && e.started {
		e.arrangeLocked(arrangeOpts{hideOtherApps: true})
		e.focusShownLocked()
		e.changed()
		return
	}
	e.switchLocked(index, switchOpts{focus: true})
}

// StepTab switches to the next (1) or previous (-1) tab, wrapping.
func (e *Engine) StepTab(step int) error {
	e.lockFor(reasonTab)
	defer e.unlock()
	if i := e.stepTargetLocked(step); i >= 0 {
		e.switchToLocked(i)
	}
	return nil
}

// stepTargetLocked returns the tab that StepTab switches to, or -1 if
// there are no tabs.
func (e *Engine) stepTargetLocked(step int) int {
	n := len(e.model.Tabs)
	if n == 0 {
		return -1
	}
	return ((e.model.Current+step)%n + n) % n
}

// LastTab switches back to the tab that was current before this one, for
// the last_tab shortcut, so that it goes back and forth between the two
// tabs used last. While the shortcut's modifiers are still held, it starts
// a hold instead, which switches once they're let go of, and each LastTab
// during the hold chooses the next tab in the order they were used (see
// hold.go). A hold starts before Bench has switched tabs too, as there are
// tabs to choose from.
func (e *Engine) LastTab() error {
	e.lockFor(reasonTab)
	defer e.unlock()
	return e.goToLastTabLocked()
}

// goToLastTabLocked is LastTab with the lock held.
func (e *Engine) goToLastTabLocked() error {
	if e.hold != nil {
		e.stepHoldLocked(1)
		return nil
	}
	if e.startHoldLocked() {
		return nil
	}
	i := e.lastTabLocked()
	if i < 0 {
		return errNoLastTab
	}
	e.switchToLocked(i)
	return nil
}

// lastTabLocked returns the index of the tab that was current before the
// current one, or -1 if there's none: Bench hasn't switched tabs since it
// opened, or that tab has been deleted, or is current again, as after the
// current tab was deleted.
func (e *Engine) lastTabLocked() int {
	m := e.model
	if e.lastTab == nil {
		return -1
	}
	i := m.IndexOf(e.lastTab)
	if i == m.Current {
		return -1
	}
	return i
}

// SetMode sets the layout mode: auto, wide, or compact.
func (e *Engine) SetMode(mode string) error {
	e.lock()
	defer e.unlock()
	switch m := layout.Mode(mode); m {
	case layout.ModeAuto, layout.ModeWide, layout.ModeCompact:
		e.model.Mode = m
	default:
		return errBadMode
	}
	e.arrangeLocked(arrangeOpts{})
	e.changed()
	return nil
}

// FocusPane focuses a pane of the current tab. Focusing another pane than
// a zoomed one shows every pane again.
func (e *Engine) FocusPane(paneID string) error {
	e.lock()
	defer e.unlock()
	t := e.model.CurrentTab()
	if paneID == layout.CompactPaneID && e.isTabsView(t) {
		return nil
	}
	p := t.Pane(paneID)
	if p == nil {
		return errNoPane
	}
	zoomed := e.zoomedLocked() != nil
	e.model.FocusPane(t, p)
	if zoomed && e.zoomedLocked() == nil {
		e.arrangeLocked(arrangeOpts{})
	}
	e.focusShownLocked()
	e.changed()
	return nil
}

// FocusUI gives Bench's own UI the keyboard, for a field that Bench draws
// over no window, such as the one that renames a tab or sub-tab in place:
// it takes the keyboard from the window or page that the focused pane
// shows, which a tab switch just before may have given it, and from a page
// that Bench being lowered after a menu would give it back to, and drops
// any focus still put off. The pane stays focused.
func (e *Engine) FocusUI() error {
	e.lock()
	defer e.unlock()
	e.newFocusLocked()
	if !e.opts.Headless {
		e.p.FocusUI()
	}
	return nil
}

// ShowItem shows an item in its pane (or in the "tabs" view's compact
// pane), and focuses the pane. A window that the user minimised comes back
// from the Dock (see showAgain).
func (e *Engine) ShowItem(paneID, itemID string) error {
	e.lock()
	defer e.unlock()
	m := e.model
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
	ti, p := m.Where(itemID)
	switch {
	case paneID == layout.CompactPaneID:
		if ti != m.Current {
			return errNotInPane
		}
	case p == nil || p.ID != paneID:
		return errNotInPane
	}
	showAgain(it)
	m.Show(itemID)
	if ti == m.Current {
		e.arrangeLocked(arrangeOpts{})
		e.focusShownLocked()
	}
	e.changed()
	return nil
}

// MoveItem moves an item to the centre of a pane of tab (joining it) or to
// an edge (splitting it), and says so, with Undo. Splits are refused in the
// "tabs" view.
func (e *Engine) MoveItem(itemID string, tab int, paneID, zone string) error {
	e.lock()
	defer e.unlock()
	return e.moveItemLocked(itemID, tab, paneID, zone)
}

// moveItemLocked is MoveItem, with the engine locked.
func (e *Engine) moveItemLocked(itemID string, tab int, paneID, zone string) error {
	m := e.model
	t := m.Tab(tab)
	if t == nil {
		return errNoTab(tab)
	}
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
	z := layout.Zone(zone)
	if e.isTabsView(t) {
		if z != layout.Center && z != "" {
			return errSplitsHidden
		}
		if paneID == layout.CompactPaneID {
			return e.moveToTabLocked(it, tab)
		}
	}
	if z == "" {
		z = layout.Center
	}
	// A pane can't be split by the only one of its sub-tabs that shows: its
	// hidden ones would be left behind, out of sight.
	if from := m.PaneIn(tab, itemID); z != layout.Center && from != nil && from.ID == paneID && e.shownCountLocked(from) < 2 {
		return errOnlyItem
	}
	floating := m.IsFloating(itemID)
	step := e.beginUndoLocked()
	moved, err := m.move(itemID, tab, paneID, z, e.stackViewLocked(t))
	if err != nil || !moved {
		return err
	}
	if floating {
		e.bringBackLocked(it)
	}
	if z != layout.Center {
		// A pane of its own is for seeing it, as Show side by side says: a
		// window that the user minimised comes back from the Dock.
		showAgain(it)
	}
	e.arrangeLocked(arrangeOpts{})
	if tab == m.Current {
		e.focusShownLocked()
	}
	e.changed()
	name := undoName{"Move Window", "moved " + e.labelOfLocked(it)}
	e.undoToastLocked("Moved "+e.labelOfLocked(it)+".", e.keepUndoLocked(step, name, "Moved "+e.labelOfLocked(it)+" back."))
	return nil
}

// ShowSideBySide is Show side by side for an item of the current tab in
// the "tabs" view, which shows the tab as one pane, without its splits.
// Rather than refuse, Bench changes to the wide layout, as choosing Wide
// does, so that the tab shows its panes, and shows the item there, in
// focus. A tab of two or more panes then shows the item in its own pane,
// beside the others, and keeps its tree, so that choosing Auto again gives
// back the view from before. A tab of one pane would show nothing side by
// side even then, so the item gets a pane of its own on the right, as
// Split Right gives it in the wide layout. The toast says
// how to go back, with Undo, which puts back the layout and the tab's
// panes. Outside the "tabs" view, it's Show side by side as the wide
// layout has it: MoveItem to the right edge of the item's pane.
func (e *Engine) ShowSideBySide(itemID string) error {
	e.lock()
	defer e.unlock()
	return e.showSideBySideLocked(itemID)
}

// showSideBySideLocked is ShowSideBySide, with the engine locked.
func (e *Engine) showSideBySideLocked(itemID string) error {
	m := e.model
	t := m.CurrentTab()
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
	p := m.PaneIn(m.Current, itemID)
	if t == nil || p == nil {
		return errNotInPane
	}
	if !e.isTabsView(t) {
		return e.moveItemLocked(itemID, m.Current, p.ID, string(layout.Right))
	}
	// Only a tab of one pane is split. Its pane must have another window
	// to show beside the item: a tab of one window has nothing.
	split := len(t.Leaves()) < 2
	if split && e.shownCountLocked(p) < 2 {
		return errOnlyItem
	}
	was := m.Mode
	step := e.beginUndoLocked()
	step.setsMode, step.mode = true, was
	m.Mode = layout.ModeWide
	if split {
		if _, err := m.move(itemID, m.Current, p.ID, layout.Right, false); err != nil {
			m.Mode = was
			return err
		}
	}
	showAgain(it)
	m.ShowIn(itemID, m.Current)
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	name := undoName{"Show Side by Side", "showed " + e.labelOfLocked(it) + " side by side"}
	e.undoToastLocked(sideBySideText(e.labelOfLocked(it), was, split), e.keepUndoLocked(step, name, backToLayoutText(was)))
	return nil
}

// MoveItemAt moves an item into a pane of tab, at index among the pane's
// sub-tabs as drawn, without the hidden ones (see hiddenLocked): before the
// sub-tab at index, or after the last for an index past them. It's a drop
// on a pane's header, or Move left and Move right in a sub-tab's menu, and
// never splits: an item moved along its own pane's strip changes places in
// it, and one from another pane joins this one there. It says so, with
// Undo, unless the item is at that place already. In the "tabs" view, its
// pane (layout.CompactPaneID) shows the whole tab, in reading order, so the
// item stays in its own pane if a sub-tab beside its new place is in it,
// and otherwise joins the pane of the sub-tab it goes before, or after. Its
// own pane stays there even if that empties it, so the wide layout keeps
// its panes, and the toast says so.
func (e *Engine) MoveItemAt(itemID string, tab int, paneID string, index int) error {
	e.lock()
	defer e.unlock()
	m := e.model
	t := m.Tab(tab)
	if t == nil {
		return errNoTab(tab)
	}
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
	to, at := paneID, -1
	keep := paneID == layout.CompactPaneID && e.isTabsView(t)
	if keep {
		to, at = e.compactPlaceLocked(t, itemID, index)
	} else if p := t.Pane(paneID); p != nil {
		at = placeAmong(p.Items, e.shownItemsLocked(p.Items), itemID, index)
	} else {
		return errNoPane
	}
	if at < 0 {
		return nil
	}
	floating := m.IsFloating(itemID)
	own := layout.PaneOf(t.Root, itemID)
	step := e.beginUndoLocked()
	moved, err := m.MoveAt(itemID, tab, to, at, keep)
	if err != nil || !moved {
		return err
	}
	if floating {
		e.bringBackLocked(it)
	}
	e.arrangeLocked(arrangeOpts{})
	if tab == m.Current {
		e.focusShownLocked()
	}
	e.changed()
	msg := "Moved " + e.labelOfLocked(it) + "."
	if own != nil && own.ID != to && len(own.Items) == 0 {
		msg += " Its old pane is empty in the wide layout."
	}
	name := undoName{"Move Window", "moved " + e.labelOfLocked(it)}
	e.undoToastLocked(msg, e.keepUndoLocked(step, name, "Moved "+e.labelOfLocked(it)+" back."))
	return nil
}

// placeAmong returns where an item goes among a pane's other items (items,
// without id), for index among the pane's sub-tabs as drawn (shown): before
// the sub-tab at index, or after the last of the pane's items for an index
// past them. It returns -1 if the item is in the pane's strip already, next
// to that place, which would leave the strip as it is.
func placeAmong(items, shown []string, id string, index int) int {
	index = min(max(index, 0), len(shown))
	if k := slices.Index(shown, id); k >= 0 && (index == k || index == k+1) {
		return -1
	}
	others := without(items, id)
	if index < len(shown) {
		if i := slices.Index(others, shown[index]); i >= 0 {
			return i
		}
	}
	return len(others)
}

// compactPlaceLocked returns the pane of tab t, and the place among its
// other items, for an item moved to index on the "tabs" view's strip, which
// shows the tab's items in reading order without the hidden ones: the
// item's own pane, if the sub-tab it goes before or after is in it, or else
// the pane of the sub-tab it goes before, or of the last. It returns -1 if
// the item is in the strip already, next to that place.
func (e *Engine) compactPlaceLocked(t *Tab, id string, index int) (string, int) {
	shown := e.shownItemsLocked(t.ItemIDs())
	index = min(max(index, 0), len(shown))
	if k := slices.Index(shown, id); k >= 0 && (index == k || index == k+1) {
		return "", -1
	}
	next, prev := "", ""
	if index < len(shown) {
		next = shown[index]
	}
	if index > 0 {
		prev = shown[index-1]
	}
	own := layout.PaneOf(t.Root, id)
	in := func(other string) bool { return other != "" && own != nil && layout.PaneOf(t.Root, other) == own }
	var p *layout.Node
	anchor, after := next, false
	switch {
	case in(next):
		p = own
	case in(prev):
		p, anchor, after = own, prev, true
	case next != "":
		p = layout.PaneOf(t.Root, next)
	case prev != "":
		p, anchor, after = layout.PaneOf(t.Root, prev), prev, true
	default:
		return "", -1
	}
	at := slices.Index(without(p.Items, id), anchor)
	if after {
		at++
	}
	return p.ID, at
}

// without returns a copy of a list of IDs without id.
func without(ids []string, id string) []string {
	return slices.DeleteFunc(slices.Clone(ids), func(o string) bool { return o == id })
}

// MoveItemToTab moves an item into a tab's focused pane, without switching
// tabs. It does nothing for an item already in that tab.
func (e *Engine) MoveItemToTab(itemID string, tab int) error {
	e.lock()
	defer e.unlock()
	if e.model.Tab(tab) == nil {
		return errNoTab(tab)
	}
	it := e.model.Items[itemID]
	if it == nil {
		return errGone
	}
	return e.moveToTabLocked(it, tab)
}

// SendToTab moves the window that the focused pane shows into tab index's
// focused pane, as MoveItemToTab does, without switching tabs: the
// send_to_tab_modifiers shortcut with 1 to 9. It's refused for an empty
// pane and for the current tab.
func (e *Engine) SendToTab(index int) error {
	e.lock()
	defer e.unlock()
	return e.sendToTabLocked(index)
}

// SendToLastTab does what SendToTab does for the tab used before this one
// (lastTabLocked), which the last_tab shortcut goes back to:
// send_to_tab_modifiers with `.
func (e *Engine) SendToLastTab() error {
	e.lock()
	defer e.unlock()
	i := e.lastTabLocked()
	if i < 0 {
		return errNoLastTabToSend
	}
	return e.sendToTabLocked(i)
}

// sendToTabLocked moves the focused pane's shown item into tab index's
// focused pane (moveToTabLocked).
func (e *Engine) sendToTabLocked(index int) error {
	m := e.model
	t := m.Tab(index)
	if t == nil {
		return errNoTab(index)
	}
	it := m.Items[e.focusedShownLocked()]
	if it == nil {
		return errEmptyPane
	}
	if index == m.Current {
		return errAlreadyIn(t.Name)
	}
	return e.moveToTabLocked(it, index)
}

// moveToTabLocked moves an item into a tab's focused pane, and says so,
// with Undo, and Go there, which switches to that tab.
func (e *Engine) moveToTabLocked(it *Item, tab int) error {
	floating := e.model.IsFloating(it.ID)
	shown := e.focusedShownLocked()
	step := e.beginUndoLocked()
	moved, err := e.model.MoveToTab(it.ID, tab)
	if err != nil || !moved {
		return err
	}
	if floating {
		e.bringBackLocked(it)
	}
	e.arrangeLocked(arrangeOpts{})
	// Keyboard focus follows the focused pane, so if it shows another item
	// now, such as the one left behind, that item gets the keyboard.
	if e.focusedShownLocked() != shown {
		e.focusShownLocked()
	}
	e.changed()
	name := e.model.Tabs[tab].Name
	named := undoName{"Move to Tab", "moved " + e.labelOfLocked(it) + " to " + name}
	t := undoToast("Moved "+e.labelOfLocked(it)+" to "+name+".", e.keepUndoLocked(step, named, "Moved "+e.labelOfLocked(it)+" back."))
	t.Second = &ToastAction{Label: "Go there", Method: "SwitchTab", Arg: tab}
	em := e.em
	e.later(func() { em.ActionToast(t) })
	return nil
}

// AddItemToTab shows an item in a tab's focused pane as well as where it
// is, without switching tabs: the same window can be in several tabs, and
// is docked in its pane in whichever is current. As nothing changes in the
// current tab, a toast says so, with Undo.
func (e *Engine) AddItemToTab(itemID string, tab int) error {
	e.lock()
	defer e.unlock()
	m := e.model
	t := m.Tab(tab)
	if t == nil {
		return errNoTab(tab)
	}
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
	step := e.beginUndoLocked()
	added, err := m.AddToTab(itemID, tab, "")
	if err != nil {
		return poppedOut(err, e.cfg.Settings.DragToAdd)
	}
	if !added {
		return errAlreadyIn(t.Name)
	}
	if tab == m.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	name := undoName{"Also Show In", "showed " + e.labelOfLocked(it) + " in " + t.Name + " too"}
	undo := e.keepUndoLocked(step, name, "Took "+e.labelOfLocked(it)+" out of "+t.Name+" again.")
	e.undoToastLocked("Showing "+e.labelOfLocked(it)+" in "+t.Name+" too.", undo)
	e.changed()
	return nil
}

// RemoveItemFromTab takes an item that's in several tabs out of one of
// them, leaving it in the others. The toast's Undo puts it back.
func (e *Engine) RemoveItemFromTab(itemID string, tab int) error {
	e.lock()
	defer e.unlock()
	m := e.model
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
	step := e.beginUndoLocked()
	if err := m.RemoveFromTab(itemID, tab); err != nil {
		return err
	}
	if tab == m.Current {
		e.arrangeLocked(arrangeOpts{})
		e.focusShownLocked()
	}
	var still []string
	for _, ti := range m.TabsOf(itemID) {
		still = append(still, m.Tabs[ti].Name)
	}
	name := m.Tabs[tab].Name
	named := undoName{"Remove from This Tab", "removed " + e.labelOfLocked(it) + " from " + name}
	undo := e.keepUndoLocked(step, named, "Put "+e.labelOfLocked(it)+" back in "+name+".")
	e.undoToastLocked("Removed "+e.labelOfLocked(it)+" from "+name+". It's still in "+joinNames(still)+".", undo)
	e.changed()
	return nil
}

// maxLabel is the most characters a name that the user gives a terminal or
// web page (RenameItem) keeps.
const maxLabel = 200

// RenameItem gives a terminal or web page a name of its own, which its
// sub-tab shows in every tab instead of the title its shell or page sets,
// and state.json keeps; or, given "", takes it away again (Use automatic
// title). The name is one line, without spaces at either end. An app's
// window always shows the app's own title.
func (e *Engine) RenameItem(itemID, label string) error {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	if it == nil {
		return errGone
	}
	if it.Kind == KindApp {
		return errAppTitle
	}
	label = strings.Join(strings.Fields(label), " ")
	if r := []rune(label); len(r) > maxLabel {
		label = strings.TrimSpace(string(r[:maxLabel]))
	}
	if label == it.Label {
		return nil
	}
	it.Label = label
	e.changed()
	return nil
}

// focusedShownLocked returns the item the current tab's focused pane
// shows, or "".
func (e *Engine) focusedShownLocked() string {
	if g, ok := e.displayLocked().focusedGeom(); ok {
		return g.node.Active
	}
	return ""
}

// stackViewLocked reports whether a tab is shown in the stack view, which
// shows the tab's top-level row as a column: the stack small-screen setting
// on a small screen. Splitting a pane of that row left or right must then
// nest, to look side by side (see Model.move).
func (e *Engine) stackViewLocked(t *Tab) bool {
	return e.sizeClassLocked() == layout.Compact && (t.SmallScreens == layout.Stack || t.SmallScreens == "")
}

// directionOf checks a direction.
func directionOf(dir string) (layout.Direction, error) {
	switch d := layout.Direction(dir); d {
	case layout.DirLeft, layout.DirRight, layout.DirUp, layout.DirDown:
		return d, nil
	}
	return "", errBadDirection
}

// edgeOf returns the zone at a pane's edge in a direction.
func edgeOf(d layout.Direction) layout.Zone {
	switch d {
	case layout.DirLeft:
		return layout.Left
	case layout.DirRight:
		return layout.Right
	case layout.DirUp:
		return layout.Top
	}
	return layout.Bottom
}

// framesOf returns a display's pane frames, for finding neighbours.
func framesOf(d display) []layout.PaneRect {
	out := make([]layout.PaneRect, len(d.panes))
	for i, g := range d.panes {
		out[i] = layout.PaneRect{ID: g.node.ID, Rect: g.frame}
	}
	return out
}

// FocusDirection focuses the pane next to the focused one in a direction,
// if there is one. A zoomed pane's neighbours are the tab's, out of sight:
// going to one shows every pane again.
func (e *Engine) FocusDirection(dir string) error {
	e.lock()
	defer e.unlock()
	d, err := directionOf(dir)
	if err != nil {
		return err
	}
	z := e.zoom
	zoomed := e.endZoomLocked()
	disp := e.displayLocked()
	if disp.compact {
		return nil
	}
	next, ok := layout.Neighbour(framesOf(disp), disp.focus, d)
	if !ok {
		if zoomed {
			e.zoom = z
		}
		return nil
	}
	t := disp.tab
	e.model.FocusPane(t, t.Pane(next))
	if zoomed {
		e.arrangeLocked(arrangeOpts{})
	}
	e.focusShownLocked()
	e.changed()
	return nil
}

// MoveDirection moves the focused pane's shown item into the neighbouring
// pane in a direction, or splits the pane at that edge if there's no
// neighbour and the pane has another item to leave behind. A zoomed pane's
// neighbours are the tab's, out of sight: moving an item shows every pane
// again. In the "tabs" view, the key to the right is Show side by side
// (ShowSideBySide), and the others are refused.
func (e *Engine) MoveDirection(dir string) error {
	e.lock()
	defer e.unlock()
	z := e.zoom
	zoomed := e.endZoomLocked()
	err := e.moveDirectionLocked(dir)
	if err != nil && zoomed {
		e.zoom = z
	}
	return err
}

func (e *Engine) moveDirectionLocked(dir string) error {
	d, err := directionOf(dir)
	if err != nil {
		return err
	}
	disp := e.displayLocked()
	if disp.compact {
		// The "tabs" view hides the tab's splits. The key to the right
		// shows the window side by side in the wide layout, as the window
		// menu's Show Side by Side (Wide Layout) does, which has its keys.
		if g, ok := disp.focusedGeom(); ok && d == layout.DirRight && g.node.Active != "" {
			return e.showSideBySideLocked(g.node.Active)
		}
		return errSplitsHidden
	}
	t := disp.tab
	// The pane as drawn, without its hidden items (see hiddenLocked).
	g, ok := disp.focusedGeom()
	if !ok || g.node.Active == "" {
		return errEmptyPane
	}
	p := g.node
	id := p.Active
	m := e.model
	if next, ok := layout.Neighbour(framesOf(disp), p.ID, d); ok {
		_, err = m.Move(id, m.Current, next, layout.Center)
	} else if len(p.Items) > 1 {
		_, err = m.move(id, m.Current, p.ID, edgeOf(d), e.stackViewLocked(t))
	} else {
		return errNothingBeside
	}
	if err != nil {
		return err
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	return nil
}

// CycleSubtab shows the next (1) or previous (-1) item in the focused pane,
// or in the "tabs" view's pane, wrapping, as drawn: hidden items are left
// out (see hiddenLocked).
func (e *Engine) CycleSubtab(step int) error {
	e.lock()
	defer e.unlock()
	disp := e.displayLocked()
	var items []string
	var active string
	if g, ok := disp.focusedGeom(); ok {
		items, active = g.node.Items, g.node.Active
	}
	next := cycle(items, active, step)
	if next == "" || next == active {
		return nil
	}
	e.model.Show(next)
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	return nil
}

// SetSizes changes a split's sizes, when the user drags a divider.
func (e *Engine) SetSizes(splitID string, sizes []float64) error {
	e.lock()
	defer e.unlock()
	if err := e.setSizesLocked(splitID, sizes); err != nil {
		return err
	}
	e.dividerSizedLocked()
	return nil
}

// dividerSizedLocked notes that the user has just resized the panes with
// one of Bench's dividers, for the grace after it (inGraceLocked): the
// mouse that's held down is Bench's, not on a window. The keys that resize
// the panes (resizeFocusedLocked) don't note it, as they leave the mouse
// to the user.
func (e *Engine) dividerSizedLocked() {
	e.sizedAt, e.edgeSized = e.clk.Now(), false
}

// setSizesLocked changes a split's sizes, and arranges the current tab if
// the split is in it.
func (e *Engine) setSizesLocked(splitID string, sizes []float64) error {
	ti, err := e.model.SetSizes(splitID, sizes)
	if err != nil {
		return err
	}
	if ti == e.model.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	return nil
}

// Rearrange arranges the current tab again. It docks again the tab's
// windows that their app or a window tool kept moving out of their panes
// (keptMoving), with all their tries, as Dock again does, but not those
// that the user dragged out.
func (e *Engine) Rearrange() error {
	e.lockFor(reasonRearrange)
	defer e.unlock()
	t := e.model.CurrentTab()
	for _, id := range t.ItemIDs() {
		if it := e.model.Items[id]; it != nil && it.keptMoving {
			it.Undocked, it.keptMoving = false, false
			delete(e.redocks, it.WindowID)
			it.Status, it.StatusText = StatusReady, ""
		}
	}
	e.recheckUnavailableLocked(t)
	e.arrangeLocked(arrangeOpts{hideOtherApps: true})
	e.toastLocked("Rearranged " + t.Name)
	return nil
}

// Redock docks a window again that the user dragged out of its pane, or
// that its app or a window tool kept moving out of it (keptMoving). Bench
// puts the window back again as many times as at first, should the app
// move it again (redockSoonLocked).
func (e *Engine) Redock(itemID string) error {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	if it == nil {
		return errGone
	}
	if !it.Undocked {
		return errNotUndocked
	}
	it.Undocked, it.keptMoving = false, false
	delete(e.redocks, it.WindowID)
	it.Status, it.StatusText = StatusReady, ""
	// The user may have minimised it while it was out of its pane.
	showAgain(it)
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	return nil
}

// SetOverlay raises Bench above the docked windows while a menu, dialog, or
// drag is open (true), and lowers it again (false). Web views are hidden
// while it's raised.
func (e *Engine) SetOverlay(open bool) error {
	e.lock()
	defer e.unlock()
	e.setOverlayLocked(open)
	return nil
}

// setOverlayLocked turns raised mode on or off. Lowered after the jump
// shortcut opened the palette, Bench gives the keyboard back to the App
// window that had it, unless something has taken the focus since
// (refocusAfterJumpLocked).
func (e *Engine) setOverlayLocked(open bool) {
	e.overlay = open
	e.p.SetRaised(open)
	if open {
		// Nothing can be dropped onto a raised Bench (dropsAllowedLocked).
		e.endDragLocked()
		e.arrangeWebLocked(e.displayLocked())
		// The user may have edited config.toml in one of Bench's own
		// terminals, which leaves Bench active, before opening Settings,
		// say.
		e.checkConfigLocked()
		return
	}
	// Settings ▸ Keyboard has closed, so it isn't recording a shortcut,
	// and Settings isn't previewing anything.
	e.pauseShortcutsLocked(false)
	e.endPreviewLocked()
	refocus := e.jump.itemID != "" && e.focusSeq.Load() == e.jump.seq
	if !refocus {
		e.jump = jumpNote{}
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusWebLaterLocked()
	e.pressCloseLaterLocked()
	if refocus {
		e.refocusLaterLocked()
	}
}

// refocusLaterLocked gives the keyboard back to the App window noted when
// the jump shortcut opened the palette (refocusAfterJumpLocked), in a
// moment (jumpRefocusDelay), once Bench is lowered or a native menu has
// closed.
func (e *Engine) refocusLaterLocked() {
	if e.jumpTimer != nil {
		e.jumpTimer.Stop()
	}
	e.jumpTimer = e.after(jumpRefocusDelay, func() {
		e.jumpTimer = nil
		e.refocusAfterJumpLocked()
	})
}

// noteJumpLocked notes, as the jump shortcut opens the palette, whether the
// focused pane shows an App window that has the keyboard: its app is
// frontmost and it's the app's focused window, not another of the app's
// windows, such as one not in a tab or a dialog. The shell then activates
// Bench to show the palette, so the
// window loses the keyboard. If the palette closes without anything taking
// the focus, as with Escape, a click outside it, or the shortcut again,
// Bench gives the keyboard back to the window (refocusAfterJumpLocked). A
// Web item's view and a terminal get it back without this: Bench's window
// remembers the view that had it, and the frontend the terminal. The
// shortcut pressed while the palette is open, which closes it, keeps the
// note, and so does the shortcut pressed while a native menu is open.
func (e *Engine) noteJumpLocked() {
	if e.menuOrDialogLocked() {
		return
	}
	e.jump = jumpNote{}
	g, ok := e.displayLocked().focusedGeom()
	if !ok {
		return
	}
	it := e.model.Items[g.node.Active]
	if it == nil || it.Kind != KindApp || it.WindowID == 0 || !e.frontmostLocked(it.BundleID) {
		return
	}
	if w, err := e.p.Window(it.WindowID); err != nil || !w.Focused {
		return
	}
	e.jump = jumpNote{itemID: it.ID, seq: e.focusSeq.Load()}
}

// frontmostLocked reports whether an app is the frontmost app.
func (e *Engine) frontmostLocked(bundleID string) bool {
	for _, app := range e.p.RunningApps() {
		if app.Active {
			return sameApp(app.BundleID, bundleID)
		}
	}
	return false
}

// refocusAfterJumpLocked gives the keyboard back to the App window noted
// when the jump shortcut opened the palette (noteJumpLocked), if nothing has
// taken the focus since: no row of the palette has focused a pane or a
// window, the user hasn't clicked one, and no other app has come forward.
// The focused pane must still show the window. If Bench has been raised
// again, by a dialog that the palette opened, such as Settings, the note
// waits until it's lowered, and while a native menu is open, such as the
// one that the palette's Window Options… opens, until it closes
// (closeMenuLocked).
func (e *Engine) refocusAfterJumpLocked() {
	if e.menuOrDialogLocked() {
		return
	}
	note := e.jump
	e.jump = jumpNote{}
	if note.itemID == "" || e.focusSeq.Load() != note.seq || e.benchHidden || e.otherAppActiveLocked() {
		return
	}
	if g, ok := e.displayLocked().focusedGeom(); ok && g.node.Active == note.itemID {
		e.focusNowLocked()
	}
}

// ShowPreview shows a live preview of an item's window or web view under
// anchor (the sub-tab, in content coordinates), when live previews are on
// and allowed.
func (e *Engine) ShowPreview(itemID string, anchor layout.Rect) error {
	e.lock()
	defer e.unlock()
	if !e.cfg.Settings.LivePreviews {
		return nil
	}
	it := e.model.Items[itemID]
	w, unmanaged := e.unmanagedLocked(itemID)
	if it == nil && !unmanaged {
		return nil
	}
	if !e.screenRec {
		if e.screenRec = e.p.ScreenRecordingAllowed(false); !e.screenRec {
			e.touch()
			return nil
		}
	}
	var target platform.PreviewTarget
	switch {
	case it == nil:
		// A window that's in no tab, from the palette's "Not in a tab".
		target = platform.PreviewTarget{Window: w.ID, Title: e.labelOfLocked(&Item{App: e.windowAppName(w), Title: w.Title})}
	case it.Kind == KindApp && it.WindowID != 0:
		target = platform.PreviewTarget{Window: it.WindowID, Title: e.labelOfLocked(it)}
	case it.Kind == KindWeb && e.webMade[it.ID]:
		target = platform.PreviewTarget{WebViewID: it.ID, Title: e.labelOfLocked(it)}
	default:
		return nil
	}
	_ = e.p.ShowPreview(target, anchor)
	return nil
}

// unmanagedLocked returns the unmanaged window that an ID in View.Floating
// names (see floatingID).
func (e *Engine) unmanagedLocked(id string) (platform.Window, bool) {
	rest, ok := strings.CutPrefix(id, floatingPrefix)
	if !ok {
		return platform.Window{}, false
	}
	n, err := strconv.ParseUint(rest, 10, 32)
	if err != nil {
		return platform.Window{}, false
	}
	w, ok := e.unmanaged[platform.WindowID(n)]
	return w, ok
}

// windowAppName returns the name to show for an unmanaged window's app, as
// the palette shows it.
func (e *Engine) windowAppName(w platform.Window) string {
	name := w.App
	if name == "" {
		name = e.appName(w.BundleID)
	}
	return apps.DisplayName(name)
}

// HidePreview hides the live preview.
func (e *Engine) HidePreview() error {
	e.p.HidePreview()
	return nil
}

// Shortcut runs a global shortcut's action (see the shortcuts package's
// Action constants) with its argument. A refusal is shown as a toast.
//
// While Bench is out of sight, a tab shortcut switches tabs and, with
// show_on_tab_shortcut on, brings Bench forward on that tab, as choosing
// the tab from the menu bar item does, so that a refusal's toast shows too
// (showOnTabShortcutLocked). last_tab's shortcut that starts a hold
// switches only once the user lets go: the hold brings Bench forward then
// (endHoldLocked), or as it shows the tabs, so that Bench doesn't come back
// on the tab being left first. The other shortcuts wait for Bench to come
// back before they move or focus windows (decision 16).
func (e *Engine) Shortcut(action, arg string) {
	// What a tab's shortcut arranges is timed from now (see timing.go).
	start := e.clk.Now()
	// During a hold after last_tab's shortcut, another shortcut lands the
	// quick tap's switch first, and while the hold shows the tabs, the
	// shortcuts that its modifiers make with other keys work on it: the
	// switches they make are a tab shortcut's.
	e.lockAt(start, reasonTabShortcut)
	held := e.holdShortcutLocked(action, arg)
	e.unlock()
	if held {
		return
	}
	var err error
	switch action {
	case shortcuts.ActionTab, shortcuts.ActionNextTab, shortcuts.ActionPreviousTab, shortcuts.ActionLastTab:
		e.lock()
		show := e.showOnTabShortcutLocked()
		to := e.shortcutTargetLocked(action, arg)
		e.unlock()
		err = e.switchShowing(to, start, reasonTabShortcut, func() error { return e.tabShortcutLocked(action, arg) })
		e.lock()
		holding := e.hold != nil
		e.unlock()
		if show && !holding {
			e.em.BringForward()
		}
	case shortcuts.ActionJump:
		e.lock()
		// The palette takes the place of the tabs that a hold shows.
		e.endHoldLocked(false)
		e.noteJumpLocked()
		e.unlock()
		e.em.Command(Command{Name: "toggle-palette"})
	case shortcuts.ActionShowAllTabs:
		e.lock()
		e.endHoldLocked(false)
		e.noteJumpLocked()
		e.unlock()
		e.em.Command(Command{Name: toggleAllTabsCommand})
	case shortcuts.ActionRearrange:
		err = e.Rearrange()
	case shortcuts.ActionFocusPane:
		err = e.FocusDirection(arg)
	case shortcuts.ActionMoveWindow:
		err = e.MoveDirection(arg)
	case shortcuts.ActionNextWindow:
		err = e.CycleSubtab(1)
	case shortcuts.ActionPreviousWindow:
		err = e.CycleSubtab(-1)
	case shortcuts.ActionZoomPane:
		err = e.ToggleZoom()
	case shortcuts.ActionGrowPane:
		err = e.GrowPane()
	case shortcuts.ActionShrinkPane:
		err = e.ShrinkPane()
	case shortcuts.ActionEqualPanes:
		err = e.EqualPanes()
	case shortcuts.ActionNextLayout:
		err = e.StepLayout(1)
	case shortcuts.ActionPreviousLayout:
		err = e.StepLayout(-1)
	case shortcuts.ActionAddFrontWindow:
		err = e.AddFrontWindow()
	case shortcuts.ActionUndo:
		// From another app, a hidden Bench comes back to show what Undo
		// put back, as it does for Add Front Window, or, with nothing to
		// undo, to say so: otherwise nothing would show that the
		// shortcut did anything.
		err = e.UndoLatest()
		e.lock()
		hidden := e.benchHidden
		e.unlock()
		if hidden {
			e.em.BringForward()
		}
	case shortcuts.ActionDropdown:
		// It never brings Bench forward: the drop-down shows over the
		// app in front.
		err = e.ToggleDropdown()
	case shortcuts.ActionSendToTab:
		var n int
		if n, err = tabNumber(arg); err == nil {
			err = e.SendToTab(n - 1)
		}
	case shortcuts.ActionSendToLastTab:
		err = e.SendToLastTab()
	}
	if err != nil {
		e.em.Toast(err.Error())
	}
}

// The bench:commands for a tab's shortcut that switchShowing sends around
// the switch: switching, with the tab it's about to switch to, which the
// title bar shows as current at once, before the View that has it current,
// and switch-refused, if the switch is refused, or ends on another tab,
// which puts the title bar back as the View has it (decision 67).
const (
	switchingCommand     = "switching"
	switchRefusedCommand = "switch-refused"
)

// switchShowing switches tabs for a tab's shortcut, or as a hold after
// last_tab's ends (pollHold), with sw, which runs with the lock held, and
// is timed for reason, as a tab shortcut's, or a link's or script's
// (carryOut), from at. For tab to, if it's 0 or more, the
// title bar shows it as current at once: switchShowing sends switching
// first, then switch-refused after sw if sw refused the switch, or the
// switch ended on another tab, as it does when a click on a tab comes in
// between the holds of the lock (decision 67). It returns sw's error.
func (e *Engine) switchShowing(to int, at time.Time, reason string, sw func() error) error {
	if to >= 0 {
		e.em.Command(Command{Name: switchingCommand, Tab: to})
	}
	e.lockAt(at, reason)
	err := sw()
	current := e.model.Current
	e.unlock()
	if to >= 0 && (err != nil || current != to) {
		e.em.Command(Command{Name: switchRefusedCommand})
	}
	return err
}

// tabShortcutLocked switches tabs for a tab shortcut: to tab arg for
// ActionTab, to the next or previous tab, or back to the tab used before
// this one.
func (e *Engine) tabShortcutLocked(action, arg string) error {
	switch action {
	case shortcuts.ActionNextTab, shortcuts.ActionPreviousTab:
		step := 1
		if action == shortcuts.ActionPreviousTab {
			step = -1
		}
		if i := e.stepTargetLocked(step); i >= 0 {
			e.switchToLocked(i)
		}
		return nil
	case shortcuts.ActionLastTab:
		return e.goToLastTabLocked()
	}
	n, err := tabNumber(arg)
	if err != nil {
		return err
	}
	return e.switchTabLocked(n - 1)
}

// shortcutTargetLocked returns the tab that a tab's shortcut is about to
// switch to, for the title bar to show as current at once, or -1: for the
// current tab, which it arranges again, a tab that isn't there, a hold
// after last_tab's shortcut, and last_tab's shortcut when it starts a
// hold, as it does while its keys are held, as they are as it's pressed:
// the hold shows the tab it switches to once they're let go of (pollHold,
// decision 60).
func (e *Engine) shortcutTargetLocked(action, arg string) int {
	if !e.started || e.hold != nil {
		return -1
	}
	to := -1
	switch action {
	case shortcuts.ActionNextTab:
		to = e.stepTargetLocked(1)
	case shortcuts.ActionPreviousTab:
		to = e.stepTargetLocked(-1)
	case shortcuts.ActionTab:
		if n, err := tabNumber(arg); err == nil && e.model.Tab(n-1) != nil {
			to = n - 1
		}
	case shortcuts.ActionLastTab:
		if e.holdModifiersLocked() == 0 {
			to = e.lastTabLocked()
		}
	}
	if to == e.model.Current {
		return -1
	}
	return to
}

// tabNumber reads a tab shortcut's argument, "1" to "9".
func tabNumber(arg string) (int, error) {
	if len(arg) == 1 && arg[0] >= '1' && arg[0] <= '9' {
		return int(arg[0] - '0'), nil
	}
	return 0, errBadSource
}

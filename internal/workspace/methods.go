package workspace

import (
	"strconv"
	"strings"

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
	e.lock()
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
	e.lock()
	defer e.unlock()
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
	e.lock()
	defer e.unlock()
	n := len(e.model.Tabs)
	if n == 0 {
		return nil
	}
	e.switchToLocked(((e.model.Current+step)%n + n) % n)
	return nil
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

// FocusPane focuses a pane of the current tab.
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
	e.model.FocusPane(t, p)
	e.focusShownLocked()
	e.changed()
	return nil
}

// ShowItem shows an item in its pane (or in the "tabs" view's compact
// pane), and focuses the pane.
func (e *Engine) ShowItem(paneID, itemID string) error {
	e.lock()
	defer e.unlock()
	m := e.model
	if m.Items[itemID] == nil {
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
	m.Show(itemID)
	if ti == m.Current {
		e.arrangeLocked(arrangeOpts{})
		e.focusShownLocked()
	}
	e.changed()
	return nil
}

// MoveItem moves an item to the centre of a pane of tab (joining it) or to
// an edge (splitting it). Splits are refused in the "tabs" view.
func (e *Engine) MoveItem(itemID string, tab int, paneID, zone string) error {
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
	moved, err := m.move(itemID, tab, paneID, z, e.stackViewLocked(t))
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
	return nil
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

// moveToTabLocked moves an item into a tab's focused pane.
func (e *Engine) moveToTabLocked(it *Item, tab int) error {
	floating := e.model.IsFloating(it.ID)
	shown := e.focusedShownLocked()
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
	return nil
}

// AddItemToTab shows an item in a tab's focused pane as well as where it
// is, without switching tabs: the same window can be in several tabs, and
// is docked in its pane in whichever is current. As nothing changes in the
// current tab, a toast says so.
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
	added, err := m.AddToTab(itemID, tab, "")
	if err != nil {
		return err
	}
	if !added {
		return errAlreadyIn(t.Name)
	}
	if tab == m.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.toastLocked("Showing " + labelOf(it) + " in " + t.Name + " too.")
	e.changed()
	return nil
}

// RemoveItemFromTab takes an item that's in several tabs out of one of
// them, leaving it in the others.
func (e *Engine) RemoveItemFromTab(itemID string, tab int) error {
	e.lock()
	defer e.unlock()
	m := e.model
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
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
	e.toastLocked("Removed " + labelOf(it) + " from " + m.Tabs[tab].Name + ". It's still in " + joinNames(still) + ".")
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
// if there is one.
func (e *Engine) FocusDirection(dir string) error {
	e.lock()
	defer e.unlock()
	d, err := directionOf(dir)
	if err != nil {
		return err
	}
	disp := e.displayLocked()
	if disp.compact {
		return nil
	}
	next, ok := layout.Neighbour(framesOf(disp), disp.focus, d)
	if !ok {
		return nil
	}
	t := disp.tab
	e.model.FocusPane(t, t.Pane(next))
	e.focusShownLocked()
	e.changed()
	return nil
}

// MoveDirection moves the focused pane's shown item into the neighbouring
// pane in a direction, or splits the pane at that edge if there's no
// neighbour and the pane has another item to leave behind.
func (e *Engine) MoveDirection(dir string) error {
	e.lock()
	defer e.unlock()
	d, err := directionOf(dir)
	if err != nil {
		return err
	}
	disp := e.displayLocked()
	if disp.compact {
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
	ti, err := e.model.SetSizes(splitID, sizes)
	if err != nil {
		return err
	}
	e.sizedAt = e.clk.Now()
	if ti == e.model.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	return nil
}

// Rearrange arranges the current tab again.
func (e *Engine) Rearrange() error {
	e.lock()
	defer e.unlock()
	t := e.model.CurrentTab()
	e.recheckUnavailableLocked(t)
	e.arrangeLocked(arrangeOpts{hideOtherApps: true})
	e.toastLocked("Rearranged " + t.Name)
	return nil
}

// Redock docks a window the user dragged out of its pane again.
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
	it.Undocked = false
	it.Status, it.StatusText = StatusReady, ""
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

// setOverlayLocked turns raised mode on or off.
func (e *Engine) setOverlayLocked(open bool) {
	e.overlay = open
	e.p.SetRaised(open)
	if open {
		e.arrangeWebLocked(e.displayLocked())
		return
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusWebLaterLocked()
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
		target = platform.PreviewTarget{Window: w.ID, Title: labelOf(&Item{App: e.windowAppName(w), Title: w.Title})}
	case it.Kind == KindApp && it.WindowID != 0:
		target = platform.PreviewTarget{Window: it.WindowID, Title: labelOf(it)}
	case it.Kind == KindWeb && e.webMade[it.ID]:
		target = platform.PreviewTarget{WebViewID: it.ID, Title: labelOf(it)}
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
func (e *Engine) Shortcut(action, arg string) {
	var err error
	switch action {
	case shortcuts.ActionTab:
		var n int
		if n, err = tabNumber(arg); err == nil {
			err = e.SwitchTab(n - 1)
		}
	case shortcuts.ActionNextTab:
		err = e.StepTab(1)
	case shortcuts.ActionPreviousTab:
		err = e.StepTab(-1)
	case shortcuts.ActionJump:
		e.em.Command(Command{Name: "toggle-palette"})
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
	}
	if err != nil {
		e.em.Toast(err.Error())
	}
}

// tabNumber reads a tab shortcut's argument, "1" to "9".
func tabNumber(arg string) (int, error) {
	if len(arg) == 1 && arg[0] >= '1' && arg[0] <= '9' {
		return int(arg[0] - '0'), nil
	}
	return 0, errBadSource
}

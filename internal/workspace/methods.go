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
// an edge (splitting it), and says so, with Undo. Splits are refused in the
// "tabs" view.
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
	step := e.beginUndoLocked()
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
	e.undoToastLocked("Moved "+labelOf(it)+".", e.keepUndoLocked(step, "Moved "+labelOf(it)+" back."))
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

// moveToTabLocked moves an item into a tab's focused pane, and says so,
// with Undo.
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
	e.undoToastLocked("Moved "+labelOf(it)+" to "+name+".", e.keepUndoLocked(step, "Moved "+labelOf(it)+" back."))
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
		return err
	}
	if !added {
		return errAlreadyIn(t.Name)
	}
	if tab == m.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	undo := e.keepUndoLocked(step, "Took "+labelOf(it)+" out of "+t.Name+" again.")
	e.undoToastLocked("Showing "+labelOf(it)+" in "+t.Name+" too.", undo)
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
	undo := e.keepUndoLocked(step, "Put "+labelOf(it)+" back in "+name+".")
	e.undoToastLocked("Removed "+labelOf(it)+" from "+name+". It's still in "+joinNames(still)+".", undo)
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

// setOverlayLocked turns raised mode on or off. Lowered after the jump
// shortcut opened the palette, Bench gives the keyboard back to the App
// window that had it, unless something has taken the focus since
// (refocusAfterJumpLocked).
func (e *Engine) setOverlayLocked(open bool) {
	e.overlay = open
	e.p.SetRaised(open)
	if open {
		e.arrangeWebLocked(e.displayLocked())
		// The user may have edited config.toml in one of Bench's own
		// terminals, which leaves Bench active, before opening Settings,
		// say.
		e.checkConfigLocked()
		return
	}
	refocus := e.jump.itemID != "" && e.focusSeq.Load() == e.jump.seq
	if !refocus {
		e.jump = jumpNote{}
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusWebLaterLocked()
	e.pressCloseLaterLocked()
	if refocus {
		if e.jumpTimer != nil {
			e.jumpTimer.Stop()
		}
		e.jumpTimer = e.after(jumpRefocusDelay, func() {
			e.jumpTimer = nil
			e.refocusAfterJumpLocked()
		})
	}
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
// note.
func (e *Engine) noteJumpLocked() {
	if e.overlay {
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
// waits until it's lowered.
func (e *Engine) refocusAfterJumpLocked() {
	if e.overlay {
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
//
// While Bench is out of sight, a tab shortcut switches tabs and, with
// show_on_tab_shortcut on, brings Bench forward on that tab, as choosing
// the tab from the menu bar item does, so that a refusal's toast shows too
// (showOnTabShortcutLocked). The other shortcuts wait for Bench to come back
// before they move or focus windows (decision 16).
func (e *Engine) Shortcut(action, arg string) {
	var err error
	switch action {
	case shortcuts.ActionTab, shortcuts.ActionNextTab, shortcuts.ActionPreviousTab:
		e.lock()
		show := e.showOnTabShortcutLocked()
		e.unlock()
		err = e.tabShortcut(action, arg)
		if show {
			e.em.BringForward()
		}
	case shortcuts.ActionJump:
		e.lock()
		e.noteJumpLocked()
		e.unlock()
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

// tabShortcut switches tabs for a tab shortcut: to tab arg for ActionTab,
// or to the next or previous tab.
func (e *Engine) tabShortcut(action, arg string) error {
	switch action {
	case shortcuts.ActionNextTab:
		return e.StepTab(1)
	case shortcuts.ActionPreviousTab:
		return e.StepTab(-1)
	}
	n, err := tabNumber(arg)
	if err != nil {
		return err
	}
	return e.SwitchTab(n - 1)
}

// tabNumber reads a tab shortcut's argument, "1" to "9".
func tabNumber(arg string) (int, error) {
	if len(arg) == 1 && arg[0] >= '1' && arg[0] <= '9' {
		return int(arg[0] - '0'), nil
	}
	return 0, errBadSource
}

package workspace

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/windows"
)

// display is the current tab as it's drawn: the displayed tree, after the
// small-screen setting, with each pane's frame and body.
type display struct {
	tab     *Tab
	class   layout.SizeClass
	compact bool // The "tabs" view: one pane holding every item.
	root    *layout.Node
	panes   []paneGeom
	focus   string
	area    layout.Rect
	gap     float64
	// zoomed is set while one pane fills the pane area on its own
	// (ZoomPane): root is that pane, and the tab's other panes aren't
	// drawn.
	zoomed bool
}

// paneGeom is a displayed pane with its frame and body, in content
// coordinates (global ones in headless mode), and its header's height: 0 for
// a pane without one.
type paneGeom struct {
	node   *layout.Node
	frame  layout.Rect
	body   layout.Rect
	header float64
}

// sizeClassLocked returns the size class in use: the mode, if it's wide or
// compact, or else the one that Auto last chose for the width of Bench's
// window (see chooseSizeClassLocked).
func (e *Engine) sizeClassLocked() layout.SizeClass {
	switch e.model.Mode {
	case layout.ModeWide:
		return layout.Wide
	case layout.ModeCompact:
		return layout.Compact
	}
	if e.autoClass == "" || e.autoBelow != e.cfg.Settings.CompactBelow {
		return layout.AutoSizeClass(e.contentWidthLocked(), e.cfg.Settings.CompactBelow, "")
	}
	return e.autoClass
}

// chooseSizeClassLocked has Auto choose the size class again for the
// width of Bench's window as it is now, as every arrangement starts: as the
// user resizes the window, tiles it, or fills the screen with it, and as it
// goes to another screen. It keeps the class it chose last until the width
// is clearly past compact_below (layout.AutoSizeClass), so that dragging
// the window's edge to and fro across it doesn't change the layout each
// time. A new compact_below chooses afresh. It chooses while the mode is
// Wide or Compact too, so that Auto has its answer ready. While the
// screens settle after they changed, or after Bench's window went to
// another screen, macOS moves and resizes the window (see screensMoved):
// Bench keeps the class it has, and chooses once they have settled. A
// settle after a wake alone doesn't hold the class, as a move then is the
// user's.
func (e *Engine) chooseSizeClassLocked() {
	if e.screensMoved && e.autoClass != "" {
		return
	}
	below := e.cfg.Settings.CompactBelow
	was := e.autoClass
	if e.autoBelow != below {
		was = ""
	}
	e.autoClass, e.autoBelow = layout.AutoSizeClass(e.contentWidthLocked(), below, was), below
}

// contentWidthLocked returns the width of Bench's window's content area,
// which Auto chooses the size class by: as the platform reports it, or,
// until the window has opened, as WindowStart opens it. Wails' web view,
// whose frame the platform reports, is 1 pt wider than the window, so a
// window that fills the screen is taken to be as wide as the screen's
// visible area, not 1 pt wider: a width up to fillSlack wider is the
// visible area's. A window wider still is taken as it is, as the screen
// can be one that Bench hasn't caught up with yet, such as the primary one
// at launch, before the window has opened on a larger display. Without a
// window of its own, as for bench arrange, Bench lays the panes out in the
// visible area of the screen, so that's its width.
func (e *Engine) contentWidthLocked() float64 {
	if !e.opts.Headless {
		if c := e.p.ContentFrame(); c.W > 0 {
			if v := e.screen.Visible.W; v > 0 && c.W > v && c.W-v <= fillSlack {
				return v
			}
			return c.W
		}
		if e.startWidth > 0 {
			return e.startWidth
		}
	}
	if e.screen.Visible.W > 0 {
		return e.screen.Visible.W
	}
	return e.screen.Frame.W
}

// isTabsView reports whether a tab is shown as one pane on this screen.
func (e *Engine) isTabsView(t *Tab) bool {
	return e.sizeClassLocked() == layout.Compact && t.SmallScreens == layout.Tabs
}

// displayLocked works out the current tab's displayed tree and geometry, as
// Bench arranges it.
func (e *Engine) displayLocked() display {
	return e.displayWithLocked(e.cfg.Settings.Gap, e.densityLocked())
}

// displayWithLocked works out the current tab's displayed tree and
// geometry with a gap between panes, and pane headers for a density in
// use: the settings', or those that Settings previews (shownDisplayLocked).
func (e *Engine) displayWithLocked(gap float64, density string) display {
	t := e.model.CurrentTab()
	d := display{tab: t, class: e.sizeClassLocked(), area: e.area, gap: gap}
	if e.opts.Headless {
		d.area = e.screen.Visible
	}
	if t == nil {
		return d
	}
	focused := t.FocusedPane()
	focusID := ""
	if focused != nil {
		focusID = focused.ID
	}
	// Only the items that aren't hidden are drawn (see hiddenLocked). The
	// "tabs" view's one pane holds the whole tab, so when the item it shows
	// is hidden, it shows the tab's most recently used other item.
	shown := t.CompactShown
	if e.hiddenLocked(e.model.Items[shown]) {
		shown = e.recentOfLocked(e.shownItemsLocked(t.ItemIDs()))
	}
	tree := e.shownTreeLocked(t)
	if z := e.zoomedLocked(); z != nil {
		// A zoomed pane fills the pane area on its own, with its own
		// sub-tabs (see panes.go).
		d.root, _, _ = layout.Find(tree, z.ID)
		d.zoomed = true
	} else {
		d.root = layout.Adapt(tree, t.SmallScreens, d.class, focusID, shown)
	}
	d.compact = d.root.IsPane() && d.root.ID == layout.CompactPaneID
	d.focus = focusID
	if d.compact {
		d.focus = layout.CompactPaneID
	}
	// The "tabs" view's pane has its sub-tabs in the title bar, so it has
	// no header, nor the gap above it. Other panes have a header as tall as
	// the density says, unless the app's own title bar stands in for it
	// (see headers.go), but never a zoomed pane, whose header says so.
	header := headerHeight(density)
	for _, pr := range layout.Frames(d.root, d.area, d.gap) {
		n, _, _ := layout.Find(d.root, pr.ID)
		g := paneGeom{node: n, frame: pr.Rect, header: header}
		switch {
		case d.compact:
			g.frame, g.header = layout.ReachTop(g.frame, d.area), 0
		case !d.zoomed && e.headerHiddenLocked(n):
			g.header = 0
		}
		g.body = layout.Body(g.frame, g.header)
		d.panes = append(d.panes, g)
	}
	return d
}

// focusedGeom returns the displayed focused pane.
func (d display) focusedGeom() (paneGeom, bool) {
	for _, g := range d.panes {
		if g.node.ID == d.focus {
			return g, true
		}
	}
	return paneGeom{}, false
}

// arrangeOpts change what arranging does.
type arrangeOpts struct {
	// hideOtherApps applies the hide_other_apps setting, as switching tabs
	// and rearranging do.
	hideOtherApps bool
	// switching is set when the user has just left another tab: the apps
	// hidden now are Bench's doing, so focus events that hiding them causes
	// aren't followed.
	switching bool
	// hidingBench puts the current tab's windows out of sight, as Bench's
	// window has been hidden.
	hidingBench bool
	// showOtherTabs puts back what hide_other_tabs hid for other tabs, as
	// it has been turned off.
	showOtherTabs bool
}

// arrangeLocked arranges the current tab: it docks, stacks, parks, and
// hides windows through the windows manager, updates statuses, shows and
// hides web views, and starts terminals shown for the first time. While
// Bench's window is hidden, it leaves windows alone (see onBenchHidden).
// Once it finds that Bench has lost Accessibility permission, it leaves
// them alone too (see lostTrustLocked).
func (e *Engine) arrangeLocked(o arrangeOpts) {
	if !e.started || e.closed || e.model.CurrentTab() == nil {
		return
	}
	e.noteArrangeStartLocked()
	applied := false
	e.chooseSizeClassLocked()
	// refocus is set when the focused pane's window turns out to have been
	// minimised by the user: the pane gets the keyboard back once it's
	// arranged (see minimise.go).
	refocus := false
	for pass := 0; pass < 2; pass++ {
		d := e.displayLocked()
		if e.updateHeadersLocked(d) {
			d = e.displayLocked()
		}
		if manage := e.trusted && (!e.benchHidden || o.hidingBench); manage {
			e.loadRunning()
			e.noteWindowToolsLocked(false)
			plan := e.planLocked(d, o)
			if o.switching || o.hidingBench {
				for _, app := range plan.Hide {
					if !app.Hidden {
						e.noteQuietLocked(app.BundleID, quietFocus)
					}
				}
			}
			res := e.win.Apply(plan)
			applied = true
			e.held.windows = max(e.held.windows, res.Touched)
			e.noteApplyErrorsLocked(res)
			changed, minimised := e.applyResultLocked(res, d)
			refocus = refocus || minimised
			if changed {
				continue // A window had gone; arrange the new tree once more.
			}
			// What arranging found, such as a window that needs more
			// room, can change which panes go without their header, at
			// once or after a wait (see headers.go). Out of passes, the
			// header timer docks for the change straight away.
			if e.updateHeadersLocked(e.displayLocked()) {
				if pass == 0 {
					continue
				}
				e.checkHeadersLocked(e.clk.Now())
			}
		}
		e.arrangeWebLocked(d)
		e.startTerminalsLocked(d.tab)
		break
	}
	if applied {
		// The last frame set: the windows', then the web views'.
		e.noteArrangedLocked()
	}
	if refocus {
		e.focusShownLocked()
	}
	e.clearSeenLocked()
	e.touch()
}

// noteApplyErrorsLocked logs what the windows manager couldn't do, and says
// in the status bar when it couldn't record parked windows, which then
// can't be put back after a crash. A failure because Bench doesn't have
// Accessibility permission may mean that macOS has taken it away (see
// lostTrustLocked).
func (e *Engine) noteApplyErrorsLocked(res windows.Result) {
	untrusted := false
	for _, err := range res.Errors {
		e.log.Warn("arranging windows", "error", err)
		untrusted = untrusted || errors.Is(err, platform.ErrNotTrusted)
	}
	var notices []string
	if res.FileErr != nil {
		notices = []string{"Bench couldn't record the windows it moved out of sight, so it might not put them back if it crashes: " + res.FileErr.Error()}
	}
	if len(notices) > 0 || len(e.notices[sourceWindows]) > 0 {
		e.setNoticesLocked(sourceWindows, notices)
		e.touch()
	}
	if untrusted {
		e.lostTrustLocked()
	}
}

// dockable reports whether an App item's window can go in a stack. A
// window that the user minimised stays in the Dock (see minimise.go).
func dockable(it *Item) bool {
	return it != nil && it.Kind == KindApp && it.WindowID != 0 && !it.Undocked && !it.minimised && !it.gone && it.Status != StatusUnavailable
}

// windowOf returns what the platform needs to know about an item's window.
func windowOf(it *Item) platform.Window {
	return platform.Window{ID: it.WindowID, PID: it.pid, BundleID: it.BundleID, App: it.App, Title: it.Title}
}

// planLocked builds the windows manager's plan for the current tab, or, as
// Bench's window is hidden, for no tab.
func (e *Engine) planLocked(d display, o arrangeOpts) windows.Plan {
	plan := windows.Plan{Screen: e.parkScreenLocked()}
	if d.area.W > 0 && d.area.H > 0 && !o.hidingBench {
		var offset layout.Rect
		if !e.opts.Headless {
			offset = e.p.ContentFrame()
		}
		// While a pane is zoomed, the tab's other panes' windows go out
		// of sight, as a pane's do while it shows a page or a terminal.
		plan.Stacks = e.unshownStacksLocked(d)
		// The manager raises each stack's windows in turn, and raising a
		// window of the frontmost app can make it that app's focused
		// window. So the focused pane's stack goes last, to keep the
		// keyboard there.
		var focused []windows.Stack
		for _, g := range d.panes {
			if e.edgeHeldLocked(d, g) {
				// The user is dragging the edge of its window: its windows
				// stay where they are until they let go (see edges.go).
				continue
			}
			s := e.stackLocked(g, offset)
			if g.node.ID == d.focus {
				focused = append(focused, s)
				continue
			}
			plan.Stacks = append(plan.Stacks, s)
		}
		plan.Stacks = append(plan.Stacks, focused...)
	}
	e.planOthersLocked(&plan, o)
	return plan
}

// parkScreenLocked returns the screen Bench is on, with its frame as it is
// now. e.screen changes only once the screens have settled, but a window
// parked against a frame that has since grown could show.
func (e *Engine) parkScreenLocked() platform.Screen {
	for _, s := range e.p.Screens() {
		if s.ID == e.screen.ID {
			return s
		}
	}
	return e.screen
}

// stackLocked builds a displayed pane's stack. Its global body is the pane
// body offset by the content frame's origin.
func (e *Engine) stackLocked(g paneGeom, offset layout.Rect) windows.Stack {
	body := g.body
	body.X += offset.X
	body.Y += offset.Y
	s := windows.Stack{PaneID: g.node.ID, Body: body}
	for _, id := range g.node.Items {
		if it := e.model.Items[id]; dockable(it) {
			s.Windows = append(s.Windows, windowOf(it))
			if it.restore {
				s.Restore = append(s.Restore, it.WindowID)
			}
		}
	}
	if active := e.model.Items[g.node.Active]; dockable(active) {
		s.Shown = active.WindowID
	}
	if e.opts.Headless && len(s.Windows) > 0 {
		s.Shown = s.Windows[0].ID
	}
	return s
}

// appPresence is where an app's items with windows are.
type appPresence struct {
	here, floating bool
	// current are the current tab's items as Bench's window is hidden,
	// when they go out of sight too.
	current []*Item
	others  []*Item
}

// planOthersLocked adds the windows and apps of other tabs to park, hide,
// and unhide, and the current tab's undocked windows to put back if they
// were parked. As Bench's window is hidden, the current tab's windows are
// treated as another tab's, whatever hide_other_tabs says. A window in the
// current tab and in other tabs counts only as the current tab's.
func (e *Engine) planOthersLocked(plan *windows.Plan, o arrangeOpts) {
	m := e.model
	byApp := map[string]*appPresence{}
	var order []string
	note := func(it *Item) *appPresence {
		key := strings.ToLower(it.BundleID)
		ap := byApp[key]
		if ap == nil {
			ap = &appPresence{}
			byApp[key] = ap
			order = append(order, key)
		}
		return ap
	}
	here := map[string]bool{}
	for _, id := range m.CurrentTab().ItemIDs() {
		here[id] = true
	}
	for ti, t := range m.Tabs {
		for _, id := range t.ItemIDs() {
			it := m.Items[id]
			if it == nil || it.Kind != KindApp || it.WindowID == 0 {
				continue
			}
			if ti != m.Current && here[id] {
				continue // It's in the current tab too, which shows it.
			}
			ap := note(it)
			switch {
			case ti == m.Current && o.hidingBench:
				ap.current = append(ap.current, it)
			case ti == m.Current:
				ap.here = true
				if it.Undocked {
					plan.Unpark = append(plan.Unpark, windowOf(it))
				}
			default:
				ap.others = append(ap.others, it)
			}
		}
	}
	if !o.hidingBench {
		// An app that Bench is opening for one of this tab's items without a
		// window counts as here too. Opening a running app activates it,
		// which would show it again, with its other tabs' windows, if it
		// were hidden now; so it stays shown, and those windows are parked.
		for _, id := range m.CurrentTab().ItemIDs() {
			if it := m.Items[id]; it != nil && it.Kind == KindApp && it.WindowID == 0 && e.launchingNow(it.BundleID) {
				note(it).here = true
			}
		}
	}
	for _, id := range m.Floating {
		if it := m.Items[id]; it != nil && it.Kind == KindApp && it.WindowID != 0 {
			note(it).floating = true
		}
	}
	hideTabs := e.cfg.Settings.HideOtherTabs
	park := func(items []*Item) {
		for _, it := range items {
			if it.Status != StatusUnavailable {
				plan.Park = append(plan.Park, windowOf(it))
			}
		}
	}
	// The browser that a dialog waits for a web app from isn't hidden while
	// the user makes the web app in it (see makingBrowserLocked), and its
	// windows in other tabs stay where they are: the browser may have opened
	// the site in one. As Bench's window hides, this tab's still go with it.
	making := strings.ToLower(e.makingBrowserLocked())
	for _, key := range order {
		ap := byApp[key]
		app, running := e.running[key]
		switch {
		case ap.here || ap.floating:
			if running {
				plan.Unhide = append(plan.Unhide, app)
			}
			park(ap.current)
			if hideTabs {
				park(ap.others)
			}
		case making != "" && key == making:
			park(ap.current)
		case (hideTabs || len(ap.current) > 0) && running && app.PID != e.ownPID:
			plan.Hide = append(plan.Hide, app)
		}
		if !hideTabs && o.showOtherTabs {
			// hide_other_tabs has just been turned off: put back what it
			// hid.
			for _, it := range ap.others {
				plan.Unpark = append(plan.Unpark, windowOf(it))
			}
			if running && len(ap.current) == 0 && !ap.here && !ap.floating && e.win.Hid(app) {
				plan.Unhide = append(plan.Unhide, app)
			}
		}
	}
	// An app that Undo has given windows back to, out of every tab, shows
	// again if Bench hid it and it has no items with windows left (see
	// putBackOutsideLocked).
	for _, key := range slices.Sorted(maps.Keys(e.putBack)) {
		if app, running := e.running[key]; running && byApp[key] == nil && e.win.Hid(app) {
			plan.Unhide = append(plan.Unhide, app)
		}
	}
	clear(e.putBack)
	if o.hideOtherApps && e.cfg.Settings.HideOtherApps {
		plan.Hide = append(plan.Hide, e.appsWithoutItemsLocked()...)
	}
}

// appsWithoutItemsLocked returns the running regular apps that have no
// items, other than Bench and the browser that a dialog waits for a web app
// from (see makingBrowserLocked).
func (e *Engine) appsWithoutItemsLocked() []platform.App {
	has := map[string]bool{}
	for _, it := range e.model.Items {
		if it.Kind == KindApp {
			has[strings.ToLower(it.BundleID)] = true
		}
	}
	if browser := e.makingBrowserLocked(); browser != "" {
		has[strings.ToLower(browser)] = true
	}
	var out []platform.App
	for _, app := range e.p.RunningApps() {
		if app.Regular && app.PID != e.ownPID && !app.Hidden && !has[strings.ToLower(app.BundleID)] {
			out = append(out, app)
		}
	}
	return out
}

// applyResultLocked updates items from what the manager did. A window that
// has gone while its app runs has closed, or its app is quitting: its item
// is left out of the panes' stacks and checked again shortly (see
// checkGoneLocked). If its app has quit, the item waits for it. A window
// that the user has minimised, which the manager left in the Dock, is
// marked minimised, and left out of the stacks too. It reports whether the
// panes' stacks changed, and whether such a window was what the focused
// pane showed.
func (e *Engine) applyResultLocked(res windows.Result, d display) (treeChanged, refocus bool) {
	m := e.model
	for _, wid := range slices.Sorted(maps.Keys(res.Status)) {
		status := res.Status[wid]
		it := m.ItemByWindow(wid)
		if it == nil {
			continue
		}
		// The manager has had it in a stack: a request to restore it is
		// met, or can't be.
		it.restore = false
		switch status {
		case windows.StatusMinimised:
			// The user minimised it before the engine heard (see
			// minimise.go).
			if !it.minimised && e.minimisableLocked(it) {
				refocus = e.userMinimisedLocked(it) || refocus
				treeChanged = true
				e.changed()
			}
		case windows.StatusNeedsRoom:
			it.Status, it.StatusText = StatusNeedsRoom, needsRoomText(it.App, d.class == layout.Compact)
		case windows.StatusFullScreen:
			it.Status, it.StatusText = StatusFullScreen, fullScreenText
		case windows.StatusUnavailable:
			it.Status, it.StatusText = StatusUnavailable, unavailableText
		case windows.StatusMissing:
			e.win.Forget(wid)
			if _, running := e.running[strings.ToLower(it.BundleID)]; running {
				// It closed, or its app is quitting and closing its
				// windows as it goes, in which case the item should wait
				// for the app to open again. Check again in a moment.
				if !it.gone {
					it.gone = true
					e.after(goneGrace, func() { e.checkGoneLocked(wid) })
				}
				treeChanged = true
			} else {
				m.Unbind(it)
			}
			e.changed()
		default:
			it.Status, it.StatusText = StatusReady, ""
		}
	}
	return treeChanged, refocus
}

// checkGoneLocked looks again for a window that Bench couldn't find while
// its app was running: if it's still gone and the app still runs, the
// window closed, so its item is removed. If the app has quit, AppTerminated
// has kept the item, waiting for the app to open again, unless Bench was
// closing the window, which has closed.
func (e *Engine) checkGoneLocked(wid platform.WindowID) {
	it := e.model.ItemByWindow(wid)
	if it == nil || !it.gone {
		return
	}
	ti, _ := e.model.Where(it.ID)
	_, err := e.p.Window(wid)
	e.loadRunning()
	_, running := e.running[strings.ToLower(it.BundleID)]
	switch {
	case errors.Is(err, platform.ErrNoWindow) && (running || it.closing):
		e.removeItemLocked(it)
	case errors.Is(err, platform.ErrNoWindow):
		e.model.Unbind(it)
	default:
		it.gone = false // It's back, or it can't be told yet.
	}
	if ti == e.model.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
}

// removeItemLocked forgets an item, closing up its pane, and ends what Bench
// runs for it.
func (e *Engine) removeItemLocked(it *Item) {
	e.model.Remove(it.ID)
	e.endItemLocked(it)
}

// endItemLocked ends what Bench runs for an item that the model has
// forgotten: a Web item's view, or a terminal's session, and the window of
// its own, if it has one, the drop-down's too.
func (e *Engine) endItemLocked(it *Item) {
	switch it.Kind {
	case KindWeb:
		e.closeWebViewLocked(it)
	case KindBuiltin:
		e.leaveDropdownLocked(it)
		if it.OwnWindow {
			e.closeTerminalWindowLocked(it.ID)
		}
		// The host keeps a session, and its output, until it's closed,
		// even once its process has ended.
		e.terms.Close(it.ID)
		delete(e.termOn, it.ID)
		e.forgetOutputLocked(it.ID)
	}
	e.changed()
}

// closeWebViewLocked closes a Web item's view, if it has one, which
// forgets its page's state and icon, and that its title is the page's
// own now (titled).
func (e *Engine) closeWebViewLocked(it *Item) {
	if e.webMade[it.ID] {
		e.p.CloseWebView(it.ID)
	}
	delete(e.webMade, it.ID)
	delete(e.webShown, it.ID)
	e.forgetPreloadLocked(it.ID)
	it.web, it.finding, it.pageIcon, it.titled = platform.WebState{}, false, 0, false
}

// arrangeWebLocked shows the web view of each displayed pane's active Web
// item at its body, below its find bar if that's open, and hides every
// other web view Bench shows. While Bench is raised, it shows none. Then
// it tells the platform which web views keep running while they're
// hidden, and schedules the next page kept running in the background to
// load, if one is waiting (see keeprunning.go).
func (e *Engine) arrangeWebLocked(d display) {
	if e.opts.Headless {
		return
	}
	want := map[string]layout.Rect{}
	if !e.overlay && !e.benchHidden {
		for _, g := range d.panes {
			it := e.model.Items[g.node.Active]
			if it != nil && it.Kind == KindWeb && !it.OwnWindow {
				want[it.ID] = webFrame(it, g.body)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(e.webShown)) {
		if _, ok := want[id]; !ok {
			e.p.HideWebView(id)
			delete(e.webShown, id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(want)) {
		body := want[id]
		if shown, ok := e.webShown[id]; ok && shown == body {
			continue
		}
		e.showWebLocked(e.model.Items[id], body)
	}
	e.keepWebViewsLocked()
	e.preloadSoonLocked()
}

// showWebLocked shows a Web item's view at a frame, creating it on first
// use. A VS Code for the Web item waits for the server.
func (e *Engine) showWebLocked(it *Item, frame layout.Rect) {
	address, ok := e.webAddressLocked(it)
	if !ok {
		return
	}
	view := platform.WebView{URL: address, Name: it.App, Profile: it.Profile, Zoom: it.Zoom}
	if err := e.p.ShowWebView(it.ID, view, frame); err != nil {
		it.Status, it.StatusText = StatusMissing, "Bench couldn't show this page"
		return
	}
	if it.Status == StatusMissing && it.VSCode == "" {
		it.Status, it.StatusText = StatusReady, ""
	}
	e.webMade[it.ID] = true
	e.webShown[it.ID] = frame
}

// webAddressLocked returns the address a Web item's view loads: the one it
// was given, not the last page it showed. It starts the VS Code server for a
// VS Code item if it isn't running.
func (e *Engine) webAddressLocked(it *Item) (string, bool) {
	if it.VSCode == "" {
		address := it.Home
		if address == "" {
			address = it.URL
		}
		return address, address != ""
	}
	if e.vs.server == nil {
		e.ensureVSCodeLocked()
		return "", false
	}
	return e.vs.server.URL(it.VSCode), true
}

// startTerminalsLocked starts the sessions of a tab's terminals that aren't
// running yet.
func (e *Engine) startTerminalsLocked(t *Tab) {
	if e.opts.Headless || t == nil {
		return
	}
	for _, id := range t.ItemIDs() {
		if it := e.model.Items[id]; it != nil && it.Kind == KindBuiltin {
			e.startTerminalLocked(it)
		}
	}
}

// startTerminalLocked starts a terminal's session the first time, with the
// output Bench kept from when it last quit, if state.json brought it back,
// or again, after its command ended. What an agent in the last session
// reported goes: the new session runs a new program.
func (e *Engine) startTerminalLocked(it *Item) {
	if e.termOn[it.ID] || e.opts.Headless {
		return
	}
	e.termOn[it.ID] = true
	e.clearAgentLocked(it)
	opts := termOptions(it)
	opts.Preface = e.savedOutputLocked(it)
	if !plainShell(it) {
		// It starts in its own folder again.
		it.LastFolder = ""
	}
	err := e.terms.Start(it.ID, opts)
	it.runNow = false
	if err != nil {
		it.Status, it.StatusText = StatusMissing, "The terminal couldn't start: "+err.Error()
		return
	}
	it.Status, it.StatusText = StatusReady, ""
	if it.Name != "" {
		// The config's name is the label until the shell sets a title,
		// which a new session hasn't yet.
		it.Title = it.Name
	} else if title := e.terms.Title(it.ID); title != "" {
		it.Title = title
	}
}

// focusShownLocked gives the keyboard focus to what the focused pane shows:
// an App item's window (activating its app), a Web item's view, or a
// terminal (activating Bench). It reports whether it did, or asked the
// frontend to. Any focus put off before no longer applies. While Bench's
// window is hidden, the focus waits until it's shown again: arranging leaves
// windows alone meanwhile, so the window to focus could still be parked, or
// where another tab's pane is.
func (e *Engine) focusShownLocked() bool {
	e.newFocusLocked()
	if e.benchHidden {
		e.focusOnShow = true
		return false
	}
	return e.focusNowLocked()
}

// newFocusLocked records that the keyboard focus is moving by a new route,
// such as the user choosing a pane or clicking a window: focus that was put
// off until a tab switch's apps opened, until Bench was lowered, or until it
// was shown no longer applies, and nor does a focus-terminal command still
// waiting to be sent.
func (e *Engine) newFocusLocked() {
	if e.wait != nil {
		e.wait.focus = false
	}
	e.webFocus = ""
	e.focusOnShow = false
	e.focusSeq.Add(1)
}

// focusNowLocked gives the keyboard focus to what the focused pane shows (see
// focusShownLocked), and reports whether it did.
func (e *Engine) focusNowLocked() bool {
	seq := e.focusSeq.Add(1)
	d := e.displayLocked()
	g, ok := d.focusedGeom()
	if !ok {
		return false
	}
	it := e.model.Items[g.node.Active]
	if it == nil {
		return false
	}
	if it.minimised {
		// Its window is in the Dock, and the pane says so: Bench takes
		// the keyboard, from whichever window had it, so that Show again
		// can be reached.
		if e.opts.Headless {
			return false
		}
		e.p.FocusUI()
		return true
	}
	switch it.Kind {
	case KindWeb:
		return e.focusWebLocked(it)
	case KindBuiltin:
		if e.opts.Headless || it.OwnWindow {
			return false
		}
		// The terminal is drawn in the UI's web view, which takes the
		// keyboard now, under the lock, as Web and App items do: done later,
		// it could undo a focus made after this one. The frontend then
		// focuses the terminal itself.
		e.p.FocusUI()
		id := it.ID
		e.later(func() { e.sendFocusTerminal(id, seq) })
		return true
	}
	if !e.trusted {
		return false
	}
	if !dockable(it) || (it.Status != StatusReady && it.Status != StatusNeedsRoom && it.Status != "") {
		return false
	}
	if err := e.p.Focus(windowOf(it)); err != nil {
		return false
	}
	now := e.clk.Now()
	for id, at := range e.ownFocus {
		if now.Sub(at) > ownFocusWindow {
			delete(e.ownFocus, id)
		}
	}
	e.ownFocus[it.WindowID] = now
	return true
}

// sendFocusTerminal sends the focus-terminal command that focus number seq
// queued, unless the focus has moved again since: the lock is released by
// then, so a newer focus can come first, and the frontend would otherwise
// give the older terminal the keystrokes.
func (e *Engine) sendFocusTerminal(itemID string, seq uint64) {
	e.focusMu.Lock()
	defer e.focusMu.Unlock()
	if e.focusSeq.Load() != seq {
		return
	}
	e.em.Command(Command{Name: "focus-terminal", ItemID: itemID})
}

// focusWebLocked gives a shown Web item's view the keyboard focus. While
// Bench is raised its views are hidden, so the focus waits until it's
// lowered.
func (e *Engine) focusWebLocked(it *Item) bool {
	if e.opts.Headless || it.OwnWindow {
		return false
	}
	if e.overlay {
		e.webFocus = it.ID
		return true
	}
	if _, shown := e.webShown[it.ID]; !shown {
		return false
	}
	e.webFocus = ""
	return e.p.FocusWebView(it.ID) == nil
}

// focusWebLaterLocked gives the keyboard focus to the Web item that was to
// get it while Bench was raised, if its pane is still focused and shows it,
// and Bench is still the active app. Bench is often lowered because another
// app has taken the keyboard, which closes an open menu, and focusing the
// view would bring Bench back in front of that app.
func (e *Engine) focusWebLaterLocked() {
	id := e.webFocus
	e.webFocus = ""
	if id == "" || e.otherAppActiveLocked() {
		return
	}
	g, ok := e.displayLocked().focusedGeom()
	if !ok || g.node.Active != id {
		return
	}
	if it := e.model.Items[id]; it != nil && it.Kind == KindWeb {
		e.focusWebLocked(it)
	}
}

// otherAppActiveLocked reports whether an app other than Bench is frontmost,
// other than one that Bench has just opened or hidden. AppActivated may not
// have said so yet (see anotherAppActivatedLocked).
func (e *Engine) otherAppActiveLocked() bool {
	for _, app := range e.p.RunningApps() {
		if app.Active && app.PID != e.ownPID && !e.quietLocked(app.BundleID) {
			return true
		}
	}
	return false
}

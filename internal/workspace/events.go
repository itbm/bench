package workspace

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// handle reacts to one platform event.
func (e *Engine) handle(ev platform.Event) {
	e.lock()
	defer e.unlock()
	if e.closed || !e.started {
		return
	}
	switch ev.Kind {
	case platform.WindowCreated:
		e.onWindowCreated(ev)
	case platform.WindowDestroyed:
		e.onWindowDestroyed(ev)
	case platform.WindowMoved, platform.WindowResized:
		e.onWindowMoved(ev)
	case platform.WindowFocused:
		e.onWindowFocused(ev)
	case platform.WindowRetitled:
		e.onWindowRetitled(ev)
	case platform.AppLaunched:
		e.onAppLaunched(ev)
	case platform.AppTerminated:
		e.onAppTerminated(ev)
	case platform.AppActivated:
		e.onAppActivated(ev)
	case platform.LoggingOut:
		e.onLoggingOut()
	case platform.ScreensChanged:
		e.scheduleScreenSettleLocked()
	case platform.BenchMoved:
		e.onBenchMoved()
	case platform.BenchHidden:
		e.onBenchHidden()
	case platform.BenchShown:
		e.onBenchShown()
	default:
		e.handleWebEvent(ev)
	}
}

// handleWebEvent reacts to web view activity.
func (e *Engine) handleWebEvent(ev platform.Event) {
	it := e.model.Items[ev.ItemID]
	if it == nil || it.Kind != KindWeb {
		return
	}
	switch ev.Kind {
	case platform.WebTitle:
		if ev.Title != "" {
			it.Title = ev.Title
			e.retitled()
		}
	case platform.WebURL:
		// A VS Code for the Web item's address holds the server's
		// connection token, which gives access to the Mac's files and
		// terminals, so it's never kept, nor saved in state.json. The item
		// loads the running server's address anyway. Its page is on the Mac
		// itself, so it has no host to show either.
		if it.VSCode != "" {
			return
		}
		page := ev.Page
		if it.page == "" && page.Host == "" {
			// Before any page has been in view, one with no web address is
			// the item's first load, such as the about:blank that the view
			// shows for an address WebKit can't load: there's no page to warn
			// about yet, so the item shows no host, as for one still loading.
			page = platform.Page{}
		}
		if ev.Body != "" {
			it.URL, it.page = ev.Body, ev.Body
		}
		// A page with no web address, such as a data: page, has no host, but
		// is on another site, and may not be secure (see platform.Page), so
		// that it can't pass for the item's own page still to load.
		it.host, it.site = page.Host, page.Site
		it.offSite = page.OffSite
		it.notSecure = (page.Host != "" || page.OffSite) && !page.Secure
		e.retitled()
	case platform.WebNotification:
		_ = e.p.Notify(platform.Notification{ItemID: it.ID, Title: ev.Title, Body: ev.Body})
	case platform.NotificationClicked:
		e.showAnywhereLocked(it)
	case platform.WebViewClosed:
		if e.model.IsFloating(it.ID) {
			e.removeItemLocked(it)
		}
	case platform.WebFocused:
		// The user clicked in the page, which has the keyboard focus now:
		// its pane is focused.
		m := e.model
		if ti, p := m.Where(it.ID); ti == m.Current && p != nil {
			e.newFocusLocked()
			if t := m.CurrentTab(); t.Focus != p.ID || p.Active != it.ID {
				m.Show(it.ID)
				e.changed()
			}
		}
	}
}

// noteAppLocked records a running app from an event.
func (e *Engine) noteAppLocked(app platform.App) {
	if app.BundleID != "" && app.PID != 0 {
		e.running[strings.ToLower(app.BundleID)] = app
	}
}

// onWindowCreated places a new standard window.
func (e *Engine) onWindowCreated(ev platform.Event) {
	if !e.trusted {
		return
	}
	e.noteAppLocked(ev.App)
	w := withApp(ev.Window, ev.App)
	if !e.acceptable(w, ev.App) || e.known(w.ID) {
		return
	}
	if e.adoptLocked([]platform.Window{w}, adoptRuntime) {
		e.arrangeLocked(arrangeOpts{})
	}
}

// onAppLaunched adopts every window an app opened as it launched, together,
// so that matching by title and creation order sees them all.
func (e *Engine) onAppLaunched(ev platform.Event) {
	e.noteAppLocked(ev.App)
	key := strings.ToLower(ev.App.BundleID)
	delete(e.openFail, key) // It's open now, whoever opened it.
	delete(e.notFound, key)
	e.appOpenedLocked(key)
	if until, ok := e.quiet[key]; ok && until.After(e.clk.Now().Add(quietFocus)) {
		// Bench opened it: it's activated as it finishes launching.
		e.quiet[key] = e.clk.Now().Add(quietFocus)
	}
	if !e.trusted || !ev.App.Regular || ev.App.PID == e.ownPID {
		return
	}
	e.startReopeningLocked(ev.App.BundleID, reopenAfterLaunch)
	ws := e.newWindowsOfLocked(ev.App)
	sortByCreated(ws)
	if e.adoptLocked(ws, adoptRuntime) {
		e.arrangeLocked(arrangeOpts{})
	}
	if _, ok := e.pendingLaunchLocked(ev.App.BundleID); ok {
		bundleID := ev.App.BundleID
		e.after(launchSettle, func() { e.checkLaunchedLocked(bundleID) })
	}
	e.touch()
}

// onWindowDestroyed removes a closed window's item. The app is still
// running (an app that quits sends AppTerminated instead).
func (e *Engine) onWindowDestroyed(ev platform.Event) {
	id := ev.Window.ID
	delete(e.redocks, id)
	if it := e.model.ItemByWindow(id); it != nil {
		e.win.Forget(id)
		e.removeItemLocked(it)
		e.arrangeLocked(arrangeOpts{})
		return
	}
	if _, ok := e.unmanaged[id]; ok {
		delete(e.unmanaged, id)
		e.touch()
	}
}

// onAppTerminated keeps a quitting app's items, without windows, so that its
// windows match back into place when it opens again. Popped-out items and
// unmanaged windows of the app are forgotten. Unless the user is logging
// out, the user quit the app, so tab switches leave it closed until it
// opens again. If another copy of the app is running, as when an updater
// starts the new version before the old one has quit, only the windows of
// the copy that quit go.
func (e *Engine) onAppTerminated(ev platform.Event) {
	app := ev.App
	if app.BundleID == "" {
		return
	}
	key := strings.ToLower(app.BundleID)
	if cur, ok := e.running[key]; !ok || app.PID == 0 || cur.PID == 0 || cur.PID == app.PID {
		delete(e.running, key)
		delete(e.reopening, key)
		e.appQuitLocked(key)
	}
	m := e.model
	for _, it := range m.orderedItems() {
		if !isAppOf(it, app.BundleID) || it.WindowID == 0 {
			continue
		}
		if app.PID != 0 && it.pid != 0 && it.pid != app.PID {
			continue
		}
		e.win.Forget(it.WindowID)
		delete(e.redocks, it.WindowID)
		if m.IsFloating(it.ID) {
			m.Remove(it.ID)
			continue
		}
		m.Unbind(it)
	}
	for id, w := range e.unmanaged {
		if sameApp(w.BundleID, app.BundleID) {
			delete(e.unmanaged, id)
		}
	}
	e.changed()
	e.arrangeLocked(arrangeOpts{})
}

// onWindowMoved notices windows of the current tab that the user moved out
// of their panes, that went to another desktop or came back, or that went in
// or out of full screen. Only a move made with the mouse held is the user
// dragging a window out; an app moving or resizing its own docked window
// (Electron apps often do, just after Bench resizes them) is put back.
func (e *Engine) onWindowMoved(ev platform.Event) {
	id := ev.Window.ID
	it := e.model.ItemByWindow(id)
	if it == nil {
		if w, ok := e.unmanaged[id]; ok {
			w.Frame = ev.Window.Frame
			e.unmanaged[id] = w
		}
		return
	}
	if ti, _ := e.model.Where(it.ID); ti != e.model.Current {
		return
	}
	cur, err := e.p.Window(id)
	switch {
	case errors.Is(err, platform.ErrOtherDesktop):
		if it.Status != StatusUnavailable {
			it.Status, it.StatusText = StatusUnavailable, unavailableText
			e.arrangeLocked(arrangeOpts{})
		}
		return
	case err != nil:
		return
	case it.Status == StatusUnavailable:
		it.Status, it.StatusText = StatusReady, ""
		e.arrangeLocked(arrangeOpts{})
		return
	case cur.FullScreen != (it.Status == StatusFullScreen):
		if !cur.FullScreen {
			it.Status, it.StatusText = StatusReady, ""
		}
		e.arrangeLocked(arrangeOpts{})
		return
	}
	if it.Undocked || cur.FullScreen || e.win.OwnChange(ev) {
		return
	}
	if _, docked := e.win.Docked(id); !docked {
		return
	}
	// Just after the user dragged one of Bench's dividers, the mouse that's
	// held down is Bench's, not on the window.
	if !ev.MouseDown || e.clk.Now().Sub(e.sizedAt) < dividerGrace {
		e.log.Debug("an app moved its docked window; putting it back", "window", id, "item", it.ID, "frame", ev.Window.Frame)
		e.redockSoonLocked(id)
		return
	}
	e.log.Debug("the user dragged a window out of its pane", "window", id, "item", it.ID, "frame", ev.Window.Frame)
	it.Undocked = true
	it.Status, it.StatusText = StatusUndocked, undockedText
	e.win.Forget(id)
	e.arrangeLocked(arrangeOpts{})
	e.changed()
}

// redockSoonLocked puts back, shortly, a docked window that its app moved or
// resized by itself. An app that keeps doing it wins after redockLimit
// tries, so that Bench and the app don't fight.
func (e *Engine) redockSoonLocked(id platform.WindowID) {
	if e.redockTimer != nil {
		// A move and a resize usually come together: answer them once.
		return
	}
	now := e.clk.Now()
	recent := e.redocks[id][:0]
	for _, at := range e.redocks[id] {
		if now.Sub(at) < redockMemory {
			recent = append(recent, at)
		}
	}
	if len(recent) >= redockLimit {
		e.redocks[id] = recent
		e.log.Debug("an app keeps moving its window; leaving it", "window", id)
		return
	}
	e.redocks[id] = append(recent, now)
	e.redockTimer = e.after(redockDelay, func() {
		e.redockTimer = nil
		e.arrangeLocked(arrangeOpts{})
	})
}

// onWindowFocused follows focus: a window of the current tab is shown in its
// pane, which is focused; one of another tab is handled as follow_focus
// says. Focus events that Bench caused itself are ignored: for a window it
// focused or raised just before, and, for windows of other tabs, while an
// app that Bench has just hidden or opened is activated (macOS activates
// the next app when one is hidden) or while the app is hidden.
func (e *Engine) onWindowFocused(ev platform.Event) {
	id := ev.Window.ID
	now := e.clk.Now()
	for wid, at := range e.ownFocus {
		if now.Sub(at) > ownFocusWindow {
			delete(e.ownFocus, wid)
		}
	}
	if _, ok := e.ownFocus[id]; ok || e.win.OwnFocus(ev) {
		return
	}
	if it := e.model.ItemByWindow(id); it != nil {
		e.followFocusLocked(it, ev, false)
		return
	}
	// macOS can report a new window's focus before its creation, as when an
	// app is launched with it. It's placed as a new window, but without
	// arranging first, which would hide it at once if it went to another
	// tab: its focus is followed as any window's.
	if !e.trusted {
		return
	}
	e.noteAppLocked(ev.App)
	w := withApp(ev.Window, ev.App)
	if !e.acceptable(w, ev.App) || !e.adoptLocked([]platform.Window{w}, adoptRuntime) {
		return
	}
	if it := e.model.ItemByWindow(id); it == nil || !e.followFocusLocked(it, ev, true) {
		e.arrangeLocked(arrangeOpts{})
	}
}

// followFocusLocked follows the focus of an item's window (see
// onWindowFocused). fresh is set for a window that has only just become the
// item's, which needs arranging whatever happens. It reports whether it
// arranged the current tab.
func (e *Engine) followFocusLocked(it *Item, ev platform.Event, fresh bool) bool {
	m := e.model
	if m.IsFloating(it.ID) {
		return false
	}
	if ti, _ := m.Where(it.ID); ti != m.Current {
		return e.followOtherTabLocked(it, ti, ev)
	}
	// Showing the item can change what's displayed even when it's already
	// its pane's active item: the "tabs" view's one pane shows whichever
	// item was shown last.
	before := e.shownLocked()
	m.Show(it.ID)
	e.newFocusLocked()
	arrange := fresh || !slices.Equal(before, e.shownLocked())
	if arrange {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	return arrange
}

// followOtherTabLocked follows the focus of a window of tab ti, which isn't
// the current tab, as follow_focus says. It reports whether it arranged the
// current tab.
func (e *Engine) followOtherTabLocked(it *Item, ti int, ev platform.Event) bool {
	m := e.model
	if ev.App.Hidden || e.quietLocked(it.BundleID) {
		e.log.Debug("ignoring focus that Bench caused", "window", it.WindowID, "item", it.ID)
		return false
	}
	e.newFocusLocked()
	if e.benchHidden {
		return false // The user is working outside Bench.
	}
	switch e.cfg.Settings.FollowFocus {
	case config.FollowSwitchTab:
		m.Show(it.ID)
		e.switchLocked(ti, switchOpts{})
		return true
	case config.FollowBringHere:
		if moved, _ := m.MoveToTab(it.ID, m.Current); moved {
			m.Show(it.ID)
			e.arrangeLocked(arrangeOpts{})
			e.changed()
			return true
		}
	}
	return false
}

// shownPane is a displayed pane: where it is, and the item it shows.
type shownPane struct {
	id, active string
	frame      layout.Rect
}

// shownLocked returns what the current tab displays, for telling whether a
// change needs arranging.
func (e *Engine) shownLocked() []shownPane {
	d := e.displayLocked()
	out := make([]shownPane, 0, len(d.panes))
	for _, g := range d.panes {
		out = append(out, shownPane{id: g.node.ID, active: g.node.Active, frame: g.frame})
	}
	return out
}

// onWindowRetitled updates the title of a window's item, or of an unmanaged
// window. A window that a tab switch is waiting for may only now have the
// title its item's filter asks for: it moves to that item.
func (e *Engine) onWindowRetitled(ev platform.Event) {
	id := ev.Window.ID
	if e.adoptRetitledLocked(id, ev.Window.Title) {
		return
	}
	if it := e.model.ItemByWindow(id); it != nil {
		if it.Title != ev.Window.Title {
			it.Title = ev.Window.Title
			e.retitled()
		}
		return
	}
	if w, ok := e.unmanaged[id]; ok {
		w.Title = ev.Window.Title
		e.unmanaged[id] = w
		e.touch()
	}
}

// onAppActivated records the active app. When Bench itself becomes active,
// apps whose unmanaged windows overlap Bench are hidden, so they don't sit
// in front of it, unless Bench's window is hidden: the user may have
// clicked another of Bench's windows, such as Acknowledgements, and
// onBenchShown hides them when the window is back. When another app
// becomes active, focus that Bench put off may no longer apply.
func (e *Engine) onAppActivated(ev platform.Event) {
	e.noteAppLocked(ev.App)
	e.benchActive = ev.App.PID == e.ownPID
	if !e.benchActive {
		e.anotherAppActivatedLocked(ev.App)
		return
	}
	if e.opts.Headless {
		return
	}
	if allowed := e.p.ScreenRecordingAllowed(false); allowed != e.screenRec {
		e.screenRec = allowed
		e.touch()
	}
	if e.trusted && !e.benchHidden {
		e.hideOverlappingLocked()
	}
}

// anotherAppActivatedLocked drops the focus that Bench put off, now that an
// app other than Bench has the keyboard, unless Bench brought the app forward
// itself, as it does when it opens or hides apps. Giving a Web item's view the
// keyboard once Bench is lowered would bring Bench back in front of that app,
// and a tab switch's put-off focus would take the keyboard from it: such a
// focus also stays while the app is the one it's for, and for a moment after
// the switch, as macOS activates the next app when Bench hides the
// frontmost one.
func (e *Engine) anotherAppActivatedLocked(app platform.App) {
	if e.quietLocked(app.BundleID) {
		return
	}
	e.webFocus = ""
	w := e.wait
	if w == nil || !w.focus || e.clk.Now().Sub(w.start) < quietFocus {
		return
	}
	if it := e.model.Items[w.focusItem]; it != nil && it.Kind == KindApp && sameApp(it.BundleID, app.BundleID) {
		return
	}
	w.focus = false
}

// hideOverlappingLocked hides apps with an unmanaged standard window over
// Bench's content area, unless the app has windows docked in the current
// tab or popped out, or is the browser that the add dialog waits for a web
// app from (see makingBrowserLocked).
func (e *Engine) hideOverlappingLocked() {
	content := e.p.ContentFrame()
	if content.W <= 0 || content.H <= 0 {
		return
	}
	keep := map[string]bool{}
	if browser := e.makingBrowserLocked(); browser != "" {
		keep[strings.ToLower(browser)] = true
	}
	for _, id := range append(e.model.CurrentTab().ItemIDs(), e.model.Floating...) {
		if it := e.model.Items[id]; it != nil && it.Kind == KindApp && it.WindowID != 0 {
			keep[strings.ToLower(it.BundleID)] = true
		}
	}
	checked := map[string]bool{}
	for _, id := range slices.Sorted(maps.Keys(e.unmanaged)) {
		key := strings.ToLower(e.unmanaged[id].BundleID)
		if keep[key] || checked[key] {
			continue
		}
		checked[key] = true
		app, ok := e.running[key]
		if !ok || app.Hidden || app.PID == e.ownPID {
			continue
		}
		if e.overlapsLocked(app, content) {
			// Through the manager, so that it's shown again on quit.
			if err := e.win.Hide(app); err != nil {
				e.log.Warn("couldn't hide an app over Bench", "app", app.BundleID, "error", err)
			}
		}
	}
}

// overlapsLocked reports whether one of an app's unmanaged standard windows
// overlaps a frame.
func (e *Engine) overlapsLocked(app platform.App, frame layout.Rect) bool {
	ws, err := e.p.Windows(app)
	if err != nil {
		return false
	}
	for _, w := range ws {
		if _, ok := e.unmanaged[w.ID]; ok && w.Standard() && !w.Minimised && platform.Overlaps(w.Frame, frame) {
			return true
		}
	}
	return false
}

// onBenchMoved docks the current tab's windows again where Bench's window
// now is. If it moved to another screen, the size class is worked out again
// once macOS settles.
func (e *Engine) onBenchMoved() {
	s := e.p.ScreenOfWindow()
	if s.ID != e.screen.ID || s.Frame != e.screen.Frame {
		e.scheduleScreenSettleLocked()
	}
	e.arrangeLocked(arrangeOpts{})
}

// onBenchHidden puts the current tab's windows out of sight with Bench's
// window, as if no tab were shown: their apps are hidden, or, for an app
// with windows popped out of Bench, its windows are parked. Web views are
// hidden. Until Bench is shown again, arranging leaves windows alone, so
// that an app the user brings forward meanwhile isn't hidden again.
func (e *Engine) onBenchHidden() {
	if e.benchHidden {
		return
	}
	e.benchHidden = true
	e.arrangeLocked(arrangeOpts{hidingBench: true})
}

// onBenchShown arranges the current tab again when Bench's window is back,
// and gives the focused pane the keyboard focus if it was to get it while
// Bench was hidden. If what the pane shows isn't ready, because a tab switch
// made meanwhile is still opening its app, the switch focuses it once it is.
// If Bench became active while its window was hidden, apps over the window
// are hidden now, as onAppActivated would have.
func (e *Engine) onBenchShown() {
	if !e.benchHidden {
		return
	}
	e.benchHidden = false
	e.arrangeLocked(arrangeOpts{})
	if e.benchActive && e.trusted && !e.opts.Headless {
		e.hideOverlappingLocked()
	}
	if e.focusOnShow {
		e.focusOnShow = false
		if e.focusNowLocked() && e.wait != nil {
			e.wait.focus = false
		}
	}
}

// noteQuietLocked records that Bench hid or opened an app: for d, focus
// events for its windows in other tabs are put down to Bench.
func (e *Engine) noteQuietLocked(bundleID string, d time.Duration) {
	key := strings.ToLower(bundleID)
	if until := e.clk.Now().Add(d); until.After(e.quiet[key]) {
		e.quiet[key] = until
	}
}

// quietLocked reports whether focus events for an app's windows in other
// tabs are put down to Bench, because it has just hidden or opened the app.
func (e *Engine) quietLocked(bundleID string) bool {
	key := strings.ToLower(bundleID)
	until, ok := e.quiet[key]
	if ok && !e.clk.Now().Before(until) {
		delete(e.quiet, key)
		return false
	}
	return ok
}

// scheduleScreenSettleLocked waits for the screens to settle, then works
// out the size class again and rearranges the current tab, without changing
// focus.
func (e *Engine) scheduleScreenSettleLocked() {
	e.screenGen++
	gen := e.screenGen
	if e.screenTimer != nil {
		e.screenTimer.Stop()
	}
	e.screenTimer = e.after(screenSettle, func() {
		if gen != e.screenGen {
			return
		}
		e.screenTimer = nil
		e.screen = e.p.ScreenOfWindow()
		e.arrangeLocked(arrangeOpts{})
		e.changed()
	})
}

// showAnywhereLocked switches to an item's tab, if it's in one, and shows
// it. A popped-out Web item's window comes forward instead.
func (e *Engine) showAnywhereLocked(it *Item) {
	m := e.model
	ti, _ := m.Where(it.ID)
	if ti < 0 {
		if it.Kind == KindWeb && it.OwnWindow && m.IsFloating(it.ID) {
			e.newFocusLocked()
			_ = e.p.FocusWebView(it.ID)
		}
		return
	}
	m.Show(it.ID)
	if ti != m.Current {
		e.switchLocked(ti, switchOpts{focus: true})
		return
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
}

// scheduleTrustCheckLocked checks for Accessibility permission every 2 s.
func (e *Engine) scheduleTrustCheckLocked() {
	if e.trustTimer != nil {
		e.trustTimer.Stop()
	}
	e.trustTimer = e.after(trustEvery, func() {
		e.trustTimer = nil
		if e.trusted {
			return
		}
		if e.p.Trusted(false) {
			e.becameTrustedLocked()
			return
		}
		e.scheduleTrustCheckLocked()
	})
}

// becameTrustedLocked starts arranging once Bench has Accessibility
// permission: it puts back windows a crashed run left parked, scans the
// windows, and switches to the current tab.
func (e *Engine) becameTrustedLocked() {
	e.trusted = true
	if e.trustTimer != nil {
		e.trustTimer.Stop()
		e.trustTimer = nil
	}
	if e.needRestore {
		e.restoreCrashed()
	}
	if !e.started {
		e.touch()
		return
	}
	e.loadRunning()
	e.scanLocked()
	e.switchLocked(e.model.Current, switchOpts{})
}

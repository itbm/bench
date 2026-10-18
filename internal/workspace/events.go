package workspace

import (
	"errors"
	"maps"
	"slices"
	"strings"

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
	case platform.ScreensChanged:
		e.scheduleScreenSettleLocked()
	case platform.BenchMoved:
		e.onBenchMoved()
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
			e.changed()
		}
	case platform.WebURL:
		if ev.Body != "" {
			it.URL = ev.Body
			e.changed()
		}
	case platform.WebNotification:
		_ = e.p.Notify(platform.Notification{ItemID: it.ID, Title: ev.Title, Body: ev.Body})
	case platform.NotificationClicked:
		e.showAnywhereLocked(it)
	case platform.WebViewClosed:
		if e.model.IsFloating(it.ID) {
			e.removeItemLocked(it)
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
	delete(e.openFail, strings.ToLower(ev.App.BundleID)) // It's open now, whoever opened it.
	if !e.trusted || !ev.App.Regular || ev.App.PID == e.ownPID {
		return
	}
	ws := e.newWindowsOfLocked(ev.App)
	sortByCreated(ws)
	if e.adoptLocked(ws, adoptRuntime) {
		e.arrangeLocked(arrangeOpts{})
	}
	e.touch()
}

// onWindowDestroyed removes a closed window's item. The app is still
// running (an app that quits sends AppTerminated instead).
func (e *Engine) onWindowDestroyed(ev platform.Event) {
	id := ev.Window.ID
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
// unmanaged windows of the app are forgotten.
func (e *Engine) onAppTerminated(ev platform.Event) {
	app := ev.App
	if app.BundleID == "" {
		return
	}
	delete(e.running, strings.ToLower(app.BundleID))
	m := e.model
	for _, it := range m.orderedItems() {
		if !isAppOf(it, app.BundleID) || it.WindowID == 0 {
			continue
		}
		if app.PID != 0 && it.pid != 0 && it.pid != app.PID {
			continue
		}
		e.win.Forget(it.WindowID)
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
// or out of full screen. Moves of windows whose app a tab switch is still
// opening are the app settling, not the user.
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
	if e.launchingNow(it.BundleID) {
		// Apps often move and resize their windows as they open. The tab
		// switch that opened the app arranges again once it has settled.
		return
	}
	it.Undocked = true
	it.Status, it.StatusText = StatusUndocked, undockedText
	e.win.Forget(id)
	e.arrangeLocked(arrangeOpts{})
	e.changed()
}

// onWindowFocused follows focus: a window of the current tab is shown in its
// pane, which is focused; one of another tab is handled as follow_focus
// says. Focus events that Bench caused itself are ignored.
func (e *Engine) onWindowFocused(ev platform.Event) {
	id := ev.Window.ID
	if at, ok := e.ownFocus[id]; ok {
		delete(e.ownFocus, id)
		if e.clk.Now().Sub(at) <= ownFocusWindow {
			return
		}
	}
	m := e.model
	it := m.ItemByWindow(id)
	if it == nil {
		e.onWindowCreated(platform.Event{Kind: platform.WindowCreated, Window: ev.Window, App: ev.App})
		return
	}
	if m.IsFloating(it.ID) {
		return
	}
	ti, p := m.Where(it.ID)
	if ti == m.Current {
		shown := p.Active == it.ID
		m.Show(it.ID)
		if !shown {
			e.arrangeLocked(arrangeOpts{})
		}
		e.changed()
		return
	}
	switch e.cfg.Settings.FollowFocus {
	case config.FollowSwitchTab:
		m.Show(it.ID)
		e.switchLocked(ti, switchOpts{})
	case config.FollowBringHere:
		if moved, _ := m.MoveToTab(it.ID, m.Current); moved {
			m.Show(it.ID)
			e.arrangeLocked(arrangeOpts{})
			e.changed()
		}
	}
}

// onWindowRetitled updates the title of a window's item, or of an unmanaged
// window.
func (e *Engine) onWindowRetitled(ev platform.Event) {
	id := ev.Window.ID
	if it := e.model.ItemByWindow(id); it != nil {
		if it.Title != ev.Window.Title {
			it.Title = ev.Window.Title
			e.changed()
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
// in front of it.
func (e *Engine) onAppActivated(ev platform.Event) {
	e.noteAppLocked(ev.App)
	if ev.App.PID != e.ownPID || e.opts.Headless {
		return
	}
	if allowed := e.p.ScreenRecordingAllowed(false); allowed != e.screenRec {
		e.screenRec = allowed
		e.touch()
	}
	if e.trusted {
		e.hideOverlappingLocked()
	}
}

// hideOverlappingLocked hides apps with an unmanaged standard window over
// Bench's content area, unless the app has windows docked in the current
// tab or popped out.
func (e *Engine) hideOverlappingLocked() {
	content := e.p.ContentFrame()
	if content.W <= 0 || content.H <= 0 {
		return
	}
	keep := map[string]bool{}
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
			_ = e.p.Hide(app)
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
// it.
func (e *Engine) showAnywhereLocked(it *Item) {
	m := e.model
	ti, _ := m.Where(it.ID)
	if ti < 0 {
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

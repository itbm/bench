package workspace

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
)

// switchOpts change what switching tabs does.
type switchOpts struct {
	// focus focuses the tab's focused pane afterwards.
	focus bool
	// done is closed when the switch has finished waiting for apps.
	done chan struct{}
}

// waitState is a tab switch waiting for apps to open.
type waitState struct {
	gen    int
	tab    *Tab
	start  time.Time
	opened map[string]bool // Items waiting for a window.
	// focus is set while the focused pane is to get the keyboard focus as
	// soon as what it shows is ready: focusPane and focusItem, the
	// displayed pane and its item at the switch.
	focus                bool
	focusPane, focusItem string
	timer                clock.Timer
	done                 chan struct{}
	// candidates are new windows of apps the switch opened that didn't
	// fit the title filters of the items waiting for them, by window ID:
	// see noteTitleCandidateLocked.
	candidates map[platform.WindowID]titleCandidate
}

// switchLocked switches to tab i: it hides or parks what belongs only to
// other tabs, opens the apps of items that have no window, arranges what's
// ready, and focuses the focused pane. If it opened apps, it checks every
// 250 ms until their windows have arrived (then waits 750 ms and arranges
// again) or the launch timeout ends. A newer switch cancels an older wait.
func (e *Engine) switchLocked(i int, o switchOpts) {
	m := e.model
	e.because(reasonSwitch)
	e.cancelWaitLocked()
	// Another switch ends a hold after last_tab's shortcut, whose tabs
	// start with the current one.
	e.endHoldLocked(false)
	if was := m.CurrentTab(); was != nil && m.IndexOf(was) != i {
		e.noteLeftTabLocked(was)
	}
	m.Current = i
	if !e.started {
		// Before Start, only remember the tab: Start arranges it.
		e.changed()
		if o.done != nil {
			close(o.done)
		}
		return
	}
	t := m.CurrentTab()
	e.recheckUnavailableLocked(t)
	opened := e.openMissingLocked(t)
	if len(opened) > 0 {
		// The wait starts before the tab is arranged, so that the items it
		// waits for show, saying their apps are opening (see
		// openingForLocked).
		e.waitGen++
		e.wait = &waitState{gen: e.waitGen, tab: t, start: e.clk.Now(), opened: opened, done: o.done}
	}
	e.arrangeLocked(arrangeOpts{hideOtherApps: true, switching: true})
	focused := false
	if o.focus {
		// What the focused pane shows is in use now, as if chosen, for the
		// jump palette's order (ItemView.Used).
		if _, id := e.focusTargetLocked(); id != "" {
			m.Use(id)
		}
		focused = e.focusShownLocked()
	}
	e.changed()
	w := e.wait
	if w == nil {
		if o.done != nil {
			close(o.done)
		}
		return
	}
	w.focus = o.focus && !focused
	w.focusPane, w.focusItem = e.focusTargetLocked()
	e.scheduleCheckLocked(checkEvery)
}

// recheckUnavailableLocked asks the platform about a tab's windows that were
// on another desktop, so that ones that have come back are docked again. It
// reports whether any has.
func (e *Engine) recheckUnavailableLocked(t *Tab) bool {
	if !e.trusted {
		return false
	}
	back := false
	for _, id := range t.ItemIDs() {
		it := e.model.Items[id]
		if it == nil || it.Kind != KindApp || it.WindowID == 0 || it.Status != StatusUnavailable {
			continue
		}
		if _, err := e.p.Window(it.WindowID); !errors.Is(err, platform.ErrOtherDesktop) {
			it.Status, it.StatusText = StatusReady, ""
			back = true
		}
	}
	return back
}

// openMissingLocked opens the apps of a tab's App items that have no window,
// if open_missing_apps is on, and returns the items now waiting. Each app is
// opened once for each url its items ask for. Until an app opens again, by
// any route, Bench leaves it alone if the user quit it, if macOS has said
// it isn't installed, or if it opened no window for that url the last time
// Bench opened it.
func (e *Engine) openMissingLocked(t *Tab) map[string]bool {
	opened := map[string]bool{}
	if !e.trusted || !e.cfg.Settings.OpenMissingApps {
		return opened
	}
	asked := map[string]bool{}
	until := e.clk.Now().Add(e.launchTimeout())
	for _, id := range t.ItemIDs() {
		it := e.model.Items[id]
		if it == nil || it.Kind != KindApp || it.WindowID != 0 {
			continue
		}
		key := strings.ToLower(it.BundleID)
		ask := openKey(it)
		if e.notFound[key] || e.quit[key] || e.noWindow[ask] {
			continue
		}
		if !asked[ask] {
			asked[ask] = true
			_, running := e.running[key]
			if rec, ok := e.askedFor[ask]; ok && !e.answeredLocked(rec) {
				switch {
				case e.clk.Now().Before(rec.at.Add(e.launchTimeout())):
					// Bench asked it a moment ago: wait for that window.
					e.launching[key] = until
					opened[id] = true
					continue
				case running:
					// It gave no window the last time it was asked.
					e.noWindow[ask] = true
					delete(e.askedFor, ask)
					continue
				}
			}
			e.askedFor[ask] = askRecord{at: e.clk.Now(), items: e.askingLocked(t, ask)}
			// Opening an app activates it, which reports its focused
			// window, which may be in another tab.
			quiet := quietFocus
			if !running {
				quiet = e.launchTimeout()
			}
			e.noteQuietLocked(it.BundleID, quiet)
			if err := e.p.OpenApp(it.BundleID, it.OpenURL); errors.Is(err, platform.ErrNotInstalled) {
				e.notFound[key] = true
				delete(e.openFail, key)
				delete(e.askedFor, ask)
				continue
			} else if err != nil {
				e.openFail[key] = true
				delete(e.askedFor, ask)
				continue
			}
			delete(e.openFail, key)
			e.launching[key] = until
		}
		if !e.openFail[key] {
			opened[id] = true
		}
	}
	return opened
}

// askRecord is when a tab switch asked an app for a window, and for which
// items (see Engine.askedFor).
type askRecord struct {
	at    time.Time
	items []string
}

// askingLocked returns the items of tab t without a window that a switch
// asks an app for, by openKey.
func (e *Engine) askingLocked(t *Tab, ask string) []string {
	var out []string
	for _, id := range t.ItemIDs() {
		if it := e.model.Items[id]; it != nil && it.Kind == KindApp && it.WindowID == 0 && openKey(it) == ask {
			out = append(out, id)
		}
	}
	return out
}

// answeredLocked reports whether an app has answered a switch that asked
// it for a window: one of the items it asked for has a window, or has
// gone, perhaps with the window it got.
func (e *Engine) answeredLocked(rec askRecord) bool {
	for _, id := range rec.items {
		if it := e.model.Items[id]; it == nil || it.WindowID != 0 {
			return true
		}
	}
	return false
}

// launchingNow reports whether Bench is waiting for an app it opened.
func (e *Engine) launchingNow(bundleID string) bool {
	until, ok := e.launching[strings.ToLower(bundleID)]
	return ok && e.clk.Now().Before(until)
}

// scheduleCheckLocked checks the wait after d.
func (e *Engine) scheduleCheckLocked(d time.Duration) {
	w := e.wait
	gen := w.gen
	w.timer = e.afterFor(reasonAppsOpened, d, func() { e.checkWaitLocked(gen) })
}

// checkWaitLocked looks for the windows a switch is waiting for. When they
// have all arrived, it waits a further 750 ms and arranges again; after the
// launch timeout it gives up, leaving the rest missing.
func (e *Engine) checkWaitLocked(gen int) {
	w := e.wait
	if w == nil || w.gen != gen {
		return
	}
	if e.model.IndexOf(w.tab) < 0 {
		e.finishWaitLocked()
		return
	}
	found := e.scanWaitingLocked(w)
	waiting := e.stillWaitingLocked(w)
	switch {
	case len(waiting) == 0:
		if found {
			e.arrangeLocked(arrangeOpts{})
		}
		e.focusWaitedLocked(w)
		w.timer = e.afterFor(reasonAppsOpened, settleWait, func() { e.settleWaitLocked(gen) })
	case e.clk.Now().Sub(w.start) >= e.launchTimeout():
		for _, it := range waiting {
			key := strings.ToLower(it.BundleID)
			delete(e.launching, key)
			// It's running but gave no window for the item: asking again, on
			// every switch, would only bring it forward.
			if _, running := e.running[key]; running {
				e.noWindow[openKey(it)] = true
			}
		}
		// Focusing now, long after the switch, could take the keyboard
		// from whatever the user has turned to.
		e.arrangeLocked(arrangeOpts{})
		e.finishWaitLocked()
	default:
		if found {
			e.arrangeLocked(arrangeOpts{})
		}
		e.focusWaitedLocked(w)
		e.scheduleCheckLocked(checkEvery)
	}
	e.touch()
}

// focusWaitedLocked focuses the focused pane as soon as what it shows is
// ready, if the switch put that off, rather than only once every app the
// switch is waiting for has opened.
func (e *Engine) focusWaitedLocked(w *waitState) {
	if e.waitFocusDueLocked(w) && e.focusNowLocked() {
		w.focus = false
	}
}

// waitFocusDueLocked reports whether a switch's put-off focus may be made
// now. That can be seconds after the switch, so only while the pane focused
// then still is, and still shows the same item, and no menu or dialog is
// open (menuOrDialogLocked): focusing an app's window would close a native
// menu. Focus by any other route, the user turning to another app, or a
// native menu opening drops it for good (see newFocusLocked,
// anotherAppActivatedLocked, and openMenuLocked): keystrokes meant for
// something else could otherwise land in the app, or in a terminal's shell.
func (e *Engine) waitFocusDueLocked(w *waitState) bool {
	if !w.focus || e.benchHidden || e.menuOrDialogLocked() || e.model.IndexOf(w.tab) != e.model.Current {
		return false
	}
	pane, item := e.focusTargetLocked()
	return pane == w.focusPane && item == w.focusItem
}

// focusTargetLocked returns the displayed focused pane and the item it
// shows.
func (e *Engine) focusTargetLocked() (pane, item string) {
	g, ok := e.displayLocked().focusedGeom()
	if !ok {
		return "", ""
	}
	return g.node.ID, g.node.Active
}

// scanWaitingLocked looks at the windows of the apps a wait is for, in case
// their events haven't arrived, and reports whether any became items.
func (e *Engine) scanWaitingLocked(w *waitState) bool {
	e.loadRunning()
	seen := map[string]bool{}
	var ws []platform.Window
	for _, it := range e.stillWaitingLocked(w) {
		key := strings.ToLower(it.BundleID)
		if seen[key] {
			continue
		}
		seen[key] = true
		app, ok := e.runningAppLocked(it.BundleID)
		if !ok {
			continue
		}
		ws = append(ws, e.newWindowsOfLocked(app)...)
	}
	sortByCreated(ws)
	return e.adoptLocked(ws, adoptRuntime)
}

// stillWaitingLocked returns the items a wait is for that have no window yet.
func (e *Engine) stillWaitingLocked(w *waitState) []*Item {
	var out []*Item
	for _, id := range w.tab.ItemIDs() {
		if !w.opened[id] {
			continue
		}
		if it := e.model.Items[id]; it != nil && it.WindowID == 0 {
			out = append(out, it)
		}
	}
	return out
}

// settleWaitLocked arranges again once apps have had time to settle, and
// ends the wait.
func (e *Engine) settleWaitLocked(gen int) {
	w := e.wait
	if w == nil || w.gen != gen {
		return
	}
	for id := range w.opened {
		if it := e.model.Items[id]; it != nil {
			delete(e.launching, strings.ToLower(it.BundleID))
		}
	}
	e.arrangeLocked(arrangeOpts{})
	if e.waitFocusDueLocked(w) {
		e.focusNowLocked()
	}
	e.finishWaitLocked()
}

// finishWaitLocked ends the wait.
func (e *Engine) finishWaitLocked() {
	w := e.wait
	if w == nil {
		return
	}
	e.wait = nil
	if w.done != nil {
		close(w.done)
	}
	e.touch()
}

// cancelWaitLocked stops a wait that a newer switch replaces. Its items stop
// showing as opening; their windows still become items if they arrive.
func (e *Engine) cancelWaitLocked() {
	w := e.wait
	if w == nil {
		return
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	for _, it := range e.stillWaitingLocked(w) {
		delete(e.launching, strings.ToLower(it.BundleID))
	}
	e.finishWaitLocked()
}

// ArrangeOnce is for `bench arrange <tab>`: it switches to the named tab,
// opens missing apps and waits for their windows (up to the launch timeout),
// and arranges the tab across the screen's visible area, showing each pane's
// first window. It returns when the arrangement is done.
func (e *Engine) ArrangeOnce(ctx context.Context, tabName string) error {
	e.lock()
	if e.closed {
		e.unlock()
		return errNotRunning
	}
	i := tabByName(e.model, tabName)
	if i < 0 {
		e.unlock()
		return errNoTabNamed(tabName)
	}
	if !e.trusted && e.p.Trusted(false) {
		e.becameTrustedLocked()
	}
	if !e.trusted {
		e.unlock()
		return errors.New(accessibilityNotice)
	}
	if !e.started {
		e.beginLocked()
	}
	done := make(chan struct{})
	e.switchLocked(i, switchOpts{done: done})
	e.unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		e.lock()
		e.cancelWaitLocked()
		e.arrangeLocked(arrangeOpts{})
		e.unlock()
		return ctx.Err()
	}
}

// tabByName returns the index of the tab with a name (an exact match, else
// ignoring case), or -1.
func tabByName(m *Model, name string) int {
	for i, t := range m.Tabs {
		if t.Name == name {
			return i
		}
	}
	for i, t := range m.Tabs {
		if strings.EqualFold(t.Name, name) {
			return i
		}
	}
	return -1
}

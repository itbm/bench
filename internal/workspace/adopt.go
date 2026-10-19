package workspace

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file decides what happens to windows Bench sees for the first time:
// at startup, and when apps open new ones.

// adoptPhase says why windows are being adopted.
type adoptPhase int

const (
	// adoptStartup: the scan at startup, or when Accessibility permission
	// arrives. Windows that no item remembers go where claims say, or
	// else stay unmanaged.
	adoptStartup adoptPhase = iota
	// adoptRuntime: new windows. They follow the rules and new_windows.
	adoptRuntime
)

// acceptable reports whether a window can become an item: a standard window
// of a regular app other than Bench.
func (e *Engine) acceptable(w platform.Window, app platform.App) bool {
	if !w.Standard() || w.ID == 0 || w.BundleID == "" || w.PID == e.ownPID {
		return false
	}
	if app.BundleID == "" {
		app = e.running[strings.ToLower(w.BundleID)]
	}
	if app.BundleID != "" && (!app.Regular || app.PID == e.ownPID) {
		return false
	}
	return true
}

// known reports whether Bench already knows a window, as an item's or as
// an unmanaged one.
func (e *Engine) known(id platform.WindowID) bool {
	if e.model.ItemByWindow(id) != nil {
		return true
	}
	_, ok := e.unmanaged[id]
	return ok
}

// runningAppLocked returns a running app by bundle ID.
func (e *Engine) runningAppLocked(bundleID string) (platform.App, bool) {
	app, ok := e.running[strings.ToLower(bundleID)]
	return app, ok
}

// newWindowsOfLocked returns an app's acceptable windows that Bench doesn't
// know yet.
func (e *Engine) newWindowsOfLocked(app platform.App) []platform.Window {
	ws, err := e.p.Windows(app)
	if err != nil {
		return nil
	}
	var out []platform.Window
	for _, w := range ws {
		w = withApp(w, app)
		if e.acceptable(w, app) && !e.known(w.ID) {
			out = append(out, w)
		}
	}
	return out
}

// withApp fills in a window's app from the app it was listed for.
func withApp(w platform.Window, app platform.App) platform.Window {
	if w.BundleID == "" {
		w.BundleID = app.BundleID
	}
	if w.PID == 0 {
		w.PID = app.PID
	}
	if w.App == "" {
		w.App = app.Name
	}
	return w
}

// scanLocked looks at every regular app's standard windows, as at startup.
// Each is matched to a remembered item by window ID and bundle ID first
// (Bench restarted while the app kept running), then by bundle ID and exact
// title, then by creation order, then by claims; the rest are unmanaged.
func (e *Engine) scanLocked() {
	m := e.model
	e.unmanaged = map[platform.WindowID]platform.Window{}
	var all []platform.Window
	for _, app := range e.p.RunningApps() {
		if !app.Regular || app.PID == e.ownPID || app.BundleID == "" {
			continue
		}
		ws, err := e.p.Windows(app)
		if err != nil {
			continue
		}
		for _, w := range ws {
			if w = withApp(w, app); e.acceptable(w, app) {
				all = append(all, w)
			}
		}
	}
	sortByCreated(all)
	live := map[platform.WindowID]bool{}
	var rest []platform.Window
	for _, w := range all {
		live[w.ID] = true
		it := m.ItemByWindow(w.ID)
		if it != nil && sameApp(it.BundleID, w.BundleID) {
			it.Title, it.pid = w.Title, w.PID
			if it.Status == "" || it.Status == StatusMissing {
				it.Status, it.StatusText = StatusReady, ""
			}
			continue
		}
		if it != nil {
			m.Unbind(it) // The ID now belongs to another app's window.
		}
		rest = append(rest, w)
	}
	e.forgetLostWindowsLocked(live)
	e.adoptLocked(rest, adoptStartup)
	e.dropDeadFloatingLocked()
	e.changed()
}

// forgetLostWindowsLocked unbinds items whose windows the scan didn't find
// and that have closed, or whose window IDs now name another app's window
// or one Bench wouldn't take (window IDs start again each login, but
// state.json keeps them). An item whose window is on another desktop is
// unavailable. One whose window couldn't be read, because its app didn't
// answer in time, say, stays bound: the window may well still be there,
// and unbinding it would lose its place.
func (e *Engine) forgetLostWindowsLocked(live map[platform.WindowID]bool) {
	for _, it := range e.model.orderedItems() {
		if it.Kind != KindApp || it.WindowID == 0 || live[it.WindowID] {
			continue
		}
		w, err := e.p.Window(it.WindowID)
		switch {
		case errors.Is(err, platform.ErrOtherDesktop):
			// The platform names the window's app when it knows it.
			if w.BundleID != "" && !sameApp(it.BundleID, w.BundleID) {
				e.model.Unbind(it)
				continue
			}
			it.Status, it.StatusText = StatusUnavailable, unavailableText
		case errors.Is(err, platform.ErrNoWindow):
			e.model.Unbind(it)
		case err == nil && (!sameApp(it.BundleID, w.BundleID) || !e.acceptable(w, platform.App{})):
			e.model.Unbind(it)
		case err == nil:
			// The scan missed it (its app's list couldn't be read, say), but
			// it's still there: as the scan does for the windows it finds.
			it.Title, it.pid = w.Title, w.PID
			if it.Status == "" || it.Status == StatusMissing {
				it.Status, it.StatusText = StatusReady, ""
			}
		}
	}
}

// dropDeadFloatingLocked forgets popped-out App items that have no window:
// they have nothing to show.
func (e *Engine) dropDeadFloatingLocked() {
	m := e.model
	for _, id := range slices.Clone(m.Floating) {
		if it := m.Items[id]; it == nil || (it.Kind == KindApp && it.WindowID == 0) {
			m.Remove(id)
		}
	}
}

// adoptLocked decides where windows Bench hasn't seen go, in order: a rule
// that floats them; the next new window the add dialog asked for; a
// remembered item (by exact title, then creation order); an item that a
// tab switch opened the app for and is waiting for; the pane of the add
// dialog that opened the app; and then, at startup, claims, or at runtime,
// rules naming a tab and new_windows. Windows should be in creation order.
// It reports whether any window became an item.
func (e *Engine) adoptLocked(ws []platform.Window, phase adoptPhase) bool {
	var fresh []platform.Window
	for _, w := range ws {
		if !e.known(w.ID) && !slices.ContainsFunc(fresh, func(f platform.Window) bool { return f.ID == w.ID }) {
			fresh = append(fresh, w)
		}
	}
	placed := false
	var rest []platform.Window
	for _, w := range fresh {
		e.noteLaunchWindowLocked(w)
		if float, _ := matchRule(e.cfg.Rules, w.BundleID, w.Title); float {
			e.unmanaged[w.ID] = w
			continue
		}
		if pl, ok := e.takePendingLocked(w.BundleID, true); ok {
			e.placePendingLocked(pl, w)
			placed = true
			continue
		}
		rest = append(rest, w)
	}
	bound, rest := e.model.MatchRemembered(rest)
	for _, it := range bound {
		e.nameItemLocked(it)
		placed = true
	}
	for _, w := range rest {
		if it := e.waitingPlaceholderLocked(w); it != nil {
			e.model.Bind(it, w)
			e.nameItemLocked(it)
			placed = true
			continue
		}
		if pl, ok := e.takePendingLocked(w.BundleID, false); ok {
			e.placePendingLocked(pl, w)
			placed = true
			continue
		}
		var ok bool
		items := len(e.model.Items)
		if phase == adoptStartup {
			ok = e.placeByClaimLocked(w)
		} else {
			ok = e.placeNewLocked(w)
		}
		if ok {
			placed = true
		} else {
			e.unmanaged[w.ID] = w
		}
		e.noteTitleCandidateLocked(w, len(e.model.Items) > items)
	}
	if placed {
		e.changed()
	} else if len(fresh) > 0 {
		e.touch()
	}
	return placed
}

// placePendingLocked puts a window in the pane a pending placement names,
// and shows it there.
func (e *Engine) placePendingLocked(pl pendingPlace, w platform.Window) {
	e.placeLocked(pl.tab, pl.tab.Pane(pl.pane), w, placeShow)
	if pl.launch != nil {
		pl.launch.placed++
	}
}

// waitingPlaceholderLocked returns an item without a window that the
// current tab switch opened the window's app for, and whose title filter
// suits the window, preferring one whose filter the title contains.
func (e *Engine) waitingPlaceholderLocked(w platform.Window) *Item {
	wt := e.wait
	if wt == nil {
		return nil
	}
	var fallback *Item
	for _, id := range wt.tab.ItemIDs() {
		it := e.model.Items[id]
		if !wt.opened[id] || !isAppOf(it, w.BundleID) || it.WindowID != 0 || !fitsFilter(it, w.Title) {
			continue
		}
		if filterMatches(it, w.Title) {
			return it
		}
		if fallback == nil {
			fallback = it
		}
	}
	return fallback
}

// titleCandidate is a window that may yet suit the title filter of an item
// a tab switch is waiting for, with the new item it became ("" if it was
// left floating).
type titleCandidate struct {
	window platform.Window
	item   string
}

// noteTitleCandidateLocked remembers a new window of an app that the
// current tab switch opened for an item whose title filter the window's
// title doesn't suit, once the window has become a new item or been left
// floating. Apps often title a window only once it has loaded: a browser
// window opened for a url starts as "New Tab", or as the page's host. If a
// title change makes the window suit the filter, adoptRetitledLocked gives
// it to the waiting item.
func (e *Engine) noteTitleCandidateLocked(w platform.Window, newItem bool) {
	wt := e.wait
	if wt == nil || !e.waitsForFilterLocked(w.BundleID) {
		return
	}
	c := titleCandidate{window: w}
	if it := e.model.ItemByWindow(w.ID); it != nil {
		if !newItem {
			return // It went to an item that was waiting for it.
		}
		c.item = it.ID
	} else if _, ok := e.unmanaged[w.ID]; !ok {
		return
	}
	if wt.candidates == nil {
		wt.candidates = map[platform.WindowID]titleCandidate{}
	}
	wt.candidates[w.ID] = c
}

// waitsForFilterLocked reports whether the current tab switch is waiting
// for a window of an app for an item with a title filter.
func (e *Engine) waitsForFilterLocked(bundleID string) bool {
	wt := e.wait
	for _, id := range wt.tab.ItemIDs() {
		if it := e.model.Items[id]; wt.opened[id] && isAppOf(it, bundleID) && it.WindowID == 0 && it.TitleFilter != "" {
			return true
		}
	}
	return false
}

// adoptRetitledLocked gives a window that the current tab switch noted as
// a candidate (see noteTitleCandidateLocked) to the item waiting for it,
// once its new title suits the item's title filter. The item the window
// became meanwhile goes. It reports whether the window moved.
func (e *Engine) adoptRetitledLocked(id platform.WindowID, title string) bool {
	wt := e.wait
	if wt == nil {
		return false
	}
	c, ok := wt.candidates[id]
	if !ok {
		return false
	}
	w := c.window
	w.Title = title
	it := e.waitingPlaceholderLocked(w)
	if it == nil || !filterMatches(it, title) {
		return false
	}
	delete(wt.candidates, id)
	m := e.model
	switch was := m.ItemByWindow(id); {
	case was != nil && was.ID == c.item:
		m.Remove(was.ID)
	case was == nil && c.item == "":
		if _, ok := e.unmanaged[id]; !ok {
			return false
		}
		delete(e.unmanaged, id)
	default:
		return false // The window has gone, or the user has placed it.
	}
	m.Bind(it, w)
	e.nameItemLocked(it)
	e.arrangeLocked(arrangeOpts{})
	e.changed()
	return true
}

// placeHow says how a placed window is shown.
type placeHow int

const (
	placeQuiet    placeHow = iota // Don't change what its pane shows.
	placeShow                     // Show it in its pane.
	placeAfterApp                 // Put it after its app's items, quietly.
)

// placeByClaimLocked puts a window where claims say, binding it to the
// claiming entry's placeholder or adding it right after the entry's last
// item. It reports false if nothing claims it.
func (e *Engine) placeByClaimLocked(w platform.Window) bool {
	ti, p, ok := e.model.Claim(e.cfg, w.BundleID, w.Title)
	if !ok {
		return false
	}
	e.placeLocked(e.model.Tabs[ti], p, w, placeAfterApp)
	return true
}

// placeNewLocked puts a new window where the rules say: a rule's tab, in
// the pane holding the app's most recently used window there, or else that
// tab's focused pane; or as new_windows says, except for the browser that
// the add dialog waits for a web app from (see makingBrowserLocked). (A
// rule that floats it was applied first.) It reports false to leave the
// window floating.
func (e *Engine) placeNewLocked(w platform.Window) bool {
	m := e.model
	if _, tabName := matchRule(e.cfg.Rules, w.BundleID, w.Title); tabName != "" {
		if ti := tabByName(m, tabName); ti >= 0 {
			p := m.RecentPane(ti, w.BundleID)
			if p == nil {
				p = m.Tabs[ti].FocusedPane()
			}
			e.placeLocked(m.Tabs[ti], p, w, placeShow)
			return true
		}
	}
	if sameApp(e.makingBrowserLocked(), w.BundleID) {
		return false
	}
	t, p := e.newWindowPaneLocked(w)
	if p == nil {
		return false
	}
	e.placeLocked(t, p, w, placeShow)
	return true
}

// newWindowPaneLocked applies new_windows: app-pane looks for the pane
// holding the app's most recently used window in the current tab, then in
// other tabs, then the pane the config gives the app (one whose entry's
// title filter claims the window, or else the first listing the app without
// one), and else uses the current tab's focused pane; focused-pane uses the
// focused pane; float leaves the window floating (a nil pane).
func (e *Engine) newWindowPaneLocked(w platform.Window) (*Tab, *layout.Node) {
	m := e.model
	cur := m.CurrentTab()
	switch e.cfg.Settings.NewWindows {
	case config.NewWindowsFloat:
		return nil, nil
	case config.NewWindowsFocusedPane:
		return cur, cur.FocusedPane()
	}
	if p := m.RecentPane(m.Current, w.BundleID); p != nil {
		return cur, p
	}
	if ti, p := m.RecentPaneElsewhere(m.Current, w.BundleID); p != nil {
		return m.Tabs[ti], p
	}
	if ti, p, ok := m.Claim(e.cfg, w.BundleID, w.Title); ok {
		return m.Tabs[ti], p
	}
	return cur, cur.FocusedPane()
}

// placeLocked puts a window in a pane: it binds to a placeholder of its app
// there that suits its title, or else becomes a new item. A nil pane means
// the tab's focused pane.
func (e *Engine) placeLocked(t *Tab, p *layout.Node, w platform.Window, how placeHow) *Item {
	m := e.model
	if m.IndexOf(t) < 0 {
		t = m.CurrentTab()
		p = nil
	}
	if p == nil {
		p = t.FocusedPane()
	}
	it := m.Placeholder(p, w.BundleID, w.Title)
	if it != nil {
		m.Bind(it, w)
	} else {
		it = &Item{ID: m.NewID(itemPrefix), Kind: KindApp, BundleID: w.BundleID}
		// A window that the pane's entry claims by title takes the entry's
		// filter, so that saving the tab doesn't turn it into an entry that
		// claims every window of the app.
		if src := filterSource(m, p, w); src != nil {
			it.TitleFilter, it.OpenURL = src.TitleFilter, src.OpenURL
		}
		at := -1
		if how == placeAfterApp {
			at = m.afterApp(p, w.BundleID)
		}
		m.Bind(it, w)
		m.Add(it, t, p, at)
	}
	e.nameItemLocked(it)
	if how == placeShow {
		m.ShowInPane(it.ID, m.IndexOf(t))
	}
	return it
}

// filterSource returns the item in a pane whose title filter the window's
// title contains, so that a new item for the window can take the same
// filter. It returns nil if there's none.
func filterSource(m *Model, p *layout.Node, w platform.Window) *Item {
	for _, id := range p.Items {
		if it := m.Items[id]; isAppOf(it, w.BundleID) && filterMatches(it, w.Title) {
			return it
		}
	}
	return nil
}

// nameItemLocked sets an App item's app name and monogram.
func (e *Engine) nameItemLocked(it *Item) {
	it.App = e.appName(it.BundleID)
	it.Mono = apps.Mono(it.App)
}

// takePendingLocked returns the pending placement for a new window of an
// app, of one kind: the next new window the add dialog asked for (once), or
// the windows of an app it opened. It drops expired placements and ones
// used up.
func (e *Engine) takePendingLocked(bundleID string, once bool) (pendingPlace, bool) {
	now := e.clk.Now()
	e.pending = slices.DeleteFunc(e.pending, func(pl pendingPlace) bool { return !now.Before(pl.until) })
	for i, pl := range e.pending {
		if !sameApp(pl.bundleID, bundleID) || pl.once != once {
			continue
		}
		if pl.once {
			e.pending = slices.Delete(e.pending, i, i+1)
		}
		return pl, true
	}
	return pendingPlace{}, false
}

// addPendingLocked sends an app's next new window (once) or windows to a
// pane, until the launch timeout.
func (e *Engine) addPendingLocked(bundleID string, t *Tab, p *layout.Node, once bool) {
	e.pending = append(e.pending, pendingPlace{
		bundleID: bundleID, tab: t, pane: p.ID, until: e.clk.Now().Add(e.launchTimeout()), once: once,
	})
	if !once {
		e.pending[len(e.pending)-1].launch = &launchState{}
	}
}

// pendingLaunchLocked returns the pending placement for the windows of an
// app the add dialog opened, if it hasn't expired.
func (e *Engine) pendingLaunchLocked(bundleID string) (pendingPlace, bool) {
	now := e.clk.Now()
	for _, pl := range e.pending {
		if sameApp(pl.bundleID, bundleID) && !pl.once && now.Before(pl.until) {
			return pl, true
		}
	}
	return pendingPlace{}, false
}

// launchState follows the windows of an app the add dialog opened.
type launchState struct {
	placed  int  // Windows sent to the dialog's pane.
	arrived int  // Windows of the app that Bench has seen.
	done    bool // Bench has opened a new window for the pane.
}

// noteLaunchWindowLocked counts a new window of an app the add dialog
// opened. A while after the first, which the app's other windows often
// follow, Bench checks whether any went to the dialog's pane.
func (e *Engine) noteLaunchWindowLocked(w platform.Window) {
	pl, ok := e.pendingLaunchLocked(w.BundleID)
	if !ok {
		return
	}
	pl.launch.arrived++
	if pl.launch.arrived == 1 {
		e.after(launchSettle, func() { e.decideLaunchLocked(pl) })
	}
}

// checkLaunchedLocked runs a while after an app the add dialog opened has
// launched. If the app hasn't opened a window yet, as apps that make theirs
// late (such as Electron apps) may not have, Bench waits for the first (see
// noteLaunchWindowLocked), or else for the launch timeout, before deciding
// whether to open one.
func (e *Engine) checkLaunchedLocked(bundleID string) {
	pl, ok := e.pendingLaunchLocked(bundleID)
	if !ok || pl.launch.arrived > 0 {
		return // The first window's check decides.
	}
	e.after(pl.until.Sub(e.clk.Now()), func() { e.decideLaunchLocked(pl) })
}

// decideLaunchLocked opens a new window for the add dialog's pane if none
// of the windows of the app it opened went there: they all belong to
// remembered items elsewhere, or the app opened none before the launch
// timeout.
func (e *Engine) decideLaunchLocked(pl pendingPlace) {
	l := pl.launch
	if l.done || l.placed > 0 {
		return
	}
	l.done = true
	e.pending = slices.DeleteFunc(e.pending, func(q pendingPlace) bool { return q.launch == l })
	app, running := e.runningAppLocked(pl.bundleID)
	p := pl.tab.Pane(pl.pane)
	if !running || p == nil || e.model.IndexOf(pl.tab) < 0 {
		return
	}
	if err := e.p.NewWindow(app); err != nil {
		e.toastLocked(fmt.Sprintf("Bench couldn't open a new %s window.", e.appName(pl.bundleID)))
		return
	}
	e.noteQuietLocked(pl.bundleID, quietFocus)
	e.addPendingLocked(pl.bundleID, pl.tab, p, true)
}

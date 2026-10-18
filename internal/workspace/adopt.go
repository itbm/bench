package workspace

import (
	"errors"
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

// forgetLostWindowsLocked unbinds items whose windows the scan didn't find,
// except windows on another desktop, which are unavailable.
func (e *Engine) forgetLostWindowsLocked(live map[platform.WindowID]bool) {
	for _, it := range e.model.orderedItems() {
		if it.Kind != KindApp || it.WindowID == 0 || live[it.WindowID] {
			continue
		}
		if _, err := e.p.Window(it.WindowID); errors.Is(err, platform.ErrOtherDesktop) {
			it.Status, it.StatusText = StatusUnavailable, unavailableText
			continue
		}
		e.model.Unbind(it)
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

// adoptLocked decides where windows Bench hasn't seen go, in order: a
// pending placement from the add dialog; a rule that floats them; a
// remembered item (by exact title, then creation order); an item that a tab
// switch opened the app for and is waiting for; and then, at startup,
// claims, or at runtime, title-filter claims, rules naming a tab, and
// new_windows. Windows should be in creation order. It reports whether
// any window became an item.
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
		if pl, ok := e.takePendingLocked(w.BundleID); ok {
			e.placeLocked(pl.tab, pl.tab.Pane(pl.pane), w, placeShow)
			placed = true
			continue
		}
		if float, _ := matchRule(e.cfg.Rules, w.BundleID, w.Title); float {
			e.unmanaged[w.ID] = w
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
		var ok bool
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
	}
	if placed {
		e.changed()
	} else if len(fresh) > 0 {
		e.touch()
	}
	return placed
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

// placeNewLocked puts a new window where the rules say: a pane whose entry's
// title filter claims it; a rule's tab, in the pane holding the app's most
// recently used window there, or else that tab's focused pane; or as
// new_windows says. It reports false to leave the window floating.
func (e *Engine) placeNewLocked(w platform.Window) bool {
	m := e.model
	if ti, p, ok := m.FilterClaim(e.cfg, w.BundleID, w.Title); ok {
		e.placeLocked(m.Tabs[ti], p, w, placeShow)
		return true
	}
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
	t, p := e.newWindowPaneLocked(w)
	if p == nil {
		return false
	}
	e.placeLocked(t, p, w, placeShow)
	return true
}

// newWindowPaneLocked applies new_windows: app-pane looks for the pane
// holding the app's most recently used window in the current tab, then in
// other tabs, then the config's pane for the app, and else uses the current
// tab's focused pane; focused-pane uses the focused pane; float leaves the
// window floating (a nil pane).
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
		m.ShowInPane(it.ID)
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
// app, dropping expired ones and ones used up.
func (e *Engine) takePendingLocked(bundleID string) (pendingPlace, bool) {
	now := e.clk.Now()
	e.pending = slices.DeleteFunc(e.pending, func(pl pendingPlace) bool { return !now.Before(pl.until) })
	for i, pl := range e.pending {
		if !sameApp(pl.bundleID, bundleID) {
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
}

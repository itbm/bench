package workspace

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file has the add dialog (AddOptions and AddToPane), and popping
// items out of Bench and bringing them back.

// Add dialog groups and sources.
const (
	groupFloating = "Windows not in a tab"
	groupNew      = "New window"
	groupBuiltin  = "Built in"
	groupApps     = "Apps"

	sourceFloating  = "floating"
	sourceNewWindow = "new-window"
	sourceApp       = "app"
	sourceTerminal  = "terminal"
	sourceWeb       = "web"
)

// floatingPrefix starts the IDs of unmanaged windows in View.Floating.
const floatingPrefix = "f"

// floatingID returns an unmanaged window's ID in View.Floating.
func floatingID(id platform.WindowID) string {
	return floatingPrefix + strconv.FormatUint(uint64(id), 10)
}

// targetPaneLocked returns the pane of the current tab that the add dialog
// adds to: the focused pane, for the "tabs" view's compact pane.
func (e *Engine) targetPaneLocked(paneID string) (*Tab, *layout.Node, error) {
	t := e.model.CurrentTab()
	if paneID == layout.CompactPaneID {
		return t, t.FocusedPane(), nil
	}
	p := t.Pane(paneID)
	if p == nil {
		return nil, nil, errNoPane
	}
	return t, p, nil
}

// AddOptions returns the add dialog's rows for a pane, in group order:
// windows not in a tab, a new window of each app in the tab, the built-in
// items, and every installed app.
func (e *Engine) AddOptions(paneID string) ([]AddOption, error) {
	e.lock()
	defer e.unlock()
	t, _, err := e.targetPaneLocked(paneID)
	if err != nil {
		return nil, err
	}
	var out []AddOption
	for _, f := range e.floatingViewsLocked() {
		out = append(out, AddOption{Group: groupFloating, Mono: f.Mono, Label: f.Title, Hint: f.App,
			Source: Source{Type: sourceFloating, ID: f.ID}})
	}
	seen := map[string]bool{}
	for _, id := range t.ItemIDs() {
		it := e.model.Items[id]
		if it == nil || it.Kind != KindApp || seen[strings.ToLower(it.BundleID)] {
			continue
		}
		seen[strings.ToLower(it.BundleID)] = true
		out = append(out, AddOption{Group: groupNew, Mono: it.Mono, Label: "New " + it.App + " window",
			Source: Source{Type: sourceNewWindow, BundleID: it.BundleID}})
	}
	out = append(out,
		AddOption{Group: groupBuiltin, Mono: apps.Mono(terminalApp), Label: "New terminal", Hint: defaultFolder,
			Source: Source{Type: sourceTerminal}},
		AddOption{Group: groupBuiltin, Mono: apps.Mono("Web page"), Label: "Web page…", Hint: "Opens in a Bench web view",
			Source: Source{Type: sourceWeb}, AsksForURL: true})
	for _, info := range e.loadInstalled() {
		name := apps.DisplayName(info.Name)
		out = append(out, AddOption{Group: groupApps, Mono: apps.Mono(name), Label: name,
			Hint: "Open, or use its open windows", Source: Source{Type: sourceApp, BundleID: info.BundleID}})
	}
	return out, nil
}

// AddToPane adds something from the add dialog to a pane of the current
// tab, and shows it there.
func (e *Engine) AddToPane(paneID string, source Source) error {
	e.lock()
	defer e.unlock()
	t, p, err := e.targetPaneLocked(paneID)
	if err != nil {
		return err
	}
	var shown *Item
	switch source.Type {
	case sourceFloating:
		shown, err = e.addFloatingLocked(t, p, source.ID)
	case sourceNewWindow:
		err = e.addNewWindowLocked(t, p, source.BundleID)
	case sourceApp:
		shown, err = e.addAppLocked(t, p, source.BundleID)
	case sourceTerminal:
		shown = e.addTerminalLocked(t, p, source.Folder)
	case sourceWeb:
		shown, err = e.addWebLocked(t, p, source.URL)
	default:
		err = errBadSource
	}
	if err != nil {
		return err
	}
	if shown != nil {
		e.model.Show(shown.ID)
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	return nil
}

// addFloatingLocked adds a popped-out item, or an unmanaged window as a new
// item.
func (e *Engine) addFloatingLocked(t *Tab, p *layout.Node, id string) (*Item, error) {
	m := e.model
	if m.IsFloating(id) {
		it := m.Items[id]
		m.Take(id)
		insertAt(p, id, -1)
		e.bringBackLocked(it)
		return it, nil
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(id, floatingPrefix), 10, 32)
	if !strings.HasPrefix(id, floatingPrefix) || err != nil {
		return nil, errGone
	}
	w, ok := e.unmanaged[platform.WindowID(n)]
	if !ok {
		return nil, errGone
	}
	delete(e.unmanaged, w.ID)
	return e.placeLocked(t, p, w, placeQuiet), nil
}

// addNewWindowLocked opens a new window of an app and sends it to the pane.
func (e *Engine) addNewWindowLocked(t *Tab, p *layout.Node, bundleID string) error {
	app, ok := e.runningAppLocked(bundleID)
	if !ok {
		_, err := e.openAppForLocked(t, p, bundleID)
		return err
	}
	if err := e.p.NewWindow(app); err != nil {
		return fmt.Errorf("Bench couldn't open a new %s window.", e.appName(bundleID))
	}
	e.addPendingLocked(bundleID, t, p, true)
	return nil
}

// addAppLocked adds an installed app's windows that aren't in a tab to the
// pane. If it isn't running, it opens it, and its first windows go to the
// pane; if it has no such windows, it opens a new one.
func (e *Engine) addAppLocked(t *Tab, p *layout.Node, bundleID string) (*Item, error) {
	app, ok := e.runningAppLocked(bundleID)
	if !ok {
		return e.openAppForLocked(t, p, bundleID)
	}
	var free []platform.Window
	for _, w := range e.unmanaged {
		if sameApp(w.BundleID, bundleID) {
			free = append(free, w)
		}
	}
	free = append(free, e.newWindowsOfLocked(app)...)
	sortByCreated(free)
	free = slices.CompactFunc(free, func(a, b platform.Window) bool { return a.ID == b.ID })
	if len(free) == 0 {
		if err := e.p.NewWindow(app); err != nil {
			return nil, fmt.Errorf("Bench couldn't open a new %s window.", e.appName(bundleID))
		}
		e.addPendingLocked(bundleID, t, p, true)
		return nil, nil
	}
	var first *Item
	for _, w := range free {
		delete(e.unmanaged, w.ID)
		it := e.placeLocked(t, p, w, placeQuiet)
		if first == nil {
			first = it
		}
	}
	return first, nil
}

// openAppForLocked opens an app that isn't running, and sends the windows it
// opens as it launches to the pane.
func (e *Engine) openAppForLocked(t *Tab, p *layout.Node, bundleID string) (*Item, error) {
	if err := e.p.OpenApp(bundleID, ""); err != nil {
		return nil, fmt.Errorf("Bench couldn't open %s.", e.appName(bundleID))
	}
	e.addPendingLocked(bundleID, t, p, false)
	return nil, nil
}

// addTerminalLocked adds a terminal in a folder ("~" if empty).
func (e *Engine) addTerminalLocked(t *Tab, p *layout.Node, folder string) *Item {
	if folder == "" {
		folder = defaultFolder
	}
	m := e.model
	it := &Item{ID: m.NewID(itemPrefix), Kind: KindBuiltin, App: terminalApp, Mono: apps.Mono(terminalApp),
		Title: terminalTitle("", folder), Folder: folder, Status: StatusReady}
	m.Add(it, t, p, -1)
	return it
}

// addWebLocked adds a Web item for an address, which must be https.
func (e *Engine) addWebLocked(t *Tab, p *layout.Node, address string) (*Item, error) {
	address, err := normaliseAddress(address)
	if err != nil {
		return nil, err
	}
	m := e.model
	name := apps.WebName(address)
	it := &Item{ID: m.NewID(itemPrefix), Kind: KindWeb, App: name, Mono: apps.Mono(name), Title: name,
		URL: address, Home: address, Profile: hostOf(address), Status: StatusReady}
	m.Add(it, t, p, -1)
	return it, nil
}

// bringBackLocked docks a popped-out Web or Built-in item's view in Bench's
// window again, as it comes back into a tab.
func (e *Engine) bringBackLocked(it *Item) {
	if !it.OwnWindow {
		return
	}
	it.OwnWindow = false
	switch it.Kind {
	case KindWeb:
		_ = e.p.DockWebView(it.ID)
		delete(e.webShown, it.ID)
	case KindBuiltin:
		em, id := e.em, it.ID
		e.later(func() { em.CloseTerminalWindow(id) })
	}
}

// PopOut takes an item out of Bench: an App item's window becomes a normal,
// free-floating window again, and a Web or Built-in item gets a window of
// its own. The item is then listed under "Not in a tab".
func (e *Engine) PopOut(itemID string) error {
	e.lock()
	it := e.model.Items[itemID]
	switch {
	case it == nil:
		e.unlock()
		return errGone
	case e.model.IsFloating(itemID):
		e.unlock()
		return nil
	case it.Kind == KindBuiltin && !e.opts.Headless:
		e.unlock()
		return e.popOutTerminal(itemID)
	}
	defer e.unlock()
	switch it.Kind {
	case KindApp:
		if it.WindowID == 0 {
			e.model.Remove(itemID)
			e.toastLocked("Removed " + labelOf(it) + " from Bench, because it has no open window.")
			e.arrangeLocked(arrangeOpts{})
			e.changed()
			return nil
		}
		_ = e.win.Unpark(it.WindowID)
		e.win.Forget(it.WindowID)
		it.Undocked = false
		it.Status, it.StatusText = StatusReady, ""
	case KindWeb:
		if err := e.popOutWebLocked(it); err != nil {
			return err
		}
	}
	e.finishPopOutLocked(it)
	return nil
}

// popOutWebLocked gives a Web item's view a window of its own, creating the
// view first if it has never been shown.
func (e *Engine) popOutWebLocked(it *Item) error {
	if !e.webMade[it.ID] {
		address, ok := e.webAddressLocked(it)
		if !ok {
			return fmt.Errorf("Bench can't pop out %s until it has loaded.", it.App)
		}
		if err := e.p.ShowWebView(it.ID, platform.WebView{URL: address, Profile: it.Profile}, layout.Rect{}); err != nil {
			return fmt.Errorf("Bench couldn't pop out %s.", labelOf(it))
		}
		e.webMade[it.ID] = true
	}
	if err := e.p.PopOutWebView(it.ID, labelOf(it)); err != nil {
		return fmt.Errorf("Bench couldn't pop out %s.", labelOf(it))
	}
	delete(e.webShown, it.ID)
	it.OwnWindow = true
	return nil
}

// popOutTerminal gives a terminal a window of its own. The Emitter makes the
// window, without the lock held.
func (e *Engine) popOutTerminal(itemID string) error {
	e.lock()
	it := e.model.Items[itemID]
	if it == nil {
		e.unlock()
		return errGone
	}
	e.startTerminalLocked(it)
	title := labelOf(it)
	e.unlock()

	if err := e.em.PopOutTerminal(itemID, title); err != nil {
		return fmt.Errorf("Bench couldn't give the terminal a window of its own: %v.", err)
	}

	e.lock()
	defer e.unlock()
	it = e.model.Items[itemID]
	if it == nil || e.model.IsFloating(itemID) {
		return nil
	}
	it.OwnWindow = true
	e.finishPopOutLocked(it)
	return nil
}

// finishPopOutLocked moves a popped-out item to Floating.
func (e *Engine) finishPopOutLocked(it *Item) {
	e.model.Take(it.ID)
	e.model.Floating = append(e.model.Floating, it.ID)
	e.toastLocked("Moved " + labelOf(it) + " out of Bench. Add it back with +.")
	e.arrangeLocked(arrangeOpts{})
	e.changed()
}

package workspace

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file has the add dialog (AddOptions, AddToPane, AddWebsite, and
// AddWebAppWhenReady), and popping items out of Bench and bringing them
// back.

// Add dialog groups and sources.
const (
	groupLaunchers = "Launchers"
	groupFloating  = "Windows not in a tab"
	groupOtherTabs = "Windows in other tabs"
	groupNew       = "New window"
	groupBuiltin   = "Built in"
	groupWebApps   = "Web apps"
	groupApps      = "Apps"

	sourceFloating  = "floating"
	sourceItem      = "item"
	sourceNewWindow = "new-window"
	sourceApp       = "app"
	sourceTerminal  = "terminal"
	sourceLauncher  = "launcher"
	sourceWeb       = "web"
)

// What AddWebsite added (WebsiteResult.Added).
const (
	addedWebApp = "web-app"
	addedWeb    = "web"
)

// makeQuiet is how long after AddWebsite opens a site in the default
// browser, or AddWebAppWhenReady finds no web app for it yet, focus events
// for the browser's windows in other tabs are put down to Bench, and Bench
// leaves the browser where it is (see makingBrowserLocked). Meanwhile the
// user is in the browser, making the web app, and the add dialog waits:
// switching tabs, as follow_focus would, would close the dialog. The dialog
// asks again well within it.
const makeQuiet = 5 * time.Second

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

// AddOptions returns the add dialog's rows for a pane, in group order: the
// launchers, windows not in a tab, the items of other tabs (to show here as
// well), a new window of each app in the tab, the built-in items, starting
// with a new terminal in the folder of the terminal that the pane shows,
// if it does, and ending with Browse…, and every installed app. The
// launchers come first, as the dialog chooses the first row that matches
// what the user types. The scan of the installed apps reads folders, so
// it's made without the lock.
func (e *Engine) AddOptions(paneID string) ([]AddOption, error) {
	installed := e.p.InstalledApps()
	e.lock()
	defer e.unlock()
	t, p, err := e.targetPaneLocked(paneID)
	if err != nil {
		return nil, err
	}
	var out []AddOption
	for _, l := range e.launcherViewsLocked() {
		out = append(out, AddOption{Group: groupLaunchers, Mono: l.Mono, Icon: l.Icon, Label: l.Name, Hint: l.Hint,
			Source: Source{Type: sourceLauncher, ID: l.Name}})
	}
	for _, f := range e.floatingViewsLocked() {
		out = append(out, AddOption{Group: groupFloating, Mono: f.Mono, Icon: f.Icon, Label: f.Title, Hint: f.App,
			Source: Source{Type: sourceFloating, ID: f.ID}})
	}
	m := e.model
	for _, it := range m.orderedItems() {
		tabs := m.TabsOf(it.ID)
		if len(tabs) == 0 || slices.Contains(tabs, m.Current) || e.hiddenLocked(it) {
			continue
		}
		var names []string
		for _, ti := range tabs {
			names = append(names, m.Tabs[ti].Name)
		}
		out = append(out, AddOption{Group: groupOtherTabs, Mono: it.Mono, Icon: e.itemIconLocked(it), Label: e.shownTitleLocked(it), Hint: joinNames(names) + " · " + it.App,
			Source: Source{Type: sourceItem, ID: it.ID}})
	}
	seen := map[string]bool{}
	for _, id := range t.ItemIDs() {
		it := e.model.Items[id]
		if it == nil || it.Kind != KindApp || seen[strings.ToLower(it.BundleID)] || e.hiddenLocked(it) {
			continue
		}
		seen[strings.ToLower(it.BundleID)] = true
		out = append(out, AddOption{Group: groupNew, Mono: it.Mono, Icon: e.appIconLocked(it.BundleID), Label: "New " + it.App + " window",
			Source: Source{Type: sourceNewWindow, BundleID: it.BundleID}})
	}
	// The hint doesn't name the browser, so that typing its name chooses
	// the browser, in Apps, rather than this row, which comes first.
	webHint, webBrowser := "Opens in a Bench web view", ""
	if maker, ok := apps.WebAppMakerFor(e.p.DefaultBrowser()); ok && e.trusted {
		webHint, webBrowser = "As a web app, or in Bench", maker.Browser
	}
	here := e.hereLocked(t, p)
	if here != "" && here != defaultFolder {
		out = append(out, AddOption{Group: groupBuiltin, Mono: apps.Mono(terminalApp), Label: "New terminal in " + here,
			Hint: "Where the terminal here is", Source: Source{Type: sourceTerminal, Folder: here}})
	}
	out = append(out,
		AddOption{Group: groupBuiltin, Mono: apps.Mono(terminalApp), Label: "New terminal", Hint: defaultFolder,
			Source: Source{Type: sourceTerminal}},
		AddOption{Group: groupBuiltin, Mono: apps.Mono(terminalApp), Label: "New terminal in…", Hint: "Choose a folder",
			Source: Source{Type: sourceTerminal, Folder: here}, ChoosesFolder: true},
		AddOption{Group: groupBuiltin, Mono: apps.Mono(terminalApp), Label: "New terminal…", Hint: "With a command, a name, or a folder",
			Source: Source{Type: sourceTerminal}, AsksForCommand: true},
		AddOption{Group: groupBuiltin, Mono: apps.Mono("Web page"), Label: "Web page…", Hint: webHint,
			Source: Source{Type: sourceWeb}, AsksForURL: true, WebAppBrowser: webBrowser},
		AddOption{Group: groupBuiltin, Mono: apps.Mono("Browse"), Label: "Browse…", Hint: "An app, or a program to run in a terminal",
			Source: Source{Type: sourceBrowse}, Browses: true})
	// Web apps follow the other apps, in a group of their own: the user may
	// have the site's desktop app installed too, with the same name. They
	// come after, because the dialog chooses the first row that matches,
	// and a web app's hint names its browser: typing "chrome" should
	// choose Chrome, not a web app that Chrome made.
	e.setInstalledLocked(installed)
	installed = e.withBrowsedLocked(installed)
	for _, web := range []bool{false, true} {
		for _, info := range installed {
			if (info.WebApp != "") != web {
				continue
			}
			name := apps.DisplayName(info.Name)
			group, hint := groupApps, "Open, or use its open windows"
			if web {
				group, hint = groupWebApps, "Web app from "+info.WebApp
			}
			out = append(out, AddOption{Group: group, Mono: apps.Mono(name), Icon: e.appIconLocked(info.BundleID), Label: name,
				Hint: hint, Source: Source{Type: sourceApp, BundleID: info.BundleID}})
		}
	}
	return out, nil
}

// hereLocked returns the folder that the shell is in of the terminal that
// pane p shows, or else the focused pane does, in tab t, for the add
// dialog's New terminal in: the one Bench last knew of (see hereOf). It
// returns "" if neither shows a terminal.
func (e *Engine) hereLocked(t *Tab, p *layout.Node) string {
	for _, n := range []*layout.Node{p, t.FocusedPane()} {
		if n == nil {
			continue
		}
		if it := e.model.Items[n.Active]; it != nil && it.Kind == KindBuiltin {
			return hereOf(it)
		}
	}
	return ""
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
	case sourceItem:
		shown, err = e.addFromTabLocked(t, p, source.ID)
	case sourceNewWindow:
		err = e.addNewWindowLocked(t, p, source.BundleID)
	case sourceApp:
		shown, err = e.addAppLocked(t, p, source.BundleID)
	case sourceTerminal:
		shown = e.addTerminalLocked(t, p, terminalOf(source))
	case sourceLauncher:
		shown, err = e.addLauncherLocked(t, p, source.ID)
	case sourceWeb:
		shown, err = e.addWebLocked(t, p, source.URL)
	default:
		err = errBadSource
	}
	if err != nil {
		return err
	}
	e.finishAddLocked(shown)
	return nil
}

// finishAddLocked shows what the add dialog added, if it's an item yet, and
// arranges the current tab. A window that's minimised comes back from the
// Dock, as the user chose it.
func (e *Engine) finishAddLocked(shown *Item) {
	if shown != nil {
		showAgain(shown)
		e.model.Show(shown.ID)
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
}

// AddWebsite adds a website to a pane of the current tab, from the add
// dialog's Web page…, as a web app where it can: they run in the browser,
// with its protections and sign-ins, which Web items don't have. The
// address is read as AddToPane's web source reads it. Then, in order:
//
//   - A page on this Mac, such as a development server at localhost:3000,
//     is a Web item at once: it comes and goes with the server, and a web
//     app of it would outlive it.
//   - If a web app of the site is installed (one whose WebURL has the same
//     host), it adds that, as the app source would: the default browser's,
//     else Safari's, else the first by name.
//   - Without Accessibility permission, which it needs to dock web apps, it
//     adds a Web item, and says why.
//   - If the default browser can make web apps, it opens the site there
//     and adds nothing: the dialog shows the browser's steps, and adds the
//     web app with AddWebAppWhenReady once the user has made it. Until
//     then, the browser's new windows stay out of the tab, and Bench
//     doesn't hide the browser (see makingBrowserLocked).
//   - Otherwise it adds a Web item, and says why.
func (e *Engine) AddWebsite(paneID, address string) (WebsiteResult, error) {
	address, err := normaliseAddress(address)
	if err != nil {
		return WebsiteResult{}, err
	}
	// Scanning reads every app's Info.plist, so it's done without the lock.
	installed := e.p.InstalledApps()
	browser := e.p.DefaultBrowser()
	e.lock()
	defer e.unlock()
	t, p, err := e.targetPaneLocked(paneID)
	if err != nil {
		return WebsiteResult{}, err
	}
	e.setInstalledLocked(installed)
	if u, err := url.Parse(address); err == nil && config.LocalHost(u.Hostname()) {
		shown, err := e.addWebLocked(t, p, address)
		if err != nil {
			return WebsiteResult{}, err
		}
		e.finishAddLocked(shown)
		return WebsiteResult{Added: addedWeb}, nil
	}
	if info, ok := webAppFor(installed, address, browser); ok {
		shown, err := e.addAppLocked(t, p, info.BundleID)
		if err != nil {
			return WebsiteResult{}, err
		}
		e.finishAddLocked(shown)
		name := apps.DisplayName(info.Name)
		return WebsiteResult{Added: addedWebApp, App: name, Browser: info.WebApp,
			Note: "Added the " + name + " web app from " + info.WebApp + "."}, nil
	}
	maker, canMake := apps.WebAppMakerFor(browser)
	var note string
	switch {
	case !e.trusted:
		note = noAccessibilityForWebApps
	case canMake:
		return e.startMakingLocked(makingWebApp{paneID: paneID, address: address, browser: browser}, maker, installed)
	case apps.CantMakeWebApps(browser):
		note = cantMakeWebApps(e.appName(browser))
	case browser != "":
		note = unknownWebAppMaker(e.appName(browser))
	default:
		note = noBrowserForWebApps
	}
	shown, err := e.addWebLocked(t, p, address)
	if err != nil {
		return WebsiteResult{}, err
	}
	e.finishAddLocked(shown)
	return WebsiteResult{Added: addedWeb, Note: note}, nil
}

// AddWebAppWhenReady adds the web app of an address's site to a pane of the
// current tab if one is installed now, as AddWebsite would, and reports
// whether it did. The add dialog calls it every second or two while the
// default browser, which AddWebsite opened the site in, makes the web app.
// The web app may not be on the address's host: a browser records the
// address the site ended on, after any redirect, or the start_url of its
// manifest, and Orion records none. So once AddWebsite has opened the
// address for the pane, the one web app installed since then is the site's
// too (see newWebApp).
func (e *Engine) AddWebAppWhenReady(paneID, address string) (bool, error) {
	address, err := normaliseAddress(address)
	if err != nil {
		return false, err
	}
	installed := e.p.InstalledApps()
	browser := e.p.DefaultBrowser()
	e.lock()
	defer e.unlock()
	t, p, err := e.targetPaneLocked(paneID)
	if err != nil {
		return false, err
	}
	e.setInstalledLocked(installed)
	m := e.makingForLocked(paneID, "", address)
	info, ok := webAppFor(installed, address, browser)
	if !ok {
		info, ok = newWebApp(installed, m)
	}
	if !ok {
		e.stillMakingLocked(browser, m)
		return false, nil
	}
	making := e.making
	e.making = nil
	shown, err := e.addMadeWebAppLocked(t, p, info.BundleID)
	if err != nil {
		e.making = making
		return false, err
	}
	e.finishAddLocked(shown)
	return true, nil
}

// makingWebApp is a web app that a dialog waits for while the user makes it
// in the default browser, which Bench opened its site in: the add dialog,
// after AddWebsite, for a pane, or the dialog that MakeWebApp opened, to
// replace a Web item.
type makingWebApp struct {
	// paneID is the add dialog's pane, which the web app is to join, or
	// itemID the Web item it's to replace. The other is "".
	paneID, itemID string
	// address is the site the browser opened: the address as AddWebsite
	// read it, or the Web item's Home.
	address string
	browser string // The default browser's bundle ID.
	// before has the web apps installed when Bench opened the site, by
	// lower-case bundle ID.
	before map[string]bool
	// until is when the dialog has stopped waiting, unless it asks again.
	until time.Time
}

// startMakingLocked opens a site in the default browser, for the user to
// make a web app of it there, and waits for the web app as m says: for
// makeQuiet, or the launch timeout if the browser isn't running yet, Bench
// leaves the browser where it is (see makingBrowserLocked), and the dialog
// asks again well within it (see stillMakingLocked). It returns the
// browser's steps.
func (e *Engine) startMakingLocked(m makingWebApp, maker apps.WebAppMaker, installed []platform.AppInfo) (WebsiteResult, error) {
	quiet := makeQuiet
	if _, running := e.runningAppLocked(m.browser); !running {
		quiet = max(e.launchTimeout(), makeQuiet)
	}
	e.noteQuietLocked(m.browser, quiet)
	m.before, m.until = webAppIDs(installed), e.clk.Now().Add(quiet)
	e.making = &m
	if err := e.p.OpenApp(m.browser, m.address); err != nil {
		e.making = nil
		return WebsiteResult{}, fmt.Errorf("Bench couldn't open %s in %s.", hostOf(m.address), maker.Browser)
	}
	return WebsiteResult{Browser: maker.Browser, Steps: maker.Steps}, nil
}

// makingForLocked returns the web app being made for a pane's add dialog
// (itemID "") or to replace a Web item (paneID ""), with the site at an
// address, or nil if no dialog waits for one.
func (e *Engine) makingForLocked(paneID, itemID, address string) *makingWebApp {
	m := e.making
	if m == nil || m.paneID != paneID || m.itemID != itemID || m.address != address {
		return nil
	}
	return m
}

// stillMakingLocked keeps waiting for a web app that a dialog asked for and
// that isn't installed yet, m if Bench opened its site in the default
// browser, which is browser: the user is still in the browser, making it.
func (e *Engine) stillMakingLocked(browser string, m *makingWebApp) {
	if browser != "" {
		e.noteQuietLocked(browser, makeQuiet)
	}
	if m == nil {
		return
	}
	if until := e.clk.Now().Add(makeQuiet); until.After(m.until) {
		m.until = until
	}
}

// webAppIDs returns the lower-case bundle IDs of the installed web apps.
func webAppIDs(installed []platform.AppInfo) map[string]bool {
	out := map[string]bool{}
	for _, info := range installed {
		if info.WebApp != "" {
			out[strings.ToLower(info.BundleID)] = true
		}
	}
	return out
}

// newWebApp returns the web app that has been installed since Bench opened
// m's site in the default browser, from any browser, whatever address it
// records. It returns false for a nil m, or if there's none, or more than
// one, which Bench can't choose between.
func newWebApp(installed []platform.AppInfo, m *makingWebApp) (platform.AppInfo, bool) {
	if m == nil {
		return platform.AppInfo{}, false
	}
	var found []platform.AppInfo
	for _, info := range installed {
		if info.WebApp != "" && !m.before[strings.ToLower(info.BundleID)] {
			found = append(found, info)
		}
	}
	if len(found) != 1 {
		return platform.AppInfo{}, false
	}
	return found[0], true
}

// makingBrowserLocked returns the bundle ID of the default browser that
// Bench has opened a site in, while a dialog waits for the user to make a
// web app there, or "". Meanwhile, Bench doesn't hide the browser, when
// Bench becomes active or as it arranges the tab, as the user goes between
// the browser and the dialog's steps, and the browser's new windows don't
// join a pane by new_windows: the user opened the site to make the web app,
// not to add the browser.
func (e *Engine) makingBrowserLocked() string {
	if e.making == nil || !e.clk.Now().Before(e.making.until) {
		return ""
	}
	return e.making.browser
}

// webAppFor returns the installed web app of an address's site: one whose
// WebURL has the same host. If there are several, it prefers the one made by
// the browser with bundle ID browser, then Safari's, then the first in
// installed, which is sorted by name.
func webAppFor(installed []platform.AppInfo, address, browser string) (platform.AppInfo, bool) {
	preferred := apps.BrowserName(browser)
	var found platform.AppInfo
	rank := 0
	for _, info := range installed {
		if info.WebApp == "" || !apps.SameWebHost(info.WebURL, address) {
			continue
		}
		r := 1
		switch {
		case preferred != "" && info.WebApp == preferred:
			r = 3
		case info.WebApp == "Safari":
			r = 2
		}
		if r > rank {
			found, rank = info, r
		}
	}
	return found, rank > 0
}

// addMadeWebAppLocked adds a web app that the user has just made, while the
// add dialog waited, to the pane, as addAppLocked would. A Chromium
// browser opens the web app it makes, and Bench may have put its window in
// this tab already, by new_windows: it moves to the pane. If the web app is
// running without a window yet, as it is just after it starts, its first
// window comes to the pane, or, if none comes soon, a new one.
func (e *Engine) addMadeWebAppLocked(t *Tab, p *layout.Node, bundleID string) (*Item, error) {
	m := e.model
	ti := m.IndexOf(t)
	var first *Item
	for _, id := range t.ItemIDs() {
		it := m.Items[id]
		if !isAppOf(it, bundleID) || it.WindowID == 0 {
			continue
		}
		if _, err := m.Move(id, ti, p.ID, layout.Center); err != nil {
			return nil, err
		}
		if first == nil {
			first = it
		}
	}
	if first != nil {
		return first, nil
	}
	app, running := e.runningAppLocked(bundleID)
	if running && !e.hasWindowLocked(app) {
		e.noteQuietLocked(bundleID, quietFocus)
		e.addPendingLocked(bundleID, t, p, false)
		pl := e.pending[len(e.pending)-1]
		e.after(launchSettle, func() { e.decideLaunchLocked(pl) })
		return nil, nil
	}
	return e.addAppLocked(t, p, bundleID)
}

// hasWindowLocked reports whether a running app has a window that could be
// an item.
func (e *Engine) hasWindowLocked(app platform.App) bool {
	ws, err := e.p.Windows(app)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(ws, func(w platform.Window) bool { return e.acceptable(withApp(w, app), app) })
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

// addFromTabLocked shows an item of other tabs in the pane as well.
func (e *Engine) addFromTabLocked(t *Tab, p *layout.Node, id string) (*Item, error) {
	m := e.model
	it := m.Items[id]
	if it == nil {
		return nil, errGone
	}
	added, err := m.AddToTab(id, m.IndexOf(t), p.ID)
	if err != nil {
		return nil, poppedOut(err, e.cfg.Settings.DragToAdd)
	}
	if !added {
		return nil, errAlreadyIn(t.Name)
	}
	return it, nil
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
	e.noteQuietLocked(bundleID, quietFocus)
	e.addPendingLocked(bundleID, t, p, true)
	return nil
}

// addAppLocked adds an installed app's windows that aren't in a tab to the
// pane: its popped-out windows, and the ones Bench doesn't manage. If it
// isn't running, it opens it, and the windows it opens as it launches that
// no remembered item claims go to the pane; if it has no such windows, it
// opens a new one.
func (e *Engine) addAppLocked(t *Tab, p *layout.Node, bundleID string) (*Item, error) {
	app, ok := e.runningAppLocked(bundleID)
	if !ok {
		return e.openAppForLocked(t, p, bundleID)
	}
	m := e.model
	var first *Item
	for _, id := range slices.Clone(m.Floating) {
		it := m.Items[id]
		if !isAppOf(it, bundleID) || it.WindowID == 0 {
			continue
		}
		m.Take(id)
		insertAt(p, id, -1)
		e.bringBackLocked(it)
		if first == nil {
			first = it
		}
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
	if len(free) == 0 && first == nil {
		if err := e.p.NewWindow(app); err != nil {
			return nil, fmt.Errorf("Bench couldn't open a new %s window.", e.appName(bundleID))
		}
		e.noteQuietLocked(bundleID, quietFocus)
		e.addPendingLocked(bundleID, t, p, true)
		return nil, nil
	}
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
// opens as it launches to the pane, apart from those that remembered items
// claim. If none comes to the pane, it opens a new window once the app has
// launched (see checkLaunchedLocked).
func (e *Engine) openAppForLocked(t *Tab, p *layout.Node, bundleID string) (*Item, error) {
	e.noteQuietLocked(bundleID, e.launchTimeout())
	if err := e.p.OpenApp(bundleID, ""); err != nil {
		return nil, fmt.Errorf("Bench couldn't open %s.", e.appName(bundleID))
	}
	e.addPendingLocked(bundleID, t, p, false)
	return nil, nil
}

// newTerminal is a terminal for the add dialog to add: its folder ("~" if
// empty), the command it types into the shell and runs, if any, whether it
// runs it again when Bench next opens, and its label until the shell sets a
// title.
type newTerminal struct {
	folder, run, name string
	startAgain        bool
}

// terminalOf returns the terminal that a terminal source asks for.
func terminalOf(source Source) newTerminal {
	return newTerminal{
		folder: strings.TrimSpace(source.Folder), run: strings.TrimSpace(source.Command),
		name: strings.TrimSpace(source.Name), startAgain: source.StartAgain == nil || *source.StartAgain,
	}
}

// addTerminalLocked adds a terminal. One with a command runs it as its
// session starts, which it does as it's shown, whatever startAgain says.
func (e *Engine) addTerminalLocked(t *Tab, p *layout.Node, nt newTerminal) *Item {
	folder := nt.folder
	if folder == "" {
		folder = defaultFolder
	}
	m := e.model
	it := &Item{ID: m.NewID(itemPrefix), Kind: KindBuiltin, App: terminalApp, Mono: apps.Mono(terminalApp),
		Title: terminalTitle(nt.name, folder), Name: nt.name, Folder: folder, Status: StatusReady,
		Run: nt.run, StartAgain: nt.startAgain && nt.run != "", runNow: nt.run != ""}
	m.Add(it, t, p, -1)
	return it
}

// addWebLocked adds a Web item for an address, which must be https, or
// http on this Mac (normaliseAddress).
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
// its own. The item is then listed under "Not in a tab". An App item with no
// window has nothing to pop out, so it's removed from every tab instead:
// the frontend calls this "Remove from Bench".
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
	inDock := false
	switch it.Kind {
	case KindApp:
		if it.WindowID == 0 {
			e.model.Remove(itemID)
			e.toastLocked("Removed " + e.labelOfLocked(it) + " from Bench.")
			e.arrangeLocked(arrangeOpts{})
			e.changed()
			return nil
		}
		// A window that the user minimised stays in the Dock, where it
		// can't be dragged onto a pane: the toast says so.
		inDock = it.minimised
		if !e.releaseWindowLocked(it) {
			e.toastLocked("Bench couldn't move " + e.labelOfLocked(it) + " back on screen yet. It will try again when it quits.")
		}
	case KindWeb:
		if err := e.popOutWebLocked(it); err != nil {
			return err
		}
	}
	e.finishPopOutLocked(it, inDock)
	return nil
}

// releaseWindowLocked lets an App item's window go free, out of Bench: it's
// put back on screen if it's parked, and the manager forgets it. If it
// can't be put back (it's on another desktop, or its app doesn't respond),
// the manager keeps it as parked, so that RestoreAll puts it back on quit,
// and releaseWindowLocked reports false.
func (e *Engine) releaseWindowLocked(it *Item) bool {
	it.Undocked = false
	it.minimised, it.restore = false, false
	it.Status, it.StatusText = StatusReady, ""
	if err := e.win.Unpark(it.WindowID); err != nil {
		e.log.Warn("couldn't put a window back on screen", "window", it.WindowID, "item", it.ID, "error", err)
		return false
	}
	e.win.Forget(it.WindowID)
	return true
}

// popOutWebLocked gives a Web item's view a window of its own, creating the
// view first if it has never been shown.
func (e *Engine) popOutWebLocked(it *Item) error {
	if !e.webMade[it.ID] {
		address, ok := e.webAddressLocked(it)
		if !ok {
			return fmt.Errorf("Bench can't pop out %s until it has loaded.", it.App)
		}
		view := platform.WebView{URL: address, Name: it.App, Profile: it.Profile, Zoom: it.Zoom}
		if err := e.p.ShowWebView(it.ID, view, layout.Rect{}); err != nil {
			return fmt.Errorf("Bench couldn't pop out %s.", e.labelOfLocked(it))
		}
		e.webMade[it.ID] = true
	}
	if err := e.p.PopOutWebView(it.ID, wholeLabelOf(it)); err != nil {
		return fmt.Errorf("Bench couldn't pop out %s.", e.labelOfLocked(it))
	}
	delete(e.webShown, it.ID)
	it.OwnWindow = true
	// Its window has no find bar: the browser's own Edit ▸ Find isn't
	// there either, so a find bar open in its pane closes.
	it.finding = false
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
	title := e.labelOfLocked(it)
	e.unlock()

	if err := e.em.PopOutTerminal(itemID, title); err != nil {
		return fmt.Errorf("Bench couldn't give the terminal a window of its own: %v.", err)
	}

	e.lock()
	defer e.unlock()
	it = e.model.Items[itemID]
	if it == nil {
		// The terminal ended while its window opened, and its removal
		// didn't know about the window.
		em := e.em
		e.later(func() { em.CloseTerminalWindow(itemID) })
		return nil
	}
	if e.model.IsFloating(itemID) {
		return nil
	}
	it.OwnWindow = true
	e.finishPopOutLocked(it, false)
	return nil
}

// TerminalWindowClosed is the shell's report that the user closed a
// popped-out terminal's window. As when a popped-out web page's window
// closes, the item goes, and its session ends.
func (e *Engine) TerminalWindowClosed(itemID string) {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	if e.closed || it == nil || it.Kind != KindBuiltin || !it.OwnWindow || !e.model.IsFloating(itemID) {
		return
	}
	it.OwnWindow = false // Its window has gone already.
	e.removeItemLocked(it)
	e.terms.Close(itemID)
}

// finishPopOutLocked moves a popped-out item to Floating. inDock is set for
// a window that's minimised, which can't be dragged onto a pane until it's
// back from the Dock.
func (e *Engine) finishPopOutLocked(it *Item, inDock bool) {
	e.model.Take(it.ID)
	e.model.Floating = append(e.model.Floating, it.ID)
	switch {
	case inDock:
		e.toastLocked("Moved " + e.labelOfLocked(it) + " out of Bench. Its window stays in the Dock. Add it back with +.")
	case e.cfg.Settings.DragToAdd:
		e.toastLocked("Moved " + e.labelOfLocked(it) + " out of Bench. To add it back, drag it onto a pane and wait a moment, or use +.")
	default:
		e.toastLocked("Moved " + e.labelOfLocked(it) + " out of Bench. Add it back with +.")
	}
	e.arrangeLocked(arrangeOpts{})
	e.changed()
}

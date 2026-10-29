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

// This file puts web apps in the place of Web items, from a Web item's menu
// (WebAppChoice, UseWebApp, MakeWebApp, and UseWebAppWhenReady): a web app
// runs in the browser, with its protections and sign-ins, which Web items
// don't have, so the user can switch a web page they have, such as the
// default config's Slack and Outlook, as the add dialog adds websites.

// What WebAppChoice offers (WebAppChoice.Offer).
const (
	offerUse  = "use"
	offerMake = "make"
)

// WebAppChoice says what a Web item's menu can offer to put a web app of its
// site in its place, worked out afresh each time:
//
//   - If a web app of the site is installed, as AddWebsite finds it by the
//     item's Home address, it offers that (UseWebApp).
//   - Else, if Bench has Accessibility permission, which it needs to dock
//     web apps, and the default browser can make them, it offers to have
//     the browser make one (MakeWebApp).
//   - Else, and for an item that isn't a web page in a tab, such as VS Code
//     for the Web or an App item, it offers nothing.
func (e *Engine) WebAppChoice(itemID string) (WebAppChoice, error) {
	// Scanning reads every app's Info.plist, so it's done without the lock.
	installed := e.p.InstalledApps()
	browser := e.p.DefaultBrowser()
	e.lock()
	defer e.unlock()
	it, err := e.webPageLocked(itemID)
	if errors.Is(err, errGone) {
		return WebAppChoice{}, err
	}
	if err != nil {
		return WebAppChoice{}, nil
	}
	e.setInstalledLocked(installed)
	if info, ok := webAppFor(installed, it.Home, browser); ok {
		return WebAppChoice{Offer: offerUse, App: apps.DisplayName(info.Name), Browser: info.WebApp}, nil
	}
	if maker, ok := apps.WebAppMakerFor(browser); ok && e.trusted {
		return WebAppChoice{Offer: offerMake, Browser: maker.Browser}, nil
	}
	return WebAppChoice{}, nil
}

// UseWebApp puts the installed web app of a Web item's site, as
// WebAppChoice offers it, in the item's place (see
// replaceWithWebAppLocked). If the item stands for entries in config.toml,
// it saves the file with App entries for the web app in their place, as Tab
// properties saves it.
func (e *Engine) UseWebApp(itemID string) error {
	installed := e.p.InstalledApps()
	browser := e.p.DefaultBrowser()
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	it, err := e.webPageLocked(itemID)
	if err != nil {
		e.unlock()
		return err
	}
	e.setInstalledLocked(installed)
	info, ok := webAppFor(installed, it.Home, browser)
	if !ok {
		e.unlock()
		return errNoWebApp(hostOf(it.Home))
	}
	saved, err := e.replaceWithWebAppLocked(it, info.BundleID, false)
	writable := e.writable
	e.unlock()
	if err != nil || saved == nil {
		return err
	}
	// The notices say if config.toml wasn't saved.
	_, err = e.saveEdit(saved, writable)
	return err
}

// MakeWebApp has the default browser make a web app of a Web item's site, to
// put in the item's place: it opens the item's Home address there and
// returns the browser's steps, and while the user follows them, Bench
// waits as AddWebsite does (see startMakingLocked), until
// UseWebAppWhenReady finds the web app. It refuses without Accessibility
// permission, which Bench needs to dock web apps, or if the browser can't
// make them.
func (e *Engine) MakeWebApp(itemID string) (WebsiteResult, error) {
	installed := e.p.InstalledApps()
	browser := e.p.DefaultBrowser()
	e.lock()
	defer e.unlock()
	it, err := e.webPageLocked(itemID)
	if err != nil {
		return WebsiteResult{}, err
	}
	e.setInstalledLocked(installed)
	maker, ok := apps.WebAppMakerFor(browser)
	switch {
	case !e.trusted:
		return WebsiteResult{}, errNeedsTrust
	case !ok:
		return WebsiteResult{}, e.cantMakeLocked(browser)
	}
	return e.startMakingLocked(makingWebApp{itemID: it.ID, address: it.Home, browser: browser}, maker, installed)
}

// UseWebAppWhenReady puts the web app of a Web item's site in the item's
// place, as UseWebApp does, if one is installed now, and reports whether it
// did. The dialog that MakeWebApp opens calls it every second or two while
// the default browser makes the web app. As for AddWebAppWhenReady, the web
// app may be on another host, so once MakeWebApp has opened the item's site,
// the one web app installed since then is the site's too. If the item has
// closed, or left its tabs, Bench stops waiting, and it refuses. It reports
// true once the web app is in place, even if config.toml then can't be
// saved, which its error says.
func (e *Engine) UseWebAppWhenReady(itemID string) (bool, error) {
	installed := e.p.InstalledApps()
	browser := e.p.DefaultBrowser()
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	it, err := e.webPageLocked(itemID)
	if err != nil {
		if m := e.making; m != nil && m.itemID == itemID {
			e.making = nil
		}
		e.unlock()
		return false, err
	}
	e.setInstalledLocked(installed)
	m := e.makingForLocked("", itemID, it.Home)
	info, ok := webAppFor(installed, it.Home, browser)
	if !ok {
		info, ok = newWebApp(installed, m)
	}
	if !ok {
		e.stillMakingLocked(browser, m)
		e.unlock()
		return false, nil
	}
	making := e.making
	e.making = nil
	saved, err := e.replaceWithWebAppLocked(it, info.BundleID, true)
	if err != nil {
		e.making = making
	}
	writable := e.writable
	e.unlock()
	if err != nil {
		return false, err
	}
	if saved == nil {
		return true, nil
	}
	// The notices say if config.toml wasn't saved.
	_, err = e.saveEdit(saved, writable)
	return true, err
}

// webPageLocked returns a Web item that a web app can replace: a web page
// of a site, not VS Code for the Web, in a tab, where the web app goes.
func (e *Engine) webPageLocked(itemID string) (*Item, error) {
	m := e.model
	it := m.Items[itemID]
	switch {
	case it == nil:
		return nil, errGone
	case it.Kind != KindWeb || it.VSCode != "" || !secureAddress(it.Home):
		return nil, errNotWebPage
	case len(m.TabsOf(itemID)) == 0:
		return nil, errPoppedOut
	}
	return it, nil
}

// cantMakeLocked says why the default browser, with bundle ID browser ("" if
// there's none), can't make a web app for MakeWebApp: it can't make them, or
// Bench doesn't know how it does.
func (e *Engine) cantMakeLocked(browser string) error {
	switch {
	case apps.CantMakeWebApps(browser):
		return fmt.Errorf("%s can't make web apps.", e.appName(browser))
	case browser != "":
		return fmt.Errorf("Bench doesn't know how %s makes web apps.", e.appName(browser))
	}
	return errNoBrowser
}

// replaceWithWebAppLocked puts a new App item for a web app in a Web item's
// place, in every tab that holds it (see Model.Replace), and closes the Web
// item's view. The web app's window comes to the new
// item, as addAppLocked and addMadeWebAppLocked find their windows:
//
//   - For a web app the user has just made (made), the window that Bench
//     has put in the current tab already, by new_windows, as a Chromium
//     browser opens the web app it makes. If it was shown in the Web item's
//     pane, the new item is.
//   - Else one that's popped out of Bench, or else that Bench doesn't
//     manage, the oldest first.
//   - Else, if the web app is running without a window, as it is just
//     after it starts, its first window, or, if none comes soon, a new one.
//   - Else, if it's running, a new window; and if it isn't, it's opened,
//     and its first window that no remembered item claims comes to the
//     item, or, if none does, a new one (see decideLaunchLocked).
//
// Until the window comes, the item says Bench is opening the web app. If
// Bench can't open the web app or a window, it refuses, leaving the Web item
// where it is.
//
// If the Web item stands for entries in config.toml, they become App entries
// for the web app, in the same place (see configWithWebAppLocked), so that
// the web page doesn't come back when the tab's layout is reset. It then
// holds state.json, and returns the config to save with saveEdit; else nil.
func (e *Engine) replaceWithWebAppLocked(web *Item, bundleID string, made bool) (*config.Config, error) {
	m := e.model
	ti, p := m.Where(web.ID)
	if p == nil {
		return nil, errPoppedOut
	}
	t := m.Tabs[ti]
	it := &Item{ID: m.NewID(itemPrefix), Kind: KindApp, BundleID: bundleID, Status: StatusMissing}
	e.nameItemLocked(it)
	it.Title = it.App
	from, w, found := e.webAppWindowLocked(bundleID, made)
	if !found {
		if err := e.openWebAppForLocked(it, t, p); err != nil {
			return nil, err
		}
	}
	cfg := e.configWithWebAppLocked(web, bundleID)
	if from != nil {
		// A window that new_windows showed in the Web item's pane was shown
		// instead of the Web item.
		for _, tj := range m.TabsOf(web.ID) {
			if q := m.PaneIn(tj, web.ID); q.Active == from.ID {
				q.Active = web.ID
			}
		}
		m.Remove(from.ID)
	}
	m.Replace(web.ID, it)
	e.closeWebViewLocked(web)
	if found {
		delete(e.unmanaged, w.ID)
		m.Bind(it, w)
		e.nameItemLocked(it)
	}
	if cfg != nil {
		e.cfg = cfg
	}
	e.arrangeLocked(arrangeOpts{})
	if ti, _ := m.Where(it.ID); ti == m.Current {
		e.focusShownLocked()
	}
	e.changed()
	if cfg == nil {
		return nil, nil
	}
	e.holdStateLocked()
	return cfg.Clone(), nil
}

// webAppWindowLocked returns a web app's window that the item replacing a
// Web item can have at once (see replaceWithWebAppLocked): for a web app
// just made (made), one that's an item of the current tab; else one that's
// popped out of Bench, with its item, or else one that Bench doesn't
// manage. It reports false if there's none.
func (e *Engine) webAppWindowLocked(bundleID string, made bool) (*Item, platform.Window, bool) {
	m := e.model
	var ids []string
	if made {
		ids = m.CurrentTab().ItemIDs()
	}
	for _, id := range append(ids, m.Floating...) {
		if it := m.Items[id]; isAppOf(it, bundleID) && it.WindowID != 0 {
			return it, windowOf(it), true
		}
	}
	app, running := e.runningAppLocked(bundleID)
	if !running {
		return nil, platform.Window{}, false
	}
	var free []platform.Window
	for _, w := range e.unmanaged {
		if sameApp(w.BundleID, bundleID) {
			free = append(free, w)
		}
	}
	free = append(free, e.newWindowsOfLocked(app)...)
	if len(free) == 0 {
		return nil, platform.Window{}, false
	}
	sortByCreated(free)
	return nil, free[0], true
}

// openWebAppForLocked has a web app's window come to an item that's to be
// put in pane p of tab t, when the web app has no window for it now (see
// replaceWithWebAppLocked): it waits for the first window of a web app
// that's running without one, or opens a new window, or opens the web app.
// The window binds to the item (see placePendingLocked), which says Bench
// is opening the web app until then.
func (e *Engine) openWebAppForLocked(it *Item, t *Tab, p *layout.Node) error {
	bundleID := it.BundleID
	pend := func(once bool) pendingPlace {
		e.addPendingLocked(bundleID, t, p, once)
		e.pending[len(e.pending)-1].item = it.ID
		return e.pending[len(e.pending)-1]
	}
	app, running := e.runningAppLocked(bundleID)
	switch {
	case !running:
		e.noteQuietLocked(bundleID, e.launchTimeout())
		if err := e.p.OpenApp(bundleID, ""); err != nil {
			return fmt.Errorf("Bench couldn't open %s.", it.App)
		}
		pend(false)
	case !e.hasWindowLocked(app):
		e.noteQuietLocked(bundleID, quietFocus)
		pl := pend(false)
		e.after(launchSettle, func() { e.decideLaunchLocked(pl) })
	default:
		if err := e.p.NewWindow(app); err != nil {
			return fmt.Errorf("Bench couldn't open a new %s window.", it.App)
		}
		e.noteQuietLocked(bundleID, quietFocus)
		pend(true)
	}
	// The item says Bench is opening the web app until its window comes,
	// or until the launch timeout, when it's hidden if it has none.
	until := e.clk.Now().Add(e.launchTimeout())
	e.launching[strings.ToLower(bundleID)] = until
	e.arrangeWhenOpeningEndsLocked(it.ID, until)
	return nil
}

// configWithWebAppLocked returns a copy of the config in which the entries
// that a Web item stands for, in the tabs that hold it, are App entries for
// a web app, in the same place; or nil if no tab's entries include it. In a
// tab, the item stands for the entry with its address that's in the same
// place among the tab's entries with that address as the item is among the
// tab's items with it, as syncEntries pairs them. An entry whose pane lists
// the web app already goes instead.
func (e *Engine) configWithWebAppLocked(web *Item, bundleID string) *config.Config {
	m := e.model
	keys := itemKeys(web)
	if len(keys) == 0 {
		return nil
	}
	// sameEntry reports whether an item was made for the Web item's entry,
	// by its first key, as entryItems tells.
	sameEntry := func(it *Item) bool {
		k := itemKeys(it)
		return len(k) > 0 && k[0] == keys[0]
	}
	cfg := e.cfg.Clone()
	changed := false
	for _, ti := range m.TabsOf(web.ID) {
		if ti >= len(cfg.Tabs) || !strings.EqualFold(cfg.Tabs[ti].Name, m.Tabs[ti].Name) {
			continue
		}
		n := 0
		for _, id := range m.Tabs[ti].ItemIDs() {
			if id == web.ID {
				break
			}
			if it := m.Items[id]; it != nil && sameEntry(it) {
				n++
			}
		}
		if replaceEntry(&cfg.Tabs[ti], keys[0], n, config.Item{App: bundleID}) {
			changed = true
		}
	}
	if !changed || len(cfg.Validate()) > 0 {
		return nil
	}
	return cfg
}

// replaceEntry replaces a config tab's nth entry with a key (counting from
// 0, in reading order) with another entry, or removes it if its pane lists
// that entry already. It reports false if the tab has no such entry.
func replaceEntry(ct *config.Tab, key string, n int, with config.Item) bool {
	for j := range ct.Panes {
		items := ct.Panes[j].Items
		for i, ci := range items {
			if entryKey(ci) != key {
				continue
			}
			if n > 0 {
				n--
				continue
			}
			if slices.ContainsFunc(items, func(o config.Item) bool { return entryKey(o) == entryKey(with) }) {
				ct.Panes[j].Items = slices.Delete(items, i, i+1)
			} else {
				items[i] = with
			}
			return true
		}
	}
	return false
}

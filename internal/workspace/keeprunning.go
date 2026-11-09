package workspace

import (
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// This file keeps chat and mail pages running (decision 76). Bench makes
// a page's web view the first time the page is shown, so a page in a tab
// not shown since Bench started posts no notifications and has no unread
// count. A page with Keep Running in the Background on (keep_running in
// config.toml) loads, hidden, soon after Bench starts, one at a time, and
// stays loaded, as a page that has been shown does. Bench turns it on
// itself the first time a page posts a notification, and says so, with
// Undo. Bench never unloads a page: that waits for a measurement of what
// pages kept running cost in memory.

const (
	// preloadDelay is how long after the first arrangement with a pane
	// area Bench waits to load the first page kept running in the
	// background, so that the tab the user sees comes first.
	preloadDelay = 3 * time.Second
	// preloadGap is how long Bench waits after loading one such page to
	// load the next, so that they don't all load at once.
	preloadGap = 2 * time.Second
)

var (
	// errKeepRunningPage refuses SetKeepRunning for an item that isn't
	// a web page, such as VS Code for the Web, which has its own server,
	// and errKeepRunningTab for a page that's in no tab.
	errKeepRunningPage = errors.New("Only web pages can keep running in the background.")
	errKeepRunningTab  = errors.New("Only a page in a tab can keep running in the background.")
	// errNoDemoPages and errNoDemoPage refuse DemoNotify.
	errNoDemoPages = errors.New("Simulated notifications are only for the browser preview.")
	errNoDemoPage  = errors.New("The current tab shows no web page to post a notification.")
)

// keepable reports whether an item is a web page that can keep running in
// the background: not VS Code for the Web, whose page needs its server.
func keepable(it *Item) bool {
	return it != nil && it.Kind == KindWeb && it.VSCode == ""
}

// preloadSoonLocked schedules the next load of a page kept running in the
// background, if one is waiting and none is scheduled: preloadDelay after
// the first arrangement with a pane area, and preloadGap after that.
func (e *Engine) preloadSoonLocked() {
	if e.opts.Headless || !e.started || e.closed || e.area.W <= 0 || e.area.H <= 0 {
		return
	}
	first := !e.preloading
	e.preloading = true
	if e.preloadTimer != nil || e.nextPreloadLocked() == nil {
		return
	}
	delay := preloadGap
	if first {
		delay = preloadDelay
	}
	e.preloadTimer = e.after(delay, e.preloadNextLocked)
}

// preloadNextLocked loads the next page kept running in the background,
// then schedules the one after it.
func (e *Engine) preloadNextLocked() {
	e.preloadTimer = nil
	if it := e.nextPreloadLocked(); it != nil {
		e.preloadTried[it.ID] = true
		e.loadWebLocked(it)
	}
	e.preloadSoonLocked()
}

// nextPreloadLocked returns the next page to load in the background: the
// first page with keep_running on that has no web view yet, and that
// Bench hasn't tried to load, in the current tab first, then in tab order.
func (e *Engine) nextPreloadLocked() *Item {
	m := e.model
	order := make([]int, 0, len(m.Tabs))
	if m.CurrentTab() != nil {
		order = append(order, m.Current)
	}
	for ti := range m.Tabs {
		if ti != m.Current {
			order = append(order, ti)
		}
	}
	for _, ti := range order {
		for _, id := range m.Tabs[ti].ItemIDs() {
			if it := m.Items[id]; e.waitsToPreloadLocked(it) {
				return it
			}
		}
	}
	return nil
}

// waitsToPreloadLocked reports whether a page kept running in the
// background has no web view yet, and Bench hasn't tried to load it.
func (e *Engine) waitsToPreloadLocked(it *Item) bool {
	return keepable(it) && it.KeepRunning && !it.OwnWindow && !e.webMade[it.ID] && !e.preloadTried[it.ID]
}

// notLoadedLocked reports whether a web page has no web view, and won't
// get one until the user shows it: it isn't kept running in the
// background, or Bench couldn't load it. Its sub-tab says so (ItemView's
// NotLoaded), as it posts no notifications and has no unread count until
// then. A page waiting to load in the background soon isn't counted.
func (e *Engine) notLoadedLocked(it *Item) bool {
	return keepable(it) && !e.opts.Headless && !it.OwnWindow && !e.webMade[it.ID] && !e.waitsToPreloadLocked(it)
}

// loadWebLocked makes a page's web view, hidden, the size of the pane
// area, and loads its address (platform.LoadWebView), which keeps it
// running while it's hidden. Showing it later moves it to its pane.
func (e *Engine) loadWebLocked(it *Item) {
	address, ok := e.webAddressLocked(it)
	if !ok {
		return
	}
	view := platform.WebView{URL: address, Name: it.App, Profile: it.Profile, Zoom: it.Zoom}
	if err := e.p.LoadWebView(it.ID, view, e.area); err != nil {
		e.log.Warn("couldn't load a page kept running in the background", "item", it.ID, "error", err)
		e.touch()
		return
	}
	e.webMade[it.ID] = true
	e.webKept[it.ID] = true
	e.touch()
}

// keepWebViewsLocked tells the platform which web views keep running while
// they're hidden (Platform.KeepWebViewRunning), where that has changed
// since it last did: those of the pages with Keep Running in the
// Background on. On the Mac, such a page loads again by itself after its
// process ends, or its first load fails, rather than wait to be shown.
func (e *Engine) keepWebViewsLocked() {
	for _, id := range slices.Sorted(maps.Keys(e.webMade)) {
		it := e.model.Items[id]
		on := keepable(it) && it.KeepRunning
		if e.webKept[id] == on {
			continue
		}
		if on {
			e.webKept[id] = true
		} else {
			delete(e.webKept, id)
		}
		e.p.KeepWebViewRunning(id, on)
	}
}

// keepRunningName names the change that a page's first notification makes
// (keepNotifierLocked), for Undo away from its toast.
func keepRunningName(app string) undoName {
	return undoName{"Keep Running in the Background", "kept " + app + " running in the background"}
}

// keepNotifierLocked turns Keep Running in the Background on for a page in
// a tab the first time it posts a notification, unless Bench or the user
// has set it already, so that its notifications and its unread count come
// after Bench starts again too. It says so once, and saves the setting.
// It's a change that Undo takes back (keepOn), from its toast, Tab ▸ Undo,
// and the recent messages, which turns it off as the user's choice. A
// first notification most often comes while the user is in another app, so
// the toast waits for Bench to be the active app (keepToasts). The event
// loop mustn't wait for cfgMu (see Close), so config.toml is saved, and the
// toast sent, on the clock's goroutine.
func (e *Engine) keepNotifierLocked(it *Item) {
	if !keepable(it) || it.KeepChosen || e.opts.Headless || len(e.model.TabsOf(it.ID)) == 0 {
		return
	}
	step := &undoStep{keepOn: it.ID, items: maps.Clone(e.model.Items), kept: map[string]*Item{}}
	it.KeepRunning, it.KeepChosen = true, true
	e.keepWebViewsLocked()
	e.changed()
	n := e.keepUndoLocked(step, keepRunningName(it.App), it.App+keepOffText)
	id, msg := it.ID, it.App+" will keep running in the background, so it can notify you."
	e.clk.AfterFunc(0, func() {
		e.cfgMu.Lock()
		defer e.cfgMu.Unlock()
		e.lock()
		page := e.model.Items[id]
		if e.closed || page == nil || !page.KeepRunning {
			e.unlock()
			return
		}
		saved, writable := e.keepRunningConfigLocked(page)
		e.unlock()
		toast := undoToast(e.keepRunningText(msg, saved, writable), n)
		e.lock()
		if !e.benchActive && !e.opts.Demo {
			e.keepToasts = append(e.keepToasts, toast)
			e.unlock()
			return
		}
		e.unlock()
		e.em.ActionToast(toast)
	})
}

// sendKeepToastsLocked sends the toasts that waited for Bench to be the
// active app (keepNotifierLocked), in order. One whose change Undo can no
// longer take back has no Undo.
func (e *Engine) sendKeepToastsLocked() {
	toasts := e.keepToasts
	e.keepToasts = nil
	if len(toasts) == 0 {
		return
	}
	for i, t := range toasts {
		if !slices.ContainsFunc(e.undo, func(s *undoStep) bool { return s.id == t.Action.Arg }) {
			toasts[i].Action = ToastAction{}
		}
	}
	em := e.em
	e.later(func() {
		for _, t := range toasts {
			if t.Action.Method == "" {
				em.Toast(t.Text)
			} else {
				em.ActionToast(t)
			}
		}
	})
}

// keepOffText is what's said once a page's Keep Running in the Background
// is turned off: the page stays loaded until Bench quits, as Bench never
// unloads a page.
const keepOffText = " will load only when you show it, from the next time Bench opens."

// SetKeepRunning turns a web page's Keep Running in the Background on or
// off, from its ⋯ menu or Pane ▸ Page, and saves it in config.toml, with
// the page's entries, and in state.json. A page turned on that has no web
// view yet loads soon, hidden. One turned off stays loaded until Bench
// quits, and doesn't load at the next start until it's shown. Either way,
// it's the user's choice: a notification no longer changes it, and Undo no
// longer takes back the notification's change (keepOn).
func (e *Engine) SetKeepRunning(itemID string, on bool) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	it := e.model.Items[itemID]
	switch {
	case e.closed || it == nil:
		e.unlock()
		return errGone
	case !keepable(it):
		e.unlock()
		return errKeepRunningPage
	case len(e.model.TabsOf(it.ID)) == 0:
		e.unlock()
		return errKeepRunningTab
	}
	it.KeepRunning, it.KeepChosen = on, true
	if n := len(e.undo); n > 0 {
		e.undo = slices.DeleteFunc(e.undo, func(s *undoStep) bool { return s.keepOn == it.ID })
		if len(e.undo) != n {
			e.touch()
		}
	}
	e.keepWebViewsLocked()
	e.changed()
	e.preloadSoonLocked()
	msg := it.App + " will keep running in the background."
	if !on {
		msg = it.App + keepOffText
	}
	saved, writable := e.keepRunningConfigLocked(it)
	e.unlock()
	e.em.Toast(e.keepRunningText(msg, saved, writable))
	return nil
}

// keepRunningConfigLocked puts a page's keep_running in its entries in
// config.toml, in every tab whose entries include it, and returns the
// config to save, with whether config.toml can be written, or nil if none
// do, or they have it already: a page added in Bench has no entry, and
// state.json alone keeps it.
func (e *Engine) keepRunningConfigLocked(page *Item) (*config.Config, bool) {
	cfg := e.cfg.Clone()
	if !keepRunningInConfig(cfg, e.model, page) || len(cfg.Validate()) > 0 {
		return nil, false
	}
	e.cfg = cfg
	return cfg.Clone(), e.writable
}

// keepRunningInConfigLocked puts each web page's keep_running, as the
// model has it, in its entries in cfg (keepRunningInConfig).
func (e *Engine) keepRunningInConfigLocked(cfg *config.Config) {
	for _, id := range slices.Sorted(maps.Keys(e.model.Items)) {
		if it := e.model.Items[id]; keepable(it) {
			keepRunningInConfig(cfg, e.model, it)
		}
	}
}

// keepRunningInConfig puts a page's keep_running, as the model has it, in
// its entries in cfg, in every tab of the model whose entries include it,
// and reports whether that changed any.
func keepRunningInConfig(cfg *config.Config, m *Model, page *Item) bool {
	keys := itemKeys(page)
	if len(keys) == 0 {
		return false
	}
	want := keepRunningField(page)
	changed := false
	for _, ti := range m.TabsOf(page.ID) {
		if !sameConfigTab(cfg, m, ti) {
			continue
		}
		ct := &cfg.Tabs[ti]
		j, i, ok := nthEntry(ct, keys[0], m.entryPlace(ti, page))
		if !ok {
			continue
		}
		if ci := &ct.Panes[j].Items[i]; !samePtr(ci.KeepRunning, want) {
			ci.KeepRunning = want
			changed = true
		}
	}
	return changed
}

// samePtr reports whether two optional values are both missing or the
// same.
func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// keepRunningText saves config.toml, if saved isn't nil (see
// keepRunningConfigLocked), and returns a toast's text, msg, saying why
// it wasn't saved, if it wasn't. The caller holds cfgMu, not the lock.
func (e *Engine) keepRunningText(msg string, saved *config.Config, writable bool) string {
	if saved == nil {
		return msg
	}
	notSaved, err := e.saveEdit(saved, writable)
	switch {
	case err != nil:
		return msg + " " + err.Error() + "."
	case notSaved != "":
		return msg + " " + notSavedText(notSaved)
	}
	return msg
}

// demoEvents is what the browser preview's platform adds, for simulating
// what pages do.
type demoEvents interface {
	EmitEvent(ev platform.Event)
}

// DemoNotify has a page post a notification, as Slack's would for a new
// message, in the browser preview, whose pages are placeholders: the page
// the focused pane shows, or else the first page the current tab shows.
// It needs the preview's platform (platform.NewDemo).
func (e *Engine) DemoNotify() error {
	de, ok := e.p.(demoEvents)
	if !ok || !e.opts.Demo {
		return errNoDemoPages
	}
	e.lock()
	d := e.displayLocked()
	panes := d.panes
	if g, ok := d.focusedGeom(); ok {
		panes = append([]paneGeom{g}, panes...)
	}
	var ev platform.Event
	for _, g := range panes {
		if it := e.model.Items[g.node.Active]; keepable(it) {
			ev = platform.Event{Kind: platform.WebNotification, ItemID: it.ID, Title: it.App, Body: "A new message"}
			break
		}
	}
	e.unlock()
	if ev.ItemID == "" {
		return errNoDemoPage
	}
	de.EmitEvent(ev)
	return nil
}

// forgetPreloadLocked lets a page be loaded in the background again once
// its web view has closed, and forgets whether that web view was kept
// running.
func (e *Engine) forgetPreloadLocked(id string) {
	delete(e.preloadTried, id)
	delete(e.webKept, id)
}

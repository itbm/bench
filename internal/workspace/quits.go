package workspace

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// This file keeps Bench from fighting the user's quits. It remembers the
// apps the user quit, and the ones that opened no window when Bench opened
// them, so that tab switches leave them alone, and it opens an app for one
// sub-tab when the user asks (OpenItem).

// logOutGrace is how long after macOS says the user is logging out,
// restarting, or shutting down, apps that quit are taken to be quitting
// because of it, not because the user quit them, and Bench quits without
// asking about its terminals (AskToQuit).
const logOutGrace = platform.LogOutGrace

// openKey identifies how Bench opens an app for an item: the app, by
// lower-case bundle ID, and the url it opens.
func openKey(it *Item) string { return strings.ToLower(it.BundleID) + "\x00" + it.OpenURL }

// loadAppStates restores what state.json remembers about apps, as of when
// Bench last ran (ran, or zero if the file doesn't say). The time Bench was
// off since then doesn't count towards how long an app has gone unseen: the
// user wasn't using Bench, nor turning away from its apps.
func (e *Engine) loadAppStates(apps map[string]config.AppState, ran time.Time) {
	off := time.Duration(0)
	if now := e.clk.Now(); !ran.IsZero() && now.After(ran) {
		off = now.Sub(ran)
	}
	for key, a := range apps {
		key = strings.ToLower(key)
		if a.Quit {
			e.quit[key] = true
		}
		if !a.Seen.IsZero() {
			e.seen[key] = a.Seen.Add(off)
		}
	}
}

// appStatesLocked returns what state.json remembers about apps: for each
// app with items, whether the user quit it, and when Bench last saw it
// running, which is now for one that's running. It's nil if there's
// nothing to remember.
func (e *Engine) appStatesLocked() map[string]config.AppState {
	var out map[string]config.AppState
	now := e.clk.Now()
	for _, it := range e.model.Items {
		if it.Kind != KindApp {
			continue
		}
		key := strings.ToLower(it.BundleID)
		if _, done := out[key]; done {
			continue
		}
		a := config.AppState{Quit: e.quit[key], Seen: e.seen[key]}
		if _, running := e.running[key]; running {
			a.Seen = now
		}
		if !a.Quit && a.Seen.IsZero() {
			continue
		}
		if out == nil {
			out = map[string]config.AppState{}
		}
		out[key] = a
	}
	return out
}

// hasAppItemsLocked reports whether an app has App items.
func (e *Engine) hasAppItemsLocked(bundleID string) bool {
	for _, it := range e.model.Items {
		if isAppOf(it, bundleID) {
			return true
		}
	}
	return false
}

// noteRunningAppsLocked records that the running apps with items are
// running: the user didn't leave them closed, and they've been seen now.
func (e *Engine) noteRunningAppsLocked() {
	now := e.clk.Now()
	for key := range e.running {
		delete(e.quit, key)
		if e.hasAppItemsLocked(key) {
			e.seen[key] = now
		}
	}
}

// appOpenedLocked records that an app has opened, whoever opened it: it's
// no longer one the user left closed, and Bench may ask it for windows
// again.
func (e *Engine) appOpenedLocked(key string) {
	if e.quit[key] {
		delete(e.quit, key)
		e.changed()
	}
	e.forgetNoWindowLocked(key)
	if e.hasAppItemsLocked(key) {
		e.seen[key] = e.clk.Now()
	}
}

// forgetNoWindowLocked forgets that an app opened no window for an item,
// and when switches asked it for one, for every url Bench opened it with.
// An ask from a moment ago, before the launch timeout, stays: the app may
// be opening to answer it.
func (e *Engine) forgetNoWindowLocked(key string) {
	for k := range e.noWindow {
		if app, _, _ := strings.Cut(k, "\x00"); app == key {
			delete(e.noWindow, k)
		}
	}
	since := e.clk.Now().Add(-e.launchTimeout())
	for k, rec := range e.askedFor {
		if app, _, _ := strings.Cut(k, "\x00"); app == key && !rec.at.After(since) {
			delete(e.askedFor, k)
		}
	}
}

// appQuitLocked records that an app with items has quit: the user quit it,
// unless the user is logging out, restarting, or shutting down, when Bench
// should open it again at the next login as it would have.
func (e *Engine) appQuitLocked(key string) {
	if !e.hasAppItemsLocked(key) {
		return
	}
	now := e.clk.Now()
	e.seen[key] = now
	if now.Before(e.loggingOut) {
		return
	}
	e.quit[key] = true
	e.changed()
}

// onLoggingOut records that the user is logging out, restarting, or
// shutting down, so that the quits that follow aren't taken for the user's.
func (e *Engine) onLoggingOut() {
	e.loggingOut = e.clk.Now().Add(logOutGrace)
	e.log.Debug("logging out: quits that follow aren't the user's")
}

// OpenItem opens the app of an App item without a window, in the current
// tab, for the user, and puts its window in the item's place, showing it
// there. Bench opens it even if the user quit it, or if macOS said it isn't
// installed, or it opened no window for the item before. If the app is
// running, Bench asks it for a new window (or opens the item's url in it),
// which goes to the item ahead of anything else. If it isn't, Bench opens
// it: a window it restores that has the title of another place goes there,
// and the item gets the first of the rest, ahead of places that only the
// order the windows were made in would give them; if none comes, Bench asks
// for a new window once the app has opened, as the add dialog does.
func (e *Engine) OpenItem(itemID string) error {
	e.lock()
	defer e.unlock()
	m := e.model
	it := m.Items[itemID]
	if it == nil {
		return errGone
	}
	t := m.CurrentTab()
	p := m.PaneIn(m.Current, itemID)
	if p == nil || it.Kind != KindApp {
		return errNotInPane
	}
	m.Show(itemID)
	if it.WindowID != 0 {
		e.arrangeLocked(arrangeOpts{})
		e.focusShownLocked()
		e.changed()
		return nil
	}
	key := strings.ToLower(it.BundleID)
	delete(e.quit, key)
	delete(e.notFound, key)
	delete(e.openFail, key)
	e.forgetNoWindowLocked(key)
	name := e.appName(it.BundleID)
	if !e.trusted {
		// Bench can't place the app's windows yet, but it can open the app.
		e.changed()
		return e.openForItemLocked(it, name)
	}
	app, running := e.runningAppLocked(it.BundleID)
	if running {
		e.noteQuietLocked(it.BundleID, quietFocus)
		var err error
		if it.OpenURL != "" {
			err = e.p.OpenApp(it.BundleID, it.OpenURL)
		} else {
			err = e.p.NewWindow(app)
		}
		if err != nil {
			e.changed()
			return fmt.Errorf("Bench couldn't open a new %s window.", name)
		}
		e.addPendingLocked(it.BundleID, t, p, true)
	} else {
		e.noteQuietLocked(it.BundleID, e.launchTimeout())
		if err := e.openForItemLocked(it, name); err != nil {
			e.changed()
			return err
		}
		e.addPendingLocked(it.BundleID, t, p, false)
	}
	until := e.clk.Now().Add(e.launchTimeout())
	e.pending[len(e.pending)-1].item = itemID
	e.launching[key] = until
	e.waitForLocked(t, map[string]bool{itemID: true})
	e.arrangeLocked(arrangeOpts{})
	e.arrangeWhenOpeningEndsLocked(itemID, until)
	e.changed()
	return nil
}

// openForItemLocked opens the app of an App item, for OpenItem, and
// records why it couldn't: macOS says it isn't installed, or it failed.
func (e *Engine) openForItemLocked(it *Item, name string) error {
	key := strings.ToLower(it.BundleID)
	err := e.p.OpenApp(it.BundleID, it.OpenURL)
	switch {
	case errors.Is(err, platform.ErrNotInstalled):
		e.notFound[key] = true
		return fmt.Errorf("Bench couldn't open %s, because it isn't installed.", name)
	case err != nil:
		e.openFail[key] = true
		return fmt.Errorf("Bench couldn't open %s.", name)
	}
	return nil
}

// waitForLocked has the current wait, or a new one, wait for more items of
// tab t to get windows, as a tab switch waits for the apps it opened, and
// focus the focused pane once what it shows is ready.
func (e *Engine) waitForLocked(t *Tab, opened map[string]bool) {
	w := e.wait
	if w == nil || w.tab != t {
		e.cancelWaitLocked()
		e.waitGen++
		w = &waitState{gen: e.waitGen, tab: t, opened: map[string]bool{}}
		e.wait = w
	} else if w.timer != nil {
		w.timer.Stop()
	}
	for id := range opened {
		w.opened[id] = true
	}
	// The wait starts again, for the items just added, and the focus is the
	// user's newest. The items it was waiting for already keep showing that
	// their apps are opening until it ends (see openingForLocked).
	w.start = e.clk.Now()
	until := w.start.Add(e.launchTimeout())
	for _, it := range e.stillWaitingLocked(w) {
		key := strings.ToLower(it.BundleID)
		if at, ok := e.launching[key]; ok && at.Before(until) {
			e.launching[key] = until
		}
	}
	w.focus = true
	w.focusPane, w.focusItem = e.focusTargetLocked()
	e.scheduleCheckLocked(checkEvery)
}

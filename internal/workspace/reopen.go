package workspace

import (
	"slices"
	"strings"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file follows apps as they open again, and forgets the places they
// don't fill. A remembered place (an App item without a window that had
// one) takes a window of its app with its exact title at any time, but one
// by the order the windows were made in only while the app is opening
// again: from its launch, or from Bench's scan at startup, for a while
// after its windows stop coming. Then the places it didn't fill are
// forgotten, apart from one for each App entry of the config in each pane,
// so that later windows go where new_windows says. Nothing is forgotten
// while the app isn't running, or until Bench has seen one of its windows.

// How long, in launch timeouts, an app counts as opening again: at least
// reopenAfterLaunch after it launches, at least reopenAfterScan after
// Bench's scan (at startup or login, or when Accessibility permission
// arrives), whenever the app launched, and at least reopenAfterWindow after
// each of its new windows. Each lasts at least reopenLeast times as many
// seconds as launch timeouts, so that a short launch_timeout, which makes
// tab switches give up sooner, doesn't cut short apps that restore their
// windows slowly.
const (
	reopenAfterLaunch = 2
	reopenAfterScan   = 4
	reopenAfterWindow = 2
	reopenLeast       = 15 * time.Second
)

// The places of an app that hasn't run for staleAfter, while Bench was
// running, are forgotten, as they would have been had it opened again.
// Bench checks a while after it starts, and then every staleEvery.
const (
	staleAfter = 30 * 24 * time.Hour
	staleEvery = time.Hour
)

// reopenState follows an app as it opens again.
type reopenState struct {
	since time.Time // When it started opening.
	floor int       // How long it counts as opening, at least, in launch timeouts from since.
	last  time.Time // When its last new window came, or zero.
	ended bool
	// waiting is set when the period ended before Bench knew any of the
	// app's windows, or while the user had hidden it, whose windows the
	// Accessibility API can leave out: the next window Bench sees starts
	// the period again.
	waiting bool
}

// hasPlacesLocked reports whether an app has remembered places in a tab:
// App items without windows that had one.
func (e *Engine) hasPlacesLocked(bundleID string) bool {
	for _, t := range e.model.Tabs {
		for _, id := range t.ItemIDs() {
			if it := e.model.Items[id]; isAppOf(it, bundleID) && it.WindowID == 0 && it.Created != 0 {
				return true
			}
		}
	}
	return false
}

// startReopeningLocked starts an app's period of opening again, for floor
// launch timeouts at least, if it has places to fill and Bench can see its
// windows.
func (e *Engine) startReopeningLocked(bundleID string, floor int) {
	if !e.trusted || !e.hasPlacesLocked(bundleID) {
		return
	}
	key := strings.ToLower(bundleID)
	st := &reopenState{since: e.clk.Now(), floor: floor}
	e.reopening[key] = st
	e.scheduleReopenCheckLocked(key, st, e.reopenDueLocked(st))
}

// reopenDueLocked returns when an app's period of opening again ends.
func (e *Engine) reopenDueLocked(st *reopenState) time.Time {
	due := st.since.Add(e.reopenSpan(st.floor))
	if after := st.last.Add(e.reopenSpan(reopenAfterWindow)); !st.last.IsZero() && after.After(due) {
		due = after
	}
	if after := e.scanned.Add(e.reopenSpan(reopenAfterScan)); !e.scanned.IsZero() && after.After(due) {
		due = after
	}
	return due
}

// reopenSpan returns n launch timeouts, or n times reopenLeast if that's
// longer.
func (e *Engine) reopenSpan(n int) time.Duration {
	return time.Duration(n) * max(e.launchTimeout(), reopenLeast)
}

// reopeningNow reports whether an app is opening again, so that its
// windows can go to its remembered places by the order they were made in.
func (e *Engine) reopeningNow(bundleID string) bool {
	st := e.reopening[strings.ToLower(bundleID)]
	return st != nil && !st.ended && e.clk.Now().Before(e.reopenDueLocked(st))
}

// noteReopenWindowLocked notes a new window of an app: it keeps the app's
// period of opening going, or starts one if the app has places to fill and
// hasn't one, as when a window comes before its app's launch is reported,
// or long after a period that ended before Bench saw any of its windows,
// such as one on another desktop.
func (e *Engine) noteReopenWindowLocked(w platform.Window) {
	key := strings.ToLower(w.BundleID)
	st := e.reopening[key]
	if st == nil || (st.ended && st.waiting) {
		e.startReopeningLocked(w.BundleID, reopenAfterLaunch)
		if st = e.reopening[key]; st == nil {
			return
		}
	}
	if !st.ended {
		st.last = e.clk.Now()
	}
}

// scheduleReopenCheckLocked checks an app's period at due.
func (e *Engine) scheduleReopenCheckLocked(key string, st *reopenState, due time.Time) {
	e.after(max(due.Sub(e.clk.Now()), 0), func() { e.checkReopenedLocked(key, st) })
}

// checkReopenedLocked ends an app's period of opening again once it's due,
// and forgets the places the app didn't fill. It waits while Bench is still
// opening the app for a place, and forgets nothing if the app has quit
// (its places wait for it to open again), or if Bench knows none of its
// windows, or the user has hidden it: the next window Bench sees starts a
// new period (see noteReopenWindowLocked).
func (e *Engine) checkReopenedLocked(key string, st *reopenState) {
	if e.reopening[key] != st || st.ended {
		return
	}
	now := e.clk.Now()
	if due := e.reopenDueLocked(st); now.Before(due) {
		e.scheduleReopenCheckLocked(key, st, due)
		return
	}
	e.loadRunning()
	app, running := e.running[key]
	if !running {
		delete(e.reopening, key)
		return
	}
	if e.launchingNow(key) {
		e.scheduleReopenCheckLocked(key, st, now.Add(e.launchTimeout()))
		return
	}
	st.ended = true
	if (app.Hidden && !e.win.Hid(app)) || !e.knowsWindowOfLocked(key) {
		st.waiting = true
		return
	}
	e.forgetPlacesLocked(key)
}

// knowsWindowOfLocked reports whether Bench knows a window of an app: one
// of its items' or one it doesn't manage.
func (e *Engine) knowsWindowOfLocked(bundleID string) bool {
	for _, it := range e.model.Items {
		if isAppOf(it, bundleID) && it.WindowID != 0 {
			return true
		}
	}
	for _, w := range e.unmanaged {
		if sameApp(w.BundleID, bundleID) {
			return true
		}
	}
	return false
}

// forgetPlacesLocked forgets an app's remembered places in every tab, apart
// from those keeping config entries' places (see anchorsLocked) and those
// Bench is opening the app for (see fillingLocked). It leaves their panes,
// however empty, and the tabs' layouts as they are: only the user's own
// actions change those. Config placeholders that never had a window stay
// too, as the config's entries. It reports how many places it forgot.
func (e *Engine) forgetPlacesLocked(bundleID string) int {
	m := e.model
	anchors := e.anchorsLocked()
	current := false
	var forgot []string
	for ti, t := range m.Tabs {
		for _, id := range t.ItemIDs() {
			it := m.Items[id]
			if !isAppOf(it, bundleID) || it.WindowID != 0 || it.Created == 0 || anchors[ti][id] || e.fillingLocked(it) {
				continue
			}
			m.DropFrom(id, ti)
			forgot = append(forgot, id)
			current = current || ti == m.Current
		}
	}
	if len(forgot) == 0 {
		return 0
	}
	e.log.Info("forgetting places an app didn't fill", "app", bundleID, "items", forgot)
	if current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	return len(forgot)
}

// anchorsLocked returns, for each tab, the App items without windows that
// keep config entries' places, so that Bench can open their apps with the
// tab: for each App entry in each pane of the tab's config, one of the
// entry's items in the pane it maps to (see paneForConfig), unless a window
// there stands for the entry already. If that pane has none, and the tab
// has no window for the entry either, the tab's first item for it keeps
// the place, wherever the user has moved it.
func (e *Engine) anchorsLocked() map[int]map[string]bool {
	m := e.model
	out := map[int]map[string]bool{}
	for ci, ct := range e.cfg.Tabs {
		for pj, cp := range ct.Panes {
			for _, entry := range cp.Items {
				if entry.App == "" {
					continue
				}
				ti, p := m.paneForConfig(e.cfg, ci, pj)
				if p == nil {
					continue
				}
				if out[ti] == nil {
					out[ti] = map[string]bool{}
				}
				if id := m.anchorFor(m.Tabs[ti], p, entryKey(entry), out[ti]); id != "" {
					out[ti][id] = true
				}
			}
		}
	}
	return out
}

// anchorFor returns the item without a window that keeps the place of an
// App entry, by its key, in pane p of tab t, other than those taken by
// other entries, or "" if a window stands for the entry there, or no item
// can (see anchorsLocked).
func (m *Model) anchorFor(t *Tab, p *layout.Node, key string, taken map[string]bool) string {
	find := func(ids []string) (window bool, free string) {
		for _, id := range ids {
			it := m.Items[id]
			if it == nil || it.Kind != KindApp || itemKeys(it)[0] != key {
				continue
			}
			if it.WindowID != 0 {
				window = true
			} else if free == "" && !taken[id] {
				free = id
			}
		}
		return window, free
	}
	window, free := find(p.Items)
	if window {
		return ""
	}
	if free != "" {
		return free
	}
	if window, free = find(t.ItemIDs()); window {
		return ""
	}
	return free
}

// fillingLocked reports whether Bench is filling an App item's place: a tab
// switch or Open waits for its window, or a placement is to give it the
// app's next window.
func (e *Engine) fillingLocked(it *Item) bool {
	if e.wait != nil && e.wait.opened[it.ID] {
		return true
	}
	now := e.clk.Now()
	return slices.ContainsFunc(e.pending, func(pl pendingPlace) bool { return pl.item == it.ID && now.Before(pl.until) })
}

// forgetStaleLocked forgets the remembered places of the apps that haven't
// run for staleAfter, as if they had opened again (see forgetPlacesLocked):
// an app the user no longer uses shouldn't keep claiming windows for panes
// where it's out of sight. An app that Bench has no record of seeing, as
// with a state.json from an older Bench, counts as seen now.
func (e *Engine) forgetStaleLocked() {
	now := e.clk.Now()
	var stale []string
	for _, it := range e.model.Items {
		if it.Kind != KindApp || it.WindowID != 0 || it.Created == 0 {
			continue
		}
		key := strings.ToLower(it.BundleID)
		if _, running := e.running[key]; running || slices.Contains(stale, key) {
			continue
		}
		seen, ok := e.seen[key]
		if !ok {
			e.seen[key] = now
			continue
		}
		if now.Sub(seen) >= staleAfter {
			stale = append(stale, key)
		}
	}
	slices.Sort(stale)
	for _, key := range stale {
		e.forgetPlacesLocked(key)
	}
}

// scheduleStaleCheckLocked checks for the places of apps that haven't run
// for a long time every staleEvery, while Bench runs. It saves state.json
// then too, if Bench remembers when it saw apps, so that the file says when
// Bench was last running (see loadAppStates).
func (e *Engine) scheduleStaleCheckLocked() {
	e.after(staleEvery, func() {
		e.forgetStaleLocked()
		if len(e.seen) > 0 {
			e.changed()
		}
		e.scheduleStaleCheckLocked()
	})
}

// DropFrom takes an item out of its pane in tab ti, and forgets it if it's
// in no tab now and not floating. Unlike Take, it leaves the pane in place
// even if it's left empty, and doesn't mark the tab custom: Bench uses it
// for what it tidies away by itself, which mustn't change the layout, nor
// where the config's claims send windows (see paneForConfig).
func (m *Model) DropFrom(id string, ti int) {
	t := m.Tab(ti)
	if t == nil {
		return
	}
	p := layout.PaneOf(t.Root, id)
	if p == nil {
		return
	}
	keep := m.keepShown(t, id)
	removeFromLeaf(p, id)
	keep()
	if len(m.TabsOf(id)) > 0 || m.IsFloating(id) {
		return
	}
	if it := m.Items[id]; it != nil && it.WindowID != 0 {
		delete(m.windows, it.WindowID)
	}
	delete(m.Items, id)
}

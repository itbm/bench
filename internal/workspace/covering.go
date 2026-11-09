package workspace

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file puts out of sight what covers Bench as it comes forward
// (decision 68). Bench's window sits below normal windows, so a window that
// isn't in a tab, such as a Finder window, can sit over its tabs, panes,
// and status bar, and a click on Bench can't bring Bench in front of it.
// When Bench becomes active, with its window shown, it hides the app of
// each such window, or, if the app has other windows to keep showing (on
// another display, say), moves only the windows over Bench out of sight,
// parking them as it parks other tabs' windows. Either way the windows
// manager records it in parked.json first, so that quitting, and a crash,
// put them back. A hidden app comes back when the user brings it forward,
// and macOS shows its windows; parked windows come back then too
// (unparkCoveredLocked), or, if they can't then, as on another desktop,
// later (retryCoveredLocked). The first time Bench does this to an app, a
// toast says so, with Keep over Bench, which adds a float rule for the app
// to config.toml (KeepOverBench), and which the recent messages offer
// again while it can (keepOverLocked); state.json remembers the apps it
// has told about. covering_windows = "leave" turns it off.

// covering is what an app has on screen while it covers Bench.
type covering struct {
	app platform.App
	// over holds its windows that aren't in a tab and cover Bench's content
	// area, and that no float rule matches.
	over []platform.Window
	// others is set if it has other windows to keep showing: on screen but
	// clear of Bench's content area, in full screen, or floating over Bench
	// by a rule.
	others bool
}

// hideCoveringLocked puts out of sight what covers Bench's content area as
// Bench comes forward, unless covering_windows is "leave": for each app
// with a window that isn't in a tab over it, the app, or, if the app has
// other windows to keep showing, those windows (see coveringLocked). It
// leaves alone an app with windows docked in the current tab or popped out,
// and the browser that the add dialog waits for a web app from (see
// makingBrowserLocked). Windows that a float rule matches, such as a
// picture-in-picture window, don't count: they're meant to stay over
// Bench, which covers the whole screen when it fills it. Of the apps it
// hasn't told the user about, the first it puts away gets a toast
// (tellCoveredLocked); the others get theirs another time. It reads the
// running apps again first: Bench may have read them last while an app
// was hidden, and shown the app since, as Keep over Bench does, and a
// hidden app is passed over.
func (e *Engine) hideCoveringLocked() {
	if e.cfg.Settings.CoveringWindows == config.CoveringLeave {
		return
	}
	content := e.p.ContentFrame()
	if content.W <= 0 || content.H <= 0 {
		return
	}
	e.loadRunning()
	keep := map[string]bool{}
	if browser := e.makingBrowserLocked(); browser != "" {
		keep[strings.ToLower(browser)] = true
	}
	for _, id := range append(e.model.CurrentTab().ItemIDs(), e.model.Floating...) {
		if it := e.model.Items[id]; it != nil && it.Kind == KindApp && it.WindowID != 0 {
			keep[strings.ToLower(it.BundleID)] = true
		}
	}
	checked := map[string]bool{}
	toasted := false
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
		c := e.coveringLocked(app, content)
		if len(c.over) == 0 {
			continue
		}
		parked, ok := e.putAwayLocked(c)
		if ok && !toasted && !e.told[key] {
			toasted = true
			e.tellCoveredLocked(app, parked)
		}
	}
}

// coveringLocked reads an app's windows, for what covers a frame (see
// covering). Windows that are minimised aren't on screen, nor are those
// that the windows manager has parked, while they're clear of the frame:
// one that something else has put back over it, as macOS can as displays
// come back, covers it again, and is parked again.
func (e *Engine) coveringLocked(app platform.App, frame layout.Rect) covering {
	c := covering{app: app}
	ws, err := e.p.Windows(app)
	if err != nil {
		return c
	}
	for _, w := range ws {
		if !w.Standard() || w.Minimised || (e.win.IsParked(w.ID) && !platform.Overlaps(w.Frame, frame)) {
			continue
		}
		_, free := e.unmanaged[w.ID]
		float, _ := matchRule(e.cfg.Rules, w.BundleID, w.Title)
		switch {
		case w.FullScreen || !platform.Overlaps(w.Frame, frame) || (free && float):
			c.others = true
		case free:
			c.over = append(c.over, w)
		}
	}
	return c
}

// putAwayLocked puts out of sight what covers Bench of an app: the app,
// hidden, or, if it has other windows to keep showing, the windows over
// Bench, parked, which unparkCoveredLocked puts back when the app comes
// forward. It does both through the windows manager, which records them
// first, so that quitting, or a crash, puts them back. It returns the
// windows it parked, and reports whether it put anything out of sight.
func (e *Engine) putAwayLocked(c covering) ([]platform.Window, bool) {
	if !c.others {
		if err := e.win.Hide(c.app); err != nil {
			e.log.Warn("couldn't hide an app over Bench", "app", c.app.BundleID, "error", err)
			return nil, false
		}
		return nil, true
	}
	key := strings.ToLower(c.app.BundleID)
	screen := e.parkScreenLocked()
	var parked []platform.Window
	for _, w := range c.over {
		err := e.win.Park(w, screen)
		if e.win.IsParked(w.ID) {
			// Recorded, even if it failed, as a slow app can still move it.
			e.covered[w.ID] = key
		}
		if err != nil {
			e.log.Warn("couldn't move a window over Bench out of sight", "window", w.ID, "app", c.app.BundleID, "error", err)
			continue
		}
		if e.win.IsParked(w.ID) {
			parked = append(parked, w)
		}
	}
	return parked, len(parked) > 0
}

// tellCoveredLocked says, once for each app, that Bench has put its
// windows out of sight because they covered Bench, and remembers that it
// has, in state.json. For an app that can float (canFloatLocked), the toast
// has Keep over Bench (KeepOverBench). parked holds the windows it parked,
// or none if it hid the app.
func (e *Engine) tellCoveredLocked(app platform.App, parked []platform.Window) {
	key := strings.ToLower(app.BundleID)
	e.told[key] = true
	e.changed()
	name := e.appName(app.BundleID)
	msg := "Hid " + name + " because it covered Bench."
	switch {
	case len(parked) == 1 && parked[0].Title != "":
		msg = "Hid the " + name + " window “" + parked[0].Title + "” because it covered Bench."
	case len(parked) == 1:
		msg = "Hid a " + name + " window because it covered Bench."
	case len(parked) > 1:
		msg = fmt.Sprintf("Hid %d %s windows because they covered Bench.", len(parked), name)
	}
	t := Toast{Text: msg}
	if e.canFloatLocked(app.BundleID) {
		t.Action = ToastAction{Label: "Keep over Bench", Method: "KeepOverBench", Item: app.BundleID}
	}
	em := e.em
	e.later(func() { em.ActionToast(t) })
}

// canFloatLocked reports whether Keep over Bench can add a float rule for
// an app: it has no App items, with windows or waiting for them, in a tab
// or "Not in a tab", and no rule sends its windows to a tab. A float rule
// for the app would keep its windows out of those items, and win over
// those rules (matchRule).
func (e *Engine) canFloatLocked(bundleID string) bool {
	for _, it := range e.model.Items {
		if it.Kind == KindApp && sameApp(it.BundleID, bundleID) {
			return false
		}
	}
	return !slices.ContainsFunc(e.cfg.Rules, func(r config.Rule) bool { return r.Tab != "" && sameApp(r.App, bundleID) })
}

// floatsLocked reports whether a rule in config.toml floats all of an
// app's windows.
func (e *Engine) floatsLocked(bundleID string) bool {
	return slices.ContainsFunc(e.cfg.Rules, func(r config.Rule) bool { return r.Float && r.Title == "" && sameApp(r.App, bundleID) })
}

// keepOverLocked returns the apps that Keep over Bench can still keep over
// Bench, by bundle ID, for the View: those whose windows Bench has said
// it put out of sight (told), that no rule floats yet, and that can float
// (canFloatLocked). The recent messages offer Keep over Bench again for
// them, as the toast that offered it may have gone too soon.
func (e *Engine) keepOverLocked() []string {
	var out []string
	for _, key := range slices.Sorted(maps.Keys(e.told)) {
		if !e.floatsLocked(key) && e.canFloatLocked(key) {
			out = append(out, key)
		}
	}
	return out
}

// unparkCoveredLocked puts back the windows of an app that Bench parked
// because they covered it, now that the app has come forward, as macOS
// shows a hidden app's windows then. One that a tab has taken since is the
// tab's to place. One that it couldn't put back stays parked, as the
// windows manager keeps it then, and Bench tries again the next time the
// app comes forward, or as the desktop changes, the screens settle, or
// Accessibility permission comes back while the app is in front
// (retryCoveredLocked): it may be on a desktop that isn't shown, in full
// screen, or its app may not have answered.
func (e *Engine) unparkCoveredLocked(bundleID string) {
	for _, id := range slices.Sorted(maps.Keys(e.covered)) {
		if !sameApp(e.covered[id], bundleID) {
			continue
		}
		if _, free := e.unmanaged[id]; !free {
			delete(e.covered, id)
			continue
		}
		if err := e.win.Unpark(id); err != nil {
			e.log.Warn("couldn't put back a window that covered Bench", "window", id, "app", bundleID, "error", err)
		}
		// Unpark forgets a window that has closed.
		if !e.win.IsParked(id) {
			delete(e.covered, id)
		}
	}
}

// retryCoveredLocked tries again to put back the parked windows of the app
// in front, if it isn't Bench, that unparkCoveredLocked couldn't put back
// as the app came forward.
func (e *Engine) retryCoveredLocked() {
	if !e.benchActive && e.activeApp != "" {
		e.unparkCoveredLocked(e.activeApp)
	}
}

// reparkCoveredLocked parks again the windows that Bench parked because
// they covered it, and that macOS has put back on screen since, as it can
// when displays come back, while their apps stay in the background: as
// the arrangement parks other tabs' windows again, and as a hidden app
// stays hidden. Those of the app in front are retryCoveredLocked's. The
// windows manager leaves alone a window that's still where it parked it.
// With covering_windows set to "leave" since, it leaves them where macOS
// put them.
func (e *Engine) reparkCoveredLocked() {
	if len(e.covered) == 0 || !e.trusted || e.cfg.Settings.CoveringWindows == config.CoveringLeave {
		return
	}
	screen := e.parkScreenLocked()
	for _, id := range slices.Sorted(maps.Keys(e.covered)) {
		w, free := e.unmanaged[id]
		if !free || !e.win.IsParked(id) || (!e.benchActive && sameApp(e.covered[id], e.activeApp)) {
			continue
		}
		if err := e.win.Park(w, screen); err != nil {
			e.log.Warn("couldn't move a window over Bench out of sight again", "window", id, "app", e.covered[id], "error", err)
		}
	}
}

// forgetCoveredLocked forgets a window that Bench parked because it
// covered Bench, now that it has closed, and the windows manager's record
// of it, as nothing can put it back.
func (e *Engine) forgetCoveredLocked(id platform.WindowID) {
	if _, ok := e.covered[id]; ok {
		delete(e.covered, id)
		e.win.Forget(id)
	}
}

// KeepOverBench lets an app's windows stay over Bench, for the toast that
// says Bench put them out of sight (tellCoveredLocked), and the recent
// messages, which keep it: it adds a rule to config.toml, `float = true`
// for the app, which also leaves its new windows out of the tabs, and
// shows again what Bench put out of sight. The toast says so, with Undo,
// which takes the rule back and puts them out of sight again. With such a
// rule already, it only shows them again. It refuses an app that can't
// float (canFloatLocked), whose windows the rule would keep out of the
// tabs.
func (e *Engine) KeepOverBench(bundleID string) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	key := strings.ToLower(bundleID)
	if e.closed || !e.told[key] {
		e.unlock()
		return errCantKeepOver
	}
	name := e.appName(bundleID)
	if e.floatsLocked(bundleID) {
		e.showCoveredLocked(key)
		e.unlock()
		e.em.Toast(name + " windows stay over Bench already.")
		return nil
	}
	if !e.canFloatLocked(bundleID) {
		e.unlock()
		return errKeepOverInTabs
	}
	cfg := e.cfg.Clone()
	cfg.Rules = append(cfg.Rules, config.Rule{App: bundleID, Float: true})
	if len(cfg.Validate()) > 0 {
		e.unlock()
		return errCantKeepOver
	}
	// The change is to config.toml's rules alone (undoStep.covering).
	step := e.beginUndoLocked()
	step.cfgRules = e.cfg.Clone().Rules
	step.covering = true
	e.cfg = cfg
	e.showCoveredLocked(key)
	named := undoName{"Keep over Bench", "kept " + name + " windows over Bench"}
	undo := e.keepUndoLocked(step, named, name+" windows no longer stay over Bench.")
	saved, writable := cfg.Clone(), e.writable
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	msg := "Added a rule to config.toml: " + name + " windows now stay over Bench, and new ones stay out of your tabs."
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.ActionToast(undoToast(msg, undo))
	return nil
}

// showCoveredLocked shows again what Bench put out of sight of an app's
// because it covered Bench: the app, if Bench hid it, and the windows it
// parked.
func (e *Engine) showCoveredLocked(key string) {
	if app, ok := e.running[key]; ok && e.win.Hid(app) {
		if err := e.win.Unhide(app); err != nil {
			e.log.Warn("couldn't show an app that covered Bench", "app", app.BundleID, "error", err)
		}
	}
	e.unparkCoveredLocked(key)
}

// DemoComeForward does, in the browser preview, what Bench does as it
// comes forward on a Mac when the user clicks it while another app is in
// front: it puts out of sight what covers it (hideCoveringLocked). The
// preview's page can't make Bench the active app. It needs the preview's
// platform (platform.NewDemo).
func (e *Engine) DemoComeForward() error {
	if _, ok := e.p.(demoWindows); !ok || !e.opts.Demo {
		return errNoDemoWindows
	}
	e.lock()
	defer e.unlock()
	if e.trusted && !e.benchHidden {
		e.hideCoveringLocked()
	}
	return nil
}

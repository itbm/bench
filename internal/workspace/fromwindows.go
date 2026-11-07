package workspace

import (
	"slices"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file makes a tab of windows that are open, for the New tab dialog's
// "Windows that are open" (NewTabFromWindows, with its preview,
// PreviewTabFromWindows), and adds the windows ticked in the add dialog to
// a pane at once (AddWindowsToPane). Windows are named by their IDs in
// View.Floating, for those not in a tab, and View.Items, for those of
// other tabs. The new tab's Undo takes it away and puts each window back
// where it was (outsideWindow).

// WindowsPreview is the New tab dialog's preview of a tab made of windows
// that are open.
type WindowsPreview struct {
	// Suggested is the layout for the number of apps that the windows are
	// of (suggestPreset).
	Suggested string `json:"suggested"`
	// Apps is that number. Each app's windows share a pane.
	Apps int `json:"apps"`
	// Root is the tab's tree, with the windows in its panes (fillPanes).
	Root *layout.Node `json:"root"`
}

// windowGroup is the windows of one app, in the order they were given:
// they share a pane, as sub-tabs.
type windowGroup struct {
	key, name string
	ids       []string
}

// outsideWindow is a window that a change took into a tab from "Not in a
// tab" (NewTabFromWindows), as it was then, for Undo to put back: a
// window that Bench didn't manage goes back to being one, and a popped-out
// item gets its window of its own back. An App window goes back to its
// frame.
type outsideWindow struct {
	// item is the popped-out item, or the item that the change made for
	// the window.
	item string
	// window is the window that Bench didn't manage, as it was then. Its
	// ID is 0 for a popped-out item.
	window platform.Window
	// frame is where the App window was.
	frame layout.Rect
	// ownWindow is set for a popped-out page or terminal that had a window
	// of its own.
	ownWindow bool
}

// suggestPreset returns the layout to suggest for a tab of the windows of
// a number of apps: one pane for each app, up to the grid of four.
func suggestPreset(apps int) string {
	switch {
	case apps <= 1:
		return layout.PresetSingle
	case apps == 2:
		return layout.PresetSide
	case apps == 3:
		return layout.PresetThree
	default:
		return layout.PresetGrid
	}
}

// fillPanes shares the windows of groups among n panes, in order: each
// app's windows go to a pane of their own, and once only the last pane is
// left, the windows of the apps after it go there too. Panes that no app
// reaches stay empty.
func fillPanes(groups []windowGroup, n int) [][]string {
	out := make([][]string, n)
	if n == 0 {
		return out
	}
	for i, g := range groups {
		p := min(i, n-1)
		out[p] = append(out[p], g.ids...)
	}
	return out
}

// appKey returns a key that's the same for the windows of one app: its
// bundle ID, or for a page or terminal, its kind and name. The New tab
// dialog groups its list of windows the same way (fromwindows.js).
func appKey(kind, bundleID, app string) string {
	if bundleID != "" {
		return strings.ToLower(bundleID)
	}
	return kind + ":" + strings.ToLower(app)
}

// windowGroupsLocked groups windows by app, the apps in the order that
// their first windows come. It leaves out windows that have gone, hidden
// ones (see hiddenLocked), and repeats.
func (e *Engine) windowGroupsLocked(ids []string) []windowGroup {
	var out []windowGroup
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		key, name, ok := e.windowAppLocked(id)
		if !ok {
			continue
		}
		i := slices.IndexFunc(out, func(g windowGroup) bool { return g.key == key })
		if i < 0 {
			out = append(out, windowGroup{key: key, name: name})
			i = len(out) - 1
		}
		out[i].ids = append(out[i].ids, id)
	}
	return out
}

// windowAppLocked returns the app of a window, for windowGroupsLocked: its
// key (appKey) and its name. It reports false for a window that has gone,
// or a hidden item.
func (e *Engine) windowAppLocked(id string) (key, name string, ok bool) {
	m := e.model
	if it := m.Items[id]; it != nil {
		if e.hiddenLocked(it) || (!m.IsFloating(id) && len(m.TabsOf(id)) == 0) {
			return "", "", false
		}
		return appKey(it.Kind, it.BundleID, it.App), it.App, true
	}
	w, found := e.unmanagedLocked(id)
	if !found {
		return "", "", false
	}
	name = e.windowAppName(w)
	return appKey(KindApp, w.BundleID, name), name, true
}

// PreviewTabFromWindows returns the New tab dialog's preview of a tab made
// of windows that are open: the layout suggested for them, and the tree
// that NewTabFromWindows would make with a preset ("" for the suggested
// one), with the windows in its panes. The preview's panes and splits get
// IDs that aren't kept.
func (e *Engine) PreviewTabFromWindows(preset string, windows []string) (WindowsPreview, error) {
	e.lock()
	defer e.unlock()
	groups := e.windowGroupsLocked(windows)
	suggested := suggestPreset(len(groups))
	if preset == "" {
		preset = suggested
	}
	root, err := layout.ApplyPreset(nil, preset, nil, e.previewIDsLocked())
	if err != nil {
		return WindowsPreview{}, errNoPreset(preset)
	}
	panes := layout.Leaves(root)
	for i, ids := range fillPanes(groups, len(panes)) {
		panes[i].Items = ids
		if len(ids) > 0 {
			panes[i].Active = ids[0]
		}
	}
	return WindowsPreview{Suggested: suggested, Apps: len(groups), Root: root}, nil
}

// NewTabFromWindows makes a tab of windows that are open: the New tab
// dialog's "Windows that are open". The tab has the draft's name, or else
// the name of the first window's app, numbered if another tab has it
// (uniqueTabName); the draft's preset, or else the one suggested for the
// number of apps (suggestPreset); and the draft's small-screen setting and
// place. Each app's windows share a pane, as sub-tabs, in order, and the
// apps after the last pane go there too (fillPanes). Windows not in a tab
// move to it. Those of other tabs show in it as well, as Also show in
// does, and stay where they are. Each pane shows its first window, the
// first pane is focused, and Bench switches to the tab. The config gets
// the tab, saved as Tab properties saves a new tab. The toast's Undo takes
// the tab away again, from config.toml too, and puts each window back
// where it was (see outsideWindow).
func (e *Engine) NewTabFromWindows(draft TabDraft, windows []string) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	m := e.model
	groups := e.windowGroupsLocked(windows)
	if len(groups) == 0 {
		e.unlock()
		if len(windows) > 0 {
			return errWindowsGone
		}
		return errNoWindows
	}
	if draft.Preset == "" {
		draft.Preset = suggestPreset(len(groups))
	}
	typed := strings.TrimSpace(draft.Name)
	if typed == "" {
		typed = groups[0].name
	}
	name := uniqueTabName(m.Tabs, typed, -1)
	cfg, err := e.draftConfigLocked(-1, name, draft)
	if err != nil {
		e.unlock()
		return err
	}
	step := e.beginUndoLocked()
	was := e.cfg.Clone()
	step.cfgTabs, step.cfgRules = was.Tabs, was.Rules
	t, err := m.AddTab(name, draft.Preset, smallScreensOf(draft.SmallScreens))
	if err != nil {
		e.unlock()
		return err
	}
	n := e.fillTabLocked(t, groups, step)
	e.cfg = cfg
	e.syncConfigTab(cfg, m.IndexOf(t))
	if draft.Place > 0 {
		e.moveTabLocked(cfg, m.IndexOf(t), min(draft.Place, len(m.Tabs))-1)
	}
	undo := e.keepUndoLocked(step, "Deleted the new tab, "+name+", and put its windows back.")
	e.switchLocked(m.IndexOf(t), switchOpts{focus: true})
	writable := e.writable
	saved := cfg.Clone()
	e.holdStateLocked()
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	msg := "Made a new tab, " + name + ", with " + windowsText(n) + "."
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.ActionToast(undoToast(msg, undo))
	return nil
}

// fillTabLocked puts the windows of groups in the panes of new tab t
// (fillPanes). Each pane shows its first window, and the first pane is
// focused. A window that a pane shows and that's minimised comes back from
// the Dock, as the user chose it, as in the add dialog (finishAddLocked).
// It notes in step where the windows that weren't in a tab were, for Undo,
// and returns how many windows it put in the tab.
func (e *Engine) fillTabLocked(t *Tab, groups []windowGroup, step *undoStep) int {
	m := e.model
	panes := t.Leaves()
	n := 0
	for i, ids := range fillPanes(groups, len(panes)) {
		p := panes[i]
		for _, id := range ids {
			was, outside := e.outsideLocked(id)
			it, err := e.addWindowLocked(t, p, id)
			if err != nil || it == nil {
				continue
			}
			if outside {
				was.item = it.ID
				step.outside = append(step.outside, was)
			}
			n++
		}
		if len(p.Items) > 0 {
			p.Active = p.Items[0]
			showAgain(m.Items[p.Active])
		}
	}
	if len(panes) > 0 {
		t.Focus = panes[0].ID
		if first := panes[0].Active; first != "" {
			m.ShowIn(first, m.IndexOf(t))
		}
	}
	return n
}

// addWindowLocked adds a window to pane p of tab t: one not in a tab moves
// there (addFloatingLocked), and an item of other tabs shows there as well
// (addFromTabLocked).
func (e *Engine) addWindowLocked(t *Tab, p *layout.Node, id string) (*Item, error) {
	m := e.model
	if m.Items[id] == nil || m.IsFloating(id) {
		return e.addFloatingLocked(t, p, id)
	}
	return e.addFromTabLocked(t, p, id)
}

// outsideLocked returns a window that isn't in a tab as it is now, for Undo
// to put back (outsideWindow), or false for one in a tab, or that has gone.
func (e *Engine) outsideLocked(id string) (outsideWindow, bool) {
	m := e.model
	if it := m.Items[id]; it != nil {
		if !m.IsFloating(id) {
			return outsideWindow{}, false
		}
		was := outsideWindow{item: id, ownWindow: it.OwnWindow}
		if it.Kind == KindApp && it.WindowID != 0 {
			if w, err := e.p.Window(it.WindowID); err == nil {
				was.frame = w.Frame
			}
		}
		return was, true
	}
	w, ok := e.unmanagedLocked(id)
	if !ok {
		return outsideWindow{}, false
	}
	if now, err := e.p.Window(w.ID); err == nil {
		w.Frame = now.Frame
	}
	return outsideWindow{window: w, frame: w.Frame}, true
}

// putBackOutsideLocked puts each window that a change took into a tab from
// "Not in a tab" back where it was, once Undo has taken it out of the tab
// again: a window that Bench didn't manage goes back to being one, and a
// popped-out page or terminal gets its window of its own back. An App
// window goes back to its frame. A window that's in a tab still, or has
// gone, stays as it is. Bench may have hidden the app of a window while
// the window was in the tab, as it does after a switch to another tab. If
// the window goes back to being one that Bench doesn't manage, and the app
// has no items with windows left, the next arrangement shows the app
// again (see putBack).
func (e *Engine) putBackOutsideLocked(outside []outsideWindow) {
	m := e.model
	for _, was := range outside {
		it := m.Items[was.item]
		if it == nil || !m.IsFloating(was.item) {
			continue
		}
		switch {
		case was.window.ID != 0:
			if it.WindowID != was.window.ID {
				continue
			}
			w := was.window
			w.Title, w.Frame = it.Title, was.frame
			m.Remove(it.ID)
			e.unmanaged[w.ID] = w
			e.moveBackLocked(w.ID, was.frame)
			e.putBack[strings.ToLower(w.BundleID)] = true
		case it.Kind == KindApp && it.WindowID != 0:
			e.moveBackLocked(it.WindowID, was.frame)
		case it.Kind == KindWeb && was.ownWindow && !it.OwnWindow:
			if err := e.popOutWebLocked(it); err != nil {
				e.log.Warn("couldn't give a page its window back", "item", it.ID, "error", err)
			}
		case it.Kind == KindBuiltin && was.ownWindow && !it.OwnWindow && !e.opts.Headless:
			id := it.ID
			e.later(func() { e.popOutAgain(id) })
		}
	}
}

// moveBackLocked moves a window that has left Bench back to a frame it had
// (see windows.Manager.MoveBack).
func (e *Engine) moveBackLocked(id platform.WindowID, frame layout.Rect) {
	if frame.W <= 0 || frame.H <= 0 {
		return
	}
	if err := e.win.MoveBack(id, frame); err != nil {
		e.log.Warn("couldn't move a window back", "window", id, "error", err)
	}
}

// popOutAgain gives a popped-out terminal back the window of its own that
// it had, once Undo has taken it out of the tab that it joined. The
// Emitter makes the window, without the lock held.
func (e *Engine) popOutAgain(itemID string) {
	e.lock()
	it := e.model.Items[itemID]
	if it == nil || it.Kind != KindBuiltin || it.OwnWindow || !e.model.IsFloating(itemID) {
		e.unlock()
		return
	}
	e.startTerminalLocked(it)
	title := labelOf(it)
	e.unlock()

	if err := e.em.PopOutTerminal(itemID, title); err != nil {
		e.log.Warn("couldn't give a terminal its window back", "item", itemID, "error", err)
		return
	}

	e.lock()
	defer e.unlock()
	it = e.model.Items[itemID]
	if it == nil || !e.model.IsFloating(itemID) {
		// It has ended, or joined a tab, while its window opened.
		em := e.em
		e.later(func() { em.CloseTerminalWindow(itemID) })
		return
	}
	it.OwnWindow = true
	e.changed()
}

// AddWindowsToPane adds the windows ticked in the add dialog to a pane of
// the current tab, in order: those not in a tab move there, and those of
// other tabs show there as well. The pane shows the first. If some
// couldn't be added, as they have closed or are in the tab already, it
// says so.
func (e *Engine) AddWindowsToPane(paneID string, windows []string) error {
	e.lock()
	defer e.unlock()
	t, p, err := e.targetPaneLocked(paneID)
	if err != nil {
		return err
	}
	ids := uniqueIDs(windows)
	if len(ids) == 0 {
		return errNoWindows
	}
	var first *Item
	var refused error
	added := 0
	for _, id := range ids {
		it, err := e.addWindowLocked(t, p, id)
		if err != nil {
			refused = err
			continue
		}
		added++
		if first == nil {
			first = it
		}
	}
	if added == 0 {
		return refused
	}
	e.finishAddLocked(first)
	if added < len(ids) {
		e.toastLocked(notAllAddedText(added, len(ids)))
	}
	return nil
}

// uniqueIDs returns ids without repeats, in order.
func uniqueIDs(ids []string) []string {
	var out []string
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// windowsText returns "1 window", or n windows.
func windowsText(n int) string {
	if n == 1 {
		return "1 window"
	}
	return strconv.Itoa(n) + " windows"
}

// notAllAddedText says that AddWindowsToPane added only some of the windows
// ticked.
func notAllAddedText(added, ticked int) string {
	rest := "The others had closed, or were in this tab already."
	if ticked-added == 1 {
		rest = "The other had closed, or was in this tab already."
	}
	return "Added " + strconv.Itoa(added) + " of the " + windowsText(ticked) + ". " + rest
}

package workspace

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file tells the user when Bench couldn't keep the arrangement saved
// in state.json, which config.LoadState sets aside as state.json.bad, or the
// arrangement of tabs that a reload of config.toml deleted, and puts back
// the arrangement of tabs it dropped because config.toml had no tabs of
// their names, once tabs have those names again.

// setAsideState is what Bench couldn't keep of the arrangement, at startup
// or since, for the notice that says so.
type setAsideState struct {
	// reason is why Bench couldn't use state.json at startup, such as that
	// it doesn't parse, for which it's set aside whole, and names the saved
	// tabs, if it could read them.
	reason error
	names  []string
	// dropped are the tabs whose arrangement Bench couldn't keep, which
	// Restore previous arrangement can still put back.
	dropped []droppedTab
	// quiet is set while Bench runs with another config because config.toml
	// has problems: Bench leaves state.json as it is, and says nothing,
	// until the file is fixed (see revealSetAsideLocked).
	quiet bool
}

// droppedTab is a tab whose arrangement Bench couldn't keep: as saved, with
// the saved items (some of which other tabs may hold), and why. floated
// holds its items that a reload of config.toml moved to Floating as it
// deleted the tab, which come back if it's restored.
type droppedTab struct {
	tab     config.TabState
	items   map[string]config.ItemState
	why     dropCause
	floated map[string]bool
}

// dropCause is why Bench couldn't keep a tab's arrangement.
type dropCause int

const (
	// droppedAtStart: config.toml had no tab of its name when Bench started.
	droppedAtStart dropCause = iota
	// droppedAtReload: config.toml had no tab of its name when Bench
	// reloaded it.
	droppedAtReload
	// droppedForOther: Bench started with another config, because
	// config.toml had problems.
	droppedForOther
)

// The notice's buttons.
var (
	actionRestoreArrangement = NoticeAction{Label: "Restore previous arrangement", Method: "RestoreArrangement"}
	actionDismissArrangement = NoticeAction{Label: "Dismiss", Method: "DismissArrangementNotice"}
)

// wholeSetAsideText is the notice that state.json was set aside whole,
// because of reason, losing the arrangement of the tabs it names.
func wholeSetAsideText(names []string, reason error) string {
	lost, start := "the arrangement", "The tabs start"
	if len(names) > 0 {
		lost, start = "the arrangement of "+joinNames(names), "They start"
	}
	return "Bench couldn't restore " + lost + ", because state.json can't be used: " + reason.Error() + ". " +
		start + " as the config lays them out. The file is kept as state.json.bad, but it can't be restored."
}

// droppedText is the notice that Bench couldn't keep the arrangement of the
// dropped tabs, saying why for each. Unless one can be restored now, it
// says how to get it back.
func droppedText(dropped []droppedTab, restorable bool) string {
	var sentences, all []string
	for _, why := range []dropCause{droppedAtStart, droppedAtReload, droppedForOther} {
		var names []string
		for _, d := range dropped {
			if d.why == why {
				names = append(names, d.tab.Name)
			}
		}
		if len(names) > 0 {
			sentences = append(sentences, droppedSentence(names, why))
			all = append(all, names...)
		}
	}
	text := strings.Join(sentences, " ")
	switch {
	case restorable:
	case len(all) == 1:
		text += " To get it back, name a tab " + all[0] + " again, then choose Restore previous arrangement here."
	default:
		text += " To get one back, give a tab its old name again, then choose Restore previous arrangement here."
	}
	return text
}

// droppedSentence says why Bench couldn't keep the arrangement of the saved
// tabs called names.
func droppedSentence(names []string, why dropCause) string {
	if why == droppedForOther {
		return "Bench started with another config while config.toml had problems, so it couldn't keep the arrangement of " +
			joinNames(names) + "."
	}
	had := "When Bench started, config.toml had"
	if why == droppedAtReload {
		had = "The config.toml that Bench reloaded had"
	}
	if len(names) == 1 {
		return had + " no tab called " + names[0] + ", so Bench couldn't keep its arrangement."
	}
	return had + " no tabs called " + joinNames(names) + ", so Bench couldn't keep their arrangement."
}

// noteSetAsideLocked records what Bench couldn't keep of state.json at
// startup: what LoadState or PeekState set aside, or the saved tabs of st
// that no tab keeps the arrangement of, by from (see modelFromTabs). While
// config.toml has problems, Bench says nothing until it's fixed.
func (e *Engine) noteSetAsideLocked(aside *config.SetAside, st *config.State, from []int) {
	if aside != nil && aside.Reason != nil {
		e.aside = &setAsideState{reason: aside.Reason, names: aside.Tabs, quiet: !e.writable}
		return
	}
	if st == nil {
		return
	}
	why := droppedAtStart
	if !e.writable {
		why = droppedForOther
	}
	var dropped []droppedTab
	for _, si := range config.Dropped(from, len(st.Tabs)) {
		dropped = append(dropped, droppedTab{tab: st.Tabs[si], items: st.Items, why: why})
	}
	if len(dropped) > 0 {
		e.aside = &setAsideState{dropped: dropped, quiet: !e.writable}
	}
}

// noteReloadDroppedLocked records the arrangement of the tabs in dropped,
// by index in st, the model before a reload of config.toml in place of old,
// which deleted them because the file has no tabs of their names, as
// startup records state.json's: the notice names them, and Restore previous
// arrangement puts them back once tabs have their names again. Their items
// that the reload moved to Floating, which wasFloating didn't hold, come
// back with them. state.json still has them, so a copy is set aside before
// Bench saves over it.
func (e *Engine) noteReloadDroppedLocked(old *config.Config, st *config.State, dropped []int, wasFloating []string) {
	var tabs []droppedTab
	for _, ti := range dropped {
		ts := st.Tabs[ti]
		if oct, ok := oldTab(old, ti, ts.Name); ok {
			ts.Entries = entryKeys(oct)
		}
		floated := map[string]bool{}
		for _, id := range layout.Items(ts.Root) {
			if e.model.IsFloating(id) && !slices.Contains(wasFloating, id) {
				floated[id] = true
			}
		}
		tabs = append(tabs, droppedTab{tab: ts, items: st.Items, why: droppedAtReload, floated: floated})
	}
	a := e.aside
	if a == nil {
		a = &setAsideState{}
		e.aside = a
	}
	// A tab dropped before with the same name gives way to the later one.
	a.dropped = slices.DeleteFunc(a.dropped, func(d droppedTab) bool {
		return slices.ContainsFunc(tabs, func(n droppedTab) bool { return strings.EqualFold(n.tab.Name, d.tab.Name) })
	})
	a.dropped = append(a.dropped, tabs...)
	if !a.quiet {
		e.setStateAsideLocked(false)
	}
}

// revealSetAsideLocked says what Bench couldn't keep of state.json, now that
// config.toml loads without problems, after Bench started with another
// config: it's about to save over state.json, so it sets it aside first,
// whole if it couldn't use it, or else as a copy.
func (e *Engine) revealSetAsideLocked() {
	a := e.aside
	if a == nil || !a.quiet {
		return
	}
	a.quiet = false
	e.setStateAsideLocked(a.reason != nil)
}

// setStateAsideLocked sets state.json aside as state.json.bad once the lock
// is released, before Bench next saves over it: whole, or as a copy.
func (e *Engine) setStateAsideLocked(whole bool) {
	if e.opts.Headless {
		return
	}
	e.later(func() {
		e.saveMu.Lock()
		defer e.saveMu.Unlock()
		set := config.SetAsideCopy
		if whole {
			set = config.SetAsideWhole
		}
		if err := set(e.statePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			e.log.Warn("couldn't set state.json aside", "error", err)
		}
	})
}

// setAsideNoticesLocked returns the notices that Bench couldn't keep the
// arrangement, with their buttons.
func (e *Engine) setAsideNoticesLocked() []NoticeView {
	a := e.aside
	if a == nil || a.quiet {
		return nil
	}
	var out []NoticeView
	if a.reason != nil {
		out = append(out, NoticeView{Text: wholeSetAsideText(a.names, a.reason), Actions: []NoticeAction{actionDismissArrangement}})
	}
	if len(a.dropped) > 0 {
		restorable := len(e.restorableLocked()) > 0
		actions := []NoticeAction{actionDismissArrangement}
		if restorable {
			actions = slices.Insert(actions, 0, actionRestoreArrangement)
		}
		out = append(out, NoticeView{Text: droppedText(a.dropped, restorable), Actions: actions})
	}
	return out
}

// restorableLocked returns the dropped tabs that Restore previous
// arrangement can put back, by index in dropped, each with the index of the
// tab with its name now.
func (e *Engine) restorableLocked() map[int]int {
	a := e.aside
	out := map[int]int{}
	if a == nil || a.quiet {
		return out
	}
	taken := map[int]bool{}
	for di, d := range a.dropped {
		ti := slices.IndexFunc(e.model.Tabs, func(t *Tab) bool { return strings.EqualFold(t.Name, d.tab.Name) })
		if ti >= 0 && !taken[ti] {
			out[di], taken[ti] = ti, true
		}
	}
	return out
}

// RestoreArrangement puts back the arrangement that Bench couldn't keep, of
// the tabs that config.toml had no tabs for when Bench started or reloaded
// it, into the tabs with their names now. A page or terminal that a tab has
// for the same entry as one of the arrangement's takes its place, so that
// its page or session goes on. The tab's other pages and terminals that the
// arrangement doesn't have move to "Not in a tab", and its windows go back
// to the arrangement's items, or else to "Not in a tab" too.
func (e *Engine) RestoreArrangement() error {
	e.lock()
	defer e.unlock()
	if e.closed {
		return errNotRunning
	}
	a := e.aside
	if a == nil || len(a.dropped) == 0 || a.quiet {
		return errNothingToRestore
	}
	pairs := e.restorableLocked()
	if len(pairs) == 0 {
		return errNoTabToRestore
	}
	m := e.model
	before := maps.Clone(m.Items)
	var names []string
	for _, di := range slices.Sorted(maps.Keys(pairs)) {
		ti := pairs[di]
		e.restoreTabLocked(ti, a.dropped[di])
		names = append(names, m.Tabs[ti].Name)
	}
	var left []droppedTab
	for di, d := range a.dropped {
		if _, ok := pairs[di]; !ok {
			left = append(left, d)
		}
	}
	a.dropped = left
	if len(a.dropped) == 0 && a.reason == nil {
		e.aside = nil
	}
	e.endRemovedItemsLocked(before)
	e.arrangeLocked(arrangeOpts{})
	e.changed()
	e.toastLocked("Restored the arrangement of " + joinNames(names))
	return nil
}

// DismissArrangementNotice takes away the notices that Bench couldn't keep
// the arrangement. The copy in state.json.bad stays.
func (e *Engine) DismissArrangementNotice() error {
	e.lock()
	defer e.unlock()
	if e.closed {
		return errNotRunning
	}
	e.aside = nil
	e.touch()
	return nil
}

// restoreTabLocked gives tab ti the arrangement of dropped tab d, as
// modelFromState would have: its tree, with new IDs for its panes and
// splits, its items, which take its entries' edits since, and the windows
// they had, if Bench has no other item for them. A page or terminal that
// only the tab has, for the same entry as one of the arrangement's, takes
// that one's place, keeping its page or session. The tab's other pages and
// terminals that the arrangement doesn't have, and no other tab has, move to
// Floating, as with Delete tab, and its App items leave the model, for
// endRemovedItemsLocked.
func (e *Engine) restoreTabLocked(ti int, d droppedTab) {
	m := e.model
	t, ts := m.Tabs[ti], d.tab
	had := t.ItemIDs()

	// The tab's pages and terminals that can stand in for the
	// arrangement's, by entry key.
	spare := map[string][]string{}
	for _, id := range had {
		it := m.Items[id]
		if it == nil || it.Kind == KindApp || len(m.TabsOf(id)) > 1 || layout.PaneOf(ts.Root, id) != nil {
			continue
		}
		if keys := itemKeys(it); len(keys) > 0 {
			spare[keys[0]] = append(spare[keys[0]], id)
		}
	}

	root := layout.Clone(ts.Root)
	panes := map[string]string{}
	renumberNodes(root, m, panes)
	stand := map[string]string{} // The items that stand in, by the arrangement's.
	var added []*Item
	for _, p := range layout.Leaves(root) {
		// A window popped out since stays out, but one that left the tab
		// when config.toml was reloaded comes back.
		p.Items = slices.DeleteFunc(p.Items, func(id string) bool { return m.IsFloating(id) && !d.floated[id] })
		for i, id := range p.Items {
			if m.Items[id] != nil {
				continue
			}
			it := itemFromState(id, d.items[id], e.appName)
			if keys := itemKeys(it); it.Kind != KindApp && len(keys) > 0 && len(spare[keys[0]]) > 0 {
				p.Items[i], spare[keys[0]] = spare[keys[0]][0], spare[keys[0]][1:]
				stand[id] = p.Items[i]
				continue
			}
			m.Items[id] = it
			added = append(added, it)
		}
		if s := stand[p.Active]; s != "" {
			p.Active = s
		}
		if !slices.Contains(p.Items, p.Active) {
			p.Active = ""
			if len(p.Items) > 0 {
				p.Active = p.Items[0]
			}
		}
	}
	m.Floating = slices.DeleteFunc(m.Floating, func(id string) bool { return layout.PaneOf(root, id) != nil })
	t.Root, t.Preset = root, ts.Preset
	t.Focus = panes[ts.Focus]
	if ts.Focus == layout.CompactPaneID {
		t.Focus = ts.Focus
	}
	t.CompactShown = ts.CompactShown
	if s := stand[t.CompactShown]; s != "" {
		t.CompactShown = s
	}
	if layout.PaneOf(root, t.CompactShown) == nil {
		t.CompactShown = ""
	}
	t.fixFocus()

	// The tab's items that the arrangement doesn't have leave it: a page or
	// terminal for Floating, so that its page or session goes on, and an App
	// item the model. A window that one of them has, and that an item of the
	// arrangement had, goes back to that item: as when an App entry claimed
	// it at startup, in the tab built from the config. One that another tab
	// has now stays there.
	used := map[string]bool{}
	for _, it := range m.orderedItems() {
		used[it.ID] = true
	}
	left := map[platform.WindowID]*Item{}
	for _, id := range had {
		switch it := m.Items[id]; {
		case it == nil || used[id]:
		case it.Kind != KindApp:
			m.Floating = append(m.Floating, id)
			e.leaveTabLocked(it)
		default:
			if it.WindowID != 0 {
				left[it.WindowID] = it
			}
			delete(m.Items, id)
		}
	}
	for _, it := range added {
		if m.ItemByWindow(it.WindowID) != nil {
			it.WindowID, it.Status = 0, ""
		}
	}
	m.reindex()
	if ci := e.cfg.TabIndex(t.Name); ci >= 0 && ts.Entries != nil {
		m.syncEntries(ti, e.cfg.Tabs[ci], ts.Entries, e.appName, map[string]Item{})
	}

	// The windows of the arrangement's items, as the startup scan finds
	// them.
	unknown := map[platform.WindowID]bool{}
	for _, it := range added {
		if m.Items[it.ID] != it || it.Kind != KindApp || it.WindowID == 0 {
			continue
		}
		w, free := e.unmanaged[it.WindowID]
		switch was := left[it.WindowID]; {
		case was != nil:
			it.Title, it.pid, it.Undocked = was.Title, was.pid, was.Undocked
			it.Status, it.StatusText = was.Status, was.StatusText
		case free && sameApp(w.BundleID, it.BundleID):
			delete(e.unmanaged, w.ID)
			it.Title, it.pid, it.Status = w.Title, w.PID, StatusReady
		case free:
			m.Unbind(it) // The ID now belongs to another app's window.
		default:
			unknown[it.WindowID] = true
		}
	}
	if e.trusted && len(unknown) > 0 {
		live := map[platform.WindowID]bool{}
		for _, it := range m.Items {
			if it.WindowID != 0 && !unknown[it.WindowID] {
				live[it.WindowID] = true
			}
		}
		e.forgetLostWindowsLocked(live)
	}
}

// renumberNodes gives a tree's panes and splits new IDs, recording each
// pane's new ID by its old one in panes.
func renumberNodes(n *layout.Node, m *Model, panes map[string]string) {
	if n == nil {
		return
	}
	id := m.nodeID(n.Type)
	if n.Type == layout.TypePane {
		panes[n.ID] = id
	}
	n.ID = id
	for _, c := range n.Children {
		renumberNodes(c, m, panes)
	}
}

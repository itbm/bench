package workspace

import (
	"cmp"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file has the rules for where windows go: claims (which pane the
// config gives an app's windows), the [[rule]] entries, finding an app's
// most recently used window, and matching windows to remembered items.
// Like the model, it's pure.

// sameApp reports whether two bundle IDs name the same app. macOS compares
// them without regard to case.
func sameApp(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

// fitsFilter reports whether a window title suits an item's title filter:
// there's no filter, or the title contains it (ignoring case).
func fitsFilter(it *Item, title string) bool {
	return it.TitleFilter == "" || containsFold(title, it.TitleFilter)
}

// filterMatches reports whether an item has a title filter that a window
// title contains.
func filterMatches(it *Item, title string) bool {
	return it.TitleFilter != "" && containsFold(title, it.TitleFilter)
}

// isAppOf reports whether an item is an App item of an app.
func isAppOf(it *Item, bundleID string) bool {
	return it != nil && it.Kind == KindApp && sameApp(it.BundleID, bundleID)
}

// orderedItems returns every item once: the tabs' items in tab and reading
// order (an item in several tabs where it's first), then the floating ones.
func (m *Model) orderedItems() []*Item {
	var out []*Item
	seen := map[string]bool{}
	add := func(id string) {
		if it := m.Items[id]; it != nil && !seen[id] {
			seen[id] = true
			out = append(out, it)
		}
	}
	for _, t := range m.Tabs {
		for _, id := range t.ItemIDs() {
			add(id)
		}
	}
	for _, id := range m.Floating {
		add(id)
	}
	return out
}

// Claim returns the pane a window of an app belongs to by claims: the pane
// whose App entry for the app has a title filter the title contains, or
// else the first pane, across all tabs in order, listing the app without a
// title. The model's items count first (they show where the config's items
// are now), then the config's entries (for an app whose items have all
// gone), mapping a config pane to the tab's pane in the same place in
// reading order.
func (m *Model) Claim(cfg *config.Config, bundleID, title string) (int, *layout.Node, bool) {
	if ti, p, ok := m.claimByItems(bundleID, title, true); ok {
		return ti, p, true
	}
	if ti, p, ok := m.claimByConfig(cfg, bundleID, title, true); ok {
		return ti, p, true
	}
	if ti, p, ok := m.claimByItems(bundleID, title, false); ok {
		return ti, p, true
	}
	return m.claimByConfig(cfg, bundleID, title, false)
}

// FilterClaim is Claim limited to title filters: it reports a pane only if
// an entry with a title filter claims the window.
func (m *Model) FilterClaim(cfg *config.Config, bundleID, title string) (int, *layout.Node, bool) {
	if ti, p, ok := m.claimByItems(bundleID, title, true); ok {
		return ti, p, true
	}
	return m.claimByConfig(cfg, bundleID, title, true)
}

// claimByItems finds the first App item of an app in a tab whose title
// filter the title contains (filtered), or that has no title filter.
func (m *Model) claimByItems(bundleID, title string, filtered bool) (int, *layout.Node, bool) {
	for ti, t := range m.Tabs {
		for _, p := range t.Leaves() {
			for _, id := range p.Items {
				it := m.Items[id]
				if !isAppOf(it, bundleID) {
					continue
				}
				if filtered && filterMatches(it, title) || !filtered && it.TitleFilter == "" {
					return ti, p, true
				}
			}
		}
	}
	return -1, nil, false
}

// claimByConfig finds the first config entry for an app whose title filter
// the title contains (filtered), or that has no title filter.
func (m *Model) claimByConfig(cfg *config.Config, bundleID, title string, filtered bool) (int, *layout.Node, bool) {
	if cfg == nil {
		return -1, nil, false
	}
	for ci, ct := range cfg.Tabs {
		for pj, cp := range ct.Panes {
			for _, entry := range cp.Items {
				if !sameApp(entry.App, bundleID) {
					continue
				}
				fits := filtered && entry.Title != "" && containsFold(title, entry.Title) ||
					!filtered && entry.Title == ""
				if !fits {
					continue
				}
				if ti, p := m.paneForConfig(cfg, ci, pj); p != nil {
					return ti, p, true
				}
			}
		}
	}
	return -1, nil, false
}

// paneForConfig maps config tab ci's pane pj to the model: the tab with the
// same name, and its pane in the same place in reading order (or its last
// pane, if it has fewer).
func (m *Model) paneForConfig(cfg *config.Config, ci, pj int) (int, *layout.Node) {
	ti := ci
	if t := m.Tab(ti); t == nil || !strings.EqualFold(t.Name, cfg.Tabs[ci].Name) {
		ti = slices.IndexFunc(m.Tabs, func(t *Tab) bool { return strings.EqualFold(t.Name, cfg.Tabs[ci].Name) })
	}
	t := m.Tab(ti)
	if t == nil {
		return -1, nil
	}
	leaves := t.Leaves()
	if len(leaves) == 0 {
		return -1, nil
	}
	return ti, leaves[min(pj, len(leaves)-1)]
}

// Placeholder returns an App item of an app in a pane that has no window and
// whose title filter suits the title, preferring one whose filter the title
// contains. It returns nil if there's none.
func (m *Model) Placeholder(p *layout.Node, bundleID, title string) *Item {
	var fallback *Item
	for _, id := range p.Items {
		it := m.Items[id]
		if !isAppOf(it, bundleID) || it.WindowID != 0 || !fitsFilter(it, title) {
			continue
		}
		if filterMatches(it, title) {
			return it
		}
		if fallback == nil {
			fallback = it
		}
	}
	return fallback
}

// afterApp returns the index just after the last item of an app in a pane,
// or -1 (the end) if the pane has none.
func (m *Model) afterApp(p *layout.Node, bundleID string) int {
	at := -1
	for i, id := range p.Items {
		if isAppOf(m.Items[id], bundleID) {
			at = i + 1
		}
	}
	return at
}

// RecentPane returns the pane of tab ti holding the app's most recently used
// window: of its App items with a window, the one used last, or the first
// in reading order if none has been used. It returns nil if the tab has no
// window of the app.
func (m *Model) RecentPane(ti int, bundleID string) *layout.Node {
	t := m.Tab(ti)
	if t == nil {
		return nil
	}
	var best *layout.Node
	bestUsed := -1
	for _, p := range t.Leaves() {
		for _, id := range p.Items {
			it := m.Items[id]
			if isAppOf(it, bundleID) && it.WindowID != 0 && it.Used > bestUsed {
				best, bestUsed = p, it.Used
			}
		}
	}
	return best
}

// RecentPaneElsewhere is RecentPane across every tab but the one at
// exclude: the most recently used window wins, or the first tab's if none
// has been used.
func (m *Model) RecentPaneElsewhere(exclude int, bundleID string) (int, *layout.Node) {
	bestTab, bestUsed := -1, -1
	var best *layout.Node
	for ti, t := range m.Tabs {
		if ti == exclude {
			continue
		}
		for _, p := range t.Leaves() {
			for _, id := range p.Items {
				it := m.Items[id]
				if isAppOf(it, bundleID) && it.WindowID != 0 && it.Used > bestUsed {
					best, bestTab, bestUsed = p, ti, it.Used
				}
			}
		}
	}
	return bestTab, best
}

// matchRule applies the [[rule]] entries to a new window: float reports
// whether a rule floats it (float rules win), and tab names the tab of the
// first rule that sends it to one.
func matchRule(rules []config.Rule, bundleID, title string) (float bool, tab string) {
	for _, r := range rules {
		if !sameApp(r.App, bundleID) || (r.Title != "" && !containsFold(title, r.Title)) {
			continue
		}
		if r.Float {
			return true, ""
		}
		if tab == "" {
			tab = r.Tab
		}
	}
	return false, tab
}

// sortByCreated sorts windows by the order they were first seen.
func sortByCreated(ws []platform.Window) {
	slices.SortStableFunc(ws, func(a, b platform.Window) int { return cmp.Compare(a.Created, b.Created) })
}

// MatchRemembered binds windows to App items that have none, among the
// app's items that had a window before (an app that quit and opened again,
// or Bench restarting after the app did): first by bundle ID and exact
// title, then by creation order. Title filters are respected throughout.
// Windows should be in creation order. It returns the windows it bound and
// those it didn't.
func (m *Model) MatchRemembered(ws []platform.Window) (bound []*Item, rest []platform.Window) {
	bound, rest = m.MatchByTitle(ws)
	more, rest := m.MatchByCreation(rest)
	return append(bound, more...), rest
}

// MatchByTitle is the first half of MatchRemembered: it binds windows to
// remembered items of their apps by exact title.
func (m *Model) MatchByTitle(ws []platform.Window) (bound []*Item, rest []platform.Window) {
	return m.matchBy(ws, m.byTitle)
}

// MatchByCreation is the second half of MatchRemembered: it binds windows
// to remembered items of their apps by creation order.
func (m *Model) MatchByCreation(ws []platform.Window) (bound []*Item, rest []platform.Window) {
	return m.matchBy(ws, m.byCreation)
}

// matchBy binds each window to the item that find returns for it, if any.
func (m *Model) matchBy(ws []platform.Window, find func(platform.Window) *Item) (bound []*Item, rest []platform.Window) {
	for _, w := range ws {
		if it := find(w); it != nil {
			m.Bind(it, w)
			bound = append(bound, it)
			continue
		}
		rest = append(rest, w)
	}
	return bound, rest
}

// byTitle returns the first remembered item of the window's app without a
// window whose last title is the window's title. Only items that had a
// window count: a placeholder made from the config has its app's name as
// its title, which is often the title of the app's main window too (as with
// Calendar or Notes), and it must be left for rules and new_windows.
func (m *Model) byTitle(w platform.Window) *Item {
	for _, it := range m.orderedItems() {
		if isAppOf(it, w.BundleID) && it.WindowID == 0 && it.Created != 0 && it.Title == w.Title && fitsFilter(it, w.Title) {
			return it
		}
	}
	return nil
}

// byCreation returns the remembered item of the window's app without a
// window that was created first, preferring one whose title filter the
// window's title contains.
func (m *Model) byCreation(w platform.Window) *Item {
	var best *Item
	better := func(it *Item) bool {
		if best == nil {
			return true
		}
		if fm, bm := filterMatches(it, w.Title), filterMatches(best, w.Title); fm != bm {
			return fm
		}
		return it.Created < best.Created
	}
	for _, it := range m.orderedItems() {
		if !isAppOf(it, w.BundleID) || it.WindowID != 0 || it.Created == 0 || !fitsFilter(it, w.Title) {
			continue
		}
		if better(it) {
			best = it
		}
	}
	return best
}

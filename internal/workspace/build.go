package workspace

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file turns the config and state.json into a model, and the model
// back into state.json and config entries.

// namer returns an app's name from its bundle ID.
type namer func(bundleID string) string

// Names and monograms of the Built-in and VS Code items.
const (
	terminalApp   = "Terminal"
	vscodeApp     = "VS Code"
	vscodeProfile = "vscode"
	defaultFolder = "~"
	defaultShell  = "zsh"
)

// smallScreensOf returns a tab's small-screen setting, stack if it's empty.
func smallScreensOf(s string) layout.SmallScreens {
	if s == "" {
		return layout.Stack
	}
	return layout.SmallScreens(s)
}

// hostOf returns an address's host in lower case, or "".
func hostOf(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// terminalTitle is a terminal's title until its session reports one.
func terminalTitle(name, folder string) string {
	if name != "" {
		return name
	}
	return defaultShell + " — " + folder
}

// itemFromConfig makes an item from a config entry: App entries become
// placeholders, waiting for a window.
func itemFromConfig(ci config.Item, appName namer) *Item {
	switch {
	case ci.App != "":
		name := appName(ci.App)
		return &Item{Kind: KindApp, App: name, BundleID: ci.App, Mono: apps.Mono(name), Title: name,
			TitleFilter: ci.Title, OpenURL: ci.URL, Status: StatusMissing}
	case ci.Web != "":
		app := ci.Name
		if app == "" {
			app = apps.WebName(ci.Web)
		}
		profile := ci.Profile
		if profile == "" {
			profile = hostOf(ci.Web)
		}
		return &Item{Kind: KindWeb, App: app, Mono: apps.Mono(app), Title: app, Name: ci.Name,
			URL: ci.Web, Home: ci.Web, Profile: profile, Status: StatusReady}
	case ci.VSCode != "":
		title := ci.Name
		if title == "" {
			title = ci.VSCode
		}
		return &Item{Kind: KindWeb, App: vscodeApp, Mono: apps.Mono(vscodeApp), Title: title, Name: ci.Name,
			VSCode: ci.VSCode, Profile: vscodeProfile, Status: StatusReady}
	default:
		folder := ci.Terminal
		if folder == "" {
			folder = defaultFolder
		}
		return &Item{Kind: KindBuiltin, App: terminalApp, Mono: apps.Mono(terminalApp),
			Title: terminalTitle(ci.Name, folder), Name: ci.Name, Folder: folder, Command: ci.Command,
			Run: ci.Run, StartAgain: ci.Run != "" && ci.StartsAgain(), Status: StatusReady}
	}
}

// modelFromConfig builds the model from the config: each tab's tree from
// its preset and sizes, with a pane for each [[tab.pane]] (and empty panes
// for the ones it leaves out). Items get IDs first, in config order, then
// panes and splits.
func modelFromConfig(cfg *config.Config, appName namer) (*Model, error) {
	m := newModel()
	paneItems := make([][][]string, len(cfg.Tabs))
	for ti, ct := range cfg.Tabs {
		paneItems[ti] = make([][]string, len(ct.Panes))
		for pj, cp := range ct.Panes {
			for _, ci := range cp.Items {
				it := itemFromConfig(ci, appName)
				it.ID = m.NewID(itemPrefix)
				m.Items[it.ID] = it
				paneItems[ti][pj] = append(paneItems[ti][pj], it.ID)
			}
		}
	}
	for ti, ct := range cfg.Tabs {
		panes := make([]*layout.Node, len(ct.Panes))
		for pj := range ct.Panes {
			panes[pj] = layout.NewPane(m.NewID(panePrefix), paneItems[ti][pj]...)
		}
		root, err := layout.Build(ct.Layout, panes, ct.Sizes, m.nodeID)
		if err != nil {
			return nil, fmt.Errorf("building tab %q: %w", ct.Name, err)
		}
		t := &Tab{Name: ct.Name, Preset: ct.Layout, SmallScreens: smallScreensOf(ct.SmallScreens), Root: root}
		t.fixFocus()
		m.Tabs = append(m.Tabs, t)
	}
	return m, nil
}

// addConfigTab appends a tab built from a config tab, as modelFromConfig
// builds each: its tree from its preset and sizes, with a pane for each
// [[tab.pane]] holding items for its entries (App entries as placeholders),
// and empty panes for the ones it leaves out.
func (m *Model) addConfigTab(ct config.Tab, appName namer) (*Tab, error) {
	panes := make([]*layout.Node, len(ct.Panes))
	var ids []string
	for pj, cp := range ct.Panes {
		var items []string
		for _, ci := range cp.Items {
			it := itemFromConfig(ci, appName)
			it.ID = m.NewID(itemPrefix)
			m.Items[it.ID] = it
			items = append(items, it.ID)
		}
		ids = append(ids, items...)
		panes[pj] = layout.NewPane(m.NewID(panePrefix), items...)
	}
	root, err := layout.Build(ct.Layout, panes, ct.Sizes, m.nodeID)
	if err != nil {
		for _, id := range ids {
			delete(m.Items, id)
		}
		return nil, fmt.Errorf("building tab %q: %w", ct.Name, err)
	}
	t := &Tab{Name: ct.Name, Preset: ct.Layout, SmallScreens: smallScreensOf(ct.SmallScreens), Root: root}
	t.fixFocus()
	m.Tabs = append(m.Tabs, t)
	return t, nil
}

// modelFromState builds the model from state.json, which LoadState has
// checked, keeping the arrangement of each tab that the config still has
// (see config.MatchTabs): the tabs take the config's names and order, a
// tab new to the config is built from it, and a tab that the config no
// longer has is dropped, with its items that no other tab has. The windows
// of those find places as other windows do at startup. Small-screen
// settings come from the config, which state.json doesn't repeat.
func modelFromState(st *config.State, cfg *config.Config, appName namer) *Model {
	return modelFromTabs(st, cfg, stateTabs(st, cfg), appName)
}

// modelFromTabs is modelFromState, given which saved tab each of the
// config's tabs keeps the arrangement of, by index, or -1 for none.
func modelFromTabs(st *config.State, cfg *config.Config, from []int, appName namer) *Model {
	m := newModel()
	m.Mode = layout.Mode(st.Mode)
	m.Seq = st.Seq
	for id, is := range st.Items {
		m.Items[id] = itemFromState(id, is, appName)
	}
	m.Floating = append([]string(nil), st.Floating...)
	tabs := make([]*Tab, len(cfg.Tabs))
	for ci, si := range from {
		if si < 0 {
			continue
		}
		ts := st.Tabs[si]
		tabs[ci] = &Tab{
			Name:         cfg.Tabs[ci].Name,
			Preset:       ts.Preset,
			FromPreset:   ts.FromPreset,
			SmallScreens: smallScreensOf(cfg.Tabs[ci].SmallScreens),
			Root:         layout.Clone(ts.Root),
			Focus:        ts.Focus,
			CompactShown: ts.CompactShown,
		}
		tabs[ci].fixFocus()
	}
	if ci := slices.Index(from, st.Current); ci >= 0 {
		m.Current = ci
	} else {
		// As when the current tab is deleted: the tab in its place.
		m.Current = max(min(st.Current, len(cfg.Tabs)-1), 0)
	}
	m.Tabs = slices.DeleteFunc(slices.Clone(tabs), func(t *Tab) bool { return t == nil })
	m.dropUnused()
	// New tabs' IDs come after the state's, so that they can't repeat one
	// that a dropped tab had: Restore previous arrangement puts it back.
	for ci, ct := range cfg.Tabs {
		if tabs[ci] != nil {
			continue
		}
		t, err := m.addConfigTab(ct, appName)
		if err != nil {
			// The config has been validated, so this would be a bug: an
			// empty tab keeps the tabs in step with the config.
			t = &Tab{Name: ct.Name, Preset: ct.Layout, SmallScreens: smallScreensOf(ct.SmallScreens),
				Root: layout.NewPane(m.NewID(panePrefix))}
			t.fixFocus()
		}
		tabs[ci] = t
	}
	m.Tabs = tabs
	m.reindex()
	given := map[string]Item{}
	for ci, si := range from {
		if si >= 0 && st.Tabs[si].Entries != nil {
			m.syncEntries(ci, cfg.Tabs[ci], st.Tabs[si].Entries, appName, given)
		}
	}
	return m
}

// stateTabs returns, for each of the config's tabs, the index of the tab in
// state.json whose arrangement it keeps, or -1 (see config.MatchTabs).
func stateTabs(st *config.State, cfg *config.Config) []int {
	return config.MatchTabs(st.TabNames(), cfg.TabNames())
}

// sameTabs reports whether state.json has the config's tabs, by name,
// ignoring case and order.
func sameTabs(st *config.State, cfg *config.Config) bool {
	had := st.TabNames()
	return len(had) == len(cfg.Tabs) && !slices.ContainsFunc(cfg.Tabs, func(ct config.Tab) bool {
		return !slices.ContainsFunc(had, func(name string) bool { return strings.EqualFold(name, ct.Name) })
	})
}

// dropUnused forgets the items that are in no tab and not floating.
func (m *Model) dropUnused() {
	used := map[string]bool{}
	for _, t := range m.Tabs {
		for _, id := range t.ItemIDs() {
			used[id] = true
		}
	}
	for _, id := range m.Floating {
		used[id] = true
	}
	for id := range m.Items {
		if !used[id] {
			delete(m.Items, id)
		}
	}
}

// entryKey identifies a config entry, for telling whether a tab's items
// include it: an App entry by its app and title filter, a Web entry by its
// address, a VS Code entry by its folder, and a terminal by its folder and
// command, and the command it types if it has one. A terminal without run
// has the key it had before run.
func entryKey(ci config.Item) string {
	switch {
	case ci.App != "":
		return "app\x00" + strings.ToLower(ci.App) + "\x00" + ci.Title
	case ci.Web != "":
		return "web\x00" + ci.Web
	case ci.VSCode != "":
		return "vscode\x00" + ci.VSCode
	}
	folder := ci.Terminal
	if folder == "" {
		folder = defaultFolder
	}
	key := "terminal\x00" + folder + "\x00" + ci.Command
	if ci.Run != "" {
		key += "\x00run\x00" + ci.Run
	}
	return key
}

// itemKeys returns the entry keys an item can stand for: an App item's
// entry, a Web item's given address and current one, a VS Code item's
// folder, or a terminal's folder and command.
func itemKeys(it *Item) []string {
	switch {
	case it.Kind == KindApp:
		return []string{entryKey(config.Item{App: it.BundleID, Title: it.TitleFilter})}
	case it.Kind == KindBuiltin:
		return []string{entryKey(config.Item{Terminal: it.Folder, Command: it.Command, Run: it.Run})}
	case it.VSCode != "":
		return []string{entryKey(config.Item{VSCode: it.VSCode})}
	}
	var keys []string
	for _, address := range []string{it.Home, it.URL} {
		if k := entryKey(config.Item{Web: address}); address != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// entryKeys returns the keys of a config tab's entries, in config order.
func entryKeys(ct config.Tab) []string {
	out := []string{}
	for _, cp := range ct.Panes {
		for _, ci := range cp.Items {
			out = append(out, entryKey(ci))
		}
	}
	return out
}

// configEntry is a config entry and the config pane it's in.
type configEntry struct {
	item config.Item
	pane int
}

// missingEntries returns tab ti's config entries that none of its items
// stand for, in config order. Each Web, VS Code, and terminal entry needs
// an item of its own; an App entry needs only one of its app's items with
// the same title filter, since it claims all of them. Entries whose keys are
// in skip (as many of each as skip lists) aren't returned.
func (m *Model) missingEntries(ti int, ct config.Tab, skip []string) []configEntry {
	have := map[string]int{}
	for _, id := range m.Tabs[ti].ItemIDs() {
		it := m.Items[id]
		if it == nil {
			continue
		}
		for _, k := range itemKeys(it) {
			have[k]++
		}
	}
	skipped := map[string]int{}
	for _, k := range skip {
		skipped[k]++
	}
	var out []configEntry
	for j, cp := range ct.Panes {
		for _, ci := range cp.Items {
			k := entryKey(ci)
			if skipped[k] > 0 {
				skipped[k]--
				if ci.App == "" {
					have[k]-- // That entry's item, if it has one.
				}
				continue
			}
			if have[k] > 0 {
				if ci.App == "" {
					have[k]--
				}
				continue
			}
			out = append(out, configEntry{item: ci, pane: j})
		}
	}
	return out
}

// addEntryItem makes an item for a config entry and adds it to the end of a
// pane of tab ti.
func (m *Model) addEntryItem(ti int, p *layout.Node, ci config.Item, appName namer) {
	it := itemFromConfig(ci, appName)
	it.ID = m.NewID(itemPrefix)
	m.Add(it, m.Tabs[ti], p, -1)
}

// syncEntries brings tab ti's items into line with config tab ct, which the
// user may have edited by hand while Bench wasn't running; saved lists the
// keys its entries had when state.json was saved. state.json still decides
// the arrangement, but:
//
//   - the items of an entry that's still there take its other fields, such
//     as a Web item's name and profile or an App item's url;
//   - a Web, VS Code, or terminal entry whose key changed (a Web item's
//     address, or a terminal's command, say) changes its item in place, if
//     it's still in the same place among the tab's other entries;
//   - the items of an entry that was removed go, leaving their panes in
//     place, as the config does;
//   - an entry that was added gets an item, unless one stands for it
//     already, in the tab's pane in the same place in reading order as its
//     config pane.
//
// An App entry whose title filter changed is a new entry: its app's windows
// that it claimed may not suit the new filter, so their items go, and the
// startup scan places the windows again.
//
// An item in several tabs can be listed in each of their entries, and
// stays in the tabs whose entries still list it. given holds, for each item
// an earlier tab's entry has given its settings, what state.json remembered
// of it: a later tab's entry that would leave the item as it was is one the
// user didn't edit, so it doesn't undo another tab's edit.
//
// So the next save from Tab properties, which writes the items back as
// entries, keeps the user's edits. Entries the user removed from the tab
// in Bench, and so are still in the config, aren't added back.
func (m *Model) syncEntries(ti int, ct config.Tab, saved []string, appName namer, given map[string]Item) {
	var entries []configEntry
	var keys []string
	for j, cp := range ct.Panes {
		for _, ci := range cp.Items {
			entries = append(entries, configEntry{item: ci, pane: j})
			keys = append(keys, entryKey(ci))
		}
	}
	from, gone := pairEntries(saved, keys)
	claimed := map[string]bool{}
	var added []configEntry
	for i, e := range entries {
		var its []*Item
		if from[i] != "" {
			its = m.entryItems(ti, from[i], claimed)
		}
		if len(its) == 0 && from[i] != keys[i] {
			// A new entry, or one edited in place whose item was closed in
			// Bench: an item added in Bench may stand for it already.
			if its = m.entryItems(ti, keys[i], claimed); len(its) == 0 {
				added = append(added, e)
			}
		}
		for _, it := range its {
			claimed[it.ID] = true
			was, ok := given[it.ID]
			if !ok {
				given[it.ID] = *it
				applyEntry(it, e.item, appName)
				continue
			}
			edited := was
			if applyEntry(&edited, e.item, appName); edited != was {
				applyEntry(it, e.item, appName)
			}
		}
	}
	t := m.Tabs[ti]
	for _, key := range gone {
		for _, it := range m.entryItems(ti, key, claimed) {
			claimed[it.ID] = true
			// Taking the item out with Take would close up its pane, and
			// take it out of its other tabs too, which keep it.
			m.DropFrom(it.ID, ti)
		}
	}
	leaves := t.Leaves()
	if len(leaves) == 0 {
		return
	}
	for _, e := range added {
		m.addEntryItem(ti, leaves[min(e.pane, len(leaves)-1)], e.item, appName)
	}
}

// pairEntries matches a tab's entry keys now with those it had when
// state.json was saved, which line up in the same order except where the
// user edited the config. from gives, for each key now, the saved key it
// continues: the same key, or, for a Web, VS Code, or terminal entry edited
// in place, the key of the entry of the same kind that it replaced between
// the same unchanged neighbours; or "" for a new entry. gone lists the
// saved keys that no entry continues.
func pairEntries(saved, keys []string) (from, gone []string) {
	// common[i][j] is the length of the longest common subsequence of
	// saved[i:] and keys[j:].
	common := make([][]int, len(saved)+1)
	for i := range common {
		common[i] = make([]int, len(keys)+1)
	}
	for i := len(saved) - 1; i >= 0; i-- {
		for j := len(keys) - 1; j >= 0; j-- {
			if saved[i] == keys[j] {
				common[i][j] = common[i+1][j+1] + 1
			} else {
				common[i][j] = max(common[i+1][j], common[i][j+1])
			}
		}
	}
	from = make([]string, len(keys))
	var removed []string // Saved keys since the last unchanged one.
	var inserted []int   // Indexes of keys since the last unchanged one.
	flush := func() {
		for _, j := range inserted {
			kind := entryKind(keys[j])
			if kind == "app" {
				continue
			}
			if k := slices.IndexFunc(removed, func(s string) bool { return entryKind(s) == kind }); k >= 0 {
				from[j] = removed[k]
				removed = slices.Delete(removed, k, k+1)
			}
		}
		gone = append(gone, removed...)
		removed, inserted = nil, nil
	}
	i, j := 0, 0
	for i < len(saved) || j < len(keys) {
		switch {
		case i < len(saved) && j < len(keys) && saved[i] == keys[j]:
			flush()
			from[j] = keys[j]
			i++
			j++
		case j == len(keys) || i < len(saved) && common[i+1][j] >= common[i][j+1]:
			removed = append(removed, saved[i])
			i++
		default:
			inserted = append(inserted, j)
			j++
		}
	}
	flush()
	return from, gone
}

// entryKind returns the kind of entry a key is for: app, web, vscode, or
// terminal.
func entryKind(key string) string {
	kind, _, _ := strings.Cut(key, "\x00")
	return kind
}

// entryItems returns the items of tab ti, in reading order, that were made
// for an entry key and aren't in claimed: all of them for an App entry,
// which claims every window of its app with its title filter, or else the
// first. A Web item counts by the address it was given, not by the page it
// has gone to since, which can be another entry's.
func (m *Model) entryItems(ti int, key string, claimed map[string]bool) []*Item {
	all := entryKind(key) == "app"
	var out []*Item
	for _, id := range m.Tabs[ti].ItemIDs() {
		it := m.Items[id]
		if it == nil || claimed[id] {
			continue
		}
		if keys := itemKeys(it); len(keys) == 0 || keys[0] != key {
			continue
		}
		out = append(out, it)
		if !all {
			break
		}
	}
	return out
}

// applyEntry gives an item its config entry's settings, keeping what
// state.json remembers of it otherwise, such as an App item's window or
// the page a Web item has gone to. An item whose name or address changed
// takes the new one as its title too, until its window or session sets
// one.
func applyEntry(it *Item, ci config.Item, appName namer) {
	fresh := itemFromConfig(ci, appName)
	switch {
	case it.Kind == KindApp:
		it.TitleFilter, it.OpenURL = fresh.TitleFilter, fresh.OpenURL
	case it.Kind == KindBuiltin:
		if it.Name != fresh.Name || it.Folder != fresh.Folder || it.Command != fresh.Command || it.Run != fresh.Run {
			it.Title = fresh.Title
		}
		it.Name, it.Folder, it.Command = fresh.Name, fresh.Folder, fresh.Command
		it.Run, it.StartAgain = fresh.Run, fresh.StartAgain
	case it.VSCode != "":
		if it.Name != fresh.Name || it.VSCode != fresh.VSCode {
			it.Title = fresh.Title
		}
		it.Name, it.VSCode = fresh.Name, fresh.VSCode
	default:
		moved := it.Home != fresh.Home
		if moved {
			it.Home, it.URL = fresh.Home, fresh.URL
		}
		if moved || it.Name != fresh.Name {
			it.Title = fresh.Title
		}
		it.Name, it.Profile, it.App, it.Mono = fresh.Name, fresh.Profile, fresh.App, fresh.Mono
	}
}

// itemFromState makes an item from what state.json remembers.
func itemFromState(id string, s config.ItemState, appName namer) *Item {
	it := &Item{
		ID: id, Kind: s.Kind, BundleID: s.BundleID, WindowID: platform.WindowID(s.WindowID),
		Title: s.Title, Created: s.Created, Name: s.Name, URL: s.URL, Home: s.Home,
		Profile: s.Profile, VSCode: s.VSCode, Folder: s.Folder, Command: s.Command,
		Run: s.Run, StartAgain: s.StartAgain, TitleFilter: s.TitleFilter, OpenURL: s.OpenURL, OwnWindow: s.OwnWindow, Status: StatusReady,
	}
	if it.Home == "" {
		it.Home = it.URL // A file from before Home was saved.
	}
	if it.Kind == KindApp {
		it.OwnWindow = false
	} else {
		it.Label = s.Label
	}
	if it.Kind == KindWeb {
		it.Zoom = savedZoom(s.Zoom)
	}
	switch {
	case it.Kind == KindApp:
		it.App = appName(it.BundleID)
		it.Status = StatusMissing
	case it.Kind == KindBuiltin:
		it.App = terminalApp
		if it.Folder == "" {
			it.Folder = defaultFolder
		}
		if plainShell(it) {
			it.LastFolder = s.LastFolder
		}
		it.savedOutput = true
		if it.Title == "" {
			it.Title = terminalTitle(it.Name, it.Folder)
		}
	case it.VSCode != "":
		it.App = vscodeApp
		// A VS Code for the Web item has no address of its own (see
		// handleWebEvent), but a state.json from an older Bench can hold
		// one, with the server's connection token in it.
		it.URL, it.Home = "", ""
	default:
		it.App = it.Name
		if it.App == "" {
			it.App = apps.WebName(it.Home)
		}
	}
	it.Mono = apps.Mono(it.App)
	if it.Title == "" {
		it.Title = it.App
	}
	return it
}

// State returns the model as state.json.
func (m *Model) State() *config.State {
	st := &config.State{
		Version:  config.StateVersion,
		Current:  m.Current,
		Mode:     string(m.Mode),
		Seq:      m.Seq,
		Items:    map[string]config.ItemState{},
		Floating: append([]string{}, m.Floating...),
	}
	for _, t := range m.Tabs {
		st.Tabs = append(st.Tabs, tabState(t))
	}
	for _, it := range m.orderedItems() {
		st.Items[it.ID] = itemState(it, m.IsFloating(it.ID))
	}
	return st
}

// tabState returns a tab as state.json saves it, with a copy of its tree.
// Only a custom tab keeps the preset it had (FromPreset).
func tabState(t *Tab) config.TabState {
	ts := config.TabState{
		Name: t.Name, Preset: t.Preset, Root: layout.Clone(t.Root),
		Focus: t.Focus, CompactShown: t.CompactShown,
	}
	if t.Preset == layout.PresetCustom {
		ts.FromPreset = t.FromPreset
	}
	return ts
}

// itemState returns an item as state.json saves it. Only a floating item
// keeps its window of its own, and only a plain shell the folder it was
// last in, where it starts again.
func itemState(it *Item, floating bool) config.ItemState {
	s := config.ItemState{
		Kind: it.Kind, BundleID: it.BundleID, WindowID: uint32(it.WindowID), Title: it.Title,
		Created: it.Created, Name: it.Name, Label: it.Label, URL: it.URL, Home: it.Home, Profile: it.Profile,
		VSCode: it.VSCode, Folder: it.Folder, Command: it.Command, TitleFilter: it.TitleFilter,
		OpenURL: it.OpenURL, OwnWindow: it.OwnWindow && floating,
		Run: it.Run, StartAgain: it.StartAgain && it.Run != "", Zoom: it.Zoom,
	}
	if plainShell(it) {
		s.LastFolder = it.LastFolder
	}
	return s
}

// configPanes returns a tab's items as [[tab.pane]] entries for a preset
// with the given number of panes: panes beyond that join the last one, and
// trailing empty panes are left out. Each pane lists each app once (with
// its title filter and url), and every Web and Built-in item.
func (m *Model) configPanes(t *Tab, slots int) []config.Pane {
	return m.configPanesKeeping(t, slots, nil)
}

// configPanesKeeping is configPanes for only the items that keep reports
// true for, or for every item if keep is nil.
func (m *Model) configPanesKeeping(t *Tab, slots int, keep func(*Item) bool) []config.Pane {
	if slots < 1 {
		slots = 1
	}
	panes := make([]config.Pane, slots)
	seen := make([]map[string]bool, slots)
	for j, p := range t.Leaves() {
		k := min(j, slots-1)
		if seen[k] == nil {
			seen[k] = map[string]bool{}
		}
		for _, id := range p.Items {
			it := m.Items[id]
			if it == nil || (keep != nil && !keep(it)) {
				continue
			}
			ci, key, ok := configItem(it)
			if !ok || (key != "" && seen[k][key]) {
				continue
			}
			if key != "" {
				seen[k][key] = true
			}
			panes[k].Items = append(panes[k].Items, ci)
		}
	}
	for len(panes) > 0 && len(panes[len(panes)-1].Items) == 0 {
		panes = panes[:len(panes)-1]
	}
	return panes
}

// configItem returns an item as a config entry, and a key that's the same
// for App items that make the same entry ("" for other kinds). It reports
// false for an item that can't be written, such as a Web item with no
// secure address.
func configItem(it *Item) (config.Item, string, bool) {
	switch {
	case it.Kind == KindApp:
		ci := config.Item{App: it.BundleID, Title: it.TitleFilter}
		if it.TitleFilter != "" {
			ci.URL = it.OpenURL
		}
		return ci, strings.ToLower(it.BundleID) + "\x00" + ci.Title + "\x00" + ci.URL, true
	case it.Kind == KindBuiltin:
		folder := it.Folder
		if folder == "" {
			folder = defaultFolder
		}
		ci := config.Item{Terminal: folder, Command: it.Command, Name: it.Name, Run: it.Run}
		if it.Run != "" {
			ci.StartAgain = config.StartAgainOf(it.StartAgain)
		}
		return ci, "", true
	case it.VSCode != "":
		return config.Item{VSCode: it.VSCode, Name: it.Name}, "", true
	}
	// The address the item was given, not the page it has navigated to
	// (such as a sign-in page, or a Slack channel).
	address := it.Home
	if !secureAddress(address) {
		address = it.URL
	}
	if !secureAddress(address) {
		return config.Item{}, "", false
	}
	ci := config.Item{Web: address, Name: it.Name}
	if it.Profile != "" && it.Profile != hostOf(address) {
		ci.Profile = it.Profile
	}
	return ci, "", true
}

// secureAddress reports whether an address can be a Web item's address in
// the config: https, with a host.
func secureAddress(address string) bool {
	_, err := normaliseAddress(address)
	return err == nil && strings.HasPrefix(strings.ToLower(address), "https://")
}

// normaliseAddress turns what the user typed into a Web item's address:
// "https://" is added if it doesn't start with a scheme (its query can hold
// another address), and anything but https with a host is refused.
func normaliseAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errNoAddress
	}
	if !apps.HasScheme(s) {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return "", errNotHTTPS
	}
	return s, nil
}

// ResetTab rebuilds tab ti from its config entry: the config's preset and
// sizes, keeping every item (layout.ApplyPreset), then moving each item
// into the pane its config entry claims, and adding items (App entries as
// placeholders) for the config entries that no item stands for any more,
// such as a closed web page. The preset is no longer custom.
func (m *Model) ResetTab(ti int, ct config.Tab, appName namer) error {
	t := m.Tab(ti)
	if t == nil {
		return errNoTab(ti)
	}
	root, err := layout.ApplyPreset(t.Root, ct.Layout, ct.Sizes, m.nodeID)
	if err != nil {
		return errNoPreset(ct.Layout)
	}
	leaves := layout.Leaves(root)
	for _, id := range layout.Items(root) {
		it := m.Items[id]
		j, ok := entryPane(ct, it)
		if !ok || j >= len(leaves) {
			continue
		}
		if from := layout.PaneOf(root, id); from != leaves[j] {
			removeFromLeaf(from, id)
			insertAt(leaves[j], id, -1)
		}
	}
	t.Root = root
	t.Preset = ct.Layout
	for _, e := range m.missingEntries(ti, ct, nil) {
		m.addEntryItem(ti, leaves[min(e.pane, len(leaves)-1)], e.item, appName)
	}
	t.fixFocus()
	if layout.PaneOf(root, t.CompactShown) == nil {
		t.CompactShown = ""
	}
	return nil
}

// entryPane returns the index of the config pane whose entry claims an
// item: for an App item, the pane listing its app with the item's own title
// filter, else with a title filter its title contains, else the first
// listing it without one; for a Web or Built-in item, the pane with the
// same address, folder, or terminal. The item's own entry comes first, so
// that another entry whose filter its title happens to contain (a "GitHub"
// entry, for a "GitHub Docs" window of a "Docs" entry) doesn't take it.
func entryPane(ct config.Tab, it *Item) (int, bool) {
	if it == nil {
		return 0, false
	}
	if it.Kind == KindApp {
		claims := []func(ci config.Item) bool{
			func(ci config.Item) bool { return ci.Title != "" && ci.Title == it.TitleFilter },
			func(ci config.Item) bool { return ci.Title != "" && containsFold(it.Title, ci.Title) },
			func(ci config.Item) bool { return ci.Title == "" },
		}
		for _, claim := range claims {
			for j, cp := range ct.Panes {
				for _, ci := range cp.Items {
					if sameApp(ci.App, it.BundleID) && claim(ci) {
						return j, true
					}
				}
			}
		}
		return 0, false
	}
	for j, cp := range ct.Panes {
		for _, ci := range cp.Items {
			switch {
			case it.Kind == KindBuiltin && ci.Terminal != "" && ci.Terminal == it.Folder && ci.Command == it.Command && ci.Run == it.Run,
				it.VSCode != "" && ci.VSCode == it.VSCode,
				it.Kind == KindWeb && it.VSCode == "" && ci.Web != "" && (ci.Web == it.Home || ci.Web == it.URL):
				return j, true
			}
		}
	}
	return 0, false
}

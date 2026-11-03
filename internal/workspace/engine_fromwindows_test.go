package workspace

import (
	"slices"
	"testing"

	"github.com/itbm/bench/internal/layout"
)

// fromWindowsConfig has two tabs: Comms, with a web page and a terminal,
// and Code, with VS Code. Zed, Brave's three windows, Obsidian, and Finder
// are in no tab.
const fromWindowsConfig = `
[[tab]]
name = "Comms"
layout = "side"
  [[tab.pane]]
  items = [{ web = "https://app.slack.com/client", profile = "slack", name = "Slack" }]
  [[tab.pane]]
  items = [{ terminal = "~/code" }]

[[tab]]
name = "Code"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.microsoft.VSCode" }]
`

// fid returns the ID in View.Floating of a window that's in no tab.
func (h *harness) fid(bundleID, title string) string {
	h.t.Helper()
	return floatingID(h.windowID(bundleID, title))
}

// viewPanes returns the items of a tab's panes, in reading order.
func viewPanes(tv TabView) [][]string {
	var out [][]string
	for _, p := range layout.Leaves(tv.Root) {
		out = append(out, p.Items)
	}
	return out
}

func TestSuggestPreset(t *testing.T) {
	for apps, want := range map[int]string{
		0: layout.PresetSingle, 1: layout.PresetSingle, 2: layout.PresetSide, 3: layout.PresetThree,
		4: layout.PresetGrid, 7: layout.PresetGrid,
	} {
		if got := suggestPreset(apps); got != want {
			t.Errorf("%d apps: %s, want %s", apps, got, want)
		}
	}
}

func TestFillPanes(t *testing.T) {
	groups := []windowGroup{{ids: []string{"a1", "a2"}}, {ids: []string{"b1"}}, {ids: []string{"c1"}}, {ids: []string{"d1", "d2"}}}
	for _, c := range []struct {
		panes int
		want  [][]string
	}{
		{1, [][]string{{"a1", "a2", "b1", "c1", "d1", "d2"}}},
		{2, [][]string{{"a1", "a2"}, {"b1", "c1", "d1", "d2"}}},
		{4, [][]string{{"a1", "a2"}, {"b1"}, {"c1"}, {"d1", "d2"}}},
		{5, [][]string{{"a1", "a2"}, {"b1"}, {"c1"}, {"d1", "d2"}, nil}},
	} {
		got := fillPanes(groups, c.panes)
		if !slices.EqualFunc(got, c.want, slices.Equal) {
			t.Errorf("%d panes: %v, want %v", c.panes, got, c.want)
		}
	}
}

// TestPreviewTabFromWindows checks the New tab dialog's preview: the
// layout suggested by the number of apps, and each app's windows in a pane
// of their own, in the order they come.
func TestPreviewTabFromWindows(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	docs, pr := h.fid(braveID, "Wails v3 docs"), h.fid(braveID, "Pull request #42 · bench")
	zed, notes := h.fid(zedID, "bench — switch.go"), h.fid(obsidianID, "Daily note")
	ticked := []string{docs, zed, pr, notes, "f999", zed}

	got, err := h.e.PreviewTabFromWindows("", ticked)
	mustOK(t, err)
	if got.Suggested != layout.PresetThree || got.Apps != 3 {
		t.Errorf("suggested %s for %d apps, want three for 3", got.Suggested, got.Apps)
	}
	panes := layout.Leaves(got.Root)
	want := [][]string{{docs, pr}, {zed}, {notes}}
	if len(panes) != 3 || !slices.EqualFunc(panes, want, func(p *layout.Node, ids []string) bool { return slices.Equal(p.Items, ids) && p.Active == ids[0] }) {
		t.Errorf("three columns: %s, want %v", treeText(got.Root), want)
	}

	// Fewer panes than apps: the last pane has the rest.
	got, err = h.e.PreviewTabFromWindows(layout.PresetSide, ticked)
	mustOK(t, err)
	if panes := layout.Leaves(got.Root); got.Suggested != layout.PresetThree || len(panes) != 2 || !slices.Equal(panes[1].Items, []string{zed, notes}) {
		t.Errorf("side by side: suggested %s, %s", got.Suggested, treeText(got.Root))
	}
	// More panes than apps: the last ones are empty.
	got, err = h.e.PreviewTabFromWindows(layout.PresetGrid, ticked[:2])
	mustOK(t, err)
	if panes := layout.Leaves(got.Root); got.Suggested != layout.PresetSide || len(panes) != 4 || len(panes[2].Items) != 0 || len(panes[3].Items) != 0 {
		t.Errorf("grid: suggested %s, %s", got.Suggested, treeText(got.Root))
	}
	// Nothing ticked: one empty pane.
	got, err = h.e.PreviewTabFromWindows("", nil)
	mustOK(t, err)
	if got.Suggested != layout.PresetSingle || got.Apps != 0 || len(layout.Items(got.Root)) != 0 {
		t.Errorf("nothing ticked: %+v", got)
	}
	_, err = h.e.PreviewTabFromWindows(layout.PresetCustom, ticked)
	mustErr(t, err, errNoPreset(layout.PresetCustom).Error())
	// The preview changes nothing.
	if v := h.view(); len(v.Tabs) != 2 || !slices.Contains(floatingIDs(v), docs) {
		t.Errorf("the preview changed the tabs or the windows not in a tab")
	}
}

// TestNewTabFromWindows checks a tab made of windows that are open: each
// app's windows in a pane, the layout and name that follow from them, the
// windows docked there, a window of another tab shown there as well, and
// config.toml saved. Undo takes the tab away and puts every window back
// where it was.
func TestNewTabFromWindows(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	mustOK(t, h.e.SwitchTab(1))
	vscode := h.view().Tabs[1].Root.Items[0]
	finderWin := h.windowID(finderID, "Downloads")
	finderFrame := h.frame(finderWin)
	docs, pr := h.fid(braveID, "Wails v3 docs"), h.fid(braveID, "Pull request #42 · bench")
	zed, finder, packages := h.fid(zedID, "bench — switch.go"), floatingID(finderWin), h.fid(braveID, "Go packages")
	before := floatingIDs(h.view())
	h.p.ResetCalls()

	mustOK(t, h.e.NewTabFromWindows(TabDraft{}, []string{docs, zed, pr, finder, vscode}))
	h.drain()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code", "Brave"}) || v.Current != 2 {
		t.Fatalf("tabs %v, current %d; want a third tab, Brave, current", got, v.Current)
	}
	tab := v.Tabs[2]
	items := viewPanes(tab)
	if tab.Preset != layout.PresetGrid || len(items) != 4 || len(items[0]) != 2 || len(items[1]) != 1 || len(items[2]) != 1 || !slices.Equal(items[3], []string{vscode}) {
		t.Fatalf("new tab: preset %s, panes %v; want the grid, Brave's two windows, Zed, Finder, and VS Code", tab.Preset, items)
	}
	brave := h.itemOf(h.windowID(braveID, "Wails v3 docs"))
	if items[0][0] != brave || items[0][1] != h.itemOf(h.windowID(braveID, "Pull request #42 · bench")) {
		t.Errorf("first pane %v, want Brave's windows in the order ticked", items[0])
	}
	if p := layout.Leaves(tab.Root)[0]; tab.Focus != p.ID || p.Active != brave {
		t.Errorf("focus %s, first pane shows %s; want the first pane, showing Wails v3 docs", tab.Focus, p.Active)
	}
	if got := floatingIDs(v); !slices.Contains(got, packages) || slices.Contains(got, docs) || slices.Contains(got, zed) || slices.Contains(got, finder) {
		t.Errorf("not in a tab: %v; want only what wasn't ticked", got)
	}
	if it := h.item(vscode); !slices.Equal(it.Tabs, []int{1, 2}) {
		t.Errorf("VS Code is in tabs %v, want Code and the new tab", it.Tabs)
	}
	if h.frame(finderWin) == finderFrame {
		t.Errorf("Finder's window wasn't docked")
	}
	if got := h.em.lastToast(); got != "Made a new tab, Brave, with 5 windows." || !h.em.lastHasAction() {
		t.Errorf("toast %q, with Undo %v", got, h.em.lastHasAction())
	}
	cfg := savedConfig(t, h)
	if len(cfg.Tabs) != 3 || cfg.Tabs[2].Name != "Brave" || cfg.Tabs[2].Layout != layout.PresetGrid || len(cfg.Tabs[2].Panes) != 4 ||
		cfg.Tabs[2].Panes[0].Items[0].App != braveID || cfg.Tabs[2].Panes[3].Items[0].App != vscodeID {
		t.Errorf("config.toml's new tab: %+v", cfg.Tabs)
	}

	undoLast(t, h)
	v = h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code"}) || v.Current != 1 {
		t.Fatalf("after Undo: tabs %v, current %d", got, v.Current)
	}
	if got := floatingIDs(v); !sameSet(got, before) {
		t.Errorf("after Undo, not in a tab: %v, want %v", got, before)
	}
	if h.itemOf(finderWin) != "" {
		t.Errorf("Finder's window is still an item")
	}
	if got := h.frame(finderWin); got != finderFrame {
		t.Errorf("Finder's window at %+v, want back at %+v", got, finderFrame)
	}
	if it := h.item(vscode); it.Tab != 1 || len(it.Tabs) != 0 {
		t.Errorf("VS Code: tab %d, tabs %v; want Code only", it.Tab, it.Tabs)
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code"}) {
		t.Errorf("config.toml's tabs %v, want the new tab gone", got)
	}
	if got := h.em.lastToast(); got != "Deleted the new tab, Brave, and put its windows back." {
		t.Errorf("toast %q", got)
	}
}

// sameSet reports whether two lists have the same strings, in any order.
func sameSet(a, b []string) bool {
	return len(a) == len(b) && !slices.ContainsFunc(a, func(s string) bool { return !slices.Contains(b, s) })
}

// TestNewTabFromWindowsDraft checks the dialog's own name, layout, and
// place, names numbered as Tab properties numbers them, and refusals.
func TestNewTabFromWindowsDraft(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	zed, notes := h.fid(zedID, "bench — switch.go"), h.fid(obsidianID, "Daily note")
	mustOK(t, h.e.NewTabFromWindows(TabDraft{Name: " Code ", Preset: layout.PresetSingle, SmallScreens: "tabs", Place: 1}, []string{zed, notes}))
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Code (1)", "Comms", "Code"}) || v.Current != 0 {
		t.Fatalf("tabs %v, current %d; want Code (1) first, current", got, v.Current)
	}
	if tab := v.Tabs[0]; tab.Preset != layout.PresetSingle || tab.SmallScreens != "tabs" || len(layout.Items(tab.Root)) != 2 {
		t.Errorf("new tab: %+v", tab)
	}
	if got := configTabNames(t, h); !slices.Equal(got, tabNames(v)) {
		t.Errorf("config.toml's tabs %v", got)
	}
	undoLast(t, h)
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code"}) || !slices.Equal(tabNames(h.view()), got) {
		t.Errorf("after Undo: tabs %v, config.toml's %v", tabNames(h.view()), got)
	}

	// With no name, the first window's app names it.
	mustOK(t, h.e.NewTabFromWindows(TabDraft{Name: "Zed"}, []string{h.fid(braveID, "Go packages")}))
	mustOK(t, h.e.NewTabFromWindows(TabDraft{}, []string{zed}))
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Zed", "Zed (1)"}) {
		t.Errorf("tabs %v", got)
	}

	mustErr(t, h.e.NewTabFromWindows(TabDraft{}, nil), "Tick at least one window.")
	mustErr(t, h.e.NewTabFromWindows(TabDraft{}, []string{"f999", "w999"}), "Those windows have closed.")
	mustErr(t, h.e.NewTabFromWindows(TabDraft{Preset: layout.PresetCustom}, []string{notes}), errNoPreset(layout.PresetCustom).Error())
	if got := tabNames(h.view()); len(got) != 4 {
		t.Errorf("a refused tab was added: %v", got)
	}
}

// TestNewTabFromPoppedOut checks that popped-out windows join the new tab,
// and that Undo pops them out again: a terminal gets a window of its own,
// and an App window goes back where it was.
func TestNewTabFromPoppedOut(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w5"))
	mustOK(t, h.e.PopOut("w3"))
	h.drain()
	zedWin := h.windowID(zedID, "bench — switch.go")
	moved := layout.Rect{X: 300, Y: 260, W: 700, H: 500}
	mustOK(t, h.p.UserMoveWindow(zedWin, moved))
	h.drain()
	if !h.e.model.Items["w5"].OwnWindow || len(h.em.popped) != 1 {
		t.Fatalf("the terminal has no window of its own")
	}

	mustOK(t, h.e.NewTabFromWindows(TabDraft{Name: "Popped"}, []string{"w5", "w3"}))
	h.drain()
	v := h.view()
	if tab := v.Tabs[len(v.Tabs)-1]; tab.Name != "Popped" || tab.Preset != layout.PresetSide || !slices.EqualFunc(viewPanes(tab), [][]string{{"w5"}, {"w3"}}, slices.Equal) {
		t.Fatalf("new tab: %s %s %v", tab.Name, tab.Preset, viewPanes(tab))
	}
	if h.e.model.Items["w5"].OwnWindow || !slices.Contains(h.em.closed, "w5") {
		t.Errorf("the terminal kept its window of its own")
	}
	if h.frame(zedWin) == moved {
		t.Errorf("Zed's window wasn't docked")
	}

	undoLast(t, h)
	v = h.view()
	if got := floatingIDs(v); !slices.Contains(got, "w5") || !slices.Contains(got, "w3") {
		t.Errorf("not in a tab: %v, want the terminal and Zed again", got)
	}
	if !h.e.model.Items["w5"].OwnWindow || len(h.em.popped) != 2 {
		t.Errorf("the terminal didn't get its window back: own window %v, windows made %v", h.e.model.Items["w5"].OwnWindow, h.em.popped)
	}
	if got := h.frame(zedWin); got != moved {
		t.Errorf("Zed's window at %+v, want back at %+v", got, moved)
	}
}

// TestNewTabFromPoppedOutPage checks that a popped-out web page joins the
// new tab, docked, and that Undo pops it out of Bench again.
func TestNewTabFromPoppedOutPage(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w1"))
	h.drain()
	if wv := h.p.WebViews()["w1"]; !wv.PoppedOut || !h.e.model.Items["w1"].OwnWindow {
		t.Fatalf("Slack isn't popped out: %+v", wv)
	}

	mustOK(t, h.e.NewTabFromWindows(TabDraft{Name: "Page"}, []string{"w1"}))
	h.drain()
	v := h.view()
	if tab := v.Tabs[len(v.Tabs)-1]; tab.Name != "Page" || !slices.EqualFunc(viewPanes(tab), [][]string{{"w1"}}, slices.Equal) {
		t.Fatalf("new tab: %s %v", tab.Name, viewPanes(tab))
	}
	if wv := h.p.WebViews()["w1"]; wv.PoppedOut || h.e.model.Items["w1"].OwnWindow {
		t.Errorf("Slack wasn't docked: %+v", wv)
	}

	undoLast(t, h)
	if got := floatingIDs(h.view()); !slices.Contains(got, "w1") {
		t.Errorf("not in a tab: %v, want Slack again", got)
	}
	if wv := h.p.WebViews()["w1"]; !wv.PoppedOut || wv.Title != "Slack" || !h.e.model.Items["w1"].OwnWindow {
		t.Errorf("Slack didn't get its window back: %+v, own window %v", wv, h.e.model.Items["w1"].OwnWindow)
	}
}

// TestNewTabFromWindowsUndoLater checks the new tab's Undo after a later
// change, which it takes back too: a window that Bench didn't manage,
// moved on to another tab since, still leaves Bench, with its own ID, and
// goes back to its frame.
func TestNewTabFromWindowsUndoLater(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	finderWin := h.windowID(finderID, "Downloads")
	finderFrame := h.frame(finderWin)
	mustOK(t, h.e.NewTabFromWindows(TabDraft{}, []string{floatingID(finderWin)}))
	h.drain()
	first := h.em.lastUndo(t)
	finder := h.itemOf(finderWin)
	mustOK(t, h.e.MoveItemToTab(finder, 0))
	h.drain()
	if it := h.item(finder); it.Tab != 0 {
		t.Fatalf("Finder's window is in tab %d, want Comms", it.Tab)
	}

	mustOK(t, h.e.Undo(first))
	h.drain()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code"}) {
		t.Fatalf("after Undo: tabs %v", got)
	}
	if !slices.Contains(floatingIDs(v), floatingID(finderWin)) || h.itemOf(finderWin) != "" {
		t.Errorf("Finder's window isn't under Not in a tab with its own ID: %v", floatingIDs(v))
	}
	if slices.Contains(layout.Items(v.Tabs[0].Root), finder) {
		t.Errorf("Finder's window is still in Comms")
	}
	if got := h.frame(finderWin); got != finderFrame {
		t.Errorf("Finder's window at %+v, want back at %+v", got, finderFrame)
	}
}

// TestNewTabFromWindowsUndoTwice checks Undo of two tabs made of the same
// popped-out window: the first toast's Undo takes back both, and the
// window goes back to where it was before the first.
func TestNewTabFromWindowsUndoTwice(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w3"))
	h.drain()
	zedWin := h.windowID(zedID, "bench — switch.go")
	before := layout.Rect{X: 300, Y: 260, W: 700, H: 500}
	mustOK(t, h.p.UserMoveWindow(zedWin, before))
	h.drain()

	mustOK(t, h.e.NewTabFromWindows(TabDraft{Name: "First"}, []string{"w3"}))
	h.drain()
	first := h.em.lastUndo(t)
	mustOK(t, h.e.PopOut("w3"))
	h.drain()
	later := layout.Rect{X: 500, Y: 300, W: 600, H: 400}
	mustOK(t, h.p.UserMoveWindow(zedWin, later))
	h.drain()
	mustOK(t, h.e.NewTabFromWindows(TabDraft{Name: "Second"}, []string{"w3"}))
	h.drain()
	if got := h.frame(zedWin); got == later {
		t.Fatalf("Zed's window wasn't docked in the second tab")
	}

	mustOK(t, h.e.Undo(first))
	h.drain()
	v := h.view()
	if got := tabNames(v); slices.Contains(got, "First") || slices.Contains(got, "Second") {
		t.Errorf("after Undo: tabs %v, want both new tabs gone", got)
	}
	if !slices.Contains(floatingIDs(v), "w3") {
		t.Errorf("not in a tab: %v, want Zed", floatingIDs(v))
	}
	if got := h.frame(zedWin); got != before {
		t.Errorf("Zed's window at %+v, want back at %+v, where it was before the first tab", got, before)
	}
}

// TestAddWindowsToPane checks the add dialog's ticked windows: those not
// in a tab move to the pane, those of other tabs show there as well, the
// first is shown, and windows that couldn't be added are named.
func TestAddWindowsToPane(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	pane := layout.Leaves(h.view().Tabs[0].Root)[0].ID
	vscode := h.view().Tabs[1].Root.Items[0]
	docs, pr := h.fid(braveID, "Wails v3 docs"), h.fid(braveID, "Pull request #42 · bench")
	mustOK(t, h.e.AddWindowsToPane(pane, []string{docs, vscode, pr, docs}))
	h.drain()
	brave := h.itemOf(h.windowID(braveID, "Wails v3 docs"))
	p := layout.Leaves(h.view().Tabs[0].Root)[0]
	if len(p.Items) != 4 || p.Items[1] != brave || p.Items[2] != vscode || p.Active != brave {
		t.Errorf("pane %v showing %s; want Slack, then the three windows, showing Wails v3 docs", p.Items, p.Active)
	}
	if it := h.item(vscode); !slices.Equal(it.Tabs, []int{0, 1}) {
		t.Errorf("VS Code is in tabs %v, want both", it.Tabs)
	}
	if slices.Contains(floatingIDs(h.view()), docs) {
		t.Errorf("Wails v3 docs is still not in a tab")
	}

	h.em.toasts = nil
	zed := h.fid(zedID, "bench — switch.go")
	mustOK(t, h.e.AddWindowsToPane(pane, []string{"f999", zed, vscode}))
	if got := h.em.lastToast(); got != "Added 1 of the 3 windows. The others had closed, or were in this tab already." {
		t.Errorf("toast %q", got)
	}
	mustOK(t, h.e.AddWindowsToPane(pane, []string{"f999", h.fid(obsidianID, "Daily note")}))
	if got := h.em.lastToast(); got != "Added 1 of the 2 windows. The other had closed, or was in this tab already." {
		t.Errorf("toast %q", got)
	}
	mustErr(t, h.e.AddWindowsToPane(pane, []string{"f999"}), errGone.Error())
	mustErr(t, h.e.AddWindowsToPane(pane, nil), "Tick at least one window.")
	mustErr(t, h.e.AddWindowsToPane("p999", []string{zed}), errNoPane.Error())
}

// TestNewTabFromWindowsLeavesOutHidden checks that a sub-tab whose app has
// quit isn't a window to make a tab of.
func TestNewTabFromWindowsLeavesOutHidden(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	vscode := h.view().Tabs[1].Root.Items[0]
	mustOK(t, h.p.QuitApp(vscodeID))
	h.drain()
	if !h.item(vscode).Hidden {
		t.Fatalf("VS Code's sub-tab isn't hidden once it has quit")
	}
	mustErr(t, h.e.NewTabFromWindows(TabDraft{}, []string{vscode}), "Those windows have closed.")
	got, err := h.e.PreviewTabFromWindows("", []string{vscode, h.fid(zedID, "bench — switch.go")})
	mustOK(t, err)
	if got.Apps != 1 {
		t.Errorf("apps %d, want only Zed", got.Apps)
	}
}

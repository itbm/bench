package workspace

import (
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// itemsOfPane returns the items of a pane of a tab, in order, as the model
// has them, hidden ones too.
func itemsOfPane(t *testing.T, h *harness, tab, pane string) []string {
	t.Helper()
	n, _, _ := layout.Find(h.tabNamed(tab).Root, pane)
	if !n.IsPane() {
		t.Fatalf("%s has no pane %s: %s", tab, pane, treeText(h.tabNamed(tab).Root))
	}
	return n.Items
}

// TestMoveItemAtReordersPane checks that a sub-tab moved along its own
// pane's strip changes places there, shown, without splitting the pane or
// making the layout custom, says so with Undo, and is kept across a restart.
func TestMoveItemAtReordersPane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItemAt("w4", 1, "p11", 0))
	if got := itemsOfPane(t, h, "Code", "p11"); !slices.Equal(got, []string{"w4", "w3"}) {
		t.Fatalf("p11 = %v, want w4 before w3", got)
	}
	v := h.view()
	if len(v.Display.Panes) != 2 || v.Display.Focus != "p11" || v.Display.Panes[0].Active != "w4" {
		t.Errorf("panes %+v, focus %s: want the two panes, with VS Code shown in p11", v.Display.Panes, v.Display.Focus)
	}
	if v.Tabs[1].Preset != "main-side" {
		t.Errorf("preset %s: a new order in a pane keeps the layout", v.Tabs[1].Preset)
	}
	vscode := h.windowID(vscodeID, "index.html — bench-ui")
	if h.frame(vscode) != h.bodyOf("p11") {
		t.Errorf("VS Code: %v, want p11's body", h.frame(vscode))
	}
	if got := h.em.lastToast(); got != "Moved VS Code — index.html — bench-ui." || !h.em.lastHasAction() {
		t.Errorf("toast %q, want one with Undo", got)
	}

	// Next to itself, at either side, it stays where it is.
	toasts := len(h.em.toasts)
	mustOK(t, h.e.MoveItemAt("w4", 1, "p11", 0))
	mustOK(t, h.e.MoveItemAt("w4", 1, "p11", 1))
	if got := itemsOfPane(t, h, "Code", "p11"); !slices.Equal(got, []string{"w4", "w3"}) || len(h.em.toasts) != toasts {
		t.Errorf("p11 = %v, %d toasts: a move to where it is should do nothing", got, len(h.em.toasts)-toasts)
	}

	// Past the last sub-tab is the end.
	mustOK(t, h.e.MoveItemAt("w4", 1, "p11", 2))
	if got := itemsOfPane(t, h, "Code", "p11"); !slices.Equal(got, []string{"w3", "w4"}) {
		t.Errorf("p11 = %v, want w4 at the end again", got)
	}
	undoLast(t, h)
	if got := itemsOfPane(t, h, "Code", "p11"); !slices.Equal(got, []string{"w4", "w3"}) {
		t.Errorf("after Undo, p11 = %v, want w4 first", got)
	}

	h.restart()
	if got := itemsOfPane(t, h, "Code", "p11"); !slices.Equal(got, []string{"w4", "w3"}) {
		t.Errorf("after a restart, p11 = %v, want the order kept", got)
	}
}

// TestMoveItemAtJoinsPane checks that a sub-tab dropped on another pane's
// strip joins that pane where it was dropped, as a drop in its middle would
// but at that place, closing up the pane it leaves if that's empty now.
func TestMoveItemAtJoinsPane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItemAt("w5", 1, "p11", 1))
	if got := itemsOfPane(t, h, "Code", "p11"); !slices.Equal(got, []string{"w3", "w5", "w4"}) {
		t.Errorf("p11 = %v, want the terminal between Zed and VS Code", got)
	}
	v := h.view()
	if got := leafIDs(v.Tabs[1]); !slices.Equal(got, []string{"p11"}) || v.Tabs[1].Preset != "custom" {
		t.Errorf("panes %v, preset %s: p12 should close up", got, v.Tabs[1].Preset)
	}
	if v.Display.Panes[0].Active != "w5" || v.Display.Focus != "p11" {
		t.Errorf("p11 shows %s, focus %s: want the terminal shown", v.Display.Panes[0].Active, v.Display.Focus)
	}
	undoLast(t, h)
	if got := leafIDs(h.view().Tabs[1]); !slices.Equal(got, []string{"p11", "p12"}) {
		t.Errorf("after Undo, panes %v", got)
	}

	// An item from another tab joins this one where it was dropped.
	mustOK(t, h.e.MoveItemAt("w7", 1, "p12", 0))
	if got := itemsOfPane(t, h, "Code", "p12"); !slices.Equal(got, []string{"w7", "w5"}) {
		t.Errorf("p12 = %v, want Obsidian first", got)
	}
	if h.item("w7").Tab != 1 {
		t.Errorf("Obsidian is in tab %d, want Code", h.item("w7").Tab)
	}

	mustErr(t, h.e.MoveItemAt("w99", 1, "p11", 0), "That window has closed.")
	mustErr(t, h.e.MoveItemAt("w3", 5, "p11", 0), "There's no tab 6.")
	mustErr(t, h.e.MoveItemAt("w3", 1, "p99", 0), "There's no such pane.")
}

// TestMoveItemAtCountsShownSubtabs checks that the index counts the
// sub-tabs as drawn, without those of windows that have closed, which keep
// their places out of sight.
func TestMoveItemAtCountsShownSubtabs(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w5", 1, "p11", "center"))
	h.p.QuitApp(zedID)
	h.settle()
	if got := h.view().Display.Panes[0].Items; !slices.Equal(got, []string{"w4", "w5"}) {
		t.Fatalf("p11 shows %v, want Zed's place hidden", got)
	}
	// Before VS Code, the first sub-tab drawn.
	mustOK(t, h.e.MoveItemAt("w5", 1, "p11", 0))
	if got := itemsOfPane(t, h, "Code", "p11"); !slices.Equal(got, []string{"w3", "w5", "w4"}) {
		t.Errorf("p11 = %v, want the terminal just before VS Code", got)
	}
	if got := h.view().Display.Panes[0].Items; !slices.Equal(got, []string{"w5", "w4"}) {
		t.Errorf("p11 shows %v", got)
	}
}

// TestMoveItemAtInTabsView checks the "tabs" view's one strip, which shows
// the whole tab in reading order: an item stays in its pane while a sub-tab
// beside its new place is in it, and otherwise joins the pane of its new
// neighbour.
func TestMoveItemAtInTabsView(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SetMode("compact"))
	mustOK(t, h.e.SwitchTab(2))
	strip := func() []string {
		t.Helper()
		d := h.view().Display
		if !d.Compact || len(d.Panes) != 1 {
			t.Fatalf("display %+v, want the tabs view", d)
		}
		return d.Panes[0].Items
	}
	if got := strip(); !slices.Equal(got, []string{"w6", "w17", "w18", "w7"}) {
		t.Fatalf("strip %v", got)
	}
	// Between Go packages and Obsidian: the last of its own pane.
	mustOK(t, h.e.MoveItemAt("w6", 2, layout.CompactPaneID, 3))
	if got := itemsOfPane(t, h, "Research", "p14"); !slices.Equal(got, []string{"w17", "w18", "w6"}) {
		t.Errorf("p14 = %v", got)
	}
	if got := h.tabNamed("Research"); got.Preset != "side" || len(leafIDs(got)) != 2 {
		t.Errorf("preset %s, panes %v: the wide layout should stay", got.Preset, leafIDs(got))
	}
	// After Obsidian: its pane, after it.
	mustOK(t, h.e.MoveItemAt("w6", 2, layout.CompactPaneID, 4))
	if got := itemsOfPane(t, h, "Research", "p15"); !slices.Equal(got, []string{"w7", "w6"}) {
		t.Errorf("p15 = %v, want Wails v3 docs after Obsidian", got)
	}
	if got := strip(); !slices.Equal(got, []string{"w17", "w18", "w7", "w6"}) {
		t.Errorf("strip %v", got)
	}
	// First: before the first sub-tab, in its pane.
	mustOK(t, h.e.MoveItemAt("w7", 2, layout.CompactPaneID, 0))
	if got := strip(); !slices.Equal(got, []string{"w7", "w17", "w18", "w6"}) {
		t.Errorf("strip %v", got)
	}
	if h.item("w7").Pane != "p14" {
		t.Errorf("Obsidian is in %s, want p14, with Pull request", h.item("w7").Pane)
	}
	if got := h.view().Display.Panes[0].Active; got != "w7" {
		t.Errorf("the strip shows %s, want Obsidian", got)
	}
	// Next to itself, nothing changes.
	toasts := len(h.em.toasts)
	mustOK(t, h.e.MoveItemAt("w7", 2, layout.CompactPaneID, 1))
	if len(h.em.toasts) != toasts {
		t.Errorf("a move next to itself said %q", h.em.lastToast())
	}
}

// TestMoveItemAtInTabsViewKeepsPanes checks that, in the "tabs" view, a
// sub-tab that's the only one in its pane can go among another pane's
// without that removing its pane: the wide layout keeps its panes and
// their sizes, and the toast says the pane is empty there.
func TestMoveItemAtInTabsViewKeepsPanes(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SetMode("compact"))
	mustOK(t, h.e.SwitchTab(2))
	before := h.tabNamed("Research")
	if got := itemsOfPane(t, h, "Research", "p15"); !slices.Equal(got, []string{"w7"}) {
		t.Fatalf("p15 = %v, want Obsidian alone", got)
	}
	mustOK(t, h.e.MoveItemAt("w7", 2, layout.CompactPaneID, 0))
	after := h.tabNamed("Research")
	if got := h.view().Display.Panes[0].Items; !slices.Equal(got, []string{"w7", "w6", "w17", "w18"}) {
		t.Errorf("strip %v, want Obsidian first", got)
	}
	if got := leafIDs(after); !slices.Equal(got, leafIDs(before)) {
		t.Errorf("panes %v, want %v: the wide layout should keep its panes", got, leafIDs(before))
	}
	if !slices.Equal(after.Root.Sizes, before.Root.Sizes) {
		t.Errorf("sizes %v, want %v", after.Root.Sizes, before.Root.Sizes)
	}
	if got := itemsOfPane(t, h, "Research", "p15"); len(got) != 0 {
		t.Errorf("p15 = %v, want it empty", got)
	}
	if got := h.em.lastToast(); got != "Moved Obsidian — Daily note. Its old pane is empty in the wide layout." || !h.em.lastHasAction() {
		t.Errorf("toast %q, want one that says so, with Undo", got)
	}
	// Undo puts it back in its own pane.
	mustOK(t, h.e.Undo(h.em.actions[len(h.em.actions)-1].Arg))
	h.drain()
	if got := itemsOfPane(t, h, "Research", "p15"); !slices.Equal(got, []string{"w7"}) {
		t.Errorf("after Undo, p15 = %v", got)
	}
	if got := h.tabNamed("Research").Preset; got != before.Preset {
		t.Errorf("after Undo, preset %s, want %s", got, before.Preset)
	}

	// The empty pane is kept across a restart.
	mustOK(t, h.e.MoveItemAt("w7", 2, layout.CompactPaneID, 0))
	h.settle()
	h.restart()
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, leafIDs(before)) {
		t.Errorf("after a restart, panes %v, want %v", got, leafIDs(before))
	}

	// In the wide layout, the pane's only window still takes its pane
	// with it.
	mustOK(t, h.e.SetMode("wide"))
	mustOK(t, h.e.MoveItemAt("w7", 2, "p15", 0))
	mustOK(t, h.e.MoveItemAt("w7", 2, "p14", 0))
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, []string{"p14"}) {
		t.Errorf("panes %v, want p15 closed up", got)
	}
}

// TestMoveTab checks that moving a tab reorders the tabs and config.toml's
// together, keeping the current tab, each window's tabs, and each tab's
// arrangement, with the shortcuts following the order, and that the order
// is kept across a restart.
func TestMoveTab(t *testing.T) {
	h := newHarness(t, setup{config: exampleConfig(t) + researchRule})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddItemToTab("w3", 2))
	mustOK(t, h.e.MoveItem("w17", 2, "p14", "right"))
	research := layout.Clone(h.tabNamed("Research").Root)
	before := readFile(t, h, config.ConfigFile)

	mustOK(t, h.e.MoveTab(2, 0))
	h.settle()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Research", "Comms", "Code"}) {
		t.Fatalf("tabs %v", got)
	}
	if v.Current != 2 {
		t.Errorf("current %d, want Code, now 3rd", v.Current)
	}
	if v.Tabs[0].Shortcut != "⌃⌥1" || v.Tabs[2].Shortcut != "⌃⌥3" {
		t.Errorf("shortcuts %q and %q, want them to follow the order", v.Tabs[0].Shortcut, v.Tabs[2].Shortcut)
	}
	if it := h.item("w3"); it.Tab != 2 || !slices.Equal(it.Tabs, []int{0, 2}) {
		t.Errorf("Zed: tab %d, tabs %v, want Code and Research", it.Tab, it.Tabs)
	}
	if it := h.item("w7"); it.Tab != 0 {
		t.Errorf("Obsidian is in tab %d, want Research", it.Tab)
	}
	sameTree(t, h, "Research", research)
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Research", "Comms", "Code"}) {
		t.Errorf("config.toml's tabs %v", got)
	}
	if cfg := savedConfig(t, h); len(cfg.Rules) != 2 || cfg.Rules[1].Tab != "Research" {
		t.Errorf("rules %+v, want them as they were", cfg.Rules)
	}
	if got := readFile(t, h, config.BackupFile); got != before {
		t.Errorf("config.toml.bak:\n%s\nwant the file from before", got)
	}
	if got := h.em.lastToast(); got != "Moved Research to 1st place (⌃⌥1)." || !h.em.lastHasAction() {
		t.Errorf("toast %q, want one with Undo", got)
	}
	bars := h.em.menuBars()
	if last := bars[len(bars)-1]; last.TabName() != "Code" || last.Tabs[0].Name != "Research" || last.Tabs[0].Shortcut != "Ctrl+Option+1" {
		t.Errorf("menu bar %+v", last)
	}

	h.restart()
	v = h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Research", "Comms", "Code"}) || v.Current != 2 {
		t.Errorf("after a restart: tabs %v, current %d", got, v.Current)
	}
	sameTree(t, h, "Research", research)
	if it := h.item("w3"); !slices.Equal(it.Tabs, []int{0, 2}) {
		t.Errorf("after a restart, Zed is in tabs %v", it.Tabs)
	}
	if len(v.Notices) != 0 {
		t.Errorf("notices %v, want none: no arrangement set aside", texts(v.Notices))
	}
}

// TestMoveTabUndo checks that Undo puts the tabs back in order, in the
// model and in config.toml, and that Move left and Move right stop at
// the ends.
func TestMoveTabUndo(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveCurrentTab(-1))
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Code", "Comms", "Research"}) || h.view().Current != 0 {
		t.Fatalf("tabs %v, current %d", got, h.view().Current)
	}
	mustOK(t, h.e.MoveItem("w3", 0, "p11", "right"))
	mustErr(t, h.e.MoveCurrentTab(-1), "Code is the first tab already.")
	mustErr(t, h.e.MoveTab(2, 3), "Research is the last tab already.")
	mustErr(t, h.e.MoveTab(5, 0), "There's no tab 6.")

	// Undo takes back the move of a window after it, then the tab's.
	i := slices.Index(h.em.toasts, "Moved Code to 1st place (⌃⌥1).")
	if i < 0 {
		t.Fatalf("no toast for the tab's move: %q", h.em.toasts)
	}
	step := h.em.actions[i].Arg
	mustOK(t, h.e.Undo(step))
	h.drain()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code", "Research"}) || v.Current != 1 {
		t.Errorf("after Undo: tabs %v, current %d", got, v.Current)
	}
	if got := leafIDs(h.tabNamed("Code")); !slices.Equal(got, []string{"p11", "p12"}) {
		t.Errorf("Code's panes %v, want the window's move taken back too", got)
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("config.toml's tabs %v", got)
	}
	if got := h.em.lastToast(); got != "Moved Code back." {
		t.Errorf("toast %q", got)
	}
}

// TestMoveTabNotSaved checks that a move is applied but not saved over a
// config.toml changed outside Bench, and that state.json then waits for it.
func TestMoveTabNotSaved(t *testing.T) {
	h := newHarness(t, setup{})
	edited := exampleConfig(t) + mailRule
	writeConfig(t, h, edited)
	mustOK(t, h.e.MoveTab(0, 2))
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Code", "Research", "Comms"}) {
		t.Errorf("tabs %v, want the move applied", got)
	}
	if got := h.em.lastToast(); !strings.HasSuffix(got, " Not saved: config.toml was changed outside Bench.") {
		t.Errorf("toast %q", got)
	}
	if got := readFile(t, h, config.ConfigFile); got != edited {
		t.Errorf("config.toml was written over:\n%s", got)
	}
	if n, _ := noticeWith(h.view().Notices, "config.toml was changed outside Bench"); !strings.HasSuffix(n.Text, outsideKeptHeld) {
		t.Errorf("notice %q, want it to say the arrangement isn't saved", n.Text)
	}
}

// TestSaveTabPlace checks Tab properties' place, which moves the tab as it
// saves, or puts a new one there.
func TestSaveTabPlace(t *testing.T) {
	h := newHarness(t, setup{})
	draft := draftOf(h.view(), "Research", "side", "tabs")
	draft.Place = 1
	mustOK(t, h.e.SaveTab(2, draft))
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Research", "Comms", "Code"}) {
		t.Errorf("tabs %v", got)
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Research", "Comms", "Code"}) {
		t.Errorf("config.toml's tabs %v", got)
	}
	draft = draftOf(h.view(), "Notes", "single", "stack")
	draft.Place = 2
	mustOK(t, h.e.SaveTab(-1, draft))
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Research", "Notes", "Comms", "Code"}) || v.Current != 1 {
		t.Errorf("tabs %v, current %d, want Notes 2nd and current", got, v.Current)
	}
	// A place past the last is the last.
	draft = draftOf(h.view(), "Notes", "single", "stack")
	draft.Place = 9
	mustOK(t, h.e.SaveTab(1, draft))
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Research", "Comms", "Code", "Notes"}) {
		t.Errorf("tabs %v", got)
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 10: "10th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 101: "101st", 111: "111th"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}

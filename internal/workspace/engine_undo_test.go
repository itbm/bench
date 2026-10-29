package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// researchRule names Research in a rule, which Delete tab takes from
// config.toml and Undo puts back.
const researchRule = "\n[[rule]]\napp = \"com.apple.Safari\"\ntab = \"Research\"\n"

// configTabNames returns the names of config.toml's tabs, in order.
func configTabNames(t *testing.T, h *harness) []string {
	t.Helper()
	var out []string
	for _, tab := range savedConfig(t, h).Tabs {
		out = append(out, tab.Name)
	}
	return out
}

// undoLast takes back the change whose toast came last, by its Undo.
func undoLast(t *testing.T, h *harness) {
	t.Helper()
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	h.drain()
}

// sameTree fails the test if a tab's tree isn't want.
func sameTree(t *testing.T, h *harness, name string, want *layout.Node) {
	t.Helper()
	if got := h.tabNamed(name).Root; !reflect.DeepEqual(got, want) {
		t.Errorf("%s's tree:\n got %s\nwant %s", name, treeText(got), treeText(want))
	}
}

// sameShape fails the test if a tab's tree isn't want, but for the IDs of
// its panes and splits, which Reopen closed tab gives anew.
func sameShape(t *testing.T, h *harness, name string, want *layout.Node) {
	t.Helper()
	strip := func(n *layout.Node) *layout.Node {
		c := layout.Clone(n)
		var walk func(*layout.Node)
		walk = func(n *layout.Node) {
			n.ID = ""
			for _, k := range n.Children {
				walk(k)
			}
		}
		walk(c)
		return c
	}
	if got := h.tabNamed(name).Root; !reflect.DeepEqual(strip(got), strip(want)) {
		t.Errorf("%s's tree:\n got %s\nwant %s", name, treeText(got), treeText(want))
	}
}

// treeText describes a tree on one line, for messages.
func treeText(n *layout.Node) string {
	if n == nil {
		return "nil"
	}
	if n.IsPane() {
		return fmt.Sprintf("%s%v(%s)", n.ID, n.Items, n.Active)
	}
	s := fmt.Sprintf("%s %s %v [", n.ID, n.Dir, n.Sizes)
	for i, c := range n.Children {
		if i > 0 {
			s += " "
		}
		s += treeText(c)
	}
	return s + "]"
}

// TestUndoDeleteTab checks that Undo after Delete tab brings the tab back
// where it was, as it was, with the places of windows that had closed, its
// entry in config.toml and its rule, and docks its windows again.
func TestUndoDeleteTab(t *testing.T) {
	h := newHarness(t, setup{config: string(config.ExampleTOML()) + researchRule, world: func(p *platform.Fake) {
		standardWorld(p)
		p.QuitApp(obsidianID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	research := layout.Clone(h.tabNamed("Research").Root)
	mustOK(t, h.e.DeleteTab(2))
	step := h.em.lastUndo(t)
	v := h.view()
	if !slices.Equal(v.ClosedTabs, []string{"Research"}) {
		t.Errorf("ClosedTabs = %v, want Research", v.ClosedTabs)
	}
	if _, ok := v.Items["w7"]; ok {
		t.Fatal("Obsidian's place should go with the tab")
	}

	mustOK(t, h.e.Undo(step))
	h.drain()
	v = h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code", "Research"}) || v.Current != 1 {
		t.Fatalf("tabs %v, current %d", got, v.Current)
	}
	sameTree(t, h, "Research", research)
	if got := floatingIDs(v); !slices.Equal(got, []string{"f8"}) {
		t.Errorf("Not in a tab: %v, want only Finder's f8", got)
	}
	if it := h.item("w7"); it.Tab != 2 || it.Pane != "p15" {
		t.Errorf("Obsidian's place: tab %d pane %s, want Research's p15", it.Tab, it.Pane)
	}
	if got := h.em.lastToast(); got != "Brought back Research." || h.em.lastHasAction() {
		t.Errorf("toast %q", got)
	}
	if len(v.ClosedTabs) != 0 {
		t.Errorf("ClosedTabs = %v, want none: the tab is back", v.ClosedTabs)
	}
	cfg := savedConfig(t, h)
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "Research"}) || len(cfg.Rules) != 2 ||
		cfg.Rules[1].Tab != "Research" {
		t.Errorf("config.toml: tabs %v, rules %+v", got, cfg.Rules)
	}
	// Research isn't the current tab, so its windows are put away again,
	// and switching to it shows them in their panes.
	docs := h.windowID(braveID, "Wails v3 docs")
	if !h.p.IsHidden(braveID) && !h.e.win.IsParked(docs) {
		t.Error("Research's windows should be put away with the tab")
	}
	mustOK(t, h.e.SwitchTab(2))
	if h.frame(docs) != h.bodyOf("p14") {
		t.Errorf("Wails v3 docs: %v, want p14's body %v", h.frame(docs), h.bodyOf("p14"))
	}
	// Each change is undone once.
	mustErr(t, h.e.Undo(step), "Bench can't undo that any more.")
}

// TestUndoCloseTab checks that Undo after Close tab brings the tab back,
// with its entry in config.toml and its rule, and what's still open, but
// not the windows, pages, and terminals that closed.
func TestUndoCloseTab(t *testing.T) {
	h, ids := closeHarness(t, "")
	mustOK(t, h.e.SwitchTab(1))
	_, err := h.e.CloseTab(0, true, false)
	mustOK(t, err)
	h.drain()
	if got := h.em.lastToast(); got != "Closed Code. "+closeUndoText {
		t.Errorf("toast %q", got)
	}
	undoLast(t, h)
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Code", "Notes"}) || v.Current != 1 {
		t.Fatalf("tabs %v, current %d; want Code back first, Notes current", got, v.Current)
	}
	if got := h.em.lastToast(); got != "Brought back Code, with what's still open." {
		t.Errorf("toast %q", got)
	}
	// Zed's window closed, and the terminal and the page ended: they stay
	// gone, and the pane they leave empty closes up. TextEdit's window,
	// which asks about unsaved changes, is back in its pane.
	for _, name := range []string{"Zed", "claude", "Docs"} {
		if _, ok := v.Items[ids[name]]; ok {
			t.Errorf("%s came back", name)
		}
	}
	if _, ok := h.terms.started[ids["claude"]]; ok {
		t.Error("the terminal started again")
	}
	code := h.tabNamed("Code")
	if got := layout.Items(code.Root); !slices.Equal(got, []string{ids["TextEdit"]}) || len(layout.Leaves(code.Root)) != 1 {
		t.Errorf("Code: %s, want TextEdit alone", treeText(code.Root))
	}
	if len(floatingIDs(v)) != 0 {
		t.Errorf("Not in a tab: %v, want nothing", floatingIDs(v))
	}
	cfg := savedConfig(t, h)
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Code", "Notes"}) || len(cfg.Rules) != 1 ||
		len(cfg.Tabs[0].Panes) != 2 {
		t.Errorf("config.toml: tabs %v, rules %+v", got, cfg.Rules)
	}
	// After the grace, TextEdit's window, which is still open, stays in its
	// tab. Its app may still be asking about the close that Undo can't take
	// back, which the toast says.
	h.advance(closeGrace)
	if it := h.item(ids["TextEdit"]); it.Tab != 0 {
		t.Errorf("TextEdit is in tab %d, want Code", it.Tab)
	}
	if got := h.em.lastToast(); got != "TextEdit may be asking about unsaved changes." {
		t.Errorf("toast %q", got)
	}
}

// TestUndoCloseOnlyTab checks that Undo after closing the only tab puts its
// layout back, with what's still open.
func TestUndoCloseOnlyTab(t *testing.T) {
	h, ids := closeHarness(t, "")
	mustOK(t, h.e.DeleteTab(1))
	before := layout.Clone(h.tabNamed("Code").Root)
	_, err := h.e.CloseTab(0, true, false)
	mustOK(t, err)
	h.drain()
	undoLast(t, h)
	if got := h.em.lastToast(); got != "Put back the layout of Code, with what's still open." {
		t.Errorf("toast %q", got)
	}
	// The second pane held the terminal and the page, which have ended.
	want := layout.Clone(before)
	want, _, _ = layout.TakeItem(want, ids["Zed"])
	want, _, _ = layout.TakeItem(want, ids["claude"])
	want, _, _ = layout.TakeItem(want, ids["Docs"])
	sameTree(t, h, "Code", want)
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Code"}) {
		t.Errorf("config.toml's tabs: %v", got)
	}
}

// TestUndoResetLayout checks that Undo after Reset layout puts the tab's
// tree back, and takes away the terminal that Reset layout opened for its
// config entry.
func TestUndoResetLayout(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "right"))
	_, err := h.e.CloseItem("w5", true, false)
	mustOK(t, err)
	h.drain()
	before := layout.Clone(h.tabNamed("Code").Root)
	mustOK(t, h.e.ResetLayout(1))
	if h.em.lastToast() != "Reset the layout of Code" {
		t.Errorf("toast %q", h.em.lastToast())
	}
	var added string
	for _, id := range layout.Items(h.tabNamed("Code").Root) {
		if h.item(id).Kind == KindBuiltin {
			added = id
		}
	}
	if added == "" {
		t.Fatalf("Reset layout should open Code's terminal again: %s", treeText(h.tabNamed("Code").Root))
	}

	undoLast(t, h)
	sameTree(t, h, "Code", before)
	if h.tabNamed("Code").Preset != layout.PresetCustom {
		t.Errorf("preset %s, want custom again", h.tabNamed("Code").Preset)
	}
	if _, ok := h.view().Items[added]; ok {
		t.Errorf("the terminal that Reset layout opened, %s, is still there", added)
	}
	if _, ok := h.terms.started[added]; ok {
		t.Error("its session wasn't ended")
	}
	if got := h.em.lastToast(); got != "Put back the layout of Code." {
		t.Errorf("toast %q", got)
	}
}

// TestUndoClosePane checks that Undo after Close this pane puts the pane
// back, with the places it was waiting for.
func TestUndoClosePane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.p.QuitApp(obsidianID))
	h.drain()
	before := layout.Clone(h.tabNamed("Research").Root)
	mustOK(t, h.e.ClosePane("p15"))
	if got := h.em.lastToast(); got != "Closed the pane." {
		t.Errorf("toast %q", got)
	}
	undoLast(t, h)
	sameTree(t, h, "Research", before)
	if it := h.item("w7"); it.Pane != "p15" {
		t.Errorf("Obsidian waits in %s, want p15", it.Pane)
	}
	if got := h.em.lastToast(); got != "Put back the pane." {
		t.Errorf("toast %q", got)
	}
	// Obsidian's window, when it opens, goes back to its place.
	mustOK(t, h.p.LaunchApp(obsidianID))
	h.p.AddWindow(obsidianID, "Daily note")
	h.settle()
	if it := h.item("w7"); it.Pane != "p15" || it.Hidden {
		t.Errorf("Obsidian's window: %+v", it)
	}
}

// TestUndoMoves checks Undo after a drag (MoveItem), a move to another tab,
// Also show in, and Remove from this tab.
func TestUndoMoves(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	code := layout.Clone(h.tabNamed("Code").Root)
	research := layout.Clone(h.tabNamed("Research").Root)
	zed := h.windowID(zedID, "bench — switch.go")

	mustOK(t, h.e.MoveItem("w3", 1, "p12", "right"))
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go." {
		t.Errorf("toast %q", got)
	}
	undoLast(t, h)
	sameTree(t, h, "Code", code)
	if h.tabNamed("Code").Preset != "main-side" {
		t.Errorf("preset %s, want main-side again", h.tabNamed("Code").Preset)
	}
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed: %v, want p11's body %v", h.frame(zed), h.bodyOf("p11"))
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go back." {
		t.Errorf("toast %q", got)
	}

	mustOK(t, h.e.MoveItemToTab("w3", 2))
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go to Research." {
		t.Errorf("toast %q", got)
	}
	undoLast(t, h)
	sameTree(t, h, "Code", code)
	sameTree(t, h, "Research", research)

	mustOK(t, h.e.AddItemToTab("w3", 2))
	undoLast(t, h)
	sameTree(t, h, "Research", research)
	if it := h.item("w3"); len(it.Tabs) != 0 {
		t.Errorf("Zed is in tabs %v, want Code alone", it.Tabs)
	}
	if got := h.em.lastToast(); got != "Took Zed — bench — switch.go out of Research again." {
		t.Errorf("toast %q", got)
	}

	mustOK(t, h.e.AddItemToTab("w3", 2))
	withZed := layout.Clone(h.tabNamed("Research").Root)
	mustOK(t, h.e.RemoveItemFromTab("w3", 2))
	h.em.lastUndo(t)
	undoLast(t, h)
	sameTree(t, h, "Research", withZed)
	if got := h.em.lastToast(); got != "Put Zed — bench — switch.go back in Research." {
		t.Errorf("toast %q", got)
	}
}

// TestUndoSeveralChanges checks that Undo takes back the change it names
// and those after it, latest first, and that what has closed since stays
// closed, and what has come since stays where it is.
func TestUndoSeveralChanges(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	code := layout.Clone(h.tabNamed("Code").Root)
	mustOK(t, h.e.MoveItem("w3", 1, "p12", "right"))
	first := h.em.lastUndo(t)
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "center"))
	second := h.em.lastUndo(t)
	// VS Code's window closes, and a new terminal comes in Zed's pane.
	mustOK(t, h.p.CloseWindow(h.windowID(vscodeID, "index.html — bench-ui")))
	h.settle()
	zedPane := h.paneOf("w3")
	before := h.view().Items
	mustOK(t, h.e.AddToPane(zedPane, Source{Type: "terminal"}))
	var extra string
	for id := range h.view().Items {
		if _, ok := before[id]; !ok {
			extra = id
		}
	}
	if extra == "" {
		t.Fatal("the new terminal has no item")
	}

	mustOK(t, h.e.Undo(first))
	h.drain()
	// Zed is back in p11, without VS Code, and the terminal, whose pane
	// has gone, is in the tab's focused pane.
	want := layout.Clone(code)
	want, _, _ = layout.TakeItem(want, "w4")
	got, _, _ := layout.TakeItem(layout.Clone(h.tabNamed("Code").Root), extra)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Code without the new terminal:\n got %s\nwant %s", treeText(got), treeText(want))
	}
	if _, ok := h.view().Items["w4"]; ok {
		t.Error("VS Code's closed window came back")
	}
	if it := h.item(extra); it.Tab != 1 {
		t.Errorf("the new terminal is in tab %d, want Code", it.Tab)
	}
	if _, ok := h.terms.started[extra]; !ok {
		t.Error("the new terminal's session ended")
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go back." {
		t.Errorf("toast %q", got)
	}
	// The later change went with it.
	mustErr(t, h.e.Undo(second), "Bench can't undo that any more.")
}

// TestUndoKeepsTen checks that Undo can take back the last ten changes,
// not older ones.
func TestUndoKeepsTen(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	var steps []int
	for i := range undoKept + 1 {
		zone := []string{"right", "center"}[i%2]
		mustOK(t, h.e.MoveItem("w3", 1, h.paneOf("w5"), zone))
		steps = append(steps, h.em.lastUndo(t))
	}
	mustErr(t, h.e.Undo(steps[0]), "Bench can't undo that any more.")
	mustOK(t, h.e.Undo(steps[1]))
}

// TestUndoAfterTabChanges checks that a change to the tabs that Undo can't
// take back, such as a new tab or a reloaded config.toml, puts the earlier
// changes out of its reach.
func TestUndoAfterTabChanges(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.DeleteTab(2))
	step := h.em.lastUndo(t)
	mustOK(t, h.e.SaveTab(-1, draftOf(h.view(), "Extra", "single", "stack")))
	mustErr(t, h.e.Undo(step), "Bench can't undo that any more.")
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Extra"}) {
		t.Errorf("tabs %v", got)
	}

	mustOK(t, h.e.MoveItem("w3", 1, "p12", "right"))
	step = h.em.lastUndo(t)
	mustOK(t, h.e.ReloadConfig())
	mustErr(t, h.e.Undo(step), "Bench can't undo that any more.")
}

// TestReopenClosedTab checks that Reopen closed tab brings back a deleted
// tab where it was, with its windows, its entry in config.toml, and its
// rule, and switches to it.
func TestReopenClosedTab(t *testing.T) {
	h := newHarness(t, setup{config: string(config.ExampleTOML()) + researchRule})
	research := layout.Clone(h.tabNamed("Research").Root)
	mustOK(t, h.e.DeleteTab(2))
	step := h.em.lastUndo(t)
	mustOK(t, h.e.DeleteTab(0))
	if got := h.view().ClosedTabs; !slices.Equal(got, []string{"Comms", "Research"}) {
		t.Errorf("ClosedTabs = %v, want the latest first", got)
	}

	mustOK(t, h.e.ReopenTab("Research"))
	h.settle()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Code", "Research"}) || v.Current != 1 {
		t.Fatalf("tabs %v, current %d", got, v.Current)
	}
	sameShape(t, h, "Research", research)
	if got := h.em.lastToast(); got != "Reopened Research." {
		t.Errorf("toast %q", got)
	}
	if got := v.ClosedTabs; !slices.Equal(got, []string{"Comms"}) {
		t.Errorf("ClosedTabs = %v, want Comms", got)
	}
	docs := h.windowID(braveID, "Wails v3 docs")
	if pane := h.paneOf("w6"); h.frame(docs) != h.bodyOf(pane) {
		t.Errorf("Wails v3 docs: %v, want %s's body %v", h.frame(docs), pane, h.bodyOf(pane))
	}
	cfg := savedConfig(t, h)
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Code", "Research"}) || len(cfg.Rules) != 2 ||
		cfg.Rules[1].Tab != "Research" {
		t.Errorf("config.toml: tabs %v, rules %+v", got, cfg.Rules)
	}
	// Undo no longer reaches the changes before it.
	mustErr(t, h.e.Undo(step), "Bench can't undo that any more.")
	mustErr(t, h.e.ReopenTab("Research"), "Bench no longer has that tab to reopen.")

	// Comms comes back first, where it was, with its pages.
	mustOK(t, h.e.ReopenTab("Comms"))
	h.settle()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("tabs %v", got)
	}
	if it := h.item("w1"); it.Tab != 0 || it.Kind != KindWeb {
		t.Errorf("Slack's page: %+v", it)
	}
}

// TestReopenClosedTabAfterRestart checks that state.json keeps the closed
// tabs, so that Reopen closed tab works after Bench restarts.
func TestReopenClosedTabAfterRestart(t *testing.T) {
	first := newHarness(t, setup{config: string(config.ExampleTOML()) + researchRule})
	research := layout.Clone(first.tabNamed("Research").Root)
	mustOK(t, first.e.DeleteTab(2))
	mustOK(t, first.e.Close())
	cfgData, err := os.ReadFile(filepath.Join(first.dir, config.ConfigFile))
	mustOK(t, err)
	stateData, err := os.ReadFile(filepath.Join(first.dir, config.StateFile))
	mustOK(t, err)

	h := newHarness(t, setup{config: string(cfgData), state: writeState(stateData)})
	v := h.view()
	if !slices.Equal(v.ClosedTabs, []string{"Research"}) || len(v.Tabs) != 2 {
		t.Fatalf("after restart: tabs %v, ClosedTabs %v", tabNames(v), v.ClosedTabs)
	}
	mustOK(t, h.e.ReopenTab("Research"))
	h.settle()
	sameShape(t, h, "Research", research)
	for _, id := range []string{"w6", "w17", "w18", "w7"} {
		if it := h.item(id); it.Tab != 2 {
			t.Errorf("%s is in tab %d, want Research", id, it.Tab)
		}
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("config.toml's tabs: %v", got)
	}
	if len(savedConfig(t, h).Rules) != 2 {
		t.Errorf("rules: %+v", savedConfig(t, h).Rules)
	}
	if len(h.view().ClosedTabs) != 0 {
		t.Errorf("ClosedTabs = %v", h.view().ClosedTabs)
	}
}

// TestReopenClosedTabNameTaken checks that a closed tab whose name another
// tab has taken since comes back numbered, and that Bench keeps the five
// latest closed tabs.
func TestReopenClosedTabNameTaken(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.DeleteTab(2))
	mustOK(t, h.e.SaveTab(-1, draftOf(h.view(), "Research", "single", "stack")))
	mustOK(t, h.e.ReopenTab("research"))
	h.settle()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Research (1)", "Research"}) {
		t.Errorf("tabs %v", got)
	}
	if got := h.em.lastToast(); got != "Reopened Research (1)." {
		t.Errorf("toast %q", got)
	}

	for i := range closedKept + 1 {
		mustOK(t, h.e.SaveTab(-1, draftOf(h.view(), fmt.Sprintf("T%d", i), "single", "stack")))
	}
	// Deleted last first, T5 is the oldest, and goes.
	for range closedKept + 1 {
		mustOK(t, h.e.DeleteTab(len(h.view().Tabs)-1))
	}
	want := []string{"T0", "T1", "T2", "T3", "T4"}
	if got := h.view().ClosedTabs; !slices.Equal(got, want) {
		t.Errorf("ClosedTabs = %v, want %v", got, want)
	}
}

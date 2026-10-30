package workspace

import (
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// shareZed puts Code's Zed window (w3) in Research's focused pane as well,
// from the Code tab, and returns its window.
func shareZed(t *testing.T, h *harness) platform.WindowID {
	t.Helper()
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddItemToTab("w3", 2))
	h.settle()
	return h.windowID(zedID, "bench — switch.go")
}

func TestAddItemToTabDocksTheWindowInEachTab(t *testing.T) {
	h := newHarness(t, setup{})
	zed := shareZed(t, h)
	if got := h.em.lastToast(); got != "Showing Zed — bench — switch.go in Research too." {
		t.Errorf("toast: %q", got)
	}
	it := h.item("w3")
	if !slices.Equal(it.Tabs, []int{1, 2}) || it.Tab != 1 || it.Pane != "p11" {
		t.Errorf("w3: tabs %v, tab %d, pane %s", it.Tabs, it.Tab, it.Pane)
	}
	if got := h.tabNamed("Research").Root.Children[0].Items; !slices.Equal(got, []string{"w6", "w17", "w18", "w3"}) {
		t.Errorf("Research's focused pane: %v", got)
	}
	if h.view().Current != 1 || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Code should stay current, with Zed in its pane")
	}

	mustOK(t, h.e.SwitchTab(2))
	it = h.item("w3")
	if it.Tab != 2 || it.Pane != "p14" {
		t.Errorf("in Research, w3 is in tab %d, pane %s", it.Tab, it.Pane)
	}
	if h.p.IsHidden(zedID) || h.e.win.IsParked(zed) || h.frame(zed) != h.bodyOf("p14") {
		t.Errorf("Zed should dock in Research's pane: hidden %v, parked %v, frame %v",
			h.p.IsHidden(zedID), h.e.win.IsParked(zed), h.frame(zed))
	}
	if !h.p.IsHidden(vscodeID) {
		t.Errorf("VS Code is only in Code, so it should be hidden")
	}

	mustOK(t, h.e.SwitchTab(1))
	if h.p.IsHidden(zedID) || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("back in Code, Zed should dock in Code's pane again: %v", h.frame(zed))
	}
	if !h.p.IsHidden(braveID) {
		t.Errorf("Brave is only in Research, so it should be hidden")
	}
}

func TestAddItemToTabParksOnlyWindowsOfOtherTabs(t *testing.T) {
	h := newHarness(t, setup{})
	// Brave's docs window joins Code: Brave's other windows, only in
	// Research, are parked rather than hidden with their app.
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.AddItemToTab("w6", 1))
	mustOK(t, h.e.SwitchTab(1))
	docs := h.windowID(braveID, "Wails v3 docs")
	pr := h.windowID(braveID, "Pull request #42 · bench")
	if h.p.IsHidden(braveID) || h.e.win.IsParked(docs) || !h.e.win.IsParked(pr) {
		t.Errorf("Brave hidden %v, docs parked %v, pull request parked %v",
			h.p.IsHidden(braveID), h.e.win.IsParked(docs), h.e.win.IsParked(pr))
	}
	if h.item("w6").Pane != "p11" || h.frame(docs) != h.bodyOf("p11") {
		t.Errorf("the docs window should dock in Code's focused pane")
	}
}

func TestAddItemToTabRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	zed := shareZed(t, h)
	mustErr(t, h.e.AddItemToTab("w3", 2), "That window is already in Research.")
	mustErr(t, h.e.AddItemToTab("w3", 1), "That window is already in Code.")
	mustErr(t, h.e.AddItemToTab("w99", 2), "That window has closed.")
	mustErr(t, h.e.AddItemToTab("w3", 5), "There's no tab 6.")
	mustOK(t, h.e.PopOut("w4"))
	mustErr(t, h.e.AddItemToTab("w4", 2), errPoppedOutDrag.Error())
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("refusals shouldn't move Zed")
	}
}

func TestAddItemToTabWorksForWebPagesAndTerminals(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.AddItemToTab("w2", 2))
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddItemToTab("w5", 2))
	mustOK(t, h.e.SwitchTab(2))
	v := h.view()
	if v.Items["w2"].Pane != "p14" || v.Items["w5"].Pane != "p14" {
		t.Errorf("Outlook in %s, the terminal in %s", v.Items["w2"].Pane, v.Items["w5"].Pane)
	}
	mustOK(t, h.e.ShowItem("p14", "w2"))
	wv := h.p.WebViews()
	if !wv["w2"].Visible || wv["w2"].Frame != v.Display.Panes[0].Body {
		t.Errorf("Outlook's web view should show in Research's pane: %+v", wv["w2"])
	}
	mustOK(t, h.e.SwitchTab(0))
	wv = h.p.WebViews()
	if !wv["w2"].Visible || h.view().Items["w2"].Pane != "p9" {
		t.Errorf("back in Comms, Outlook should show in its own pane: %+v", wv["w2"])
	}
}

func TestRemoveItemFromTab(t *testing.T) {
	h := newHarness(t, setup{})
	zed := shareZed(t, h)
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.RemoveItemFromTab("w3", 2))
	if got := h.em.lastToast(); got != "Removed Zed — bench — switch.go from Research. It's still in Code." {
		t.Errorf("toast: %q", got)
	}
	it := h.item("w3")
	if it.Tabs != nil || it.Tab != 1 || it.Pane != "p11" {
		t.Errorf("w3: tabs %v, tab %d, pane %s", it.Tabs, it.Tab, it.Pane)
	}
	if !h.p.IsHidden(zedID) && !h.e.win.IsParked(zed) {
		t.Errorf("Zed belongs to Code alone now, so it should be out of sight")
	}
	mustErr(t, h.e.RemoveItemFromTab("w3", 1), "That window is only in this tab. To take it out of Bench, pop it out.")
	mustErr(t, h.e.RemoveItemFromTab("w3", 2), "That window isn't in that tab.")
	mustErr(t, h.e.RemoveItemFromTab("w99", 2), "That window has closed.")
	mustErr(t, h.e.RemoveItemFromTab("w3", 7), "There's no tab 8.")
}

func TestMovingAnItemInSeveralTabs(t *testing.T) {
	h := newHarness(t, setup{})
	shareZed(t, h)
	mustOK(t, h.e.SwitchTab(2))
	// Within Research: Code keeps its place.
	mustOK(t, h.e.MoveItem("w3", 2, "p15", "center"))
	if h.item("w3").Pane != "p15" || !slices.Contains(h.tabNamed("Code").Root.Children[0].Items, "w3") {
		t.Errorf("w3 should move within Research only")
	}
	// To Code, where it is already: it only leaves Research.
	mustOK(t, h.e.MoveItemToTab("w3", 1))
	it := h.item("w3")
	if it.Tabs != nil || it.Tab != 1 || it.Pane != "p11" {
		t.Errorf("w3: tabs %v, tab %d, pane %s", it.Tabs, it.Tab, it.Pane)
	}
	// To Comms, where it isn't: it moves there from Code, the only tab
	// holding it.
	mustOK(t, h.e.AddItemToTab("w3", 2))
	mustOK(t, h.e.MoveItemToTab("w3", 0))
	if got := h.e.View().Items["w3"].Tabs; !slices.Equal(got, []int{0, 1}) {
		t.Errorf("w3 should leave Research, the current tab, for Comms: %v", got)
	}
}

func TestClosingAWindowInSeveralTabs(t *testing.T) {
	h := newHarness(t, setup{})
	zed := shareZed(t, h)
	mustOK(t, h.p.CloseWindow(zed))
	h.settle()
	if _, ok := h.view().Items["w3"]; ok {
		t.Errorf("w3 should be gone")
	}
	for _, name := range []string{"Code", "Research"} {
		if slices.Contains(leafItems(h.tabNamed(name)), "w3") {
			t.Errorf("%s still holds w3", name)
		}
	}
}

func TestRetitlingAWindowInSeveralTabs(t *testing.T) {
	h := newHarness(t, setup{})
	zed := shareZed(t, h)
	mustOK(t, h.p.RetitleWindow(zed, "bench — model.go"))
	h.drain()
	mustOK(t, h.e.SwitchTab(2))
	if got := h.item("w3").Title; got != "bench — model.go" {
		t.Errorf("title in Research: %q", got)
	}
}

func TestFocusingAWindowInSeveralTabsStaysInTheCurrentTab(t *testing.T) {
	h := newHarness(t, setup{})
	zed := shareZed(t, h)
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.ShowItem("p14", "w6"))
	h.advance(time.Second)
	mustOK(t, h.p.FocusWindow(zed))
	h.drain()
	v := h.view()
	if v.Current != 2 {
		t.Errorf("focusing Zed switched to tab %d", v.Current)
	}
	if v.Display.Panes[0].Active != "w3" {
		t.Errorf("Zed should be shown in Research's pane: %s", v.Display.Panes[0].Active)
	}
}

func TestPoppingOutAnItemInSeveralTabs(t *testing.T) {
	h := newHarness(t, setup{})
	shareZed(t, h)
	mustOK(t, h.e.PopOut("w3"))
	if !slices.Contains(floatingIDs(h.view()), "w3") {
		t.Errorf("w3 should be out of Bench")
	}
	for _, name := range []string{"Code", "Research"} {
		if slices.Contains(leafItems(h.tabNamed(name)), "w3") {
			t.Errorf("%s still holds w3", name)
		}
	}
}

func TestDeletingATabKeepsItsWindowsInOtherTabs(t *testing.T) {
	h := newHarness(t, setup{})
	zed := shareZed(t, h)
	mustOK(t, h.e.DeleteTab(2))
	if slices.Contains(floatingIDs(h.view()), "w3") {
		t.Errorf("w3 is still in Code, so it shouldn't float")
	}
	it := h.item("w3")
	if it.Tabs != nil || it.Pane != "p11" || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("w3: tabs %v, pane %s, frame %v", it.Tabs, it.Pane, h.frame(zed))
	}
}

func TestAddDialogOffersOtherTabsWindows(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	opts, err := h.e.AddOptions("p15")
	mustOK(t, err)
	var others []AddOption
	for _, o := range opts {
		if o.Group == groupOtherTabs {
			others = append(others, o)
		}
	}
	want := []AddOption{
		{Group: groupOtherTabs, Mono: "Sl", Label: "Slack", Hint: "Comms · Slack", Source: Source{Type: sourceItem, ID: "w1"}},
		{Group: groupOtherTabs, Mono: "Ou", Label: "Outlook", Hint: "Comms · Outlook", Source: Source{Type: sourceItem, ID: "w2"}},
		{Group: groupOtherTabs, Mono: "Zd", Icon: appIcon(zedID), Label: "bench — switch.go", Hint: "Code · Zed", Source: Source{Type: sourceItem, ID: "w3"}},
		{Group: groupOtherTabs, Mono: "VS", Icon: appIcon(vscodeID), Label: "index.html — bench-ui", Hint: "Code · VS Code", Source: Source{Type: sourceItem, ID: "w4"}},
		{Group: groupOtherTabs, Mono: ">_", Label: "zsh — ~/code", Hint: "Code · Terminal", Source: Source{Type: sourceItem, ID: "w5"}},
	}
	if !slices.Equal(others, want) {
		t.Errorf("other tabs' windows:\n got %+v\nwant %+v", others, want)
	}

	mustOK(t, h.e.AddToPane("p15", Source{Type: sourceItem, ID: "w3"}))
	zed := h.windowID(zedID, "bench — switch.go")
	v := h.view()
	if v.Items["w3"].Pane != "p15" || v.Display.Focus != "p15" || h.frame(zed) != h.bodyOf("p15") {
		t.Errorf("Zed should be shown in p15: pane %s, focus %s", v.Items["w3"].Pane, v.Display.Focus)
	}
	if !slices.Equal(v.Items["w3"].Tabs, []int{1, 2}) {
		t.Errorf("w3's tabs: %v", v.Items["w3"].Tabs)
	}
	mustErr(t, h.e.AddToPane("p15", Source{Type: sourceItem, ID: "w3"}), "That window is already in Research.")
	mustErr(t, h.e.AddToPane("p15", Source{Type: sourceItem, ID: "w99"}), "That window has closed.")
}

func TestItemInSeveralTabsIsSavedAndRestored(t *testing.T) {
	data := savedState(t, func(h *harness) { shareZed(t, h) })
	h := newHarness(t, setup{state: writeState(data)})
	it := h.item("w3")
	if !slices.Equal(it.Tabs, []int{1, 2}) {
		t.Fatalf("w3's tabs after a restart: %v", it.Tabs)
	}
	zed := h.windowID(zedID, "bench — switch.go")
	if got := h.itemOf(zed); got != "w3" {
		t.Errorf("Zed's window should match w3 again: %q", got)
	}
	mustOK(t, h.e.SwitchTab(2))
	if h.frame(zed) != h.bodyOf("p14") {
		t.Errorf("Zed should dock in Research after a restart")
	}
}

func TestAppRestartRebindsAnItemInSeveralTabs(t *testing.T) {
	h := newHarness(t, setup{})
	shareZed(t, h)
	h.p.QuitApp(zedID)
	h.drain()
	if it := h.item("w3"); it.Status != StatusMissing || !slices.Equal(it.Tabs, []int{1, 2}) {
		t.Fatalf("w3 should wait for Zed in both tabs: %+v", it)
	}
	h.p.InstallApp(zedID, "Zed", platform.AppLaunchWindows("bench — switch.go", "scratch"))
	h.p.LaunchApp(zedID)
	h.drain()
	zed := h.windowID(zedID, "bench — switch.go")
	if got := h.itemOf(zed); got != "w3" {
		t.Errorf("Zed's window should match w3 again: %q", got)
	}
	if got := h.itemOf(h.windowID(zedID, "scratch")); got == "" || got == "w3" {
		t.Errorf("the second window should be a new item: %q", got)
	}
	if it := h.item("w3"); !slices.Equal(it.Tabs, []int{1, 2}) || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("w3 should be back in both tabs, docked in Code: %+v", it)
	}
	mustOK(t, h.e.SwitchTab(2))
	if h.frame(zed) != h.bodyOf("p14") {
		t.Errorf("Zed should dock in Research too")
	}
}

// leafItems returns the items of every pane of a tab.
func leafItems(tv TabView) []string {
	var out []string
	for _, p := range layout.Leaves(tv.Root) {
		out = append(out, p.Items...)
	}
	return out
}

func TestMoveToTabWhereAnItemIsFocusesTheItemLeftBehind(t *testing.T) {
	h := newHarness(t, setup{})
	shareZed(t, h)
	// Code's p11 shows Zed, which is in Research too, with VS Code behind
	// it.
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.p.ResetCalls()
	mustOK(t, h.e.MoveItemToTab("w3", 2))
	if it := h.item("w3"); it.Tab != 2 || it.Tabs != nil {
		t.Errorf("w3 should only be in Research now: %+v", it)
	}
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if focus := h.focusCalls(); !slices.Contains(focus, code) {
		t.Errorf("VS Code, left showing in the focused pane, should be focused: %v", focus)
	}
}

// twoTabCommandConfig has a terminal that runs a command in a tab of its
// own, and another tab.
const twoTabCommandConfig = commandConfig + `
[[tab]]
name = "Notes"
layout = "single"
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
`

func TestEndedCommandTerminalStaysInEachOfItsTabs(t *testing.T) {
	h := newHarness(t, setup{config: twoTabCommandConfig})
	id := ""
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			id = it.ID
		}
	}
	mustOK(t, h.e.AddItemToTab(id, 1))
	h.terms.end(id)
	h.terms.cb.Exit(id, nil)
	h.clk.Advance(frame)
	if it := h.item(id); !it.Ended || !slices.Equal(it.Tabs, []int{0, 1}) {
		t.Fatalf("the ended command should stay in both tabs: %+v", it)
	}
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.TermInput(id, "\r"))
	if it := h.item(id); it.Ended || !h.terms.Running(id) || !slices.Equal(it.Tabs, []int{0, 1}) {
		t.Errorf("Return in Notes should run it again, still in both tabs: %+v", it)
	}
}

func TestEndedShellLeavesEachOfItsTabs(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddItemToTab("w5", 2))
	mustOK(t, h.e.SwitchTab(2))
	h.terms.end("w5")
	h.terms.cb.Exit("w5", nil)
	h.clk.Advance(frame)
	if _, ok := h.view().Items["w5"]; ok {
		t.Fatal("the shell's item should go")
	}
	for _, name := range []string{"Code", "Research"} {
		if slices.Contains(leafItems(h.tabNamed(name)), "w5") {
			t.Errorf("%s still holds w5", name)
		}
	}
	h.terms.mu.Lock()
	_, kept := h.terms.started["w5"]
	h.terms.mu.Unlock()
	if kept {
		t.Error("the host should let the ended session go")
	}
}

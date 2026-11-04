package workspace

import (
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// TestRenameItemNamesATerminal checks that a terminal's own name wins over
// the titles its shell sets, everywhere Bench names it, that state.json
// keeps it, and that Use automatic title ("") takes it away again.
func TestRenameItemNamesATerminal(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	auto := h.item("w5").Title
	mustOK(t, h.e.RenameItem("w5", "  API \n server  "))
	it := h.item("w5")
	if it.Title != "API server" || it.Label != "API server" || it.AutoTitle != auto {
		t.Fatalf("renamed: title %q, label %q, automatic %q; want API server, API server, %q", it.Title, it.Label, it.AutoTitle, auto)
	}
	// The shell's title changes what the name hides, not the name.
	h.terms.cb.Title("w5", "vim — ~/code", true)
	h.clk.Advance(frame)
	if it := h.item("w5"); it.Title != "API server" || it.AutoTitle != "vim — ~/code" {
		t.Errorf("after the shell's title: title %q, automatic %q; want API server, vim — ~/code", it.Title, it.AutoTitle)
	}
	// Toasts, the add dialog, and the question before quitting use it.
	mustOK(t, h.e.MoveItemToTab("w5", 0))
	if got := h.em.lastToast(); got != "Moved Terminal — API server to Comms." {
		t.Errorf("toast: %q", got)
	}
	mustOK(t, h.e.SwitchTab(2))
	opts, err := h.e.AddOptions("p14")
	mustOK(t, err)
	if !slices.ContainsFunc(opts, func(o AddOption) bool { return o.Source.ID == "w5" && o.Label == "API server" }) {
		t.Errorf("the add dialog should list the terminal by its name: %+v", opts)
	}
	h.terms.setBusy("w5", "npm run dev")
	if b, ok := h.e.busyTerminalLocked(h.e.model.Items["w5"]); !ok || b.Title != "API server" {
		t.Errorf("busy terminal: %+v %v, want it named API server", b, ok)
	}

	// state.json keeps the name.
	h.settle()
	h.restart()
	if it := h.item("w5"); it.Title != "API server" || it.Label != "API server" {
		t.Errorf("after a restart: title %q, label %q; want API server", it.Title, it.Label)
	}

	// Use automatic title.
	mustOK(t, h.e.RenameItem("w5", ""))
	if it := h.item("w5"); it.Title == "API server" || it.Label != "" || it.AutoTitle != "" {
		t.Errorf("cleared: title %q, label %q, automatic %q; want the shell's title", it.Title, it.Label, it.AutoTitle)
	}
}

// TestRenameItemNamesAWebPage checks that a web page's own name wins over
// its page's title, and is cut short past maxLabel characters.
func TestRenameItemNamesAWebPage(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.RenameItem("w1", "Team chat"))
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w1", Title: "# general"})
	h.drain()
	if it := h.item("w1"); it.Title != "Team chat" || it.AutoTitle != "# general" {
		t.Errorf("after the page's title: title %q, automatic %q; want Team chat, # general", it.Title, it.AutoTitle)
	}
	mustOK(t, h.e.RenameItem("w1", strings.Repeat("é", maxLabel+20)))
	if got := []rune(h.item("w1").Title); len(got) != maxLabel {
		t.Errorf("a long name has %d characters, want %d", len(got), maxLabel)
	}
	mustOK(t, h.e.RenameItem("w1", "   "))
	if it := h.item("w1"); it.Title != "# general" || it.Label != "" {
		t.Errorf("a blank name: title %q, label %q; want the page's title", it.Title, it.Label)
	}
}

// TestRenameItemRefusesAppWindows checks that an app's window keeps its
// app's title, and that a closed item can't be renamed.
func TestRenameItemRefusesAppWindows(t *testing.T) {
	h := newHarness(t, setup{})
	mustErr(t, h.e.RenameItem("w3", "Editor"), errAppTitle.Error())
	if it := h.item("w3"); it.Title != "bench — switch.go" || it.Label != "" {
		t.Errorf("app window: title %q, label %q", it.Title, it.Label)
	}
	mustErr(t, h.e.RenameItem("w99", "Gone"), errGone.Error())
}

// TestItemLabelInState checks that state.json saves a Web or Built-in
// item's name, and that an App item never takes one from it.
func TestItemLabelInState(t *testing.T) {
	m := defaultModel(t)
	m.Items["w5"].Label = "API server"
	m.Items["w3"].Label = "Editor"
	st := m.State()
	if st.Items["w5"].Label != "API server" {
		t.Errorf("state.json: terminal label %q", st.Items["w5"].Label)
	}
	if got := itemFromState("w5", st.Items["w5"], testNamer); got.Label != "API server" {
		t.Errorf("terminal from state.json: label %q", got.Label)
	}
	if got := itemFromState("w3", config.ItemState{Kind: KindApp, BundleID: zedID, Title: "x", Label: "Editor"}, testNamer); got.Label != "" {
		t.Errorf("an App item took a label from state.json: %q", got.Label)
	}
}

// TestMoveItemToNewTab checks Move to new tab: a single-layout tab named
// after the window's app, after the others, with the window in it, which
// Bench switches to and saves in config.toml.
func TestMoveItemToNewTab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.p.ResetCalls()
	mustOK(t, h.e.MoveItemToNewTab("w4", false))
	v := h.view()
	if len(v.Tabs) != 4 || v.Current != 3 {
		t.Fatalf("tabs %d, current %d; want a fourth tab, current", len(v.Tabs), v.Current)
	}
	tab := v.Tabs[3]
	if tab.Name != "VS Code" || tab.Preset != layout.PresetSingle || tab.SmallScreens != "stack" || !slices.Equal(layout.Items(tab.Root), []string{"w4"}) {
		t.Errorf("new tab: %+v, items %v", tab, layout.Items(tab.Root))
	}
	if it := h.item("w4"); it.Tab != 3 || len(it.Tabs) != 0 {
		t.Errorf("w4: tab %d, tabs %v; want only the new tab", it.Tab, it.Tabs)
	}
	if code := v.Tabs[1]; slices.Contains(layout.Items(code.Root), "w4") || code.Preset != layout.PresetCustom {
		t.Errorf("Code: items %v, preset %s; want w4 gone and the layout custom", layout.Items(code.Root), code.Preset)
	}
	if got := h.em.lastToast(); got != "Moved VS Code — index.html — bench-ui to a new tab, VS Code." {
		t.Errorf("toast: %q", got)
	}
	vscode := h.windowID(vscodeID, "index.html — bench-ui")
	if !slices.Contains(h.focusCalls(), vscode) {
		t.Errorf("the window should have the keyboard in its new tab: focused %v", h.focusCalls())
	}
	cfg := savedConfig(t, h)
	if len(cfg.Tabs) != 4 {
		t.Fatalf("config.toml: %d tabs", len(cfg.Tabs))
	}
	ct := cfg.Tabs[3]
	if ct.Name != "VS Code" || ct.Layout != layout.PresetSingle || len(ct.Panes) != 1 || len(ct.Panes[0].Items) != 1 || ct.Panes[0].Items[0].App != vscodeID {
		t.Errorf("config tab: %+v", ct)
	}

	// The tab and its window are there after a restart.
	h.settle()
	h.restart()
	v = h.view()
	if len(v.Tabs) != 4 || v.Tabs[3].Name != "VS Code" || !slices.Equal(layout.Items(v.Tabs[3].Root), []string{"w4"}) {
		t.Errorf("after a restart: %+v", v.Tabs)
	}
}

// TestAlsoShowInNewTab checks Also show in new tab, and that new tabs'
// names are numbered as Tab properties numbers them.
func TestAlsoShowInNewTab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToNewTab("w6", true))
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToNewTab("w17", false))
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToNewTab("w18", true))
	v := h.view()
	var names []string
	for _, tab := range v.Tabs {
		names = append(names, tab.Name)
	}
	if want := []string{"Comms", "Code", "Research", "Brave", "Brave (1)", "Brave (2)"}; !slices.Equal(names, want) {
		t.Fatalf("tabs %v, want %v", names, want)
	}
	if v.Current != 5 {
		t.Errorf("current %d, want the last new tab", v.Current)
	}
	if it := h.item("w6"); !slices.Equal(it.Tabs, []int{2, 3}) {
		t.Errorf("w6 is in tabs %v, want Research and Brave", it.Tabs)
	}
	if research := v.Tabs[2]; !slices.Equal(layout.Items(research.Root), []string{"w6", "w18", "w7"}) {
		t.Errorf("Research: %v; want w6 and w18 still there, and w17 gone", layout.Items(research.Root))
	}
	if got := h.em.lastToast(); got != "Showing Brave — Go packages in a new tab, Brave (2), too." {
		t.Errorf("toast: %q", got)
	}
	var saved []string
	for _, ct := range savedConfig(t, h).Tabs {
		saved = append(saved, ct.Name)
	}
	if !slices.Equal(saved, names) {
		t.Errorf("config.toml's tabs %v, want %v", saved, names)
	}
}

// TestMoveItemToNewTabUndo checks that Undo takes a new tab for a window
// away again, from config.toml too, putting the window back in its pane,
// and that the changes before it can still be taken back.
func TestMoveItemToNewTabUndo(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItemToTab("w3", 2))
	before := h.em.lastUndo(t)
	code := layout.Clone(h.tabNamed("Code").Root)
	mustOK(t, h.e.MoveItemToNewTab("w4", false))
	if !h.em.lastHasAction() {
		t.Fatalf("toast %q has no Undo", h.em.lastToast())
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	h.drain()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code", "Research"}) || v.Current != 1 {
		t.Fatalf("after Undo: tabs %v, current %d", got, v.Current)
	}
	if got := treeText(h.tabNamed("Code").Root); got != treeText(code) {
		t.Errorf("Code: %s, want %s", got, treeText(code))
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("config.toml's tabs %v, want the new tab gone", got)
	}
	if got := h.em.lastToast(); got != "Moved VS Code — index.html — bench-ui back." {
		t.Errorf("toast %q", got)
	}
	// The move before it is still there to take back.
	mustOK(t, h.e.Undo(before))
	h.drain()
	if h.item("w3").Tab != 1 {
		t.Errorf("Zed is in tab %d, want Code again", h.item("w3").Tab)
	}

	// Also show in new tab: Undo leaves the window where it was.
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToNewTab("w6", true))
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	h.drain()
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("config.toml's tabs %v", got)
	}
	if it := h.item("w6"); it.Tab != 2 || len(it.Tabs) != 0 {
		t.Errorf("w6: tab %d, tabs %v; want Research only", it.Tab, it.Tabs)
	}
	if got := h.em.lastToast(); got != "Deleted the new tab, Brave." {
		t.Errorf("toast %q", got)
	}
}

// TestMoveItemToNewTabFromOutside checks that a popped-out terminal comes
// back into Bench in its new tab, while Also show in new tab refuses it,
// and that a window that has gone is refused.
func TestMoveItemToNewTabFromOutside(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w5"))
	h.drain()
	mustErr(t, h.e.MoveItemToNewTab("w5", true), "That window is out of Bench. To add it back, drag it onto a pane and wait a moment, or use +.")
	if len(h.view().Tabs) != 3 {
		t.Fatalf("a refused new tab was added")
	}
	s := h.view().Settings
	s.DragToAdd = false
	mustOK(t, h.e.SaveSettings(s))
	mustErr(t, h.e.MoveItemToNewTab("w5", true), "That window is out of Bench. Add it to a pane with +.")
	mustOK(t, h.e.MoveItemToNewTab("w5", false))
	v := h.view()
	if len(v.Tabs) != 4 || v.Tabs[3].Name != "Terminal" || !slices.Equal(layout.Items(v.Tabs[3].Root), []string{"w5"}) {
		t.Fatalf("tabs: %+v", v.Tabs)
	}
	if slices.Contains(floatingIDs(v), "w5") || h.e.model.Items["w5"].OwnWindow {
		t.Errorf("the terminal should be back in Bench, without a window of its own")
	}
	mustErr(t, h.e.MoveItemToNewTab("w99", false), errGone.Error())
}

func TestNewTabWith(t *testing.T) {
	m := defaultModel(t)
	ti, err := m.NewTabWith("w3", "Zed", false)
	if err != nil || ti != 3 {
		t.Fatalf("NewTabWith: %d %v", ti, err)
	}
	mustValid(t, m)
	tab := m.Tabs[3]
	if tab.Name != "Zed" || tab.Preset != layout.PresetSingle || !equalPanes(paneItems(tab), [][]string{{"w3"}}) {
		t.Errorf("new tab: %s %s %v", tab.Name, tab.Preset, paneItems(tab))
	}
	if tab.FocusedPane().Active != "w3" || tab.CompactShown != "w3" {
		t.Errorf("w3 should show in the new tab")
	}
	if m.Tabs[1].Preset != layout.PresetCustom || slices.Contains(m.Tabs[1].ItemIDs(), "w3") {
		t.Errorf("Code: %s %v; want w3 gone and the layout custom", m.Tabs[1].Preset, paneItems(m.Tabs[1]))
	}
	if m.Current != 0 {
		t.Errorf("current changed to %d", m.Current)
	}
	ti, err = m.NewTabWith("w4", "VS Code", true)
	if err != nil || ti != 4 || !slices.Equal(m.TabsOf("w4"), []int{1, 4}) {
		t.Errorf("also: %d %v, in tabs %v", ti, err, m.TabsOf("w4"))
	}
	mustValid(t, m)
	if _, err := m.NewTabWith("nope", "Nope", false); err != errGone {
		t.Errorf("unknown item: %v", err)
	}
}

// TestRenameTabInPlace renames a tab as the field in the title bar does:
// SaveTab with the custom preset and the tab's own small-screen setting.
// The tab keeps its panes, its preset, and its config layout, and a name
// that another tab has is numbered.
func TestRenameTabInPlace(t *testing.T) {
	h := newHarness(t, setup{})
	before := h.view().Tabs[2]
	layoutBefore := h.e.cfg.Tabs[2].Layout
	mustOK(t, h.e.SaveTab(2, TabDraft{Name: "code", Preset: "custom", SmallScreens: before.SmallScreens}))
	after := h.view().Tabs[2]
	if after.Name != "code (1)" {
		t.Errorf("renamed to %q, want code (1)", after.Name)
	}
	if !slices.Equal(leafIDs(after), leafIDs(before)) || after.Preset != before.Preset || after.SmallScreens != before.SmallScreens {
		t.Errorf("after = %v %s %s, want %v %s %s", leafIDs(after), after.Preset, after.SmallScreens, leafIDs(before), before.Preset, before.SmallScreens)
	}
	if ct := savedConfig(t, h).Tabs[2]; ct.Name != "code (1)" || ct.Layout != layoutBefore {
		t.Errorf("config.toml has %q with layout %q, want code (1) with %q", ct.Name, ct.Layout, layoutBefore)
	}
}

// TestFocusUI checks that the field that renames a tab keeps the keyboard
// from what a click's tab switch just before focused: FocusUI gives Bench's
// own UI the keyboard, and drops a terminal's focus still on its way and a
// page's that Bench being lowered would make.
func TestFocusUI(t *testing.T) {
	h := newHarness(t, setup{config: `
[[tab]]
name = "Work"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }, { web = "https://example.com", name = "Docs" }]
`})
	h.takeCommands()
	h.p.ResetCalls()
	// A switch's focus of a terminal is sent once the lock is released:
	// FocusUI comes first.
	h.e.lock()
	h.e.model.Show("w1")
	h.e.focusShownLocked()
	queued := h.e.out
	h.e.out = nil
	h.e.unlock()
	mustOK(t, h.e.FocusUI())
	for _, f := range queued {
		f()
	}
	if cmds := h.takeCommands(); len(cmds) != 0 {
		t.Errorf("commands %v, want none: the field has the keyboard", cmds)
	}
	if n := len(h.calls("FocusUI")); n != 2 {
		t.Errorf("%d FocusUI calls, want the switch's and the field's", n)
	}

	// A page's focus put off while Bench is raised for a menu.
	mustOK(t, h.e.CycleSubtab(1))
	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.FocusPane(h.view().Display.Focus))
	h.p.ResetCalls()
	mustOK(t, h.e.FocusUI())
	mustOK(t, h.e.SetOverlay(false))
	if calls := h.calls("FocusWebView"); len(calls) != 0 {
		t.Errorf("the page took the keyboard back: %v", calls)
	}
	if got := h.p.FocusedWebView(); got != "" {
		t.Errorf("focused web view %q, want none", got)
	}
}

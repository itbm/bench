package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// draftOf returns a draft for a tab with the view's settings.
func draftOf(v View, name, preset, small string) TabDraft {
	return TabDraft{Name: name, Preset: preset, SmallScreens: small, Settings: v.Settings}
}

// savedConfig reads config.toml back.
func savedConfig(t *testing.T, h *harness) *config.Config {
	t.Helper()
	cfg, problems, err := config.Load(filepath.Join(h.dir, config.ConfigFile))
	if err != nil || len(problems) > 0 {
		t.Fatalf("config.toml: %v %v", err, problems)
	}
	return cfg
}

func TestPreviewTree(t *testing.T) {
	h := newHarness(t, setup{})
	tree, err := h.e.PreviewTree(1, "three")
	mustOK(t, err)
	leaves := layout.Leaves(tree)
	if len(leaves) != 3 || leaves[0].ID != "p11" || !slices.Equal(leaves[0].Items, []string{"w3", "w4"}) ||
		!slices.Equal(leaves[1].Items, []string{"w5"}) {
		t.Errorf("three: %+v", leaves)
	}
	if v := h.view(); len(layout.Leaves(v.Tabs[1].Root)) != 2 {
		t.Errorf("PreviewTree changed the tab")
	}
	// The same preset keeps the tab's sizes.
	mustOK(t, h.e.SetSizes("s13", []float64{70, 30}))
	tree, _ = h.e.PreviewTree(1, "main-side")
	if !slices.Equal(tree.Sizes, []float64{70, 30}) {
		t.Errorf("unchanged preset: sizes %v", tree.Sizes)
	}
	tree, _ = h.e.PreviewTree(1, "side")
	if !slices.Equal(tree.Sizes, []float64{50, 50}) {
		t.Errorf("another preset: sizes %v", tree.Sizes)
	}
	tree, _ = h.e.PreviewTree(-1, "grid")
	if len(layout.Leaves(tree)) != 4 || len(layout.Items(tree)) != 0 {
		t.Errorf("new tab: %+v", tree)
	}
	tree, _ = h.e.PreviewTree(1, "custom")
	if len(layout.Leaves(tree)) != 2 {
		t.Errorf("custom: %+v", tree)
	}
	if _, err := h.e.PreviewTree(-1, "custom"); err == nil {
		t.Errorf("a new tab can't be custom")
	}
	mustErr(t, func() error { _, err := h.e.PreviewTree(1, "nope"); return err }(), "There's no layout called nope.")
	mustErr(t, func() error { _, err := h.e.PreviewTree(9, "single"); return err }(), "There's no tab 10.")
}

func TestSaveTabAddsTab(t *testing.T) {
	h := newHarness(t, setup{})
	v := h.view()
	mustOK(t, h.e.SaveTab(-1, draftOf(v, "  Notes ", "side", "same")))
	v = h.view()
	if len(v.Tabs) != 4 || v.Current != 3 || v.Tabs[3].Name != "Notes" || v.Tabs[3].SmallScreens != "same" {
		t.Fatalf("tabs: %d current %d %+v", len(v.Tabs), v.Current, v.Tabs[len(v.Tabs)-1])
	}
	if len(layout.Leaves(v.Tabs[3].Root)) != 2 || v.Tabs[3].Windows != 0 || v.Tabs[3].Shortcut != "⌃⌥4" {
		t.Errorf("new tab: %+v", v.Tabs[3])
	}
	if got := h.em.lastToast(); got != "Saved Notes" {
		t.Errorf("toast: %q", got)
	}
	cfg := savedConfig(t, h)
	if len(cfg.Tabs) != 4 || cfg.Tabs[3].Name != "Notes" || cfg.Tabs[3].Layout != "side" || cfg.Tabs[3].SmallScreens != "same" {
		t.Errorf("config.toml: %+v", cfg.Tabs)
	}
	if _, err := os.Stat(filepath.Join(h.dir, config.BackupFile)); err != nil {
		t.Errorf("config.toml.bak: %v", err)
	}
}

func TestSaveTabEditsTab(t *testing.T) {
	h := newHarness(t, setup{config: string(config.ExampleTOML()) + "\n[[rule]]\napp = \"com.apple.Safari\"\ntab = \"Research\"\n"})
	mustOK(t, h.e.SwitchTab(2))
	v := h.view()
	d := draftOf(v, "Reading", "three", "stack")
	d.Settings.Gap = 12
	d.Settings.FollowFocus = "off"
	mustOK(t, h.e.SaveTab(2, d))
	v = h.view()
	tab := v.Tabs[2]
	if tab.Name != "Reading" || tab.Preset != "three" || tab.SmallScreens != "stack" || len(layout.Leaves(tab.Root)) != 3 {
		t.Fatalf("tab: %+v", tab)
	}
	if !slices.Equal(layout.Items(tab.Root), []string{"w6", "w17", "w18", "w7"}) {
		t.Errorf("items kept in order: %v", layout.Items(tab.Root))
	}
	if v.Settings.Gap != 12 || v.Settings.FollowFocus != "off" {
		t.Errorf("settings: %+v", v.Settings)
	}
	cfg := savedConfig(t, h)
	ct := cfg.Tabs[2]
	if ct.Name != "Reading" || ct.Layout != "three" || ct.SmallScreens != "" || len(ct.Sizes) != 0 {
		t.Errorf("config tab: %+v", ct)
	}
	if len(ct.Panes) != 2 || ct.Panes[0].Items[0].App != braveID || len(ct.Panes[0].Items) != 1 || ct.Panes[1].Items[0].App != obsidianID {
		t.Errorf("config panes (one entry per app per pane): %+v", ct.Panes)
	}
	if cfg.Settings.Gap != 12 || cfg.Settings.FollowFocus != "off" {
		t.Errorf("config settings: %+v", cfg.Settings)
	}
	if cfg.Rules[len(cfg.Rules)-1].Tab != "Reading" {
		t.Errorf("rules naming the tab should follow the rename: %+v", cfg.Rules)
	}
	// Gap changes the frames.
	if p := v.Display.Panes[0]; p.Rect.X != 12 {
		t.Errorf("gap: %v", p.Rect)
	}
}

// TestSaveTabLeavesOutAppsWithNoWindow checks that saving a tab doesn't
// write an app that has quit into config.toml, unless the file already
// lists it, while its sub-tab stays.
func TestSaveTabLeavesOutAppsWithNoWindow(t *testing.T) {
	h := newHarness(t, setup{config: string(config.ExampleTOML()) + "\n[[rule]]\napp = \"com.apple.Safari\"\ntab = \"Research\"\n"})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.p.LaunchApp(safariID))
	h.drain()
	apple := h.itemOf(h.windowID(safariID, "Apple"))
	if apple == "" {
		t.Fatal("Safari's window should be in Research")
	}
	h.p.QuitApp(safariID)
	h.p.QuitApp(obsidianID)
	h.drain()
	if it := h.item(apple); !it.NoWindow || it.Status != StatusMissing {
		t.Fatalf("Safari's sub-tab: %+v, want it kept with no window", it)
	}
	mustOK(t, h.e.SaveTab(2, draftOf(h.view(), "Research", "side", "tabs")))
	for _, p := range savedConfig(t, h).Tabs[2].Panes {
		for _, it := range p.Items {
			if it.App == safariID {
				t.Errorf("Safari, which has quit, was written: %+v", p.Items)
			}
		}
	}
	if ct := savedConfig(t, h).Tabs[2]; len(ct.Panes) != 2 || ct.Panes[1].Items[0].App != obsidianID {
		t.Errorf("Obsidian, which the file lists, should stay: %+v", ct.Panes)
	}
	if !slices.Contains(layout.Items(h.view().Tabs[2].Root), apple) {
		t.Errorf("Safari's sub-tab should stay in the tab")
	}
}

func TestSaveTabCustomKeepsTree(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w4", 1, "p11", "bottom"))
	before := h.view().Tabs[1].Root
	mustOK(t, h.e.SaveTab(1, draftOf(h.view(), "Code", "custom", "stack")))
	if after := h.view().Tabs[1]; !slices.Equal(leafIDs(after), leafIDsOf(before)) || after.Preset != "custom" {
		t.Errorf("custom should keep the tree: %v", leafIDs(after))
	}
	if ct := savedConfig(t, h).Tabs[1]; ct.Layout != "main-side" || len(ct.Panes) != 2 {
		t.Errorf("custom keeps the config's layout: %+v", ct)
	}
}

func leafIDsOf(root *layout.Node) []string {
	var out []string
	for _, p := range layout.Leaves(root) {
		out = append(out, p.ID)
	}
	return out
}

func TestSaveTabRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	v := h.view()
	mustErr(t, h.e.SaveTab(1, draftOf(v, "  ", "side", "stack")), "Give the tab a name.")
	mustErr(t, h.e.SaveTab(1, draftOf(v, "comms", "side", "stack")), "There's already a tab called Comms.")
	mustErr(t, h.e.SaveTab(-1, draftOf(v, "New", "custom", "stack")), "There's no layout called custom.")
	mustErr(t, h.e.SaveTab(1, draftOf(v, "Code", "spiral", "stack")), "There's no layout called spiral.")
	bad := draftOf(v, "Code", "side", "stack")
	bad.Settings.LaunchTimeout = "soon"
	if err := h.e.SaveTab(1, bad); err == nil {
		t.Errorf("a bad launch timeout should be refused")
	}
	bad = draftOf(v, "Code", "side", "stack")
	bad.Settings.NewWindows = "sideways"
	if err := h.e.SaveTab(1, bad); err == nil || !strings.HasPrefix(err.Error(), "Bench can't save that") {
		t.Errorf("a bad setting: %v", err)
	}
	// Renaming a tab to its own name in another case is fine.
	mustOK(t, h.e.SaveTab(1, draftOf(v, "CODE", "main-side", "stack")))
}

func TestSaveTabAsksForScreenRecording(t *testing.T) {
	h := newHarness(t, setup{})
	d := draftOf(h.view(), "Comms", "side", "tabs")
	d.Settings.LivePreviews = true
	mustOK(t, h.e.SaveTab(0, d))
	if h.p.ScreenRecordingRequests() != 1 {
		t.Errorf("turning on live previews should ask for Screen Recording")
	}
}

func TestSaveTabWithConfigProblems(t *testing.T) {
	bad := "[[tab]]\nname = \"Mine\"\nlayout = \"nope\"\n"
	h := newHarness(t, setup{config: bad})
	mustOK(t, h.e.SaveTab(-1, draftOf(h.view(), "Extra", "single", "stack")))
	if len(h.view().Tabs) != 2 {
		t.Errorf("the change should apply")
	}
	data, _ := os.ReadFile(filepath.Join(h.dir, config.ConfigFile))
	if string(data) != bad {
		t.Errorf("config.toml was written while it has problems")
	}
}

func TestDeleteTab(t *testing.T) {
	h := newHarness(t, setup{config: string(config.ExampleTOML()) + "\n[[rule]]\napp = \"com.apple.Safari\"\ntab = \"Research\"\n",
		world: func(p *platform.Fake) {
			standardWorld(p)
			p.QuitApp(obsidianID)
		}})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItem("w5", 2, "p14", "center"))
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.DeleteTab(2))
	v := h.view()
	if len(v.Tabs) != 2 || v.Current != 1 {
		t.Fatalf("tabs %d current %d", len(v.Tabs), v.Current)
	}
	ids := floatingIDs(v)
	for _, id := range []string{"w6", "w17", "w18", "w5"} {
		if !slices.Contains(ids, id) {
			t.Errorf("%s should be in Floating: %v", id, ids)
		}
	}
	if _, ok := v.Items["w7"]; ok {
		t.Errorf("Obsidian's placeholder has no window, so it's dropped")
	}
	if got := h.em.lastToast(); got != "Deleted Research. Its windows are in Jump to… under “Not in a tab”." {
		t.Errorf("toast: %q", got)
	}
	docs := h.windowID(braveID, "Wails v3 docs")
	if h.p.IsHidden(braveID) || h.e.win.IsParked(docs) {
		t.Errorf("the deleted tab's windows should be on screen")
	}
	cfg := savedConfig(t, h)
	if len(cfg.Tabs) != 2 || len(cfg.Rules) != 1 {
		t.Errorf("config: %d tabs, rules %+v", len(cfg.Tabs), cfg.Rules)
	}
	// Deleting the current tab moves to the one that takes its place.
	mustOK(t, h.e.DeleteTab(1))
	if v := h.view(); v.Current != 0 || len(v.Tabs) != 1 {
		t.Errorf("current %d tabs %d", v.Current, len(v.Tabs))
	}
	mustErr(t, h.e.DeleteTab(0), "You can't delete the last tab.")
	mustErr(t, h.e.DeleteTab(4), "There's no tab 5.")
}

func TestResetLayout(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "right"))
	mustOK(t, h.e.MoveItem("w5", 1, "p11", "center"))
	if h.view().Tabs[1].Preset != "custom" {
		t.Fatal("the tab should be custom")
	}
	mustOK(t, h.e.ResetLayout(1))
	v := h.view()
	tab := v.Tabs[1]
	if tab.Preset != "main-side" || !slices.Equal(tab.Root.Sizes, []float64{65, 35}) {
		t.Errorf("preset %s sizes %v", tab.Preset, tab.Root.Sizes)
	}
	leaves := layout.Leaves(tab.Root)
	if len(leaves) != 2 || !slices.Contains(leaves[0].Items, "w3") || !slices.Contains(leaves[0].Items, "w4") ||
		!slices.Equal(leaves[1].Items, []string{"w5"}) {
		t.Errorf("items in their config panes: %+v %+v", leaves[0].Items, leaves[1].Items)
	}
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if h.frame(code) != h.bodyOf(leaves[0].ID) && !h.e.win.IsParked(code) {
		t.Errorf("VS Code should be back in the first pane")
	}
	mustErr(t, h.e.ResetLayout(5), "There's no tab 6.")
}

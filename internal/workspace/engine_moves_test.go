package workspace

import (
	"slices"
	"testing"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestMoveItemJoinsPane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.p.ResetCalls()
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "center"))
	v := h.view()
	if v.Items["w4"].Pane != "p12" || v.Display.Focus != "p12" || v.Tabs[1].Preset != "custom" {
		t.Errorf("w4 in %s, focus %s, preset %s", v.Items["w4"].Pane, v.Display.Focus, v.Tabs[1].Preset)
	}
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if h.frame(code) != h.bodyOf("p12") {
		t.Errorf("VS Code should dock in its new pane")
	}
	if !slices.Contains(h.focusCalls(), code) {
		t.Errorf("the moved window should be focused: %v", h.focusCalls())
	}
	// Moving to its own pane's centre does nothing.
	h.p.ResetCalls()
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "center"))
	if len(h.calls("SetFrame", "Focus")) != 0 {
		t.Errorf("a move to its own pane's centre did something")
	}
}

func TestMoveItemSplits(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItem("w17", 2, "p14", "right"))
	v := h.view()
	fresh := v.Items["w17"].Pane
	if fresh == "p14" || v.Display.Focus != fresh || len(v.Display.Panes) != 3 {
		t.Fatalf("w17 in %s, focus %s, %d panes", fresh, v.Display.Focus, len(v.Display.Panes))
	}
	pr := h.windowID(braveID, "Pull request #42 · bench")
	docs := h.windowID(braveID, "Wails v3 docs")
	if h.frame(pr) != h.bodyOf(fresh) || h.frame(docs) != h.bodyOf("p14") {
		t.Errorf("side by side: %v %v", h.frame(pr), h.frame(docs))
	}
	if h.frame(pr).X <= h.frame(docs).X {
		t.Errorf("the new pane should be on the right")
	}
}

func TestMoveItemRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustErr(t, h.e.MoveItem("w5", 1, "p12", "right"),
		"This pane has only one window, so there's nothing to show beside it. Add another window first, or drag one here.")
	mustErr(t, h.e.MoveItem("w99", 1, "p12", "center"), "That window has closed.")
	mustErr(t, h.e.MoveItem("w3", 5, "p12", "center"), "There's no tab 6.")
	mustErr(t, h.e.MoveItem("w3", 1, "p99", "center"), "There's no such pane.")
	mustErr(t, h.e.MoveItem("w3", 1, "p12", "middle"), "That isn't a place to put a window.")
}

func TestPaneCollapsesAndLastPaneStays(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w5", 1, "p11", "center"))
	v := h.view()
	if leaves := leafIDs(v.Tabs[1]); !slices.Equal(leaves, []string{"p11"}) {
		t.Errorf("p12 should close up: %v", leaves)
	}
	// Every item leaves: the last pane stays, empty.
	for _, id := range []string{"w3", "w4", "w5"} {
		mustOK(t, h.e.MoveItemToTab(id, 0))
	}
	v = h.view()
	if leaves := leafIDs(v.Tabs[1]); !slices.Equal(leaves, []string{"p11"}) || len(v.Display.Panes[0].Items) != 0 {
		t.Errorf("the last pane should stay, empty: %v %+v", leaves, v.Display.Panes)
	}
	if v.Tabs[1].Windows != 0 || v.Tabs[1].Panes != 1 {
		t.Errorf("counts: %d windows, %d panes", v.Tabs[1].Windows, v.Tabs[1].Panes)
	}
}

func TestMoveItemToTab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItemToTab("w4", 2))
	v := h.view()
	if v.Current != 1 {
		t.Errorf("MoveItemToTab mustn't switch tabs")
	}
	if it := v.Items["w4"]; it.Tab != 2 || it.Pane != "p14" {
		t.Errorf("w4: tab %d pane %s", it.Tab, it.Pane)
	}
	if active := layout.Leaves(v.Tabs[2].Root)[0].Active; active != "w4" {
		t.Errorf("it should be shown in Research's focused pane: %s", active)
	}
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if !h.p.IsHidden(vscodeID) {
		t.Errorf("VS Code's only item is in another tab now, so it should be hidden (parked %v)", h.e.win.IsParked(code))
	}
	before := h.view()
	mustOK(t, h.e.MoveItemToTab("w4", 2))
	if after := h.view(); !slices.Equal(layout.Items(after.Tabs[2].Root), layout.Items(before.Tabs[2].Root)) {
		t.Errorf("moving to its own tab should do nothing")
	}
}

func TestFocusDirection(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.FocusDirection("right"))
	if got := h.view().Display.Focus; got != "p12" {
		t.Errorf("right: %s", got)
	}
	mustOK(t, h.e.FocusDirection("right"))
	if got := h.view().Display.Focus; got != "p12" {
		t.Errorf("no pane to the right: %s", got)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.FocusDirection("left"))
	zed := h.windowID(zedID, "bench — switch.go")
	if got := h.view().Display.Focus; got != "p11" || !slices.Equal(h.focusCalls(), []platform.WindowID{zed}) {
		t.Errorf("left: %s, focus calls %v", got, h.focusCalls())
	}
	mustErr(t, h.e.FocusDirection("sideways"), "That isn't a direction: use left, right, up, or down.")
}

func TestMoveDirection(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// Zed into the terminal's pane, to the right.
	mustOK(t, h.e.MoveDirection("right"))
	v := h.view()
	if v.Items["w3"].Pane != "p12" || v.Display.Focus != "p12" {
		t.Fatalf("w3 in %s, focus %s", v.Items["w3"].Pane, v.Display.Focus)
	}
	// No pane to the right and two items: split at that edge.
	mustOK(t, h.e.MoveDirection("right"))
	v = h.view()
	if len(v.Display.Panes) != 3 || v.Items["w3"].Pane == "p12" {
		t.Fatalf("split: %d panes, w3 in %s", len(v.Display.Panes), v.Items["w3"].Pane)
	}
	// Alone in its pane, with nothing to the right: refused.
	mustErr(t, h.e.MoveDirection("right"), "There's nothing to move it next to.")
	// Up and down have no neighbours in a row either.
	mustErr(t, h.e.MoveDirection("up"), "There's nothing to move it next to.")
}

func TestCycleSubtab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	tests := []struct {
		step int
		want string
	}{{1, "w4"}, {1, "w3"}, {-1, "w4"}}
	for _, tc := range tests {
		mustOK(t, h.e.CycleSubtab(tc.step))
		if got := h.view().Display.Panes[0].Active; got != tc.want {
			t.Errorf("CycleSubtab(%d): %s, want %s", tc.step, got, tc.want)
		}
	}
	code := h.windowID(vscodeID, "index.html — bench-ui")
	z := h.p.ZOrder()
	if z[0] != code && slices.Index(z, code) > slices.Index(z, h.windowID(zedID, "bench — switch.go")) {
		t.Errorf("VS Code should be in front after cycling to it")
	}
}

func TestSetSizesRearranges(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SetSizes("s13", []float64{50, 50}))
	v := h.view()
	if !slices.Equal(v.Tabs[1].Root.Sizes, []float64{50, 50}) || v.Tabs[1].Preset != "main-side" {
		t.Errorf("sizes %v preset %s", v.Tabs[1].Root.Sizes, v.Tabs[1].Preset)
	}
	zed := h.windowID(zedID, "bench — switch.go")
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed should follow the divider")
	}
	mustErr(t, h.e.SetSizes("s99", []float64{50, 50}), "There's no such split.")
	mustErr(t, h.e.SetSizes("s13", []float64{100}), "Those sizes don't fit that split.")
}

func TestShowItemAndFocusPane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustErr(t, h.e.ShowItem("p12", "w3"), "That window isn't in that pane.")
	mustErr(t, h.e.ShowItem("p11", "w99"), "That window has closed.")
	mustOK(t, h.e.FocusPane("p12"))
	if got := h.view().Display.Focus; got != "p12" {
		t.Errorf("focus: %s", got)
	}
	mustErr(t, h.e.FocusPane("p99"), "There's no such pane.")
	// ShowItem works for another tab's item too, without switching.
	mustOK(t, h.e.ShowItem("p14", "w18"))
	if v := h.view(); v.Current != 1 || layout.Leaves(v.Tabs[2].Root)[0].Active != "w18" {
		t.Errorf("ShowItem in another tab: current %d", v.Current)
	}
}

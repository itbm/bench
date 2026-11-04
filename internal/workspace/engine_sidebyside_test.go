package workspace

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/layout"
)

// TestSizeClassFollowsBenchsWidth checks that Auto chooses the size class by
// the width of Bench's window, as the user resizes or tiles it, not by the
// width of the screen, and that a compact window stays compact until it's
// 40 pt wider than compact_below, so that dragging its edge near it doesn't
// change the layout to and fro. A new compact_below chooses afresh.
func TestSizeClassFollowsBenchsWidth(t *testing.T) {
	h := newHarness(t, setup{})
	steps := []struct {
		name  string
		width float64
		want  string
	}{
		{"filling the monitor", 2560, "wide"},
		{"tiled into half the monitor", 1280, "compact"},
		{"at compact_below", 1800, "compact"},
		{"just short of 40 pt wider", 1839.5, "compact"},
		{"40 pt wider", 1840, "wide"},
		{"back at compact_below", 1800, "wide"},
		{"just narrower", 1799, "compact"},
		{"a little wider again", 1820, "compact"},
	}
	for _, s := range steps {
		h.moveBench(layout.Rect{Y: 25, W: s.width, H: 1415})
		v := h.view()
		if v.SizeClass != s.want || v.WindowWidth != s.width || v.ScreenWidth != 2560 {
			t.Errorf("%s: %s for a window %v wide on a screen %v wide, want %s for a window %v wide on the monitor",
				s.name, v.SizeClass, v.WindowWidth, v.ScreenWidth, s.want, s.width)
		}
	}
	if got := h.view().SizeReason; !strings.HasSuffix(got, ", chosen for a window 1,820 pt wide") {
		t.Errorf("the reason is %q, want it to name the window's width", got)
	}

	// At 1,820 pt, a compact_below of 1,810 is wide, although the window
	// was compact.
	s := h.view().Settings
	s.CompactBelow = 1810
	mustOK(t, h.e.SaveSettings(s))
	if got := h.view().SizeClass; got != "wide" {
		t.Errorf("with compact_below 1,810, a window 1,820 pt wide is %s, want wide", got)
	}

	// Wails' web view is 1 pt wider than the window, so a window that fills
	// the screen is as wide as the screen's visible area.
	h.moveBench(layout.Rect{Y: 24, W: 2561, H: 1416})
	if got := h.view().WindowWidth; got != 2560 {
		t.Errorf("a window filling the monitor is %v wide, want 2560", got)
	}

	// A window much wider than the screen Bench knows of, as at launch
	// before the screens settle, is as wide as it is: the screen may be
	// one Bench hasn't caught up with yet.
	l := newHarness(t, setup{screen: laptop})
	l.moveBench(layout.Rect{Y: 24, W: 2561, H: 1416})
	if v := l.view(); v.WindowWidth != 2561 || v.SizeClass != "wide" {
		t.Errorf("a window 2,561 pt wide, on what Bench takes for the laptop, is %v wide and %s, want 2561 and wide", v.WindowWidth, v.SizeClass)
	}

	// Wide and Compact are the user's: the width doesn't change them.
	mustOK(t, h.e.SetMode("compact"))
	if got := h.view().SizeClass; got != "compact" {
		t.Errorf("Compact on a window 2,560 pt wide is %s", got)
	}
	mustOK(t, h.e.SetMode("wide"))
	h.moveBench(layout.Rect{Y: 25, W: 700, H: 900})
	if got := h.view().SizeClass; got != "wide" {
		t.Errorf("Wide on a window 700 pt wide is %s", got)
	}
	// Auto again chooses for the window as it is.
	mustOK(t, h.e.SetMode("auto"))
	if got := h.view().SizeClass; got != "compact" {
		t.Errorf("Auto on a window 700 pt wide is %s", got)
	}
}

// TestShowSideBySide checks Show side by side in the "tabs" view: it
// changes to the wide layout, which shows the tab's panes, with the window
// shown in its pane and focused, and says how to go back, with Undo, which
// puts back the layout's mode and the arrangement. A tab of two or more
// panes keeps its tree, so that Auto gives back the view from before.
func TestShowSideBySide(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(2))
	before := h.view().Tabs[2].Root
	obsidian := layout.Leaves(before)[1]
	if !slices.Equal(obsidian.Items, []string{"w7"}) {
		t.Fatalf("Research's second pane holds %v, want Obsidian alone", obsidian.Items)
	}
	if d := h.view().Display; !d.Compact || len(d.Panes) != 1 {
		t.Fatalf("Research on the laptop: compact %v, %d panes; want the \"tabs\" view", d.Compact, len(d.Panes))
	}

	// Obsidian has a pane of its own, which the wide layout shows.
	mustOK(t, h.e.ShowSideBySide("w7"))
	v := h.view()
	if v.Mode != "wide" || v.SizeClass != "wide" || v.Display.Compact || len(v.Display.Panes) != 2 || v.Display.Focus != obsidian.ID {
		t.Errorf("after Show side by side: mode %s, class %s, compact %v, %d panes, focus %s; want wide, with Obsidian's pane focused",
			v.Mode, v.SizeClass, v.Display.Compact, len(v.Display.Panes), v.Display.Focus)
	}
	if !reflect.DeepEqual(v.Tabs[2].Root, before) {
		t.Errorf("the tab's tree changed for a window with a pane of its own: %+v", v.Tabs[2].Root)
	}
	if got, want := h.em.lastToast(), "Changed to the wide layout, to show Obsidian — Daily note side by side. To go back, choose Auto at the right of the title bar."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	if h.frame(h.windowID(obsidianID, "Daily note")) != h.bodyOf(obsidian.ID) {
		t.Error("Obsidian isn't docked in its pane")
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if v := h.view(); v.Mode != "auto" || !v.Display.Compact {
		t.Errorf("after Undo: mode %s, compact %v; want Auto, in the \"tabs\" view", v.Mode, v.Display.Compact)
	}
	if got, want := h.em.lastToast(), "Back to Auto, which chooses the layout by the window's width."; got != want {
		t.Errorf("Undo's toast %q, want %q", got, want)
	}

	// A Brave window shares its pane: the wide layout shows it there,
	// beside Obsidian's pane, and the tree stays as it was, so that
	// choosing Auto again gives back the view from before.
	brave := layout.Leaves(before)[0]
	mustOK(t, h.e.ShowSideBySide("w17"))
	v = h.view()
	if len(v.Display.Panes) != 2 || v.Display.Focus != brave.ID || v.Display.Panes[0].Active != "w17" {
		t.Errorf("after Show side by side for a window that shares its pane: %d panes, focus %s, Brave's pane shows %s; want 2, with w17 shown and focused",
			len(v.Display.Panes), v.Display.Focus, v.Display.Panes[0].Active)
	}
	if !reflect.DeepEqual(layout.Leaves(v.Tabs[2].Root)[0].Items, brave.Items) || len(layout.Leaves(v.Tabs[2].Root)) != 2 {
		t.Errorf("the tab's tree changed for a tab of two panes: %+v", v.Tabs[2].Root)
	}
	if got := h.em.lastToast(); !strings.HasPrefix(got, "Changed to the wide layout, to show Brave — ") || !strings.HasSuffix(got, " side by side. To go back, choose Auto at the right of the title bar.") {
		t.Errorf("toast %q", got)
	}
	mustOK(t, h.e.SetMode("auto"))
	if v := h.view(); !v.Display.Compact || len(layout.Leaves(v.Tabs[2].Root)) != 2 {
		t.Errorf("after choosing Auto: compact %v, tree %+v; want the \"tabs\" view, and two panes", v.Display.Compact, v.Tabs[2].Root)
	}

	// The move keys' → does the same, for the window shown.
	mustOK(t, h.e.ShowItem(layout.CompactPaneID, "w18"))
	mustOK(t, h.e.MoveDirection("right"))
	if v := h.view(); v.Mode != "wide" || v.Display.Focus != brave.ID || v.Display.Panes[0].Active != "w18" {
		t.Errorf("after → in the \"tabs\" view: mode %s, focus %s, Brave's pane shows %s; want Wide, with w18 shown and focused",
			v.Mode, v.Display.Focus, v.Display.Panes[0].Active)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if v := h.view(); v.Mode != "auto" || !v.Display.Compact {
		t.Errorf("after Undo: mode %s, compact %v; want Auto, in the \"tabs\" view", v.Mode, v.Display.Compact)
	}

	// With Compact chosen by hand, the toast says to choose Compact, and
	// Undo goes back to it.
	mustOK(t, h.e.SetMode("compact"))
	mustOK(t, h.e.ShowSideBySide("w7"))
	if got := h.em.lastToast(); !strings.HasSuffix(got, "To go back, choose Compact at the right of the title bar.") {
		t.Errorf("from Compact: toast %q", got)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if v := h.view(); v.Mode != "compact" || h.em.lastToast() != "Back to the compact layout." {
		t.Errorf("Undo from Compact: mode %s, toast %q", v.Mode, h.em.lastToast())
	}
	mustOK(t, h.e.SetMode("auto"))

	// Refusals: a window that has gone, and one in another tab.
	mustErr(t, h.e.ShowSideBySide("nope"), "That window has closed.")
	other := h.view().Tabs[1].Root
	mustErr(t, h.e.ShowSideBySide(layout.Items(other)[0]), "That window isn't in that pane.")
	if v := h.view(); v.Mode != "auto" {
		t.Errorf("a refusal changed the mode to %s", v.Mode)
	}

	// A tab of one pane shows nothing side by side in the wide layout
	// either: the window gets a pane of its own on the right, as Show side
	// by side (split right) gives it, and the toast says that Undo removes
	// it.
	mustOK(t, h.e.MoveItemToTab("w7", 1))
	one := h.view().Tabs[2].Root
	if !one.IsPane() {
		t.Fatalf("with Obsidian gone, Research is %+v, want one pane", one)
	}
	mustOK(t, h.e.ShowSideBySide("w17"))
	v = h.view()
	var shown *PaneView
	for i, p := range v.Display.Panes {
		if p.ID == v.Display.Focus {
			shown = &v.Display.Panes[i]
		}
	}
	if v.Mode != "wide" || len(v.Display.Panes) != 2 || shown == nil || !slices.Equal(shown.Items, []string{"w17"}) || shown.Active != "w17" {
		t.Errorf("after Show side by side in a tab of one pane: mode %s, %d panes, the focused pane %+v; want Wide, with a pane of its own for w17",
			v.Mode, len(v.Display.Panes), shown)
	}
	if got, want := h.em.lastToast(), "Changed to the wide layout, and gave Brave — Pull request #42 · bench a pane of its own, to show it side by side. Undo removes the pane and goes back to Auto."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if v := h.view(); v.Mode != "auto" || !v.Display.Compact || !reflect.DeepEqual(v.Tabs[2].Root, one) {
		t.Errorf("after Undo: mode %s, compact %v, tree %+v; want Auto, and the tree from before", v.Mode, v.Display.Compact, v.Tabs[2].Root)
	}

	// A tab with one window has nothing to show it beside.
	for _, id := range []string{"w17", "w18"} {
		mustOK(t, h.e.MoveItemToTab(id, 1))
	}
	mustErr(t, h.e.ShowSideBySide("w6"), errOnlyItem.Error())
	mustErr(t, h.e.MoveDirection("right"), errOnlyItem.Error())
	if v := h.view(); v.Mode != "auto" {
		t.Errorf("a refusal changed the mode to %s", v.Mode)
	}
}

// TestShowSideBySideOutsideTheTabsView checks that, where the tab shows its
// panes, Show side by side is Show side by side (split right), and leaves
// the mode as it is.
func TestShowSideBySideOutsideTheTabsView(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	panes := len(h.view().Display.Panes)
	mustOK(t, h.e.ShowSideBySide("w17"))
	v := h.view()
	if v.Mode != "auto" || len(v.Display.Panes) != panes+1 {
		t.Errorf("mode %s, %d panes; want Auto, with one pane more than %d", v.Mode, len(v.Display.Panes), panes)
	}
}

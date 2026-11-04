package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// researchPanes switches to Research and returns its panes: Brave's, with
// three windows, and Obsidian's, with one.
func researchPanes(t *testing.T, h *harness) (brave, notes string) {
	t.Helper()
	mustOK(t, h.e.SwitchTab(2))
	h.advance(time.Second)
	ids := leafIDs(h.tabNamed("Research"))
	if len(ids) != 2 {
		t.Fatalf("Research has panes %v", ids)
	}
	return ids[0], ids[1]
}

// braveWindows returns the standard world's three Brave windows.
func braveWindows(h *harness) []platform.WindowID {
	var out []platform.WindowID
	for _, title := range []string{"Wails v3 docs", "Pull request #42 · bench", "Go packages"} {
		out = append(out, h.windowID(braveID, title))
	}
	return out
}

// shownWindow returns the window of the item that a pane shows.
func (h *harness) shownWindow(paneID string) platform.WindowID {
	h.t.Helper()
	active := h.paneView(paneID).Active
	h.e.lock()
	defer h.e.unlock()
	if it := h.e.model.Items[active]; it != nil && it.WindowID != 0 {
		return it.WindowID
	}
	h.t.Fatalf("pane %s shows %q, which has no window", paneID, active)
	return 0
}

// TestZoomPaneParksTheOtherPanes checks that a zoomed pane fills the pane
// area on its own, with its header, while the other panes' windows go out
// of sight, and that they come back where they were as every pane shows
// again.
func TestZoomPaneParksTheOtherPanes(t *testing.T) {
	h := newHarness(t, setup{})
	brave, notes := researchPanes(t, h)
	note := h.windowID(obsidianID, "Daily note")
	shown := h.shownWindow(brave)
	before := h.view().Display
	braveFrame := h.frame(shown)
	h.saveSetting(func(s *SettingsView) { s.PaneHeaders = config.PaneHeadersAuto })
	if h.paneView(notes).Header != 0 {
		t.Fatal("with pane_headers auto, Obsidian's pane should have no header")
	}

	mustOK(t, h.e.ZoomPane(notes))
	h.advance(50 * time.Millisecond)
	d := h.view().Display
	if d.Zoomed != notes || d.Focus != notes || d.Root.ID != notes || len(d.Panes) != 1 || len(d.Dividers) != 0 {
		t.Fatalf("zoomed: Zoomed %q, focus %q, root %q, %d panes, %d dividers; want Obsidian's pane alone",
			d.Zoomed, d.Focus, d.Root.ID, len(d.Panes), len(d.Dividers))
	}
	p := d.Panes[0]
	gap := h.view().Settings.Gap
	area := layout.Rect{X: d.Area.X + gap, Y: d.Area.Y + gap, W: d.Area.W - 2*gap, H: d.Area.H - 2*gap}
	if p.Rect != area || p.Header != layout.HeaderHeight {
		t.Errorf("the zoomed pane at %v with header %v; want the whole pane area %v, with a header to say so", p.Rect, p.Header, area)
	}
	if got, want := h.frame(note), h.bodyOf(notes); got != want || h.e.win.IsParked(note) {
		t.Errorf("Obsidian at %v, want the zoomed pane's body %v", got, want)
	}
	for _, w := range braveWindows(h) {
		if !h.e.win.IsParked(w) {
			t.Errorf("Brave window %d isn't parked while Obsidian's pane is zoomed", w)
		}
	}
	if !h.lastMenuBar().Zoomed {
		t.Error("the View menu isn't told that a pane is zoomed")
	}
	// The tab keeps its panes, which the tab bar and status bar count.
	if tv := h.tabNamed("Research"); len(leafIDs(tv)) != 2 || !reflect.DeepEqual(tv.Root.Sizes, before.Root.Sizes) {
		t.Errorf("zooming changed the tab: %s", treeText(tv.Root))
	}

	// Zooming the zoomed pane again does nothing.
	mustOK(t, h.e.ZoomPane(notes))
	if h.view().Display.Zoomed != notes {
		t.Error("zooming the zoomed pane again ended the zoom")
	}

	mustOK(t, h.e.ShowAllPanes())
	h.advance(50 * time.Millisecond)
	d = h.view().Display
	if d.Zoomed != "" || len(d.Panes) != 2 || d.Focus != notes {
		t.Fatalf("after Show All Panes: Zoomed %q, %d panes, focus %q", d.Zoomed, len(d.Panes), d.Focus)
	}
	if h.e.win.IsParked(shown) || h.frame(shown) != braveFrame {
		t.Errorf("Brave's shown window at %v, want it back at %v", h.frame(shown), braveFrame)
	}
	if h.paneView(notes).Header != 0 {
		t.Error("Obsidian's pane keeps its header after the zoom, with pane_headers auto")
	}
	if h.lastMenuBar().Zoomed {
		t.Error("the View menu still says a pane is zoomed")
	}
	// Showing every pane while none is zoomed does nothing.
	mustOK(t, h.e.ShowAllPanes())
}

// TestZoomPaneToggles checks the zoom_pane shortcut, which zooms the
// focused pane, and shows every pane again while one is zoomed.
func TestZoomPaneToggles(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	mustOK(t, h.e.FocusPane(brave))
	h.e.Shortcut(shortcuts.ActionZoomPane, "")
	if got := h.view().Display.Zoomed; got != brave {
		t.Fatalf("zoom_pane zoomed %q, want the focused pane %s", got, brave)
	}
	h.e.Shortcut(shortcuts.ActionZoomPane, "")
	if got := h.view().Display.Zoomed; got != "" {
		t.Errorf("zoom_pane again left %q zoomed", got)
	}
	// "" is the focused pane for ZoomPane too.
	mustOK(t, h.e.ZoomPane(""))
	if got := h.view().Display.Zoomed; got != brave {
		t.Errorf("ZoomPane(\"\") zoomed %q, want %s", got, brave)
	}
}

// TestZoomEnds checks that a zoom ends as the tab changes, as another pane
// is focused, by a click or the focus_pane shortcut, and as a window moves
// out of the zoomed pane, but not as the focus_pane shortcut finds nothing
// beside it.
func TestZoomEnds(t *testing.T) {
	h := newHarness(t, setup{})
	brave, notes := researchPanes(t, h)
	note := h.windowID(obsidianID, "Daily note")
	zoomed := func(when, want string) {
		t.Helper()
		h.advance(50 * time.Millisecond)
		if got := h.view().Display.Zoomed; got != want {
			t.Errorf("%s: zoomed %q, want %q", when, got, want)
		}
	}

	mustOK(t, h.e.ZoomPane(notes))
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SwitchTab(2))
	zoomed("back on the tab", "")
	if len(h.view().Display.Panes) != 2 || h.e.win.IsParked(h.shownWindow(brave)) {
		t.Error("back on the tab, it should show every pane, with their windows")
	}

	mustOK(t, h.e.ZoomPane(notes))
	mustOK(t, h.e.FocusPane(brave))
	zoomed("another pane focused", "")
	if h.e.win.IsParked(h.shownWindow(brave)) {
		t.Error("focusing another pane should bring its window back")
	}

	mustOK(t, h.e.ZoomPane(notes))
	mustOK(t, h.e.FocusDirection("right"))
	zoomed("nothing to the right", notes)
	mustOK(t, h.e.FocusDirection("left"))
	zoomed("the pane to the left", "")
	if got := h.view().Display.Focus; got != brave {
		t.Errorf("focus_pane left from the zoomed pane focused %q, want %s", got, brave)
	}

	// A move that can't be made leaves the zoom as it is; one that can
	// shows every pane, with the window in its new place.
	mustOK(t, h.e.ZoomPane(notes))
	mustErr(t, h.e.MoveDirection("right"), errNothingBeside.Error())
	zoomed("a move refused", notes)
	mustOK(t, h.e.MoveDirection("left"))
	zoomed("a window moved out", "")
	if got, want := h.frame(note), h.bodyOf(brave); got != want {
		t.Errorf("Obsidian at %v, want Brave's pane's body %v", got, want)
	}
}

// TestZoomEndsWithAMoveOutOfSight checks that a window moved from a zoomed
// pane to one out of sight, as Move to pane does, shows every pane again,
// with the window in its new place, rather than parking it there: the move
// focuses the pane it goes to.
func TestZoomEndsWithAMoveOutOfSight(t *testing.T) {
	h := newHarness(t, setup{})
	brave, notes := researchPanes(t, h)
	mustOK(t, h.e.ZoomPane(brave))
	w := h.windowID(braveID, "Wails v3 docs")
	mustOK(t, h.e.MoveItem(h.itemOf(w), 2, notes, "center"))
	h.advance(50 * time.Millisecond)
	if got := h.view().Display.Zoomed; got != "" {
		t.Errorf("after Move to pane, %q is zoomed", got)
	}
	if h.e.win.IsParked(w) || h.frame(w) != h.bodyOf(notes) {
		t.Errorf("the window moved is at %v, want the notes pane's body %v", h.frame(w), h.bodyOf(notes))
	}

	// A move along the zoomed pane's own sub-tabs keeps the zoom.
	mustOK(t, h.e.ZoomPane(brave))
	pr := h.itemOf(h.windowID(braveID, "Pull request #42 · bench"))
	mustOK(t, h.e.MoveItemAt(pr, 2, brave, 0))
	h.advance(50 * time.Millisecond)
	if got := h.view().Display.Zoomed; got != brave {
		t.Errorf("after a move along the zoomed pane, zoomed %q, want %s", got, brave)
	}
	// A move to the pane out of sight, along its sub-tabs, ends it.
	mustOK(t, h.e.MoveItemAt(pr, 2, notes, 0))
	h.advance(50 * time.Millisecond)
	if got := h.view().Display.Zoomed; got != "" {
		t.Errorf("after a move to the other pane's sub-tabs, %q is zoomed", got)
	}
}

// TestZoomRefused checks that a tab with only one pane, and the "tabs"
// view, which shows one pane anyway, can't zoom one.
func TestZoomRefused(t *testing.T) {
	h := newHarness(t, setup{config: `
[[tab]]
name = "Notes"
layout = "single"
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]

[[tab]]
name = "Research"
layout = "side"
small_screens = "tabs"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
`})
	mustErr(t, h.e.ZoomPane(""), errZoomOnePane.Error())
	h.e.Shortcut(shortcuts.ActionZoomPane, "")
	if got := h.em.lastToast(); got != errZoomOnePane.Error() {
		t.Errorf("zoom_pane on one pane said %q", got)
	}
	mustOK(t, h.e.SwitchTab(1))
	panes := leafIDs(h.tabNamed("Research"))
	mustErr(t, h.e.ZoomPane("nope"), errNoPane.Error())
	mustOK(t, h.e.ZoomPane(panes[1]))
	// Compact shows Research in the "tabs" view: the zoom ends.
	mustOK(t, h.e.SetMode("compact"))
	h.advance(50 * time.Millisecond)
	if d := h.view().Display; !d.Compact || d.Zoomed != "" {
		t.Fatalf("compact: Compact %v, Zoomed %q", d.Compact, d.Zoomed)
	}
	mustErr(t, h.e.ZoomPane(panes[0]), errZoomTabsView.Error())
	mustOK(t, h.e.SetMode("wide"))
	h.advance(50 * time.Millisecond)
	if got := h.view().Display.Zoomed; got != "" {
		t.Errorf("a zoom came back with the wide layout: %q", got)
	}
}

// TestZoomIsNotSaved checks the decision that a zoom is for a moment:
// state.json doesn't keep it, and Bench starts with every pane shown.
func TestZoomIsNotSaved(t *testing.T) {
	first := newHarness(t, setup{})
	_, notes := researchPanes(t, first)
	mustOK(t, first.e.ZoomPane(notes))
	first.settle()
	mustOK(t, first.e.Close())
	data, err := os.ReadFile(filepath.Join(first.dir, config.StateFile))
	mustOK(t, err)
	if strings.Contains(strings.ToLower(string(data)), "zoom") {
		t.Errorf("state.json keeps the zoom:\n%s", data)
	}

	h := newHarness(t, setup{state: writeState(data)})
	v := h.view()
	if v.Current != 2 || v.Display.Zoomed != "" || len(v.Display.Panes) != 2 {
		t.Errorf("after restart: tab %d, Zoomed %q, %d panes; want Research with every pane", v.Current, v.Display.Zoomed, len(v.Display.Panes))
	}
}

// TestEvenSizes checks that double-clicking a divider gives its split's
// panes equal sizes, with Undo, and that a split already even is left as
// it is.
func TestEvenSizes(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	shown := h.shownWindow(brave)
	root := h.tabNamed("Research").Root
	if !slices.Equal(root.Sizes, []float64{60, 40}) {
		t.Fatalf("Research's sizes are %v", root.Sizes)
	}
	before := h.frame(shown)

	mustOK(t, h.e.EvenSizes(root.ID))
	h.drain()
	if got := h.tabNamed("Research").Root.Sizes; !slices.Equal(got, []float64{50, 50}) {
		t.Errorf("sizes %v, want 50, 50", got)
	}
	if got, want := h.frame(shown), h.bodyOf(brave); got != want || got == before {
		t.Errorf("Brave at %v, want the narrower pane's body %v", got, want)
	}
	if got := h.em.lastToast(); got != "Made the panes the same size." {
		t.Errorf("toast %q", got)
	}
	undoLast(t, h)
	if got := h.tabNamed("Research").Root.Sizes; !slices.Equal(got, []float64{60, 40}) {
		t.Errorf("after Undo, sizes %v, want 60, 40", got)
	}
	if got := h.frame(shown); got != before {
		t.Errorf("after Undo, Brave at %v, want %v", got, before)
	}

	mustOK(t, h.e.SetSizes(root.ID, []float64{50.01, 49.99}))
	toasts := len(h.em.toasts)
	mustOK(t, h.e.EvenSizes(root.ID))
	if len(h.em.toasts) != toasts {
		t.Errorf("sizes that look even already were made even, with toast %q", h.em.lastToast())
	}
	mustErr(t, h.e.EvenSizes("nope"), errNoSplit.Error())
	mustErr(t, h.e.EvenSizes(brave), errNoSplit.Error())
}

// TestSplitPaneAndCancel checks Split right and Split down with an empty
// pane: the empty pane, focused, for the add dialog to fill, and the tab
// just as it was if the dialog closes without adding anything.
func TestSplitPaneAndCancel(t *testing.T) {
	h := newHarness(t, setup{})
	_, notes := researchPanes(t, h)
	note := h.windowID(obsidianID, "Daily note")
	tree := layout.Clone(h.tabNamed("Research").Root)
	focus := h.view().Display.Focus

	id, err := h.e.SplitPane(notes, "right")
	mustOK(t, err)
	h.drain()
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got[1:], []string{notes, id}) {
		t.Fatalf("panes %v, want Obsidian's, then the new one", got)
	}
	if d := h.view().Display; d.Focus != id || len(h.paneView(id).Items) != 0 {
		t.Errorf("the new pane %s: focus %q, items %v; want it focused and empty", id, d.Focus, h.paneView(id).Items)
	}
	if got, want := h.frame(note), h.bodyOf(notes); got != want {
		t.Errorf("Obsidian at %v, want its narrower pane's body %v", got, want)
	}
	mustOK(t, h.e.CancelSplit(id))
	h.drain()
	sameTree(t, h, "Research", tree)
	if got := h.view().Display.Focus; got != focus {
		t.Errorf("focus %q after the add dialog closed, want %q", got, focus)
	}
	// Only the pane that SplitPane made goes, and only once.
	mustOK(t, h.e.CancelSplit(notes))
	sameTree(t, h, "Research", tree)

	// Split down, then something else changes the tab: the empty pane
	// closes up on its own, and the change stays.
	id, err = h.e.SplitPane(notes, "bottom")
	mustOK(t, err)
	root := h.tabNamed("Research").Root
	mustOK(t, h.e.SetSizes(root.ID, []float64{70, 30}))
	mustOK(t, h.e.CancelSplit(id))
	tv := h.tabNamed("Research")
	if got := leafIDs(tv); len(got) != 2 || slices.Contains(got, id) {
		t.Errorf("panes %v, want the empty pane gone", got)
	}
	if !slices.Equal(tv.Root.Sizes, []float64{70, 30}) {
		t.Errorf("sizes %v, want the 70, 30 set since", tv.Root.Sizes)
	}

	// A pane that something was added to stays.
	id, err = h.e.SplitPane(notes, "right")
	mustOK(t, err)
	mustOK(t, h.e.AddToPane(id, Source{Type: sourceTerminal}))
	mustOK(t, h.e.CancelSplit(id))
	if got := leafIDs(h.tabNamed("Research")); !slices.Contains(got, id) {
		t.Errorf("the pane with a terminal in it went: %v", got)
	}

	// So does one that a new window is on its way to.
	id, err = h.e.SplitPane(notes, "bottom")
	mustOK(t, err)
	mustOK(t, h.e.AddToPane(id, Source{Type: sourceNewWindow, BundleID: braveID}))
	mustOK(t, h.e.CancelSplit(id))
	if got := leafIDs(h.tabNamed("Research")); !slices.Contains(got, id) {
		t.Errorf("the pane awaiting a new Brave window went: %v", got)
	}

	mustErr(t, func() error { _, err := h.e.SplitPane(notes, "center"); return err }(), errBadZone.Error())
	mustErr(t, func() error { _, err := h.e.SplitPane("nope", "right"); return err }(), errNoPane.Error())
	mustOK(t, h.e.SetMode("compact"))
	mustErr(t, func() error { _, err := h.e.SplitPane(notes, "right"); return err }(), errSplitsHidden.Error())
}

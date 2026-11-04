package workspace

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/shortcuts"
)

// rootSizes returns a tab's top-level sizes, rounded to 0.01.
func rootSizes(h *harness, name string) []float64 {
	var out []float64
	for _, s := range h.tabNamed(name).Root.Sizes {
		out = append(out, float64(int(s*100+0.5))/100)
	}
	return out
}

// wantToast fails the test if the latest toast isn't want.
func wantToast(t *testing.T, h *harness, want string) {
	t.Helper()
	if got := h.em.lastToast(); got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
}

// TestGrowAndShrinkPane checks that Grow Pane and Shrink Pane change the
// focused pane's size by 5% of its split, move its windows, and say how the
// split's panes share it; that a run of them shares one undo step; and that
// Undo puts the sizes back as they were before the first.
func TestGrowAndShrinkPane(t *testing.T) {
	h := newHarness(t, setup{})
	brave, notes := researchPanes(t, h)
	mustOK(t, h.e.FocusPane(brave))
	shown := h.shownWindow(brave)
	before := h.frame(shown)

	mustOK(t, h.e.GrowPane())
	h.drain()
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{65, 35}) {
		t.Errorf("after Grow Pane, sizes %v, want 65, 35", got)
	}
	wantToast(t, h, "Brave 65% · Obsidian 35%")
	step := h.em.lastUndo(t)

	mustOK(t, h.e.GrowPane())
	h.drain()
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{70, 30}) {
		t.Errorf("after Grow Pane twice, sizes %v, want 70, 30", got)
	}
	wantToast(t, h, "Brave 70% · Obsidian 30%")
	if got := h.em.lastUndo(t); got != step {
		t.Errorf("the second Grow Pane has undo step %d, want the first's, %d", got, step)
	}
	if got, want := h.frame(shown), h.bodyOf(brave); got != want || got == before {
		t.Errorf("Brave at %v, want the wider pane's body %v", got, want)
	}

	// The other pane, focused, shrinks: still the same run.
	mustOK(t, h.e.FocusPane(notes))
	mustOK(t, h.e.ShrinkPane())
	h.drain()
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{75, 25}) {
		t.Errorf("after Shrink Pane on Obsidian, sizes %v, want 75, 25", got)
	}
	wantToast(t, h, "Brave 75% · Obsidian 25%")
	if got := h.em.lastUndo(t); got != step {
		t.Errorf("Shrink Pane has undo step %d, want the run's, %d", got, step)
	}

	undoLast(t, h)
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{60, 40}) {
		t.Errorf("after Undo, sizes %v, want 60, 40", got)
	}
	wantToast(t, h, "Put back the sizes of the panes.")
	if got := h.frame(shown); got != before {
		t.Errorf("after Undo, Brave at %v, want %v", got, before)
	}

	// After the Undo, which focuses Brave's pane again, as it was, a new
	// change starts a new step.
	mustOK(t, h.e.ShrinkPane())
	if got := h.em.lastUndo(t); got == step {
		t.Errorf("Shrink Pane after Undo shares the undone step %d", step)
	}
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{55, 45}) {
		t.Errorf("after Shrink Pane on Brave, sizes %v, want 55, 45", got)
	}
}

// TestArrangeRunNames checks how Undo away from its toast names a run of
// these keys (decision 65): by the command, while every change in the run
// has the same name, and otherwise by what they did together, as Undo takes
// them all back, with "Undid:" after UndoLatest.
func TestArrangeRunNames(t *testing.T) {
	h := newHarness(t, setup{})
	brave, notes := researchPanes(t, h)
	mustOK(t, h.e.FocusPane(brave))
	wantUndo := func(name, did string) {
		t.Helper()
		v := h.view()
		if v.Undo == nil || v.Undo.Name != name || v.Undo.Did != did {
			t.Errorf("Undo = %+v, want %q, %q", v.Undo, name, did)
		}
		if m := h.menuBarNow(); m.Undo != name {
			t.Errorf("the menu bar's Undo names %q, want %q", m.Undo, name)
		}
	}

	mustOK(t, h.e.GrowPane())
	wantUndo("Grow Pane", "made Brave larger")
	mustOK(t, h.e.GrowPane())
	wantUndo("Grow Pane", "made Brave larger")
	// Another pane, in the same run, changes what the run did.
	mustOK(t, h.e.FocusPane(notes))
	mustOK(t, h.e.GrowPane())
	wantUndo("Resize Panes", "resized the panes")
	mustOK(t, h.e.UndoLatest())
	h.drain()
	wantToast(t, h, "Undid: resized the panes.")
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{60, 40}) {
		t.Errorf("after UndoLatest, sizes %v, want 60, 40", got)
	}

	// Undo focuses Brave's pane again, as it was.
	mustOK(t, h.e.ShrinkPane())
	wantUndo("Shrink Pane", "made Brave smaller")
	h.advance(arrangeRunFor + time.Second)
	mustOK(t, h.e.EqualPanes())
	wantUndo("Make Panes Equal", "made the panes the same size")

	h.advance(arrangeRunFor + time.Second)
	mustOK(t, h.e.StepLayout(1))
	wantUndo("Next Layout", "changed the layout of Research")
	mustOK(t, h.e.StepLayout(1))
	wantUndo("Next Layout", "changed the layout of Research")
	mustOK(t, h.e.StepLayout(-1))
	wantUndo("Change Layout", "changed the layout of Research")
	// Sizes after a new layout, in the same run: still the layout's.
	mustOK(t, h.e.GrowPane())
	wantUndo("Change Layout", "changed the layout of Research")
	mustOK(t, h.e.UndoLatest())
	h.drain()
	wantToast(t, h, "Undid: changed the layout of Research.")
	wantUndo("Make Panes Equal", "made the panes the same size")
}

// TestArrangeRunEnds checks that a change by these keys starts a new undo
// step after the toast's time, after another change, and after a change of
// the tree by other means, such as a dragged divider.
func TestArrangeRunEnds(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	mustOK(t, h.e.FocusPane(brave))
	root := h.tabNamed("Research").Root.ID

	mustOK(t, h.e.GrowPane())
	first := h.em.lastUndo(t)
	h.advance(arrangeRunFor + time.Second)
	mustOK(t, h.e.GrowPane())
	second := h.em.lastUndo(t)
	if second == first {
		t.Errorf("a Grow Pane %v later shares the first's undo step", arrangeRunFor+time.Second)
	}

	mustOK(t, h.e.SetSizes(root, []float64{55, 45}))
	mustOK(t, h.e.GrowPane())
	third := h.em.lastUndo(t)
	if third == second {
		t.Error("a Grow Pane after a dragged divider shares the step before it")
	}
	wantToast(t, h, "Brave 60% · Obsidian 40%")

	mustOK(t, h.e.EvenSizes(root))
	mustOK(t, h.e.GrowPane())
	if got := h.em.lastUndo(t); got == third {
		t.Error("a Grow Pane after another change shares the step before it")
	}

	// Undo of the last takes back only the last.
	undoLast(t, h)
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{50, 50}) {
		t.Errorf("after Undo, sizes %v, want 50, 50", got)
	}
}

// TestPaneLimits checks that no pane goes below layout.MinSize, and that a
// pane as large or as small as it can be says so, by the way its split is
// drawn: as a row, as a column, or as the stack view's column.
func TestPaneLimits(t *testing.T) {
	h := newHarness(t, setup{})
	brave, notes := researchPanes(t, h)
	root := h.tabNamed("Research").Root.ID
	mustOK(t, h.e.FocusPane(brave))
	mustOK(t, h.e.SetSizes(root, []float64{88, 12}))
	mustOK(t, h.e.GrowPane())
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{90, 10}) {
		t.Errorf("Grow Pane at 88%%: sizes %v, want 90, 10", got)
	}
	wantToast(t, h, "Brave 90% · Obsidian 10%")
	mustErr(t, h.e.GrowPane(), "This pane is as wide as it can be.")
	mustOK(t, h.e.FocusPane(notes))
	mustErr(t, h.e.ShrinkPane(), "This pane is as narrow as it can be.")
	if a := h.view().Display.Arrange; a.Grow != true || a.Shrink != false {
		t.Errorf("at 10%%, Arrange is %+v, want Grow and not Shrink", a)
	}

	// Top and bottom is a column.
	mustOK(t, h.e.StepLayout(-1)) // Side by side, then Grid of four...
	mustOK(t, h.e.StepLayout(-1)) // ...then Top and bottom.
	tv := h.tabNamed("Research")
	if tv.Preset != layout.PresetTopBottom {
		t.Fatalf("Research's layout is %s, want top-bottom", tv.Preset)
	}
	top := leafIDs(tv)[0]
	mustOK(t, h.e.FocusPane(top))
	mustOK(t, h.e.SetSizes(tv.Root.ID, []float64{90, 10}))
	mustErr(t, h.e.GrowPane(), "This pane is as tall as it can be.")
	mustOK(t, h.e.FocusPane(leafIDs(tv)[1]))
	mustErr(t, h.e.ShrinkPane(), "This pane is as short as it can be.")

	// The stack view draws Code's row as a column.
	s := newHarness(t, setup{screen: laptop})
	mustOK(t, s.e.SwitchTab(1))
	s.advance(time.Second)
	code := s.tabNamed("Code")
	mustOK(t, s.e.FocusPane(leafIDs(code)[0]))
	mustOK(t, s.e.SetSizes(code.Root.ID, []float64{90, 10}))
	mustErr(t, s.e.GrowPane(), "This pane is as tall as it can be.")
	mustOK(t, s.e.ShrinkPane())
	if got := rootSizes(s, "Code"); !slices.Equal(got, []float64{85, 15}) {
		t.Errorf("Shrink Pane in the stack view: sizes %v, want 85, 15", got)
	}
}

// TestResizeNested checks that Grow Pane changes the split that holds the
// focused pane, not the tab's top-level one, and names a split's panes in
// the toast.
func TestResizeNested(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	mustOK(t, h.e.StepLayout(1)) // Three columns: the panes, then an empty one.
	mustOK(t, h.e.StepLayout(1)) // Top and bottom.
	mustOK(t, h.e.StepLayout(1)) // Grid of four: two rows in a column.
	tv := h.tabNamed("Code")
	if tv.Preset != layout.PresetGrid {
		t.Fatalf("Code's layout is %s, want grid", tv.Preset)
	}
	panes := leafIDs(tv)
	mustOK(t, h.e.FocusPane(panes[3]))
	mustOK(t, h.e.GrowPane())
	h.drain()
	tv = h.tabNamed("Code")
	if got := tv.Root.Sizes; !slices.Equal(got, []float64{50, 50}) {
		t.Errorf("the column's sizes are %v, want 50, 50 still", got)
	}
	if got := tv.Root.Children[1].Sizes; !slices.Equal(got, []float64{45, 55}) {
		t.Errorf("the bottom row's sizes are %v, want 45, 55", got)
	}
	wantToast(t, h, "Empty pane 45% · Empty pane 55%")

	// A split's panes are named together.
	h.e.lock()
	text := h.e.sizesTextLocked(h.e.model.CurrentTab().Root)
	h.e.unlock()
	if text != "Zed and Terminal 50% · Empty pane and Empty pane 50%" {
		t.Errorf("the column's text is %q", text)
	}
}

// TestResizeRefused checks that Grow Pane and Shrink Pane are refused while
// the pane is zoomed and on a tab with one pane, and that the keys say why.
func TestResizeRefused(t *testing.T) {
	h := newHarness(t, setup{config: `
[[tab]]
name = "Notes"
layout = "single"
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]

[[tab]]
name = "Research"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
`})
	mustErr(t, h.e.GrowPane(), errResizeOnePane.Error())
	mustErr(t, h.e.ShrinkPane(), errResizeOnePane.Error())
	mustErr(t, h.e.EqualPanes(), errResizeOnePane.Error())
	h.e.Shortcut(shortcuts.ActionGrowPane, "")
	wantToast(t, h, errResizeOnePane.Error())
	if a := h.view().Display.Arrange; a != (ArrangeView{NextLayout: "Side by side", PreviousLayout: "Grid of four"}) {
		t.Errorf("one pane: Arrange is %+v", a)
	}

	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	panes := leafIDs(h.tabNamed("Research"))
	mustOK(t, h.e.ZoomPane(panes[0]))
	mustErr(t, h.e.GrowPane(), errResizeZoomed.Error())
	mustErr(t, h.e.ShrinkPane(), errResizeZoomed.Error())
	h.e.Shortcut(shortcuts.ActionShrinkPane, "")
	wantToast(t, h, errResizeZoomed.Error())
	if a := h.view().Display.Arrange; a.Grow || a.Shrink || a.Equal || a.NextLayout != "Main and side" {
		t.Errorf("zoomed, with even sizes: Arrange is %+v, want only the layouts", a)
	}
	if got := h.view().Display.Zoomed; got != panes[0] {
		t.Errorf("a refusal ended the zoom: zoomed %q", got)
	}
	mustErr(t, h.e.EqualPanes(), errAlreadyEqual.Error())
}

// TestArrangeRefusedInTheTabsView checks that the "tabs" view, which shows
// one pane at a time, refuses all five, and that the palette and the menus
// get nothing to offer.
func TestArrangeRefusedInTheTabsView(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	if d := h.view().Display; !d.Compact || h.view().Current != 0 {
		t.Fatalf("on the laptop, Compact %v, tab %d; want Comms in the tabs view", d.Compact, h.view().Current)
	}
	before := h.tabNamed("Comms").Root
	for name, f := range map[string]func() error{
		"GrowPane": h.e.GrowPane, "ShrinkPane": h.e.ShrinkPane, "EqualPanes": h.e.EqualPanes,
		"StepLayout(1)": func() error { return h.e.StepLayout(1) }, "StepLayout(-1)": func() error { return h.e.StepLayout(-1) },
	} {
		if err := f(); err == nil || err.Error() != errArrangeTabsView.Error() {
			t.Errorf("%s in the tabs view: %v", name, err)
		}
	}
	for _, a := range []string{shortcuts.ActionGrowPane, shortcuts.ActionShrinkPane, shortcuts.ActionEqualPanes, shortcuts.ActionNextLayout, shortcuts.ActionPreviousLayout} {
		h.e.Shortcut(a, "")
		wantToast(t, h, errArrangeTabsView.Error())
	}
	if a := h.view().Display.Arrange; a != (ArrangeView{}) {
		t.Errorf("tabs view: Arrange is %+v, want nothing", a)
	}
	if got := h.tabNamed("Comms").Root; !reflect.DeepEqual(got, before) {
		t.Errorf("the refusals changed Comms's tree to %s", treeText(got))
	}
}

// TestEqualPanes checks that Make Panes Equal gives every split of the tab
// equal sizes, shows every pane of a zoomed tab, and shares a run's undo
// step with a change of layout before it.
func TestEqualPanes(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	shown := h.shownWindow(brave)
	before := h.frame(shown)
	mustOK(t, h.e.ZoomPane(brave))
	mustOK(t, h.e.EqualPanes())
	h.drain()
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{50, 50}) {
		t.Errorf("sizes %v, want 50, 50", got)
	}
	if got := h.view().Display.Zoomed; got != "" {
		t.Errorf("Make Panes Equal left %q zoomed", got)
	}
	if got, want := h.frame(shown), h.bodyOf(brave); got != want {
		t.Errorf("Brave at %v, want its pane's body %v", got, want)
	}
	wantToast(t, h, "Made the panes the same size.")
	mustErr(t, h.e.EqualPanes(), errAlreadyEqual.Error())
	undoLast(t, h)
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{60, 40}) {
		t.Errorf("after Undo, sizes %v, want 60, 40", got)
	}
	if got := h.frame(shown); got != before {
		t.Errorf("after Undo, Brave at %v, want %v", got, before)
	}

	// Code, in Three columns, then equal: one step, which puts back the
	// layout.
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	codeBefore := layout.Clone(h.tabNamed("Code").Root)
	mustOK(t, h.e.StepLayout(1))
	step := h.em.lastUndo(t)
	if got := rootSizes(h, "Code"); !slices.Equal(got, []float64{40, 30, 30}) {
		t.Errorf("Three columns' sizes %v", got)
	}
	mustOK(t, h.e.EqualPanes())
	if got := rootSizes(h, "Code"); !slices.Equal(got, []float64{33.33, 33.33, 33.33}) {
		t.Errorf("after Make Panes Equal, sizes %v", got)
	}
	if got := h.em.lastUndo(t); got != step {
		t.Errorf("Make Panes Equal after Next Layout has step %d, want %d", got, step)
	}
	undoLast(t, h)
	wantToast(t, h, "Put back the layout.")
	sameTree(t, h, "Code", codeBefore)
}

// TestStepLayout checks that Next Layout gives the tab the six layouts in
// turn, keeping its windows in order, leaving out the empty pane that a
// layout with more panes made and skipping a layout with too few, that
// Previous Layout goes the other way, and that Undo puts the tab back.
func TestStepLayout(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	tv := h.tabNamed("Code")
	before := layout.Clone(tv.Root)
	items := paneItemsOf(tv)
	if tv.Preset != layout.PresetMainSide || len(items) != 2 {
		t.Fatalf("Code is %s with panes %v", tv.Preset, items)
	}
	zed := h.shownWindow(leafIDs(tv)[0])
	if a := h.view().Display.Arrange; a.NextLayout != "Three columns" || a.PreviousLayout != "Side by side" {
		t.Errorf("Arrange is %+v", a)
	}

	var step int
	for i, want := range []struct {
		preset, label string
		panes         int
	}{
		{layout.PresetThree, "Three columns", 3},
		{layout.PresetTopBottom, "Top and bottom", 2},
		{layout.PresetGrid, "Grid of four", 4},
		{layout.PresetSide, "Side by side", 2}, // Not Single: it has one pane.
		{layout.PresetMainSide, "Main and side", 2},
	} {
		mustOK(t, h.e.StepLayout(1))
		h.drain()
		tv := h.tabNamed("Code")
		if tv.Preset != want.preset || len(leafIDs(tv)) != want.panes {
			t.Fatalf("Next Layout %d: %s with %d panes, want %s with %d", i+1, tv.Preset, len(leafIDs(tv)), want.preset, want.panes)
		}
		if got := paneItemsOf(tv); !reflect.DeepEqual(got[:2], items) || slices.ContainsFunc(got[2:], func(p []string) bool { return len(p) > 0 }) {
			t.Errorf("%s: panes %v, want %v first, then empty ones", want.label, got, items)
		}
		wantToast(t, h, "Layout: "+want.label)
		if i == 0 {
			step = h.em.lastUndo(t)
		} else if got := h.em.lastUndo(t); got != step {
			t.Errorf("%s has undo step %d, want the run's, %d", want.label, got, step)
		}
		if got, body := h.frame(zed), h.bodyOf(leafIDs(tv)[0]); got != body {
			t.Errorf("%s: Zed at %v, want the first pane's body %v", want.label, got, body)
		}
	}
	if got := rootSizes(h, "Code"); !slices.Equal(got, []float64{65, 35}) {
		t.Errorf("Main and side again has sizes %v, its own are 65, 35", got)
	}

	mustOK(t, h.e.StepLayout(-1))
	if got := h.tabNamed("Code").Preset; got != layout.PresetSide {
		t.Errorf("Previous Layout from Main and side gave %s, want side", got)
	}
	h.e.Shortcut(shortcuts.ActionPreviousLayout, "")
	if got := h.tabNamed("Code").Preset; got != layout.PresetGrid {
		t.Errorf("previous_layout from Side by side gave %s, want grid (not single)", got)
	}
	h.e.Shortcut(shortcuts.ActionNextLayout, "")
	wantToast(t, h, "Layout: Side by side")
	undoLast(t, h)
	wantToast(t, h, "Put back the layout.")
	sameTree(t, h, "Code", before)
	if got := h.tabNamed("Code").Preset; got != layout.PresetMainSide {
		t.Errorf("after Undo, Code is %s", got)
	}
	if got, want := h.frame(zed), h.bodyOf(leafIDs(h.tabNamed("Code"))[0]); got != want {
		t.Errorf("after Undo, Zed at %v, want %v", got, want)
	}
}

// paneItemsOf returns a tab's panes' items, in reading order.
func paneItemsOf(tv TabView) [][]string {
	var out [][]string
	for _, p := range layout.Leaves(tv.Root) {
		out = append(out, slices.Clone(p.Items))
	}
	return out
}

// TestStepLayoutEnds checks that a zoomed pane shows every pane again with
// the new layout, that a tab whose panes only one layout, or none, has
// room for is refused, and that a tree with no layout's shape goes to the
// first layout, or the last.
func TestStepLayoutEnds(t *testing.T) {
	h := newHarness(t, setup{config: `
[[tab]]
name = "All"
layout = "grid"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ app = "com.microsoft.VSCode" }]
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
`})
	want := errOnlyLayout("Grid of four", 4).Error()
	mustErr(t, h.e.StepLayout(1), want)
	mustErr(t, h.e.StepLayout(-1), want)
	h.e.Shortcut(shortcuts.ActionNextLayout, "")
	wantToast(t, h, want)
	if a := h.view().Display.Arrange; a.NextLayout != "" || a.PreviousLayout != "" {
		t.Errorf("Arrange is %+v, want no layouts", a)
	}

	// A tree with five panes, by other means: no layout has room.
	h.e.lock()
	five := &Tab{Preset: layout.PresetCustom, Root: &layout.Node{ID: "s", Type: layout.TypeSplit, Dir: layout.Row}}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		five.Root.Children = append(five.Root.Children, &layout.Node{ID: id, Type: layout.TypePane, Items: []string{id + "1"}})
	}
	_, err := h.e.nextLayoutLocked(five, 1)
	// A custom tree with no layout's shape, a pane beside a column of two,
	// goes to the first layout with room, or the last.
	nested := &Tab{Preset: layout.PresetCustom, Root: &layout.Node{ID: "s", Type: layout.TypeSplit, Dir: layout.Row, Children: []*layout.Node{
		five.Root.Children[0], {ID: "s2", Type: layout.TypeSplit, Dir: layout.Column, Children: five.Root.Children[1:3]},
	}}}
	next, _ := h.e.nextLayoutLocked(nested, 1)
	prev, _ := h.e.nextLayoutLocked(nested, -1)
	h.e.unlock()
	mustErr(t, err, errNoLayoutFits(5).Error())
	if next.ID != layout.PresetThree || prev.ID != layout.PresetGrid {
		t.Errorf("a custom tree of three: next %s, previous %s; want three and grid", next.ID, prev.ID)
	}

	z := newHarness(t, setup{})
	brave, _ := researchPanes(t, z)
	mustOK(t, z.e.ZoomPane(brave))
	z.e.Shortcut(shortcuts.ActionNextLayout, "")
	z.drain()
	if got := z.tabNamed("Research").Preset; got != layout.PresetMainSide {
		t.Errorf("next_layout from Side by side gave %s", got)
	}
	if got := z.view().Display.Zoomed; got != "" {
		t.Errorf("Next Layout left %q zoomed", got)
	}
	wantToast(t, z, "Layout: Main and side")
}

// TestStepLayoutFromAMovedTab checks that a tab marked custom, because a
// window moved into it or between its panes, goes on from the layout it
// had while its tree keeps that layout's shape, after a restart too; and
// from the layout whose shape its tree has, by its sizes, when the tab
// doesn't know the layout it had, as from an older state.json.
func TestStepLayoutFromAMovedTab(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	mustOK(t, h.e.FocusPane(brave))
	// send_to_tab 2 sends the shown Brave window to Code's focused pane.
	h.e.Shortcut(shortcuts.ActionSendToTab, "2")
	if code, research := h.tabNamed("Code"), h.tabNamed("Research"); code.Preset != layout.PresetCustom || research.Preset != layout.PresetCustom {
		t.Fatalf("after send_to_tab, Code is %s and Research %s, want both custom", code.Preset, research.Preset)
	}
	mustOK(t, h.e.SwitchTab(1))
	if a := h.view().Display.Arrange; a.NextLayout != "Three columns" || a.PreviousLayout != "Side by side" {
		t.Errorf("Code, Main and side before the window came: Arrange is %+v", a)
	}
	mustOK(t, h.e.StepLayout(1))
	if got := h.tabNamed("Code").Preset; got != layout.PresetThree {
		t.Errorf("Next Layout on Code gave %s, want three", got)
	}
	wantToast(t, h, "Layout: Three columns")

	// Research, Side by side at 60% and 40% before the window left, after
	// a restart.
	h.settle()
	h.restart()
	mustOK(t, h.e.SwitchTab(2))
	if a := h.view().Display.Arrange; a.NextLayout != "Main and side" || a.PreviousLayout != "Grid of four" {
		t.Errorf("Research, Side by side before the window left: Arrange is %+v", a)
	}

	// Without the layout it had, 60% and 40% are nearer Main and side's
	// sizes than Side by side's, and a row of three is Three columns, so
	// Next Layout gives Top and bottom, or for three panes with windows in
	// them, Grid of four.
	h.e.lock()
	research := h.e.model.Tabs[2]
	research.FromPreset = ""
	a := h.e.arrangeViewLocked(research)
	three := &Tab{Preset: layout.PresetCustom, FromPreset: layout.PresetSide, Root: &layout.Node{ID: "s", Type: layout.TypeSplit, Dir: layout.Row, Sizes: []float64{40, 30, 30}}}
	for _, id := range []string{"a", "b", "c"} {
		three.Root.Children = append(three.Root.Children, &layout.Node{ID: id, Type: layout.TypePane})
	}
	nextOfThree, _ := h.e.nextLayoutLocked(three, 1)
	for _, p := range three.Root.Children {
		p.Items = []string{p.ID + "1"}
	}
	fullThree, _ := h.e.nextLayoutLocked(three, 1)
	h.e.unlock()
	if a.NextLayout != "Three columns" || a.PreviousLayout != "Side by side" {
		t.Errorf("Research at 60%%, 40%%, not knowing its layout: Arrange is %+v", a)
	}
	if nextOfThree.ID != layout.PresetTopBottom || fullThree.ID != layout.PresetGrid {
		t.Errorf("a row of three, once Side by side: next %s, or %s with windows in each pane; want top-bottom and grid", nextOfThree.ID, fullThree.ID)
	}
}

// TestStepLayoutWithTheAddDialogOpen checks that a new layout keeps the
// empty pane that SplitPane made while the add dialog is open for it, so
// that what the dialog adds goes there, and that the pane closes up if the
// dialog closes without adding anything.
func TestStepLayoutWithTheAddDialogOpen(t *testing.T) {
	h := newHarness(t, setup{})
	_, notes := researchPanes(t, h)
	id, err := h.e.SplitPane(notes, "right")
	mustOK(t, err)
	if a := h.view().Display.Arrange; a.NextLayout != "Grid of four" {
		t.Errorf("Research as three columns, one empty: Arrange is %+v, want Grid of four next", a)
	}
	mustOK(t, h.e.StepLayout(1))
	h.drain()
	tv := h.tabNamed("Research")
	if tv.Preset != layout.PresetGrid || !slices.Contains(leafIDs(tv), id) {
		t.Fatalf("after Next Layout, Research is %s with panes %v, want grid with %s", tv.Preset, leafIDs(tv), id)
	}
	mustOK(t, h.e.AddToPane(id, Source{Type: sourceTerminal}))
	if items := h.paneView(id).Items; len(items) != 1 {
		t.Errorf("the add dialog's pane %s has %v, want the new terminal", id, items)
	}

	// Once the dialog has closed without adding anything, its pane goes,
	// and the tab no longer has Grid of four's shape.
	h2 := newHarness(t, setup{})
	_, notes = researchPanes(t, h2)
	id, err = h2.e.SplitPane(notes, "right")
	mustOK(t, err)
	mustOK(t, h2.e.StepLayout(1))
	mustOK(t, h2.e.CancelSplit(id))
	tv = h2.tabNamed("Research")
	if slices.Contains(leafIDs(tv), id) || tv.Preset != layout.PresetCustom {
		t.Errorf("after the add dialog closed, Research is %s with panes %v; want custom, without %s", tv.Preset, leafIDs(tv), id)
	}
}

// TestArrangeShortcuts checks the five actions' shortcuts and what the view
// offers for them.
func TestArrangeShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	mustOK(t, h.e.FocusPane(brave))
	if a := h.view().Display.Arrange; a != (ArrangeView{Grow: true, Shrink: true, Equal: true, NextLayout: "Main and side", PreviousLayout: "Grid of four"}) {
		t.Errorf("Research: Arrange is %+v", a)
	}
	h.e.Shortcut(shortcuts.ActionGrowPane, "")
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{65, 35}) {
		t.Errorf("grow_pane: sizes %v", got)
	}
	h.e.Shortcut(shortcuts.ActionShrinkPane, "")
	h.e.Shortcut(shortcuts.ActionShrinkPane, "")
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{55, 45}) {
		t.Errorf("shrink_pane twice: sizes %v", got)
	}
	h.e.Shortcut(shortcuts.ActionEqualPanes, "")
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{50, 50}) {
		t.Errorf("equal_panes: sizes %v", got)
	}
	if a := h.view().Display.Arrange; a.Equal {
		t.Error("with equal sizes, Arrange still offers Make Panes Equal")
	}
	wantToast(t, h, "Made the panes the same size.")
	undoLast(t, h)
	if got := rootSizes(h, "Research"); !slices.Equal(got, []float64{60, 40}) {
		t.Errorf("after Undo of the run, sizes %v, want 60, 40", got)
	}
}

// TestPercents checks the toasts' whole percentages, which add up to 100.
func TestPercents(t *testing.T) {
	for _, c := range []struct {
		in   []float64
		want []int
	}{
		{[]float64{65, 35}, []int{65, 35}},
		{[]float64{100.0 / 3, 100.0 / 3, 100.0 / 3}, []int{34, 33, 33}},
		{[]float64{37.5, 62.5}, []int{38, 62}},
		{[]float64{10.2, 44.9, 44.9}, []int{10, 45, 45}},
	} {
		if got := percents(c.in); !slices.Equal(got, c.want) {
			t.Errorf("percents(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestStepLayoutLeavesTheConfig checks that a layout from these keys is the
// live arrangement only: config.toml keeps the tab's layout until Tab
// properties saves the tab, which then keeps the live one, and Reset
// layout gives back the file's.
func TestStepLayoutLeavesTheConfig(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	mustOK(t, h.e.FocusPane(brave))
	mustOK(t, h.e.StepLayout(1))
	h.settle()
	if got := savedConfig(t, h).Tabs[2].Layout; got != layout.PresetSide {
		t.Errorf("after Next Layout, config.toml has Research as %s, want side still", got)
	}
	mustOK(t, h.e.ResetLayout(2))
	if got := h.tabNamed("Research"); got.Preset != layout.PresetSide || !slices.Equal(rootSizes(h, "Research"), []float64{60, 40}) {
		t.Errorf("after Reset layout, Research is %s with sizes %v, want side at 60, 40", got.Preset, got.Root.Sizes)
	}

	mustOK(t, h.e.StepLayout(1))
	mustOK(t, h.e.SaveTab(2, TabDraft{Name: "Research", Preset: h.tabNamed("Research").Preset, SmallScreens: "tabs"}))
	h.settle()
	if got := savedConfig(t, h).Tabs[2].Layout; got != layout.PresetMainSide {
		t.Errorf("after Tab properties' Save, config.toml has Research as %s, want main-side", got)
	}
}

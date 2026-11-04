package workspace

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// These tests cover dragging a docked window's edge to resize its pane
// (decision 62, edges.go).

// gridConfig is a Grid of four, a column of two rows, with an app in each
// pane: Zed top left, VS Code top right, Brave bottom left, and Obsidian
// bottom right.
const gridConfig = `
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
`

// holdEdge drags a docked window's edges, or a corner, to frame, with the
// mouse button held, as the user would, and doesn't let go.
func (h *harness) holdEdge(id platform.WindowID, frame layout.Rect) {
	h.t.Helper()
	h.p.SetMouseHeld(true)
	mustOK(h.t, h.p.UserMoveWindow(id, frame))
	h.drain()
}

// releaseEdge lets go of the mouse button, and waits for Bench to notice.
func (h *harness) releaseEdge() {
	h.p.SetMouseHeld(false)
	h.advance(2 * edgePoll)
}

// dragEdge drags a docked window's edges to frame and lets go.
func (h *harness) dragEdge(id platform.WindowID, frame layout.Rect) {
	h.t.Helper()
	h.holdEdge(id, frame)
	h.releaseEdge()
}

// sizesOf returns the sizes of a split of a tab's own tree.
func (h *harness) sizesOf(tab, splitID string) []float64 {
	h.t.Helper()
	n, _, _ := layout.Find(h.tabNamed(tab).Root, splitID)
	if n == nil {
		h.t.Fatalf("%s has no split %s", tab, splitID)
	}
	return slices.Clone(n.Sizes)
}

// near reports whether two values are within a point of each other, as
// sizes rounded to hundredths put a pane's edge.
func near(a, b float64) bool { return math.Abs(a-b) <= 1 }

// codeZed switches to Code and returns Zed's window, docked in p11, the
// left pane of a row of two, beside the terminal's p12.
func codeZed(t *testing.T, h *harness) platform.WindowID {
	t.Helper()
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	zed := h.windowID(zedID, "bench — switch.go")
	if h.frame(zed) != h.bodyOf("p11") {
		t.Fatalf("Zed is at %v, want docked in p11 at %v", h.frame(zed), h.bodyOf("p11"))
	}
	return zed
}

func TestDraggingAnInnerEdgeResizesThePanes(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	split := h.tabNamed("Code").Root.ID
	f := h.frame(zed)
	f.W += 300
	h.holdEdge(zed, f)
	// While the user holds the edge, Bench leaves the window where it is,
	// even as it arranges the tab again.
	h.advance(time.Second)
	mustOK(t, h.e.SwitchTab(1))
	h.drain()
	if got := h.frame(zed); got != f {
		t.Fatalf("while held, Zed is at %v, want %v, where the user put it", got, f)
	}
	if it := h.item("w3"); it.Status != StatusReady {
		t.Fatalf("while held, w3 is %s", it.Status)
	}
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{65, 35}) {
		t.Fatalf("the sizes changed to %v before the user let go", got)
	}

	h.releaseEdge()
	body := h.bodyOf("p11")
	if !near(body.X+body.W, f.X+f.W) {
		t.Errorf("p11's body ends at %v, want %v, where the user let go of Zed's edge", body.X+body.W, f.X+f.W)
	}
	if got := h.frame(zed); got != body {
		t.Errorf("Zed is at %v, want docked in p11 at %v", got, body)
	}
	if got := h.sizesOf("Code", split); got[0] <= 65 || !near(got[0]+got[1], 100) {
		t.Errorf("sizes %v, want the first larger than 65", got)
	}
	if it := h.item("w3"); it.Status != StatusReady {
		t.Errorf("w3 is %s, want ready", it.Status)
	}
	wantToast(t, h, "Resized the panes.")
	// Undo away from its toast names it, as no command does (decision 65).
	if v := h.view(); v.Undo == nil || v.Undo.Name != "Resize Panes" || v.Undo.Did != "resized the panes" {
		t.Errorf("Undo = %+v, want Resize Panes", v.Undo)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	wantToast(t, h, "Put back the sizes of the panes.")
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{65, 35}) {
		t.Errorf("after Undo, sizes %v, want [65 35]", got)
	}
	if got := h.frame(zed); got != h.bodyOf("p11") {
		t.Errorf("after Undo, Zed is at %v, want docked in p11 at %v", got, h.bodyOf("p11"))
	}
}

// TestDraggingTheLeftEdgeOfTheRightPane checks an edge that makes a pane
// larger to the left, and a quick drag, let go of before Bench first asks.
func TestDraggingTheLeftEdgeOfTheRightPane(t *testing.T) {
	h := newHarness(t, setup{})
	brave, notes := researchPanes(t, h)
	h.settle()
	obsidian := h.windowID(obsidianID, "Daily note")
	split := h.tabNamed("Research").Root.ID
	f := h.frame(obsidian)
	f.X -= 200
	f.W += 200
	mustOK(t, h.p.UserMoveWindow(obsidian, f))
	h.advance(2 * edgePoll)
	body := h.bodyOf(notes)
	if !near(body.X, f.X) || h.frame(obsidian) != body {
		t.Errorf("Obsidian at %v, its pane's body at %v; want the body from %v", h.frame(obsidian), body, f.X)
	}
	if got := h.sizesOf("Research", split); got[0] >= 60 {
		t.Errorf("sizes %v, want the first smaller than 60", got)
	}
	if shown := h.shownWindow(brave); h.frame(shown) != h.bodyOf(brave) {
		t.Errorf("Brave is at %v, want docked in its smaller pane at %v", h.frame(shown), h.bodyOf(brave))
	}
	wantToast(t, h, "Resized the panes.")
}

// TestDraggingAnOuterEdgeUndocks checks that an edge that faces the edge of
// the pane area, and a move of the whole window, take it out of its pane
// at once, as before.
func TestDraggingAnOuterEdgeUndocks(t *testing.T) {
	for _, tc := range []struct {
		name string
		move func(f *layout.Rect)
	}{
		{"left edge", func(f *layout.Rect) { f.X += 100; f.W -= 100 }},
		{"top edge", func(f *layout.Rect) { f.Y += 100; f.H -= 100 }},
		{"bottom edge", func(f *layout.Rect) { f.H -= 100 }},
		{"title bar", func(f *layout.Rect) { f.X += 200 }},
		{"title bar, downwards", func(f *layout.Rect) { f.Y += 200 }},
		{"left and right edges", func(f *layout.Rect) { f.X += 50; f.W += 100 }},
		{"inner and outer edges", func(f *layout.Rect) { f.W += 100; f.H -= 100 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			zed := codeZed(t, h)
			split := h.tabNamed("Code").Root.ID
			f := h.frame(zed)
			tc.move(&f)
			h.p.SetMouseHeld(true)
			mustOK(t, h.p.UserMoveWindow(zed, f))
			h.drain()
			if it := h.item("w3"); it.Status != StatusUndocked {
				t.Fatalf("w3 is %s, want undocked", it.Status)
			}
			h.releaseEdge()
			if got := h.frame(zed); got != f {
				t.Errorf("Zed is at %v, want %v, where the user put it", got, f)
			}
			if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{65, 35}) {
				t.Errorf("sizes %v, want [65 35]", got)
			}
		})
	}
}

// TestDraggingAnEdgeThatDoesntMove checks that a drag that ends where it
// began puts the window back, with no toast.
func TestDraggingAnEdgeThatDoesntMove(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	body := h.bodyOf("p11")
	f := body
	f.W += 40
	h.holdEdge(zed, f)
	f.W -= 39
	mustOK(t, h.p.UserMoveWindow(zed, f))
	h.releaseEdge()
	if got := h.frame(zed); got != body {
		t.Errorf("Zed is at %v, want back in p11 at %v", got, body)
	}
	if it := h.item("w3"); it.Status != StatusReady {
		t.Errorf("w3 is %s, want ready", it.Status)
	}
	if got := h.em.lastToast(); got == "Resized the panes." {
		t.Errorf("toast %q for a drag that ended where it began", got)
	}
}

// TestDraggingAnEdgeThenTheWindow checks that a window that the user moves
// after dragging its edge, before they let go, comes out of its pane.
func TestDraggingAnEdgeThenTheWindow(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	f := h.frame(zed)
	f.W += 100
	h.holdEdge(zed, f)
	f.X += 300
	mustOK(t, h.p.UserMoveWindow(zed, f))
	h.drain()
	if it := h.item("w3"); it.Status != StatusUndocked {
		t.Fatalf("w3 is %s, want undocked", it.Status)
	}
	h.releaseEdge()
	if got := h.frame(zed); got != f {
		t.Errorf("Zed is at %v, want %v", got, f)
	}
}

func TestDraggingAnEdgeStopsAtTheMinimumSize(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	split := h.tabNamed("Code").Root.ID
	f := h.frame(zed)
	f.W = 2560 - f.X
	h.dragEdge(zed, f)
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{100 - layout.MinSize, layout.MinSize}) {
		t.Fatalf("sizes %v, want [%v %v]", got, 100-layout.MinSize, layout.MinSize)
	}
	if got := h.frame(zed); got != h.bodyOf("p11") {
		t.Errorf("Zed is at %v, want docked in p11 at %v", got, h.bodyOf("p11"))
	}
	wantToast(t, h, "Resized the panes.")

	// Once the pane is as wide as it can be, a drag only says so.
	h.advance(2 * time.Second)
	f = h.frame(zed)
	f.W += 50
	h.dragEdge(zed, f)
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{100 - layout.MinSize, layout.MinSize}) {
		t.Errorf("sizes %v after a drag past the limit", got)
	}
	if got := h.frame(zed); got != h.bodyOf("p11") {
		t.Errorf("Zed is at %v, want back in p11 at %v", got, h.bodyOf("p11"))
	}
	wantToast(t, h, "This pane is as wide as it can be.")
	if h.em.lastHasAction() {
		t.Error("the toast at the limit has an action, want none")
	}
	if it := h.item("w3"); it.Status != StatusReady {
		t.Errorf("w3 is %s, want ready", it.Status)
	}
}

// TestDraggingEdgesInNestedSplits checks a corner, whose two edges face the
// dividers of two splits, one inside the other, and an edge whose divider
// is in the split two levels up.
func TestDraggingEdgesInNestedSplits(t *testing.T) {
	h := newHarness(t, setup{config: gridConfig})
	h.settle()
	root := h.tabNamed("All").Root
	if root.Dir != layout.Column || len(root.Children) != 2 {
		t.Fatalf("the grid is %+v, want a column of two rows", root)
	}
	column, top, bottom := root.ID, root.Children[0].ID, root.Children[1].ID
	panes := leafIDs(h.tabNamed("All"))
	zed := h.windowID(zedID, "bench — switch.go")
	obsidian := h.windowID(obsidianID, "Daily note")

	// Zed's bottom right corner: both splits change, in one step.
	f := h.frame(zed)
	f.W += 200
	f.H += 100
	h.dragEdge(zed, f)
	body := h.bodyOf(panes[0])
	if !near(body.X+body.W, f.X+f.W) || !near(body.Y+body.H, f.Y+f.H) {
		t.Errorf("Zed's pane's body is %v, want its corner at %v,%v", body, f.X+f.W, f.Y+f.H)
	}
	if got := h.sizesOf("All", top); got[0] <= 50 {
		t.Errorf("the top row's sizes are %v, want the first larger than 50", got)
	}
	if got := h.sizesOf("All", column); got[0] <= 50 {
		t.Errorf("the column's sizes are %v, want the first larger than 50", got)
	}
	if got := h.sizesOf("All", bottom); !slices.Equal(got, []float64{50, 50}) {
		t.Errorf("the bottom row's sizes are %v, want [50 50]", got)
	}
	wantToast(t, h, "Resized the panes.")
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	for _, id := range []string{column, top, bottom} {
		if got := h.sizesOf("All", id); !slices.Equal(got, []float64{50, 50}) {
			t.Errorf("after Undo, %s's sizes are %v, want [50 50]", id, got)
		}
	}

	// Obsidian's top edge, bottom right: its row has no divider above it,
	// so it's the column's.
	h.advance(2 * time.Second)
	f = h.frame(obsidian)
	f.Y -= 150
	f.H += 150
	h.dragEdge(obsidian, f)
	if body := h.bodyOf(panes[3]); !near(body.Y, f.Y) || h.frame(obsidian) != body {
		t.Errorf("Obsidian at %v, its pane's body at %v; want the body from %v", h.frame(obsidian), body, f.Y)
	}
	if got := h.sizesOf("All", column); got[1] <= 50 {
		t.Errorf("the column's sizes are %v, want the second larger than 50", got)
	}
	if got := h.sizesOf("All", bottom); !slices.Equal(got, []float64{50, 50}) {
		t.Errorf("the bottom row's sizes are %v, want [50 50]", got)
	}
}

// TestDraggingAnEdgeInTheStackView checks the bottom edge of the top pane
// of a tab that the small-screen setting shows as a stack.
func TestDraggingAnEdgeInTheStackView(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	zed := codeZed(t, h)
	if d := h.view().Display; d.Root.Dir != layout.Column {
		t.Fatalf("Code is shown as %+v, want a stack", d.Root)
	}
	split := h.tabNamed("Code").Root.ID
	f := h.frame(zed)
	f.H += 120
	h.dragEdge(zed, f)
	body := h.bodyOf("p11")
	if !near(body.Y+body.H, f.Y+f.H) || h.frame(zed) != body {
		t.Errorf("Zed at %v, p11's body at %v; want the body to %v", h.frame(zed), body, f.Y+f.H)
	}
	if got := h.sizesOf("Code", split); got[0] <= 65 {
		t.Errorf("sizes %v, want the first larger than 65", got)
	}
	wantToast(t, h, "Resized the panes.")
}

// TestDraggingAnEdgeWithNoNeighbour checks the "tabs" view and a zoomed
// pane, whose one pane has no pane beside it: a drag of any edge takes the
// window out, as before.
func TestDraggingAnEdgeWithNoNeighbour(t *testing.T) {
	t.Run("tabs view", func(t *testing.T) {
		h := newHarness(t, setup{screen: laptop})
		researchPanes(t, h)
		h.settle()
		d := h.view().Display
		if !d.Compact {
			t.Fatal("Research isn't in the tabs view on the laptop")
		}
		item := d.Panes[0].Active
		win := h.windowOfItem(item)
		f := h.frame(win)
		f.W -= 300
		h.dragEdge(win, f)
		if it := h.item(item); it.Status != StatusUndocked {
			t.Errorf("%s is %s, want undocked", item, it.Status)
		}
		if got := h.frame(win); got != f {
			t.Errorf("the window is at %v, want %v", got, f)
		}
	})
	t.Run("zoomed pane", func(t *testing.T) {
		h := newHarness(t, setup{})
		zed := codeZed(t, h)
		mustOK(t, h.e.ZoomPane("p11"))
		h.settle()
		split := h.tabNamed("Code").Root.ID
		f := h.frame(zed)
		f.W -= 300
		h.dragEdge(zed, f)
		if it := h.item("w3"); it.Status != StatusUndocked {
			t.Errorf("w3 is %s, want undocked", it.Status)
		}
		if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{65, 35}) {
			t.Errorf("sizes %v, want [65 35]", got)
		}
	})
}

// TestDraggingAnEdgeWithTheTracker checks a drag that the drag tracker
// follows: the drop ends it at once, and just after the user dragged one
// of Bench's dividers, when a move with the mouse held doesn't count as
// theirs, it's still a drag of the edge.
func TestDraggingAnEdgeWithTheTracker(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	split := h.tabNamed("Code").Root.ID
	mustOK(t, h.e.SetSizes(split, []float64{60, 40}))
	h.clk.Advance(400 * time.Millisecond)
	f := h.frame(zed)
	f.W += 250
	at := platform.Point{X: f.X + f.W, Y: f.Y + f.H/2}
	mustOK(t, h.p.DragWindow(zed, f, at))
	h.drain()
	h.advance(time.Second)
	if got := h.frame(zed); got != f {
		t.Fatalf("while dragged, Zed is at %v, want %v", got, f)
	}
	if _, ok := h.p.DropGuideShown(); ok {
		t.Error("a drop guide shows for a drag of an edge")
	}
	mustOK(t, h.p.DropWindow(zed, at))
	h.drain()
	body := h.bodyOf("p11")
	if !near(body.X+body.W, f.X+f.W) || h.frame(zed) != body {
		t.Errorf("Zed at %v, p11's body at %v; want the body to %v", h.frame(zed), body, f.X+f.W)
	}
	if got := h.sizesOf("Code", split); got[0] <= 60 {
		t.Errorf("sizes %v, want the first larger than 60", got)
	}
	wantToast(t, h, "Resized the panes.")
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{60, 40}) {
		t.Errorf("after Undo, sizes %v, want [60 40]", got)
	}
}

// TestDraggingAnEdgeAgainAtOnce checks a second drag of an inner edge just
// after the first resized the panes, within dividerGrace, without the
// tracker: Bench saw the user let go of the first, so the press is a new
// one, and theirs.
func TestDraggingAnEdgeAgainAtOnce(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	split := h.tabNamed("Code").Root.ID
	f := h.frame(zed)
	f.W += 200
	h.dragEdge(zed, f)
	first := h.sizesOf("Code", split)
	if first[0] <= 65 {
		t.Fatalf("after the first drag, sizes %v, want the first larger than 65", first)
	}

	h.advance(300 * time.Millisecond)
	f = h.frame(zed)
	f.W += 150
	h.holdEdge(zed, f)
	h.advance(500 * time.Millisecond)
	if got := h.frame(zed); got != f {
		t.Fatalf("while held, Zed is at %v, want %v, where the user put it", got, f)
	}
	h.releaseEdge()
	body := h.bodyOf("p11")
	if !near(body.X+body.W, f.X+f.W) || h.frame(zed) != body {
		t.Errorf("Zed at %v, p11's body at %v; want the body to %v", h.frame(zed), body, f.X+f.W)
	}
	if got := h.sizesOf("Code", split); got[0] <= first[0] {
		t.Errorf("sizes %v, want the first larger than %v", got, first[0])
	}
	wantToast(t, h, "Resized the panes.")
	// Each drag is a step of its own.
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if got := h.sizesOf("Code", split); !slices.Equal(got, first) {
		t.Errorf("after Undo, sizes %v, want %v", got, first)
	}
}

// TestDraggingADividerAfterAnEdge checks that a drag of one of Bench's
// dividers after a drag of an edge gives the whole grace again: a move of a
// docked window's inner edge, with the mouse held on the divider beside it,
// is put back.
func TestDraggingADividerAfterAnEdge(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	split := h.tabNamed("Code").Root.ID
	f := h.frame(zed)
	f.W += 200
	h.dragEdge(zed, f)
	wantToast(t, h, "Resized the panes.")
	h.advance(2 * time.Second)

	mustOK(t, h.e.SetSizes(split, []float64{60, 40}))
	h.advance(400 * time.Millisecond)
	f = h.frame(zed)
	f.W -= 3
	h.p.SetMouseHeld(true)
	mustOK(t, h.p.UserMoveWindow(zed, f))
	h.advance(time.Second)
	if got := h.frame(zed); got != h.bodyOf("p11") {
		t.Errorf("Zed is at %v, want put back in p11 at %v", got, h.bodyOf("p11"))
	}
	h.p.SetMouseHeld(false)
	h.advance(2 * edgePoll)
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{60, 40}) {
		t.Errorf("sizes %v, want [60 40], as the divider left them", got)
	}
}

// TestMovesJustAfterAnEdgeResize checks the grace that a drag of an edge
// gives the windows that dock again after it resized the panes: for
// dividerGrace, a move with the mouse held that would take a docked
// window out of its pane is put back, as Bench's or its app's.
func TestMovesJustAfterAnEdgeResize(t *testing.T) {
	h := newHarness(t, setup{})
	brave, _ := researchPanes(t, h)
	h.settle()
	obsidian := h.windowID(obsidianID, "Daily note")
	f := h.frame(obsidian)
	f.X -= 200
	f.W += 200
	h.dragEdge(obsidian, f)
	wantToast(t, h, "Resized the panes.")
	shown, item := h.shownWindow(brave), h.paneView(brave).Active
	body := h.bodyOf(brave)

	h.advance(300 * time.Millisecond)
	moved := body
	moved.X += 100 // By its title bar.
	h.p.SetMouseHeld(true)
	mustOK(t, h.p.UserMoveWindow(shown, moved))
	h.drain()
	if it := h.item(item); it.Status == StatusUndocked {
		t.Fatalf("a move just after the resize undocked %s", item)
	}
	h.p.SetMouseHeld(false)
	h.advance(time.Second)
	if got := h.frame(shown); got != body {
		t.Errorf("Brave is at %v, want put back in its pane at %v", got, body)
	}

	// Past the grace, the same move is the user's.
	h.advance(time.Second)
	h.p.SetMouseHeld(true)
	mustOK(t, h.p.UserMoveWindow(shown, moved))
	h.drain()
	if it := h.item(item); it.Status != StatusUndocked {
		t.Errorf("%s is %s after the grace, want undocked", item, it.Status)
	}
}

// TestAnotherWindowMovesDuringAnEdgeDrag checks that, while the user holds
// one window's edge, a move of another docked window with the mouse held,
// as when the edge goes over it, is its app's: there's one mouse. Once the
// button is up, such a move shows that the user has let go, before Bench
// next asks.
func TestAnotherWindowMovesDuringAnEdgeDrag(t *testing.T) {
	for _, tc := range []struct {
		name string
		held bool
	}{
		{"held", true},
		{"let go", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			brave, notes := researchPanes(t, h)
			h.settle()
			win := h.shownWindow(brave)
			obsidian := h.windowID(obsidianID, "Daily note")
			split := h.tabNamed("Research").Root.ID
			f := h.frame(win)
			f.W += 300
			h.holdEdge(win, f)
			h.p.SetMouseHeld(tc.held)
			nudged := h.frame(obsidian)
			nudged.X++
			mustOK(t, h.p.UserMoveWindow(obsidian, nudged))
			h.drain()
			if tc.held {
				h.advance(500 * time.Millisecond)
				if got := h.frame(win); got != f {
					t.Fatalf("while held, Brave is at %v, want %v, where the user put it", got, f)
				}
				if got := h.sizesOf("Research", split); !slices.Equal(got, []float64{60, 40}) {
					t.Fatalf("the sizes changed to %v before the user let go", got)
				}
				h.releaseEdge()
			}
			body := h.bodyOf(brave)
			if !near(body.X+body.W, f.X+f.W) || h.frame(win) != body {
				t.Errorf("Brave at %v, its pane's body at %v; want the body to %v", h.frame(win), body, f.X+f.W)
			}
			if got := h.frame(obsidian); got != h.bodyOf(notes) {
				t.Errorf("Obsidian is at %v, want docked at %v", got, h.bodyOf(notes))
			}
			if got := h.sizesOf("Research", split); got[0] <= 60 {
				t.Errorf("sizes %v, want the first larger than 60", got)
			}
			wantToast(t, h, "Resized the panes.")
		})
	}
}

// TestHoldingAnEdgeTooLongUndocks checks that Bench takes the window out
// of its pane, as before, if the mouse button seems held for edgeLimit.
func TestHoldingAnEdgeTooLongUndocks(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	f := h.frame(zed)
	f.W += 100
	h.holdEdge(zed, f)
	h.advance(edgeLimit - time.Second)
	if it := h.item("w3"); it.Status != StatusReady {
		t.Fatalf("w3 is %s before the limit", it.Status)
	}
	h.advance(2 * time.Second)
	if it := h.item("w3"); it.Status != StatusUndocked {
		t.Errorf("w3 is %s after the limit, want undocked", it.Status)
	}
	if got := h.frame(zed); got != f {
		t.Errorf("Zed is at %v, want %v", got, f)
	}
}

// TestDraggingAnEdgeWhileTheTabChanges checks that a window whose tab is
// no longer shown when the user lets go doesn't resize any panes.
func TestDraggingAnEdgeWhileTheTabChanges(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	split := h.tabNamed("Code").Root.ID
	f := h.frame(zed)
	f.W += 200
	h.holdEdge(zed, f)
	mustOK(t, h.e.SwitchTab(2))
	h.releaseEdge()
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{65, 35}) {
		t.Errorf("sizes %v, want [65 35]", got)
	}
	if got := h.em.lastToast(); got == "Resized the panes." {
		t.Errorf("toast %q after the tab changed", got)
	}
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if got := h.frame(zed); got != h.bodyOf("p11") {
		t.Errorf("back in Code, Zed is at %v, want docked in p11 at %v", got, h.bodyOf("p11"))
	}
}

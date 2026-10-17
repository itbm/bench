package layout

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
)

// randomSplitTree returns a random tree of splits and panes, up to depth
// levels deep, with random directions and sizes. IDs come from *seq.
func randomSplitTree(r *rand.Rand, depth int, seq *int) *Node {
	*seq++
	if depth == 0 || r.IntN(3) == 0 {
		return NewPane(fmt.Sprintf("p%d", *seq))
	}
	id := fmt.Sprintf("s%d", *seq)
	n := 2 + r.IntN(3)
	children := make([]*Node, n)
	sizes := make([]float64, n)
	for i := range children {
		children[i] = randomSplitTree(r, depth-1, seq)
		sizes[i] = 20 + r.Float64()*80
	}
	dir := Row
	if r.IntN(2) == 0 {
		dir = Column
	}
	return NewSplit(id, dir, sizes, children...)
}

// TestFramesKeepTheGap checks the spec's promise for random deep trees: a
// gap of g, give or take a point of rounding, between any two neighbouring
// panes, however deeply nested, and at every edge of the pane area. It also
// checks that no two frames overlap.
func TestFramesKeepTheGap(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	const gap = 8
	checked := 0
	for range 2000 {
		seq := 0
		root := randomSplitTree(r, 4, &seq)
		area := Rect{
			X: float64(r.IntN(200)),
			Y: float64(r.IntN(60)),
			W: float64(1000 + r.IntN(3000)),
			H: float64(700 + r.IntN(1500)),
		}
		frames := Frames(root, area, gap)
		if tooSmall(frames, gap) {
			continue
		}
		checked++
		for _, f := range frames {
			checkInsideEdges(t, root, area, gap, f)
		}
		for i, a := range frames {
			for _, b := range frames[i+1:] {
				if overlaps(a.Rect, b.Rect) {
					t.Fatalf("panes %s %v and %s %v overlap\ntree: %s", a.ID, a.Rect, b.ID, b.Rect, describe(root))
				}
				if d, ok := neighbourGap(a.Rect, b.Rect, gap); ok && math.Abs(d-gap) > 1 {
					t.Fatalf("panes %s %v and %s %v are %v pt apart, want %v±1\ntree: %s", a.ID, a.Rect, b.ID, b.Rect, d, gap, describe(root))
				}
			}
		}
		byID := make(map[string]Rect, len(frames))
		for _, f := range frames {
			byID[f.ID] = f.Rect
		}
		for _, d := range Dividers(root, area, gap) {
			checkDividerGap(t, root, byID, d, gap)
		}
	}
	if checked < 500 {
		t.Fatalf("only %d trees were big enough to check", checked)
	}
}

// tooSmall reports whether any frame is too small for its gaps to be
// meaningful, because the area has been divided too finely.
func tooSmall(frames []PaneRect, gap float64) bool {
	for _, f := range frames {
		if f.Rect.W < 2*gap || f.Rect.H < 2*gap {
			return true
		}
	}
	return false
}

// checkInsideEdges checks that a frame lies inside the pane area with at
// least g-1 points to spare on every side, and that a frame at an edge of
// the area is g±1 points from it.
func checkInsideEdges(t *testing.T, root *Node, area Rect, gap float64, f PaneRect) {
	t.Helper()
	left := f.Rect.X - area.X
	top := f.Rect.Y - area.Y
	right := area.X + area.W - (f.Rect.X + f.Rect.W)
	bottom := area.Y + area.H - (f.Rect.Y + f.Rect.H)
	for _, side := range []struct {
		name string
		d    float64
	}{{"left", left}, {"top", top}, {"right", right}, {"bottom", bottom}} {
		if side.d < gap-1 {
			t.Fatalf("pane %s %v is %v pt from the %s edge of %v, want at least %v\ntree: %s",
				f.ID, f.Rect, side.d, side.name, area, gap-1, describe(root))
		}
		// A pane near an edge is at the edge, with no other pane between.
		if side.d < 2*gap && math.Abs(side.d-gap) > 1 {
			t.Fatalf("pane %s %v is %v pt from the %s edge of %v, want %v±1\ntree: %s",
				f.ID, f.Rect, side.d, side.name, area, gap, describe(root))
		}
	}
}

// neighbourGap returns the distance between two frames that face each other
// across a gap less than twice g wide: side by side with rows in common, or
// one above the other with columns in common. It reports false for frames
// that aren't neighbours.
func neighbourGap(a, b Rect, gap float64) (float64, bool) {
	rowsInCommon := min(a.Y+a.H, b.Y+b.H) > max(a.Y, b.Y)
	columnsInCommon := min(a.X+a.W, b.X+b.W) > max(a.X, b.X)
	for _, d := range []struct {
		dist   float64
		facing bool
	}{
		{b.X - (a.X + a.W), rowsInCommon},
		{a.X - (b.X + b.W), rowsInCommon},
		{b.Y - (a.Y + a.H), columnsInCommon},
		{a.Y - (b.Y + b.H), columnsInCommon},
	} {
		if d.facing && d.dist >= 0 && d.dist < 2*gap {
			return d.dist, true
		}
	}
	return 0, false
}

// overlaps reports whether two rectangles share any area.
func overlaps(a, b Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

// checkDividerGap checks that a divider is g±1 across, lies within its
// split's frame, and sits between the two children it divides: it overlaps
// neither child if that child is a pane, and a pane nested deeper by at most
// a point, because a split's frame and its panes' frames are rounded
// separately. Frames holds each pane's frame by ID.
func checkDividerGap(t *testing.T, root *Node, frames map[string]Rect, d DividerRect, gap float64) {
	t.Helper()
	across, along, splitAlong := d.Rect.W, d.Rect.H, d.Split.H
	if d.Dir == Column {
		across, along, splitAlong = d.Rect.H, d.Rect.W, d.Split.W
	}
	if math.Abs(across-gap) > 1 {
		t.Fatalf("divider %s/%d %v is %v pt across, want %v±1\ntree: %s", d.SplitID, d.Index, d.Rect, across, gap, describe(root))
	}
	if along != splitAlong {
		t.Fatalf("divider %s/%d %v doesn't span its split %v\ntree: %s", d.SplitID, d.Index, d.Rect, d.Split, describe(root))
	}
	if d.Rect.X < d.Split.X || d.Rect.Y < d.Split.Y ||
		d.Rect.X+d.Rect.W > d.Split.X+d.Split.W || d.Rect.Y+d.Rect.H > d.Split.Y+d.Split.H {
		t.Fatalf("divider %s/%d %v isn't inside its split %v\ntree: %s", d.SplitID, d.Index, d.Rect, d.Split, describe(root))
	}
	split, _, _ := Find(root, d.SplitID)
	for _, side := range []*Node{split.Children[d.Index], split.Children[d.Index+1]} {
		allowed := 0.0
		if side.IsSplit() {
			allowed = 1
		}
		for _, p := range Leaves(side) {
			f := frames[p.ID]
			if o := overlapAcross(f, d); o > allowed {
				t.Fatalf("divider %s/%d %v overlaps pane %s %v by %v pt, want at most %v\ntree: %s",
					d.SplitID, d.Index, d.Rect, p.ID, f, o, allowed, describe(root))
			}
		}
	}
}

// overlapAcross returns how far, across the divider, a frame overlaps it.
func overlapAcross(f Rect, d DividerRect) float64 {
	if d.Dir == Column {
		return max(0, min(f.Y+f.H, d.Rect.Y+d.Rect.H)-max(f.Y, d.Rect.Y))
	}
	return max(0, min(f.X+f.W, d.Rect.X+d.Rect.W)-max(f.X, d.Rect.X))
}

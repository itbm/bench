package layout

import (
	"math"
	"slices"
	"testing"
)

// TestFramesSpecCases runs every case in the spec's Layout maths table.
func TestFramesSpecCases(t *testing.T) {
	tests := []struct {
		name string
		area Rect
		gap  float64
		tree func(t *testing.T) *Node
		want []Rect
	}{
		{
			name: "main-side 65, 35 on a monitor",
			area: Rect{0, 25, 2560, 1415},
			gap:  8,
			tree: func(t *testing.T) *Node { return mustBuild(t, PresetMainSide, []float64{65, 35}) },
			want: []Rect{{8, 33, 1651, 1399}, {1667, 33, 885, 1399}},
		},
		{
			name: "side 50, 50 on a monitor",
			area: Rect{0, 25, 2560, 1415},
			gap:  8,
			tree: func(t *testing.T) *Node { return mustBuild(t, PresetSide, []float64{50, 50}) },
			want: []Rect{{8, 33, 1268, 1399}, {1284, 33, 1268, 1399}},
		},
		{
			name: "side 45, 55 on a monitor",
			area: Rect{0, 25, 2560, 1415},
			gap:  8,
			tree: func(t *testing.T) *Node { return mustBuild(t, PresetSide, []float64{45, 55}) },
			want: []Rect{{8, 33, 1140, 1399}, {1156, 33, 1396, 1399}},
		},
		{
			name: "top-bottom 65, 35 on a laptop",
			area: Rect{0, 33, 1512, 949},
			gap:  8,
			tree: func(t *testing.T) *Node { return mustBuild(t, PresetTopBottom, []float64{65, 35}) },
			want: []Rect{{8, 41, 1496, 604}, {8, 653, 1496, 321}},
		},
		{
			name: "side 50, 50 in 1000 by 800",
			area: Rect{0, 0, 1000, 800},
			gap:  8,
			tree: func(t *testing.T) *Node { return mustBuild(t, PresetSide, []float64{50, 50}) },
			want: []Rect{{8, 8, 488, 784}, {504, 8, 488, 784}},
		},
		{
			name: "grid in 1000 by 800",
			area: Rect{0, 0, 1000, 800},
			gap:  8,
			tree: func(t *testing.T) *Node { return mustBuild(t, PresetGrid, nil) },
			want: []Rect{{8, 8, 488, 388}, {504, 8, 488, 388}, {8, 404, 488, 388}, {504, 404, 488, 388}},
		},
		{
			name: "row 65, 35 with the right side a column 50, 50",
			area: Rect{0, 25, 2560, 1415},
			gap:  8,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Row, []float64{65, 35},
					NewPane("p1"),
					NewSplit("s2", Column, []float64{50, 50}, NewPane("p2"), NewPane("p3")))
			},
			want: []Rect{{8, 33, 1651, 1399}, {1667, 33, 885, 696}, {1667, 737, 885, 696}},
		},
		{
			name: "single with no gap",
			area: Rect{100, 25, 500, 400},
			gap:  0,
			tree: func(t *testing.T) *Node { return mustBuild(t, PresetSingle, nil) },
			want: []Rect{{100, 25, 500, 400}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.tree(t)
			got := Frames(root, tt.area, tt.gap)
			if !slices.Equal(rects(got), tt.want) {
				t.Errorf("Frames:\n got %v\nwant %v", rects(got), tt.want)
			}
			if !slices.Equal(ids(Leaves(root)), paneIDs(got)) {
				t.Errorf("Frames IDs %v aren't the panes in reading order %v", paneIDs(got), ids(Leaves(root)))
			}
		})
	}
}

// paneIDs returns the IDs of prs.
func paneIDs(prs []PaneRect) []string {
	out := make([]string, len(prs))
	for i, pr := range prs {
		out[i] = pr.ID
	}
	return out
}

func TestRound(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{0, 0},
		{0.4, 0},
		{0.5, 1},
		{1.5, 2},
		{2.5, 3},
		{-0.5, 0},
		{-1.5, -1},
		{-0.6, -1},
		{1650.8, 1651},
		{695.5, 696},
		// Float error just below .5 still rounds up.
		{0.49999999999999994, 1},
		{695.4999999999999, 696},
		{0.4999, 0},
	}
	for _, tt := range tests {
		if got := Round(tt.in); got != tt.want {
			t.Errorf("Round(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestUnitRects(t *testing.T) {
	tests := []struct {
		name string
		tree *Node
		want []Rect
	}{
		{"pane", NewPane("p1"), []Rect{{0, 0, 1, 1}}},
		{
			"row 25, 75",
			NewSplit("s1", Row, []float64{25, 75}, NewPane("p1"), NewPane("p2")),
			[]Rect{{0, 0, 0.25, 1}, {0.25, 0, 0.75, 1}},
		},
		{
			"column 50, 50 of rows 50, 50",
			NewSplit("s1", Column, []float64{50, 50},
				NewSplit("s2", Row, []float64{50, 50}, NewPane("p1"), NewPane("p2")),
				NewSplit("s3", Row, []float64{50, 50}, NewPane("p3"), NewPane("p4"))),
			[]Rect{{0, 0, 0.5, 0.5}, {0.5, 0, 0.5, 0.5}, {0, 0.5, 0.5, 0.5}, {0.5, 0.5, 0.5, 0.5}},
		},
		{
			"sizes are normalised",
			NewSplit("s1", Row, []float64{1, 3}, NewPane("p1"), NewPane("p2")),
			[]Rect{{0, 0, 0.25, 1}, {0.25, 0, 0.75, 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rects(UnitRects(tt.tree))
			if !slices.EqualFunc(got, tt.want, rectNear) {
				t.Errorf("UnitRects = %v, want %v", got, tt.want)
			}
		})
	}
}

// rectNear reports whether two rectangles are equal within float error.
func rectNear(a, b Rect) bool {
	return near([]float64{a.X, a.Y, a.W, a.H}, []float64{b.X, b.Y, b.W, b.H})
}

// TestUnitRectsTile checks that the unit rectangles of a deep tree cover the
// unit square exactly, with no slivers from float error.
func TestUnitRectsTile(t *testing.T) {
	root := NewSplit("s1", Row, []float64{33.3, 33.3, 33.4},
		NewPane("p1"),
		NewSplit("s2", Column, []float64{1, 1, 1}, NewPane("p2"), NewPane("p3"), NewPane("p4")),
		NewPane("p5"))
	var area float64
	for _, u := range UnitRects(root) {
		area += u.Rect.W * u.Rect.H
		if u.Rect.X+u.Rect.W > 1+1e-12 || u.Rect.Y+u.Rect.H > 1+1e-12 {
			t.Errorf("%s runs past the edge: %v", u.ID, u.Rect)
		}
	}
	if math.Abs(area-1) > 1e-12 {
		t.Errorf("unit rectangles cover %v, want 1", area)
	}
	last := UnitRects(root)[4].Rect
	if last.X+last.W != 1 {
		t.Errorf("last pane ends at %v, want exactly 1", last.X+last.W)
	}
}

func TestFrameClampsTinyAreas(t *testing.T) {
	got := Frame(Rect{0, 0, 1, 1}, Rect{0, 0, 10, 6}, 8)
	want := Rect{8, 8, 0, 0}
	if got != want {
		t.Errorf("Frame in a tiny area = %v, want %v", got, want)
	}
}

func TestDividers(t *testing.T) {
	tests := []struct {
		name string
		area Rect
		gap  float64
		tree func(t *testing.T) *Node
		want []DividerRect
	}{
		{
			name: "single has none",
			area: Rect{0, 0, 1000, 800},
			gap:  8,
			tree: func(t *testing.T) *Node { return NewPane("p1") },
			want: nil,
		},
		{
			name: "side 50, 50",
			area: Rect{0, 0, 1000, 800},
			gap:  8,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Row, []float64{50, 50}, NewPane("p1"), NewPane("p2"))
			},
			want: []DividerRect{
				{SplitID: "s1", Index: 0, Dir: Row, Rect: Rect{496, 8, 8, 784}, Split: Rect{8, 8, 984, 784}, Sizes: []float64{50, 50}},
			},
		},
		{
			name: "three 40, 30, 30",
			area: Rect{0, 0, 1000, 800},
			gap:  8,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Row, []float64{40, 30, 30}, NewPane("p1"), NewPane("p2"), NewPane("p3"))
			},
			// Frames: {8, 8, 389, 784}, {405, 8, 290, 784}, {702, 8, 290, 784}.
			// Rounding makes the second gap 7 pt.
			want: []DividerRect{
				{SplitID: "s1", Index: 0, Dir: Row, Rect: Rect{397, 8, 8, 784}, Split: Rect{8, 8, 984, 784}, Sizes: []float64{40, 30, 30}},
				{SplitID: "s1", Index: 1, Dir: Row, Rect: Rect{695, 8, 7, 784}, Split: Rect{8, 8, 984, 784}, Sizes: []float64{40, 30, 30}},
			},
		},
		{
			name: "grid",
			area: Rect{0, 0, 1000, 800},
			gap:  8,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Column, []float64{50, 50},
					NewSplit("s2", Row, []float64{50, 50}, NewPane("p1"), NewPane("p2")),
					NewSplit("s3", Row, []float64{50, 50}, NewPane("p3"), NewPane("p4")))
			},
			want: []DividerRect{
				{SplitID: "s1", Index: 0, Dir: Column, Rect: Rect{8, 396, 984, 8}, Split: Rect{8, 8, 984, 784}, Sizes: []float64{50, 50}},
				{SplitID: "s2", Index: 0, Dir: Row, Rect: Rect{496, 8, 8, 388}, Split: Rect{8, 8, 984, 388}, Sizes: []float64{50, 50}},
				{SplitID: "s3", Index: 0, Dir: Row, Rect: Rect{496, 404, 8, 388}, Split: Rect{8, 404, 984, 388}, Sizes: []float64{50, 50}},
			},
		},
		{
			name: "nested spec case",
			area: Rect{0, 25, 2560, 1415},
			gap:  8,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Row, []float64{65, 35},
					NewPane("p1"),
					NewSplit("s2", Column, []float64{50, 50}, NewPane("p2"), NewPane("p3")))
			},
			want: []DividerRect{
				{SplitID: "s1", Index: 0, Dir: Row, Rect: Rect{1659, 33, 8, 1399}, Split: Rect{8, 33, 2544, 1399}, Sizes: []float64{65, 35}},
				{SplitID: "s2", Index: 0, Dir: Column, Rect: Rect{1667, 729, 885, 8}, Split: Rect{1667, 33, 885, 1399}, Sizes: []float64{50, 50}},
			},
		},
		{
			name: "no gap: 1 pt across, centred where the panes meet",
			area: Rect{0, 0, 1000, 800},
			gap:  0,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Column, []float64{50, 50}, NewPane("p1"), NewPane("p2"))
			},
			want: []DividerRect{
				{SplitID: "s1", Index: 0, Dir: Column, Rect: Rect{0, 399.5, 1000, 1}, Split: Rect{0, 0, 1000, 800}, Sizes: []float64{50, 50}},
			},
		},
		{
			name: "no gap in a row",
			area: Rect{0, 0, 1000, 800},
			gap:  0,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Row, []float64{30, 70}, NewPane("p1"), NewPane("p2"))
			},
			want: []DividerRect{
				{SplitID: "s1", Index: 0, Dir: Row, Rect: Rect{299.5, 0, 1, 800}, Split: Rect{0, 0, 1000, 800}, Sizes: []float64{30, 70}},
			},
		},
		{
			name: "a gap of exactly 1 pt is kept",
			area: Rect{0, 0, 1001, 801},
			gap:  1,
			tree: func(*testing.T) *Node {
				return NewSplit("s1", Row, []float64{50, 50}, NewPane("p1"), NewPane("p2"))
			},
			// Frames: {1, 1, 499, 799} and {501, 1, 499, 799}.
			want: []DividerRect{
				{SplitID: "s1", Index: 0, Dir: Row, Rect: Rect{500, 1, 1, 799}, Split: Rect{1, 1, 999, 799}, Sizes: []float64{50, 50}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Dividers(tt.tree(t), tt.area, tt.gap)
			if len(got) != len(tt.want) {
				t.Fatalf("Dividers = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				g, w := got[i], tt.want[i]
				if g.SplitID != w.SplitID || g.Index != w.Index || g.Dir != w.Dir || g.Rect != w.Rect || g.Split != w.Split || !near(g.Sizes, w.Sizes) {
					t.Errorf("divider %d:\n got %+v\nwant %+v", i, g, w)
				}
			}
		})
	}
}

// TestDividersAtLeastOnePoint checks the 1 pt minimum where rounding makes
// neighbouring frames overlap.
func TestDividersAtLeastOnePoint(t *testing.T) {
	// With no gap in a 1 pt wide area at x = 0.5, 50, 50 gives frames
	// {1, 0, 1, 10} and {1, 0, 1, 10}, which overlap by a point. The
	// divider is 1 pt across, centred on the middle of the overlap.
	root := NewSplit("s1", Row, []float64{50, 50}, NewPane("p1"), NewPane("p2"))
	area := Rect{0.5, 0, 1, 10}
	frames := Frames(root, area, 0)
	if frames[0].Rect.X+frames[0].Rect.W <= frames[1].Rect.X {
		t.Fatalf("frames %v don't overlap, so this test checks nothing", rects(frames))
	}
	got := Dividers(root, area, 0)
	want := Rect{1, 0, 1, 10}
	if len(got) != 1 || got[0].Rect != want {
		t.Errorf("Dividers = %+v, want one with rectangle %v", got, want)
	}
}

func TestDividerSizesAreCopies(t *testing.T) {
	root := NewSplit("s1", Row, []float64{50, 50}, NewPane("p1"), NewPane("p2"))
	d := Dividers(root, Rect{0, 0, 100, 100}, 8)
	d[0].Sizes[0] = 1
	if root.Sizes[0] != 50 {
		t.Errorf("changing a divider's sizes changed the tree: %v", root.Sizes)
	}
}

func TestReachTop(t *testing.T) {
	area := Rect{0, 33, 1512, 949}
	tests := []struct {
		frame, want Rect
	}{
		// The one pane of a 4-point gap reaches up over the gap above it,
		// keeping its sides and bottom.
		{Frame(unitRect, area, 4), Rect{4, 33, 1504, 945}},
		{Rect{8, 41, 100, 50}, Rect{8, 33, 100, 58}},
		// A frame already at the top stays as it is.
		{Rect{0, 33, 100, 50}, Rect{0, 33, 100, 50}},
		// One with no height left keeps none.
		{Rect{0, 10, 100, 5}, Rect{0, 33, 100, 0}},
	}
	for _, tt := range tests {
		if got := ReachTop(tt.frame, area); got != tt.want {
			t.Errorf("ReachTop(%v, %v) = %v, want %v", tt.frame, area, got, tt.want)
		}
	}
}

func TestBody(t *testing.T) {
	tests := []struct {
		frame  Rect
		header float64
		want   Rect
	}{
		{Rect{8, 33, 1651, 1399}, HeaderHeight, Rect{9, 75, 1649, 1356}},
		{Rect{0, 0, 100, 100}, HeaderHeight, Rect{1, 42, 98, 57}},
		{Rect{10, 10, 1, 20}, HeaderHeight, Rect{11, 52, 0, 0}},
		// The compact density's header is 8 pt shorter, so the body is 8 pt
		// taller.
		{Rect{8, 33, 1651, 1399}, CompactHeaderHeight, Rect{9, 67, 1649, 1364}},
		{Rect{0, 0, 100, 100}, CompactHeaderHeight, Rect{1, 34, 98, 65}},
		{Rect{10, 10, 1, 20}, CompactHeaderHeight, Rect{11, 44, 0, 0}},
		// With no header there's no divider either: only the border.
		{Rect{8, 33, 1651, 1399}, 0, Rect{9, 34, 1649, 1397}},
		{Rect{0, 0, 100, 100}, 0, Rect{1, 1, 98, 98}},
		{Rect{10, 10, 1, 1}, 0, Rect{11, 11, 0, 0}},
	}
	for _, tt := range tests {
		if got := Body(tt.frame, tt.header); got != tt.want {
			t.Errorf("Body(%v, %v) = %v, want %v", tt.frame, tt.header, got, tt.want)
		}
	}
}

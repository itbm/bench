package layout

import (
	"math"
	"testing"
)

func TestDividerBeside(t *testing.T) {
	// A row of a pane and a column, whose second child is a row:
	//
	//	p | q
	//	  |------
	//	  | r | s
	root := NewSplit("R", Row, []float64{40, 60},
		NewPane("p", "a"),
		NewSplit("C", Column, []float64{50, 50},
			NewPane("q", "b"),
			NewSplit("R2", Row, []float64{50, 50}, NewPane("r", "c"), NewPane("s", "d")),
		),
	)
	area := Rect{W: 1000, H: 800}
	dividers := Dividers(root, area, 4)
	tests := []struct {
		pane  string
		side  Direction
		split string
		index int
		ok    bool
	}{
		{"p", DirRight, "R", 0, true},
		{"p", DirLeft, "", 0, false},
		{"p", DirUp, "", 0, false},
		{"p", DirDown, "", 0, false},
		{"q", DirLeft, "R", 0, true},
		{"q", DirDown, "C", 0, true},
		{"q", DirUp, "", 0, false},
		{"q", DirRight, "", 0, false},
		// r is the first of its row, so its left is the outer row's
		// divider, two levels up.
		{"r", DirLeft, "R", 0, true},
		{"r", DirRight, "R2", 0, true},
		{"r", DirUp, "C", 0, true},
		{"r", DirDown, "", 0, false},
		{"s", DirLeft, "R2", 0, true},
		{"s", DirUp, "C", 0, true},
		{"s", DirRight, "", 0, false},
		{"nowhere", DirLeft, "", 0, false},
		{"C", DirUp, "", 0, false}, // A split, not a pane.
		{"r", Direction("sideways"), "", 0, false},
	}
	for _, tt := range tests {
		d, ok := DividerBeside(root, dividers, tt.pane, tt.side)
		if ok != tt.ok || ok && (d.SplitID != tt.split || d.Index != tt.index) {
			t.Errorf("DividerBeside(%s, %s) = %s/%d, %v; want %s/%d, %v", tt.pane, tt.side, d.SplitID, d.Index, ok, tt.split, tt.index, tt.ok)
		}
	}
}

func TestDividerBesideInThreeColumns(t *testing.T) {
	root := NewSplit("R", Row, []float64{40, 30, 30}, NewPane("a"), NewPane("b"), NewPane("c"))
	dividers := Dividers(root, Rect{W: 1000, H: 800}, 8)
	for _, tt := range []struct {
		side  Direction
		index int
	}{{DirLeft, 0}, {DirRight, 1}} {
		d, ok := DividerBeside(root, dividers, "b", tt.side)
		if !ok || d.SplitID != "R" || d.Index != tt.index {
			t.Errorf("the middle pane's %s divider = %s/%d, %v; want R/%d", tt.side, d.SplitID, d.Index, ok, tt.index)
		}
	}
	// A tree of one pane has no dividers.
	if _, ok := DividerBeside(NewPane("only"), nil, "only", DirRight); ok {
		t.Error("a lone pane has a divider beside it")
	}
}

func TestSizesAt(t *testing.T) {
	area := Rect{W: 1000, H: 800}
	const gap = 8
	side := NewSplit("s", Row, []float64{50, 50}, NewPane("a"), NewPane("b"))
	d := Dividers(side, area, gap)[0]
	if got := d.Middle(); got != 500 {
		t.Fatalf("the divider's middle is at %v, want 500", got)
	}
	tests := []struct {
		name string
		pos  float64
		want []float64
	}{
		{"where it is", 500, []float64{50, 50}},
		{"100 points right", 600, []float64{60.08, 39.92}},
		{"past the left limit", 0, []float64{MinSize, 100 - MinSize}},
		{"past the right limit", 2000, []float64{100 - MinSize, MinSize}},
	}
	for _, tt := range tests {
		got, ok := SizesAt(d, tt.pos, gap)
		if !ok || !near(got, tt.want) {
			t.Errorf("%s: SizesAt = %v, %v; want %v", tt.name, got, ok, tt.want)
		}
	}
	// The sizes put the gap's middle back under pos, give or take a point
	// of rounding.
	sizes, _ := SizesAt(d, 600, gap)
	moved := NewSplit("s", Row, sizes, NewPane("a"), NewPane("b"))
	if got := Dividers(moved, area, gap)[0].Middle(); math.Abs(got-600) > 1 {
		t.Errorf("with %v, the divider's middle is at %v, want 600", sizes, got)
	}
}

func TestSizesAtChangesOnlyItsTwoChildren(t *testing.T) {
	area := Rect{W: 1000, H: 800}
	three := NewSplit("s", Column, []float64{40, 30, 30}, NewPane("a"), NewPane("b"), NewPane("c"))
	d := Dividers(three, area, 4)[1]
	got, ok := SizesAt(d, d.Middle()+80, 4)
	if !ok || got[0] != 40 || got[1] <= 30 || got[2] >= 30 || math.Abs(got[1]+got[2]-60) > 1e-9 {
		t.Errorf("moving the second divider of a column down gave %v, %v", got, ok)
	}
	// Two children that have less than twice MinSize between them can't
	// change.
	tight := DividerRect{SplitID: "s", Index: 0, Dir: Row, Split: Rect{W: 1000, H: 800}, Sizes: []float64{8, 8, 84}}
	if got, ok := SizesAt(tight, 50, 4); ok {
		t.Errorf("SizesAt for 8 and 8 = %v, want none", got)
	}
	empty := DividerRect{SplitID: "s", Index: 0, Dir: Row, Split: Rect{}, Sizes: []float64{50, 50}}
	if got, ok := SizesAt(empty, 0, 0); ok {
		t.Errorf("SizesAt for a split with no length = %v, want none", got)
	}
	wrong := DividerRect{SplitID: "s", Index: 1, Dir: Row, Split: Rect{W: 1000, H: 800}, Sizes: []float64{50, 50}}
	if got, ok := SizesAt(wrong, 500, 4); ok {
		t.Errorf("SizesAt for a divider past the last child = %v, want none", got)
	}
}

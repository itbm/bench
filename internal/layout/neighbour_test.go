package layout

import "testing"

func TestNeighbour(t *testing.T) {
	area := Rect{0, 25, 2560, 1415}
	// p1 on the left; p2 above p3 on the right.
	nested := Frames(NewSplit("s1", Row, []float64{65, 35},
		NewPane("p1"),
		NewSplit("s2", Column, []float64{50, 50}, NewPane("p2"), NewPane("p3"))), area, 8)
	grid := Frames(mustBuild(t, PresetGrid, nil, NewPane("p1"), NewPane("p2"), NewPane("p3"), NewPane("p4")), Rect{0, 0, 1000, 800}, 8)
	// With no gap, neighbours touch, and rounding can make them overlap by
	// a point.
	touching := []PaneRect{
		{ID: "a", Rect: Rect{0, 0, 101, 100}},
		{ID: "b", Rect: Rect{100, 0, 100, 100}},
		{ID: "c", Rect: Rect{98, 0, 100, 100}},
	}
	// A grid of four on a wide screen whose bottom-right pane was split at
	// its right edge into x and q: x, bottom middle, has a nearer centre to
	// p1's than p2, which is beside p1.
	splitGrid := Frames(NewSplit("s1", Column, []float64{50, 50},
		NewSplit("s2", Row, []float64{50, 50}, NewPane("p1"), NewPane("p2")),
		NewSplit("s3", Row, []float64{50, 25, 25}, NewPane("p3"), NewPane("x"), NewPane("q"))), area, 8)
	// A thin pane beside a tall one, and a far one in line with its centre.
	far := []PaneRect{
		{ID: "tall", Rect: Rect{0, 0, 100, 1000}},
		{ID: "thin", Rect: Rect{108, 0, 100, 100}},
		{ID: "far", Rect: Rect{300, 450, 100, 100}},
	}
	// Nothing beside a: the nearest centre wins.
	diagonal := []PaneRect{
		{ID: "a", Rect: Rect{0, 0, 100, 100}},
		{ID: "b", Rect: Rect{200, 200, 100, 100}},
		{ID: "c", Rect: Rect{400, 150, 100, 100}},
	}
	tests := []struct {
		name   string
		frames []PaneRect
		pane   string
		dir    Direction
		want   string
		ok     bool
	}{
		{"nested, right of p1: p2 is a point nearer than p3", nested, "p1", DirRight, "p2", true},
		{"nested, left of p2", nested, "p2", DirLeft, "p1", true},
		{"nested, left of p3", nested, "p3", DirLeft, "p1", true},
		{"nested, below p2", nested, "p2", DirDown, "p3", true},
		{"nested, above p3", nested, "p3", DirUp, "p2", true},
		{"nested, above p1", nested, "p1", DirUp, "", false},
		{"nested, right of p2", nested, "p2", DirRight, "", false},
		{"grid, right of p1", grid, "p1", DirRight, "p2", true},
		{"grid, below p1", grid, "p1", DirDown, "p3", true},
		{"grid, left of p4", grid, "p4", DirLeft, "p3", true},
		{"grid, above p4", grid, "p4", DirUp, "p2", true},
		{"grid, left of p1", grid, "p1", DirLeft, "", false},
		{"split grid, right of p1: p2 beside beats x diagonally across", splitGrid, "p1", DirRight, "p2", true},
		{"split grid, below p1", splitGrid, "p1", DirDown, "p3", true},
		{"split grid, above q", splitGrid, "q", DirUp, "p2", true},
		{"split grid, left of q", splitGrid, "q", DirLeft, "x", true},
		{"thin pane beside beats a far one in line", far, "tall", DirRight, "thin", true},
		{"nothing beside: nearest centre", diagonal, "a", DirRight, "b", true},
		{"touching, 1 pt overlap allowed", touching, "a", DirRight, "b", true},
		{"touching, 1 pt overlap allowed the other way", touching, "b", DirLeft, "a", true},
		{"unknown pane", grid, "p9", DirRight, "", false},
		{"unknown direction", grid, "p1", "across", "", false},
		{"no frames", nil, "p1", DirRight, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Neighbour(tt.frames, tt.pane, tt.dir)
			if got != tt.want || ok != tt.ok {
				t.Errorf("Neighbour(%q, %q) = %q, %v; want %q, %v", tt.pane, tt.dir, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNeighbourNearestCentre(t *testing.T) {
	// Three panes to the right of a tall one: the middle one is nearest.
	frames := []PaneRect{
		{ID: "left", Rect: Rect{0, 0, 100, 300}},
		{ID: "top", Rect: Rect{108, 0, 100, 92}},
		{ID: "middle", Rect: Rect{108, 100, 100, 100}},
		{ID: "bottom", Rect: Rect{108, 208, 100, 92}},
		{ID: "far", Rect: Rect{300, 100, 100, 100}},
	}
	if got, ok := Neighbour(frames, "left", DirRight); got != "middle" || !ok {
		t.Errorf("Neighbour = %q, %v; want middle", got, ok)
	}
	if got, ok := Neighbour(frames, "far", DirLeft); got != "middle" || !ok {
		t.Errorf("Neighbour = %q, %v; want middle", got, ok)
	}
}

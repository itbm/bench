package layout

import (
	"errors"
	"math"
	"testing"
)

func TestValidate(t *testing.T) {
	pane := func(id string, items ...string) *Node { return NewPane(id, items...) }
	row := func(id string, children ...*Node) *Node { return NewSplit(id, Row, nil, children...) }
	tests := []struct {
		name string
		tree func() *Node
		ok   bool
	}{
		{"a pane", func() *Node { return pane("p1", "w1") }, true},
		{"an empty pane", func() *Node { return pane("p1") }, true},
		{"a nested tree", sample, true},
		{"every preset", func() *Node { return mustBuild(t, PresetGrid, nil) }, true},
		{"a pane showing nothing", func() *Node { p := pane("p1", "w1"); p.Active = ""; return p }, true},
		{"nil", func() *Node { return nil }, false},
		{"unknown type", func() *Node { return &Node{Type: "box", ID: "b1"} }, false},
		{"no type", func() *Node { return &Node{ID: "b1"} }, false},
		{"no ID", func() *Node { return pane("") }, false},
		{"a child with no ID", func() *Node { return row("s1", pane("p1"), pane("")) }, false},
		{"repeated pane ID", func() *Node { return row("s1", pane("p1"), pane("p1")) }, false},
		{"a pane with a split's ID", func() *Node { return row("s1", pane("s1"), pane("p2")) }, false},
		{"a tree that contains itself", func() *Node {
			s := row("s1", pane("p1"), pane("p2"))
			s.Children[1] = s
			return s
		}, false},
		{"bad direction", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Dir = "diagonal"; return s }, false},
		{"no direction", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Dir = ""; return s }, false},
		{"one child", func() *Node { return row("s1", pane("p1")) }, false},
		{"no children", func() *Node { return row("s1") }, false},
		{"a missing child", func() *Node { return row("s1", pane("p1"), nil) }, false},
		{"too few sizes", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Sizes = []float64{100}; return s }, false},
		{"too many sizes", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Sizes = []float64{50, 25, 25}; return s }, false},
		{"a zero size", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Sizes = []float64{0, 100}; return s }, false},
		{"a NaN size", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Sizes = []float64{math.NaN(), 100}; return s }, false},
		{"a split with items", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Items = []string{"w1"}; return s }, false},
		{"a split showing an item", func() *Node { s := row("s1", pane("p1"), pane("p2")); s.Active = "w1"; return s }, false},
		{"a pane with children", func() *Node { p := pane("p1"); p.Children = []*Node{pane("p2")}; return p }, false},
		{"an item in two panes", func() *Node { return row("s1", pane("p1", "w1"), pane("p2", "w1")) }, false},
		{"an item twice in a pane", func() *Node { return pane("p1", "w1", "w1") }, false},
		{"an item with no ID", func() *Node { return pane("p1", "w1", "") }, false},
		{"showing an item it doesn't hold", func() *Node { p := pane("p1", "w1"); p.Active = "w2"; return p }, false},
		{"an empty pane showing an item", func() *Node { p := pane("p1"); p.Active = "w1"; return p }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.tree())
			if (err == nil) != tt.ok {
				t.Fatalf("Validate = %v, want ok %v", err, tt.ok)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Errorf("Validate error %v doesn't wrap ErrInvalid", err)
			}
		})
	}
}

func TestValidateMessage(t *testing.T) {
	root := sample()
	root.Children[1].Children[0].Active = "w9"
	want := `invalid layout tree: pane "p2" shows "w9", which isn't one of its items`
	if err := Validate(root); err == nil || err.Error() != want {
		t.Errorf("Validate = %v, want %q", err, want)
	}
}

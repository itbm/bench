package layout

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

// counter returns a NewID that makes "np1", "ns2", and so on, which can't
// clash with the hand-written IDs in the tests.
func counter() NewID {
	n := 0
	return func(nodeType string) string {
		n++
		if nodeType == TypeSplit {
			return fmt.Sprintf("ns%d", n)
		}
		return fmt.Sprintf("np%d", n)
	}
}

// mustBuild builds a preset or fails the test.
func mustBuild(t *testing.T, presetID string, sizes []float64, panes ...*Node) *Node {
	t.Helper()
	root, err := Build(presetID, panes, sizes, counter())
	if err != nil {
		t.Fatalf("Build(%q): %v", presetID, err)
	}
	return root
}

// mustValidate fails the test if root isn't valid.
func mustValidate(t *testing.T, root *Node) {
	t.Helper()
	if err := Validate(root); err != nil {
		t.Fatalf("Validate: %v\ntree: %s", err, describe(root))
	}
}

// near reports whether two float slices are equal within a small tolerance.
func near(a, b []float64) bool {
	return slices.EqualFunc(a, b, func(x, y float64) bool { return math.Abs(x-y) < 1e-9 })
}

// describe returns a compact text form of a tree, for failure messages.
func describe(n *Node) string {
	switch {
	case n == nil:
		return "<nil>"
	case n.IsPane():
		return fmt.Sprintf("%s%v*%s", n.ID, n.Items, n.Active)
	}
	s := fmt.Sprintf("%s:%s%v(", n.ID, n.Dir, n.Sizes)
	for i, c := range n.Children {
		if i > 0 {
			s += " "
		}
		s += describe(c)
	}
	return s + ")"
}

// rects returns the rectangles of prs.
func rects(prs []PaneRect) []Rect {
	out := make([]Rect, len(prs))
	for i, pr := range prs {
		out[i] = pr.Rect
	}
	return out
}

// ids returns the IDs of nodes.
func ids(nodes []*Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

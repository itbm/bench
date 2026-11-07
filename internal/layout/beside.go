package layout

import (
	"math"
	"slices"
)

// DividerBeside returns the divider along one side of a pane, from the
// dividers that Dividers returns for root: the gap between the pane, or
// the split that holds it, and the child next to it on that side, in the
// split nearest the pane that runs that way (a row for left and right, a
// column for up and down) and has a child on that side. It reports false
// when the pane isn't in root, the side is unknown, or that side of the
// pane is the edge of the pane area, with no pane beyond it.
func DividerBeside(root *Node, dividers []DividerRect, paneID string, side Direction) (DividerRect, bool) {
	var after bool
	switch side {
	case DirLeft, DirUp:
	case DirRight, DirDown:
		after = true
	default:
		return DividerRect{}, false
	}
	across := side == DirLeft || side == DirRight
	path := pathTo(root, paneID)
	if len(path) == 0 || !path[len(path)-1].node.IsPane() {
		return DividerRect{}, false
	}
	// From the split nearest the pane out to the root.
	for k := len(path) - 2; k >= 0; k-- {
		split, branch := path[k].node, path[k+1].index
		if (split.Dir == Column) == across {
			continue // It runs the other way.
		}
		index := branch - 1
		if after {
			index = branch
		}
		if index < 0 || index >= len(split.Children)-1 {
			continue // The pane's branch is at that end of it.
		}
		i := slices.IndexFunc(dividers, func(d DividerRect) bool { return d.SplitID == split.ID && d.Index == index })
		if i < 0 {
			return DividerRect{}, false
		}
		return dividers[i], true
	}
	return DividerRect{}, false
}

// pathStep is a node on the way down a tree to another, and its index in
// its parent: -1 for the root.
type pathStep struct {
	node  *Node
	index int
}

// pathTo returns the nodes from root down to the node with the given ID,
// both included, or nil if it isn't in the tree.
func pathTo(root *Node, id string) []pathStep {
	var walk func(n *Node, index int) []pathStep
	walk = func(n *Node, index int) []pathStep {
		if n == nil {
			return nil
		}
		if n.ID == id {
			return []pathStep{{n, index}}
		}
		if !n.IsSplit() {
			return nil
		}
		for i, c := range n.Children {
			if rest := walk(c, i); rest != nil {
				return append([]pathStep{{n, index}}, rest...)
			}
		}
		return nil
	}
	return walk(root, -1)
}

// Middle returns where the middle of a divider is, across its split: x
// for a row, y for a column, in the coordinates of its frames.
func (d DividerRect) Middle() float64 {
	if d.Dir == Column {
		return d.Rect.Y + d.Rect.H/2
	}
	return d.Rect.X + d.Rect.W/2
}

// SizesAt returns the sizes of a divider's split with the divider's middle
// at pos, across the split (x for a row, y for a column), with gap points
// between panes, as dragging the divider gives them (sizesAt in the
// frontend's drag.js). Positions map to sizes through Frame, so that the
// gap between the divider's two children is centred on pos. Only those two
// children change, and each stays at MinSize or more: a pos beyond that
// gives the limit. The two new sizes are rounded to hundredths. SizesAt
// reports false when the two children have less than twice MinSize between
// them, or the split has no length.
func SizesAt(d DividerRect, pos, gap float64) ([]float64, bool) {
	start, length := d.Split.X, d.Split.W
	if d.Dir == Column {
		start, length = d.Split.Y, d.Split.H
	}
	i := d.Index
	if !(length+gap > 0) || i < 0 || i+1 >= len(d.Sizes) || math.IsNaN(pos) {
		return nil, false
	}
	at := (pos - start + gap/2) / (length + gap) * 100
	var before float64
	for _, s := range d.Sizes[:i] {
		before += s
	}
	pair := d.Sizes[i] + d.Sizes[i+1]
	if !(pair >= 2*MinSize) {
		return nil, false
	}
	a := hundredths(min(max(at-before, MinSize), pair-MinSize))
	sizes := slices.Clone(d.Sizes)
	sizes[i] = a
	sizes[i+1] = hundredths(pair - a)
	return sizes, true
}

// hundredths rounds a size to hundredths of a percentage point, as the
// frontend's divider drags do.
func hundredths(v float64) float64 {
	return math.Round(v*100) / 100
}

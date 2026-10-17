package layout

import (
	"errors"
	"slices"
)

// ErrNotFound is wrapped by the errors this package returns when a pane,
// split, or item isn't in the tree.
var ErrNotFound = errors.New("not found")

// NewPane returns a pane holding items, showing the first.
func NewPane(id string, items ...string) *Node {
	p := &Node{Type: TypePane, ID: id}
	if len(items) > 0 {
		p.Items = slices.Clone(items)
		p.Active = p.Items[0]
	}
	return p
}

// NewSplit returns a split. Sizes are normalised to len(children) entries
// that add up to 100.
func NewSplit(id string, dir Dir, sizes []float64, children ...*Node) *Node {
	return &Node{
		Type:     TypeSplit,
		ID:       id,
		Dir:      dir,
		Sizes:    Normalise(sizes, len(children)),
		Children: slices.Clone(children),
	}
}

// Clone returns a deep copy of n.
func Clone(n *Node) *Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Sizes = slices.Clone(n.Sizes)
	c.Items = slices.Clone(n.Items)
	if n.Children != nil {
		c.Children = make([]*Node, len(n.Children))
		for i, child := range n.Children {
			c.Children[i] = Clone(child)
		}
	}
	return &c
}

// walk calls fn for n and every node below it, depth first with children in
// order. It only descends into splits, and skips nil children.
func walk(n *Node, fn func(*Node)) {
	if n == nil {
		return
	}
	fn(n)
	if !n.IsSplit() {
		return
	}
	for _, child := range n.Children {
		walk(child, fn)
	}
}

// Leaves returns the panes in reading order (depth first, children in order).
func Leaves(n *Node) []*Node {
	var panes []*Node
	walk(n, func(m *Node) {
		if m.IsPane() {
			panes = append(panes, m)
		}
	})
	return panes
}

// Items returns every item in the tree, in reading order.
func Items(n *Node) []string {
	var items []string
	for _, p := range Leaves(n) {
		items = append(items, p.Items...)
	}
	return items
}

// Find returns the node with the given ID (a pane or a split), its parent, and
// its index in the parent. Parent is nil for the root. Node is nil if there's
// no such node. The index is -1 when there's no parent.
func Find(root *Node, id string) (node, parent *Node, index int) {
	if root == nil {
		return nil, nil, -1
	}
	if root.ID == id {
		return root, nil, -1
	}
	return findBelow(root, id)
}

// findBelow looks for id among the descendants of n.
func findBelow(n *Node, id string) (node, parent *Node, index int) {
	if !n.IsSplit() {
		return nil, nil, -1
	}
	for i, child := range n.Children {
		if child == nil {
			continue
		}
		if child.ID == id {
			return child, n, i
		}
		if node, parent, index := findBelow(child, id); node != nil {
			return node, parent, index
		}
	}
	return nil, nil, -1
}

// parentOf returns the split holding target (compared by pointer) and
// target's index in it, or nil if target is root or isn't in the tree.
func parentOf(n, target *Node) (*Node, int) {
	if !n.IsSplit() {
		return nil, -1
	}
	for i, child := range n.Children {
		if child == target {
			return n, i
		}
		if parent, index := parentOf(child, target); parent != nil {
			return parent, index
		}
	}
	return nil, -1
}

// replaceNode puts repl where old is in the tree and returns the new root.
func replaceNode(root, old, repl *Node) *Node {
	if root == old {
		return repl
	}
	if parent, index := parentOf(root, old); parent != nil {
		parent.Children[index] = repl
	}
	return root
}

// PaneOf returns the pane holding an item, or nil.
func PaneOf(root *Node, itemID string) *Node {
	for _, p := range Leaves(root) {
		if slices.Contains(p.Items, itemID) {
			return p
		}
	}
	return nil
}

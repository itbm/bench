package layout

import (
	"errors"
	"fmt"
	"slices"
)

// The editing functions change the tree they're given in place, and those
// that can replace the root return the new root. Nothing in this package is
// shared between trees, so callers only need to stop two goroutines editing
// the same tree at once.

// findPane returns the pane with the given ID, its parent, and its index in
// the parent.
func findPane(root *Node, id string) (pane, parent *Node, index int, err error) {
	pane, parent, index = Find(root, id)
	if pane == nil {
		return nil, nil, -1, fmt.Errorf("pane %q: %w", id, ErrNotFound)
	}
	if !pane.IsPane() {
		return nil, nil, -1, fmt.Errorf("%q is a split, not a pane", id)
	}
	return pane, parent, index, nil
}

// Insert appends an item to a pane and shows it. It's an error if the item is
// already in the tree, because an item can only be in one pane.
func Insert(root *Node, paneID, itemID string) error {
	if itemID == "" {
		return errors.New("can't insert an item with no ID")
	}
	p, _, _, err := findPane(root, paneID)
	if err != nil {
		return fmt.Errorf("insert %q: %w", itemID, err)
	}
	if q := PaneOf(root, itemID); q != nil {
		return fmt.Errorf("insert %q: it's already in pane %q", itemID, q.ID)
	}
	p.Items = append(p.Items, itemID)
	p.Active = itemID
	return nil
}

// TakeItem removes an item from its pane. If the pane was showing it, the pane
// shows the item that took its place, or else the one before. If that empties
// the pane and the pane isn't the root, the pane is removed and its parent
// closes up (see RemovePane). It returns the new root and the ID of the pane
// the item was in, or ok=false if no pane holds the item.
func TakeItem(root *Node, itemID string) (newRoot *Node, fromPane string, ok bool) {
	p := PaneOf(root, itemID)
	if p == nil {
		return root, "", false
	}
	i := slices.Index(p.Items, itemID)
	p.Items = slices.Delete(p.Items, i, i+1)
	if p.Active == itemID {
		switch {
		case i < len(p.Items):
			p.Active = p.Items[i]
		case i > 0:
			p.Active = p.Items[i-1]
		default:
			p.Active = ""
		}
	}
	if len(p.Items) == 0 {
		p.Items = nil
		// RemovePane leaves the root pane alone. It can only fail if
		// another node has p's ID, and then the tree is left as it is.
		if r, err := RemovePane(root, p.ID); err == nil {
			root = r
		}
	}
	return root, p.ID, true
}

// RemovePane removes a pane and returns the new root. Its parent split's
// remaining sizes are renormalised to 100; a split left with one child is
// replaced by that child. Removing the root pane does nothing, because the
// last pane in a tab stays.
func RemovePane(root *Node, paneID string) (*Node, error) {
	_, parent, i, err := findPane(root, paneID)
	if err != nil {
		return root, fmt.Errorf("remove pane: %w", err)
	}
	if parent == nil {
		return root, nil
	}
	sizes := Normalise(parent.Sizes, len(parent.Children))
	parent.Children = slices.Delete(parent.Children, i, i+1)
	parent.Sizes = Normalise(slices.Delete(sizes, i, i+1), len(parent.Children))
	if len(parent.Children) == 1 {
		return replaceNode(root, parent, parent.Children[0]), nil
	}
	return root, nil
}

// zoneDir returns the split direction for an edge zone, and whether the new
// node goes before the pane.
func zoneDir(zone Zone) (dir Dir, before, ok bool) {
	switch zone {
	case Left:
		return Row, true, true
	case Right:
		return Row, false, true
	case Top:
		return Column, true, true
	case Bottom:
		return Column, false, true
	}
	return "", false, false
}

// Split puts fresh beside a pane, on the side given by zone (not Center), and
// returns the new root. If the pane's parent split already runs in that
// direction, fresh becomes a sibling, and the pane's size is shared equally
// between the two. Otherwise the pane is replaced by a new 50/50 split
// holding both. newID names the new split.
//
// Fresh must be a valid tree whose IDs and items aren't already in root.
func Split(root *Node, paneID string, zone Zone, fresh *Node, newID NewID) (*Node, error) {
	dir, before, ok := zoneDir(zone)
	if !ok {
		return root, fmt.Errorf("split pane %q: zone %q isn't left, right, top, or bottom", paneID, zone)
	}
	p, parent, i, err := findPane(root, paneID)
	if err != nil {
		return root, fmt.Errorf("split pane: %w", err)
	}
	if err := checkFresh(root, fresh); err != nil {
		return root, fmt.Errorf("split pane %q: %w", paneID, err)
	}
	if parent != nil && parent.Dir == dir {
		sizes := Normalise(parent.Sizes, len(parent.Children))
		half := sizes[i] / 2
		sizes[i] = half
		at := i + 1
		if before {
			at = i
		}
		parent.Children = slices.Insert(parent.Children, at, fresh)
		parent.Sizes = slices.Insert(sizes, at, half)
		return root, nil
	}
	id, err := newSplitID(newID, root, fresh)
	if err != nil {
		return root, fmt.Errorf("split pane %q: %w", paneID, err)
	}
	children := []*Node{p, fresh}
	if before {
		children = []*Node{fresh, p}
	}
	return replaceNode(root, p, NewSplit(id, dir, []float64{50, 50}, children...)), nil
}

// checkFresh checks that fresh is a valid tree that can join root without
// repeating an ID or an item.
func checkFresh(root, fresh *Node) error {
	if fresh == nil {
		return errors.New("there's nothing to put beside it")
	}
	if err := Validate(fresh); err != nil {
		return fmt.Errorf("new node: %w", err)
	}
	ids := idsOf(root)
	var clash error
	walk(fresh, func(n *Node) {
		if clash != nil {
			return
		}
		if ids[n.ID] {
			clash = fmt.Errorf("ID %q is already in the tree", n.ID)
			return
		}
		for _, item := range n.Items {
			if q := PaneOf(root, item); q != nil {
				clash = fmt.Errorf("item %q is already in pane %q", item, q.ID)
				return
			}
		}
	})
	return clash
}

// idsOf returns the IDs of every node in the trees.
func idsOf(trees ...*Node) map[string]bool {
	ids := make(map[string]bool)
	for _, t := range trees {
		walk(t, func(n *Node) { ids[n.ID] = true })
	}
	return ids
}

// newSplitID asks newID for a split ID, and checks it's not empty and not
// already used in any of the trees.
func newSplitID(newID NewID, trees ...*Node) (string, error) {
	if newID == nil {
		return "", errors.New("a new split needs an ID, but there's no ID function")
	}
	id := newID(TypeSplit)
	if id == "" {
		return "", errors.New("the ID function returned an empty split ID")
	}
	if idsOf(trees...)[id] {
		return "", fmt.Errorf("the ID function returned %q, which is already in the tree", id)
	}
	return id, nil
}

// SetSizes sets a split's sizes, which must be one positive number per child.
// They're normalised to add up to 100.
func SetSizes(root *Node, splitID string, sizes []float64) error {
	n, _, _ := Find(root, splitID)
	if n == nil {
		return fmt.Errorf("set sizes: split %q: %w", splitID, ErrNotFound)
	}
	if !n.IsSplit() {
		return fmt.Errorf("set sizes: %q is a pane, not a split", splitID)
	}
	if len(sizes) != len(n.Children) {
		return fmt.Errorf("set sizes: split %q has %d children, but got %d sizes", splitID, len(n.Children), len(sizes))
	}
	if !allPositive(sizes) {
		return fmt.Errorf("set sizes: split %q: sizes must be positive numbers, but got %v", splitID, sizes)
	}
	n.Sizes = Normalise(sizes, len(n.Children))
	return nil
}

package layout

import (
	"errors"
	"fmt"
	"slices"
)

// ErrInvalid is wrapped by every error Validate returns.
var ErrInvalid = errors.New("invalid layout tree")

// Validate checks a tree's structure: known node types, unique non-empty IDs,
// splits with a valid direction, two or more children and one size per child,
// panes with no children, no item in two panes, and each pane's active item
// (if set) among its items.
//
// It also checks that the tree isn't nil, that sizes are positive finite
// numbers, that splits hold no items, and that every item has an ID and
// appears only once.
func Validate(root *Node) error {
	if root == nil {
		return fmt.Errorf("%w: there's no tree", ErrInvalid)
	}
	v := validator{ids: make(map[string]bool), items: make(map[string]string)}
	if err := v.check(root, "the root"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}

// validator walks a tree, remembering the IDs and items it has seen.
type validator struct {
	ids   map[string]bool
	items map[string]string // Item ID to the ID of the pane holding it.
}

// check validates n, which is at the place described by where.
func (v *validator) check(n *Node, where string) error {
	if n == nil {
		return fmt.Errorf("%s is missing", where)
	}
	if n.ID == "" {
		return fmt.Errorf("%s has no ID", where)
	}
	// Checking for repeats before going deeper also stops a tree that
	// contains itself.
	if v.ids[n.ID] {
		return fmt.Errorf("ID %q is used more than once", n.ID)
	}
	v.ids[n.ID] = true
	switch n.Type {
	case TypeSplit:
		return v.checkSplit(n)
	case TypePane:
		return v.checkPane(n)
	}
	return fmt.Errorf("node %q has unknown type %q", n.ID, n.Type)
}

// checkSplit validates a split and everything below it.
func (v *validator) checkSplit(n *Node) error {
	if n.Dir != Row && n.Dir != Column {
		return fmt.Errorf("split %q has direction %q, but it must be row or column", n.ID, n.Dir)
	}
	if len(n.Children) < 2 {
		return fmt.Errorf("split %q has %d children, but it needs at least 2", n.ID, len(n.Children))
	}
	if len(n.Sizes) != len(n.Children) {
		return fmt.Errorf("split %q has %d sizes for %d children", n.ID, len(n.Sizes), len(n.Children))
	}
	if !allPositive(n.Sizes) {
		return fmt.Errorf("split %q has sizes %v, but they must be positive numbers", n.ID, n.Sizes)
	}
	if len(n.Items) > 0 || n.Active != "" {
		return fmt.Errorf("split %q holds items, but only panes can", n.ID)
	}
	for i, child := range n.Children {
		if err := v.check(child, fmt.Sprintf("child %d of split %q", i+1, n.ID)); err != nil {
			return err
		}
	}
	return nil
}

// checkPane validates a pane.
func (v *validator) checkPane(n *Node) error {
	if len(n.Children) > 0 {
		return fmt.Errorf("pane %q has children, but panes can't", n.ID)
	}
	for _, item := range n.Items {
		if item == "" {
			return fmt.Errorf("pane %q holds an item with no ID", n.ID)
		}
		if other, seen := v.items[item]; seen {
			if other == n.ID {
				return fmt.Errorf("item %q is in pane %q twice", item, n.ID)
			}
			return fmt.Errorf("item %q is in pane %q and pane %q", item, other, n.ID)
		}
		v.items[item] = n.ID
	}
	if n.Active != "" && !slices.Contains(n.Items, n.Active) {
		return fmt.Errorf("pane %q shows %q, which isn't one of its items", n.ID, n.Active)
	}
	return nil
}

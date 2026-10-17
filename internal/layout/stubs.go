package layout

// This file fixes the package's exported API so that other packages can build
// against it. Each function is implemented in its own file; delete this file
// once they all are.

// NewPane returns a pane holding items, showing the first.
func NewPane(id string, items ...string) *Node { panic("not implemented") }

// NewSplit returns a split. Sizes are normalised to len(children) entries
// that add up to 100.
func NewSplit(id string, dir Dir, sizes []float64, children ...*Node) *Node {
	panic("not implemented")
}

// Presets returns the six presets in editor order.
func Presets() []Preset { panic("not implemented") }

// LookupPreset returns the preset with the given ID.
func LookupPreset(id string) (Preset, bool) { panic("not implemented") }

// Build returns the tree for a preset, placing panes in reading order and
// padding with new empty panes. Sizes apply to the top-level split; nil or a
// wrong length means the preset's defaults. It's an error to pass more panes
// than the preset has, or an unknown preset.
func Build(presetID string, panes []*Node, sizes []float64, newID NewID) (*Node, error) {
	panic("not implemented")
}

// ApplyPreset replaces a tree with a preset, keeping every item in order:
// old pane 1's items go into new pane 1, and so on, and the extra panes'
// items join the last new pane. New panes reuse the old panes' IDs in reading
// order where they can, so focus survives. Each new pane keeps the active
// item of the first old pane that lands in it.
func ApplyPreset(old *Node, presetID string, sizes []float64, newID NewID) (*Node, error) {
	panic("not implemented")
}

// Clone returns a deep copy of n.
func Clone(n *Node) *Node { panic("not implemented") }

// Leaves returns the panes in reading order (depth first, children in order).
func Leaves(n *Node) []*Node { panic("not implemented") }

// Items returns every item in the tree, in reading order.
func Items(n *Node) []string { panic("not implemented") }

// Find returns the node with the given ID (a pane or a split), its parent, and
// its index in the parent. Parent is nil for the root. Node is nil if there's
// no such node.
func Find(root *Node, id string) (node, parent *Node, index int) { panic("not implemented") }

// PaneOf returns the pane holding an item, or nil.
func PaneOf(root *Node, itemID string) *Node { panic("not implemented") }

// Insert appends an item to a pane and shows it.
func Insert(root *Node, paneID, itemID string) error { panic("not implemented") }

// TakeItem removes an item from its pane. If the pane was showing it, the pane
// shows the item that took its place, or else the one before. If that empties
// the pane and the pane isn't the root, the pane is removed and its parent
// closes up (see RemovePane). It returns the new root and the ID of the pane
// the item was in, or ok=false if no pane holds the item.
func TakeItem(root *Node, itemID string) (newRoot *Node, fromPane string, ok bool) {
	panic("not implemented")
}

// RemovePane removes a pane and returns the new root. Its parent split's
// remaining sizes are renormalised to 100; a split left with one child is
// replaced by that child. Removing the root pane does nothing, because the
// last pane in a tab stays.
func RemovePane(root *Node, paneID string) (*Node, error) { panic("not implemented") }

// Split puts fresh beside a pane, on the side given by zone (not Center), and
// returns the new root. If the pane's parent split already runs in that
// direction, fresh becomes a sibling, and the pane's size is shared equally
// between the two. Otherwise the pane is replaced by a new 50/50 split
// holding both. newID names the new split.
func Split(root *Node, paneID string, zone Zone, fresh *Node, newID NewID) (*Node, error) {
	panic("not implemented")
}

// SetSizes sets a split's sizes, which must be one positive number per child.
// They're normalised to add up to 100.
func SetSizes(root *Node, splitID string, sizes []float64) error { panic("not implemented") }

// Normalise returns n sizes that add up to 100, in proportion to sizes. If
// sizes has the wrong length, or any size isn't a positive finite number, it
// returns equal sizes.
func Normalise(sizes []float64, n int) []float64 { panic("not implemented") }

// UnitRects returns each pane's unit rectangle (fractions of the pane area),
// in reading order.
func UnitRects(root *Node) []PaneRect { panic("not implemented") }

// Frame turns a unit rectangle into a frame in the pane area, with gap g
// between panes and at every edge (see the spec's Layout maths).
func Frame(u Rect, area Rect, gap float64) Rect { panic("not implemented") }

// Frames returns each pane's frame, in reading order.
func Frames(root *Node, area Rect, gap float64) []PaneRect { panic("not implemented") }

// Dividers returns the draggable gaps between each split's children.
func Dividers(root *Node, area Rect, gap float64) []DividerRect { panic("not implemented") }

// Body returns the part of a pane's frame below its header, where the item is
// shown: the frame inset by PaneBorder, below HeaderHeight and HeaderDivider.
func Body(frame Rect) Rect { panic("not implemented") }

// Round is floor(v + 0.5), with a tiny allowance for floating-point error.
func Round(v float64) float64 { panic("not implemented") }

// ChooseSizeClass picks the size class: the mode if it's wide or compact,
// otherwise compact when the screen is narrower than compactBelow.
func ChooseSizeClass(mode Mode, screenWidth, compactBelow float64) SizeClass {
	panic("not implemented")
}

// Adapt returns the tree to display for a size class, without changing root.
// Wide, or the Same setting: a copy of root. Stack: a copy whose top-level row
// (if the root is a row) becomes a column. Tabs: a single pane with ID
// CompactPaneID holding every item in reading order, showing compactShown if
// it's in the tree, else the focused pane's active item, else the first.
func Adapt(root *Node, setting SmallScreens, class SizeClass, focusedPane, compactShown string) *Node {
	panic("not implemented")
}

// Neighbour returns the pane next to paneID in a direction: of the panes
// lying wholly on that side (allowing 1 pt of overlap), the one whose centre
// is nearest to paneID's centre.
func Neighbour(frames []PaneRect, paneID string, dir Direction) (string, bool) {
	panic("not implemented")
}

// Validate checks a tree's structure: known node types, unique non-empty IDs,
// splits with a valid direction, two or more children and one size per child,
// panes with no children, no item in two panes, and each pane's active item
// (if set) among its items.
func Validate(root *Node) error { panic("not implemented") }

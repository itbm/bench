package layout

import "slices"

// WideAgainSlack is how many points wider than compactBelow a window in the
// compact size class must grow before Auto chooses the wide one again (see
// AutoSizeClass), so that dragging the window's edge to and fro across
// compactBelow doesn't change the layout each time. The welcome guide's
// tip has the same number (WIDE_AGAIN_SLACK in frontend/js/guide.js).
const WideAgainSlack = 40

// ChooseSizeClass picks the size class: the mode if it's wide or compact,
// otherwise compact when width, the width of Bench's window (its content
// area), is narrower than compactBelow.
func ChooseSizeClass(mode Mode, width, compactBelow float64) SizeClass {
	switch mode {
	case ModeWide:
		return Wide
	case ModeCompact:
		return Compact
	}
	return AutoSizeClass(width, compactBelow, "")
}

// AutoSizeClass is the size class that Auto chooses for a window width
// points wide, given the one it chose last, was ("" for none): compact
// when the width is narrower than compactBelow, and wide when it's at
// least that, except that a window that was compact stays compact until
// it's WideAgainSlack points wider than compactBelow.
func AutoSizeClass(width, compactBelow float64, was SizeClass) SizeClass {
	if width < compactBelow || was == Compact && width < compactBelow+WideAgainSlack {
		return Compact
	}
	return Wide
}

// Adapt returns the tree to display for a size class, without changing root.
// Wide, or the Same setting: a copy of root. Stack: a copy whose top-level row
// (if the root is a row) becomes a column. Tabs: a single pane with ID
// CompactPaneID holding every item in reading order, showing compactShown if
// it's in the tree, else the focused pane's active item, else the first.
//
// An empty setting is treated as Stack, the default for a tab that doesn't
// set one; any other unknown setting is treated as Same. Adapt returns nil
// for a nil root.
func Adapt(root *Node, setting SmallScreens, class SizeClass, focusedPane, compactShown string) *Node {
	if root == nil {
		return nil
	}
	if class != Compact {
		return Clone(root)
	}
	switch setting {
	case Stack, "":
		c := Clone(root)
		if c.IsSplit() && c.Dir == Row {
			c.Dir = Column
		}
		return c
	case Tabs:
		return compactPane(root, focusedPane, compactShown)
	default:
		return Clone(root)
	}
}

// compactPane returns the one pane of the Tabs setting.
func compactPane(root *Node, focusedPane, compactShown string) *Node {
	p := NewPane(CompactPaneID, Items(root)...)
	if compactShown != "" && slices.Contains(p.Items, compactShown) {
		p.Active = compactShown
	} else if active := focusedActive(root, focusedPane); active != "" {
		p.Active = active
	}
	return p
}

// focusedActive returns the focused pane's active item if it's one of the
// pane's items, or "".
func focusedActive(root *Node, focusedPane string) string {
	n, _, _ := Find(root, focusedPane)
	if !n.IsPane() || n.Active == "" || !slices.Contains(n.Items, n.Active) {
		return ""
	}
	return n.Active
}

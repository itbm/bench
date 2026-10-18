package layout

import (
	"math"
	"slices"
)

// unitRect is the whole pane area as a unit rectangle.
var unitRect = Rect{X: 0, Y: 0, W: 1, H: 1}

// roundSlack is added before flooring in Round, so that a value a hair below
// .5 because of floating-point error still rounds up.
const roundSlack = 1e-9

// Round is floor(v + 0.5), with a tiny allowance for floating-point error.
func Round(v float64) float64 {
	return math.Floor(v + 0.5 + roundSlack)
}

// visitUnits calls fn for n and every node below it, depth first with
// children in order, with each node's unit rectangle.
func visitUnits(n *Node, u Rect, fn func(n *Node, u Rect)) {
	if n == nil {
		return
	}
	fn(n, u)
	if !n.IsSplit() {
		return
	}
	for i, cu := range childUnits(n, u) {
		visitUnits(n.Children[i], cu, fn)
	}
}

// childUnits divides a split's unit rectangle u among its children, along
// its direction, by its sizes. A split with any direction but Column is
// treated as a row. The last child always ends at u's far edge, so float
// error can't leave a sliver.
func childUnits(n *Node, u Rect) []Rect {
	sizes := Normalise(n.Sizes, len(n.Children))
	out := make([]Rect, len(sizes))
	var sum float64
	for i, s := range sizes {
		start := sum / 100
		sum += s
		end := sum / 100
		if i == len(sizes)-1 {
			end = 1
		}
		if n.Dir == Column {
			out[i] = Rect{X: u.X, Y: u.Y + start*u.H, W: u.W, H: (end - start) * u.H}
		} else {
			out[i] = Rect{X: u.X + start*u.W, Y: u.Y, W: (end - start) * u.W, H: u.H}
		}
	}
	return out
}

// UnitRects returns each pane's unit rectangle (fractions of the pane area),
// in reading order.
func UnitRects(root *Node) []PaneRect {
	var out []PaneRect
	visitUnits(root, unitRect, func(n *Node, u Rect) {
		if n.IsPane() {
			out = append(out, PaneRect{ID: n.ID, Rect: u})
		}
	})
	return out
}

// Frame turns a unit rectangle into a frame in the pane area, with gap g
// between panes and at every edge (see the spec's Layout maths). A width or
// height that would be negative, in an area too small for its gaps, is 0.
func Frame(u Rect, area Rect, gap float64) Rect {
	half := gap / 2
	x0, w0 := area.X+half, area.W-gap
	y0, h0 := area.Y+half, area.H-gap
	return Rect{
		X: Round(x0 + u.X*w0 + half),
		Y: Round(y0 + u.Y*h0 + half),
		W: max(0, Round(u.W*w0-gap)),
		H: max(0, Round(u.H*h0-gap)),
	}
}

// Frames returns each pane's frame, in reading order.
func Frames(root *Node, area Rect, gap float64) []PaneRect {
	units := UnitRects(root)
	for i := range units {
		units[i].Rect = Frame(units[i].Rect, area, gap)
	}
	return units
}

// Dividers returns the draggable gaps between each split's children: one for
// each neighbouring pair, from the end of child i's frame to the start of
// child i+1's, at least 1 pt across, and spanning the split's frame in the
// other direction. A split's frame comes from its unit rectangle through
// Frame, like a pane's. Splits come depth first in reading order, and each
// split's dividers in order.
//
// A divider less than a point across is widened to 1 pt about its middle.
// Because a split's frame and the frames of the panes inside it are rounded
// separately, a divider beside a nested split can overlap one of that
// split's panes by up to a point.
func Dividers(root *Node, area Rect, gap float64) []DividerRect {
	var out []DividerRect
	visitUnits(root, unitRect, func(n *Node, u Rect) {
		if !n.IsSplit() || len(n.Children) < 2 {
			return
		}
		dir := Row
		if n.Dir == Column {
			dir = Column
		}
		frame := Frame(u, area, gap)
		sizes := Normalise(n.Sizes, len(n.Children))
		units := childUnits(n, u)
		for i := range len(units) - 1 {
			a, b := Frame(units[i], area, gap), Frame(units[i+1], area, gap)
			out = append(out, DividerRect{
				SplitID: n.ID,
				Index:   i,
				Dir:     dir,
				Rect:    gapBetween(dir, a, b, frame),
				Split:   frame,
				Sizes:   slices.Clone(sizes),
			})
		}
	})
	return out
}

// gapBetween returns the gap between neighbouring frames a and b of a split
// running in dir, spanning the split's frame across. When the gap is less
// than a point (with no gap, the frames touch, or overlap by a point after
// rounding), the result is 1 pt across, centred on the middle of the gap, so
// that its centre is still where the two frames meet.
func gapBetween(dir Dir, a, b, split Rect) Rect {
	if dir == Column {
		start, size := span(a.Y+a.H, b.Y)
		return Rect{X: split.X, Y: start, W: split.W, H: size}
	}
	start, size := span(a.X+a.W, b.X)
	return Rect{X: start, Y: split.Y, W: size, H: split.H}
}

// minDivider is the smallest a divider can be across, in points.
const minDivider = 1

// span returns the start and size of the gap between one frame, which ends
// at aEnd, and the next, which starts at bStart: at least minDivider across,
// and centred on the gap's middle when it has to grow.
func span(aEnd, bStart float64) (start, size float64) {
	if bStart-aEnd >= minDivider {
		return aEnd, bStart - aEnd
	}
	return (aEnd+bStart)/2 - minDivider/2.0, minDivider
}

// Body returns the part of a pane's frame below its header, where the item is
// shown: the frame inset by PaneBorder, below HeaderHeight and HeaderDivider.
// A frame too small for its chrome gives a body with no width or height.
func Body(frame Rect) Rect {
	top := float64(PaneBorder + HeaderHeight + HeaderDivider)
	return Rect{
		X: frame.X + PaneBorder,
		Y: frame.Y + top,
		W: max(0, frame.W-2*PaneBorder),
		H: max(0, frame.H-top-PaneBorder),
	}
}

package mac

import (
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// The drop guide (Platform.ShowDropGuide) is a panel over the pane that a
// window is dragged to, above every window, which lets the mouse through.
// guideMarks lays out what the native side draws in it, in the panel's own
// coordinates, which are AppKit's (bottom-left origin, y up, points): the
// part of the pane the window would take, a mark in the middle of the
// pane's body and, if the pane can be split, one near the middle of each
// of its edges, the one the pointer is on highlighted, and a label below
// the middle mark that says what a drop does. The native side draws the
// panel's outline itself.

// Kinds of mark, and the side each mark stands for. Each has a BENCH_GUIDE_
// constant in bridge_darwin.h with the same value.
const (
	guideArea   = 0 // The part of the pane the window would take.
	guideMark   = 1 // A target.
	guideMarkOn = 2 // The target the pointer is on.
	guideLabel  = 3 // Where the label goes.

	guideMiddle = 0
	guideLeft   = 1
	guideRight  = 2
	guideTop    = 3
	guideBottom = 4
)

// The marks' sizes, in points.
const (
	guideMarkSize    = 40
	guideEdgeInset   = 16
	guideLabelWidth  = 220
	guideLabelHeight = 24
	guideLabelGap    = 8
)

// guideItem is something the drop guide draws: its kind, the side a mark
// stands for, and where, in the panel's coordinates.
type guideItem struct {
	kind, side int
	rect       layout.Rect
}

// guideSides are the sides of the marks, with the zone each stands for.
var guideSides = []struct {
	side int
	zone layout.Zone
}{
	{guideMiddle, layout.Center},
	{guideLeft, layout.Left},
	{guideRight, layout.Right},
	{guideTop, layout.Top},
	{guideBottom, layout.Bottom},
}

// guideMarks lays out the drop guide for g, in the panel's coordinates:
// the panel covers g.Pane.
func guideMarks(g platform.DropGuide) []guideItem {
	local := func(r layout.Rect) layout.Rect {
		return layout.Rect{X: r.X - g.Pane.X, Y: g.Pane.Y + g.Pane.H - (r.Y + r.H), W: r.W, H: r.H}
	}
	out := []guideItem{{kind: guideArea, side: sideOf(layout.Zone(g.Zone)), rect: local(g.Target)}}
	b := g.Body
	if b.W <= 0 || b.H <= 0 {
		b = g.Pane
	}
	s := float64(guideMarkSize)
	midX, midY := b.X+(b.W-s)/2, b.Y+(b.H-s)/2
	at := map[int]layout.Rect{
		guideMiddle: {X: midX, Y: midY, W: s, H: s},
		guideLeft:   {X: b.X + guideEdgeInset, Y: midY, W: s, H: s},
		guideRight:  {X: b.X + b.W - guideEdgeInset - s, Y: midY, W: s, H: s},
		guideTop:    {X: midX, Y: b.Y + guideEdgeInset, W: s, H: s},
		guideBottom: {X: midX, Y: b.Y + b.H - guideEdgeInset - s, W: s, H: s},
	}
	for _, side := range guideSides {
		if side.side != guideMiddle && !g.Splits {
			continue
		}
		kind := guideMark
		if string(side.zone) == g.Zone {
			kind = guideMarkOn
		}
		out = append(out, guideItem{kind: kind, side: side.side, rect: local(at[side.side])})
	}
	w := min(float64(guideLabelWidth), b.W-2*guideEdgeInset)
	if w > 0 && g.Label != "" {
		label := layout.Rect{X: b.X + (b.W-w)/2, Y: midY + s + guideLabelGap, W: w, H: guideLabelHeight}
		out = append(out, guideItem{kind: guideLabel, side: guideMiddle, rect: local(label)})
	}
	return out
}

// sideOf returns the side of the mark that stands for a zone.
func sideOf(zone layout.Zone) int {
	for _, s := range guideSides {
		if s.zone == zone {
			return s.side
		}
	}
	return guideMiddle
}

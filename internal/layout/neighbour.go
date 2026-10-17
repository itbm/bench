package layout

import "math"

// overlapAllowance is how far, in points, a pane may overlap the one it's
// beside and still count as wholly on that side.
const overlapAllowance = 1

// Neighbour returns the pane next to paneID in a direction: of the panes
// lying wholly on that side (allowing 1 pt of overlap), the one whose centre
// is nearest to paneID's centre. If two are equally near, the first in frames
// wins. It reports false if paneID isn't in frames, the direction is unknown,
// or there's no pane on that side.
func Neighbour(frames []PaneRect, paneID string, dir Direction) (string, bool) {
	me, ok := frameOf(frames, paneID)
	if !ok {
		return "", false
	}
	best, bestDist, found := "", math.Inf(1), false
	for _, f := range frames {
		if f.ID == paneID || !onSide(me, f.Rect, dir) {
			continue
		}
		if d := centreDistance(me, f.Rect); !found || d < bestDist {
			best, bestDist, found = f.ID, d, true
		}
	}
	return best, found
}

// frameOf returns the frame of the pane with the given ID.
func frameOf(frames []PaneRect, id string) (Rect, bool) {
	for _, f := range frames {
		if f.ID == id {
			return f.Rect, true
		}
	}
	return Rect{}, false
}

// onSide reports whether q lies wholly on the dir side of r, allowing
// overlapAllowance.
func onSide(r, q Rect, dir Direction) bool {
	switch dir {
	case DirRight:
		return q.X >= r.X+r.W-overlapAllowance
	case DirLeft:
		return q.X+q.W <= r.X+overlapAllowance
	case DirDown:
		return q.Y >= r.Y+r.H-overlapAllowance
	case DirUp:
		return q.Y+q.H <= r.Y+overlapAllowance
	}
	return false
}

// centreDistance returns the distance between the centres of a and b.
func centreDistance(a, b Rect) float64 {
	return math.Hypot((b.X+b.W/2)-(a.X+a.W/2), (b.Y+b.H/2)-(a.Y+a.H/2))
}

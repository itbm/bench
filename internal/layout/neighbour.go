package layout

import "math"

// overlapAllowance is how far, in points, a pane may overlap the one it's
// beside and still count as wholly on that side.
const overlapAllowance = 1

// Neighbour returns the pane next to paneID in a direction. Of the panes
// lying wholly on that side (allowing 1 pt of overlap), the ones beside
// paneID, which also overlap it on the other axis, come first: of those, the
// ones whose facing edge is nearest (within 1 pt), and of them the one whose
// centre is nearest to paneID's centre. Only if no pane on that side is
// beside paneID does Neighbour take the nearest centre of them all. If two
// are equally near, the first in frames wins. It reports false if paneID
// isn't in frames, the direction is unknown, or there's no pane on that side.
func Neighbour(frames []PaneRect, paneID string, dir Direction) (string, bool) {
	me, ok := frameOf(frames, paneID)
	if !ok {
		return "", false
	}
	// Ranking by centre alone let a narrow pane diagonally across, or a far
	// one, beat the pane right beside, so first find the nearest edge of the
	// panes beside.
	nearestEdge, beside := math.Inf(1), false
	for _, f := range frames {
		if f.ID != paneID && onSide(me, f.Rect, dir) && overlapsAcross(me, f.Rect, dir) {
			nearestEdge, beside = min(nearestEdge, edgeDistance(me, f.Rect, dir)), true
		}
	}
	best, bestDist, found := "", math.Inf(1), false
	for _, f := range frames {
		if f.ID == paneID || !onSide(me, f.Rect, dir) {
			continue
		}
		if beside && (!overlapsAcross(me, f.Rect, dir) || edgeDistance(me, f.Rect, dir) > nearestEdge+overlapAllowance) {
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

// overlapsAcross reports whether q overlaps r by more than overlapAllowance
// on the axis across dir: vertically for left and right, horizontally for up
// and down.
func overlapsAcross(r, q Rect, dir Direction) bool {
	switch dir {
	case DirLeft, DirRight:
		return min(r.Y+r.H, q.Y+q.H)-max(r.Y, q.Y) > overlapAllowance
	case DirUp, DirDown:
		return min(r.X+r.W, q.X+q.W)-max(r.X, q.X) > overlapAllowance
	}
	return false
}

// edgeDistance returns the distance from r's edge on the dir side to q's
// facing edge.
func edgeDistance(r, q Rect, dir Direction) float64 {
	switch dir {
	case DirRight:
		return q.X - (r.X + r.W)
	case DirLeft:
		return r.X - (q.X + q.W)
	case DirDown:
		return q.Y - (r.Y + r.H)
	case DirUp:
		return r.Y - (q.Y + q.H)
	}
	return math.Inf(1)
}

// centreDistance returns the distance between the centres of a and b.
func centreDistance(a, b Rect) float64 {
	return math.Hypot((b.X+b.W/2)-(a.X+a.W/2), (b.Y+b.H/2)-(a.Y+a.H/2))
}

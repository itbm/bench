package layout

// Drop is where a dragged item lands for a point over a pane.
type Drop struct {
	// Strip is set for a point in the pane's header, which holds its strip
	// of sub-tabs: the item goes between two sub-tabs there, and the pane
	// never splits.
	Strip bool
	// Zone is where on the pane's body: an edge, which splits the pane
	// there, or Center, which joins it. It's Center for a point in the
	// header.
	Zone Zone
}

// EdgeShare is how far into a pane's body, as a share of its width or
// height, a drop on one of its edges splits the pane there: the outer
// quarter on each side.
const EdgeShare = 0.25

// DropAt says where a dragged item lands for a point (x, y) over a pane's
// frame, whose header is header points tall (0 for none). It's the rule
// that the frontend follows as a sub-tab is dragged (drag.js), for Go to
// follow when it finds drop targets itself (decision 38 in
// docs/architecture.md). The header, buttons and all, is the pane's strip
// of sub-tabs, which never splits. On the body, the outer quarter on a side
// splits the pane there, the nearest side winning, and the middle joins it.
// With splits false, as in the "tabs" view or while a small screen turns
// splits off, all of the body joins the pane. The "tabs" view's strip is in
// the title bar, outside any pane's frame, and the place among a strip's
// sub-tabs depends on how wide the frontend draws them, so neither is
// decided here.
func DropAt(frame Rect, header, x, y float64, splits bool) Drop {
	body := Body(frame, header)
	if header > 0 && y < body.Y {
		return Drop{Strip: true, Zone: Center}
	}
	if !splits || body.W <= 0 || body.H <= 0 {
		return Drop{Zone: Center}
	}
	px, py := (x-body.X)/body.W, (y-body.Y)/body.H
	// The order breaks ties as drag.js does: left, right, top, bottom.
	zone, dist := Left, px
	for _, e := range []struct {
		zone Zone
		dist float64
	}{{Right, 1 - px}, {Top, py}, {Bottom, 1 - py}} {
		if e.dist < dist {
			zone, dist = e.zone, e.dist
		}
	}
	if dist < EdgeShare {
		return Drop{Zone: zone}
	}
	return Drop{Zone: Center}
}

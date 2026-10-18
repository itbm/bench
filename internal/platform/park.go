package platform

import "github.com/itbm/bench/internal/layout"

// ParkLine is how much of a parked window stays on screen, in points. macOS
// doesn't allow a window fully off-screen.
const ParkLine = 1

// ParkPlace returns where to park a window of the given size on target:
// below the bottom edge, outside the visible area, so that only a 1-point
// line stays on screen (macOS doesn't allow a window fully off-screen). It
// reports false if every such place would show part of the window on
// another display; the window should then be minimised instead.
//
// It prefers the bottom-right corner, and uses the bottom-left one if
// another display touches the bottom-right corner. If both corners touch
// another display, it slides the window along the bottom edge to the place
// that keeps clear of every other display with the shortest line on screen
// (still 1 point high), preferring the right-hand one of two.
//
// Only size's width and height are used. With F as target's whole Frame, the
// bottom-right corner puts the window's top-left at (F.X+F.W-1, F.Y+F.H-1),
// and the bottom-left corner at (F.X-W+1, F.Y+F.H-1). A place touches
// another display when the parked window would overlap that display's
// frame.
func ParkPlace(screens []Screen, target Screen, size layout.Rect) (layout.Rect, bool) {
	f := target.Frame
	y := f.Y + f.H - ParkLine
	// Every place keeps at least a 1-point overlap with the target, so the
	// window's left edge lies between the two corners'.
	minX, maxX := f.X-size.W+ParkLine, f.X+f.W-ParkLine
	// Along the edge, the line left on screen is shortest at a corner or
	// right beside another display, so those are the only places to try.
	xs := []float64{maxX, minX}
	for _, s := range screens {
		if !sameScreen(s, target) {
			xs = append(xs, s.Frame.X+s.Frame.W, s.Frame.X-size.W)
		}
	}
	var best layout.Rect
	bestLine, found := 0.0, false
	for _, x := range xs {
		if x < minX || x > maxX {
			continue
		}
		r := layout.Rect{X: x, Y: y, W: size.W, H: size.H}
		if touchesOther(screens, target, r) {
			continue
		}
		// Of two places with the same line, the one further right wins,
		// as the bottom-right corner does.
		line := min(x+size.W, f.X+f.W) - max(x, f.X)
		if !found || line < bestLine || (line == bestLine && x > best.X) {
			best, bestLine, found = r, line, true
		}
	}
	return best, found
}

// ParkFrame is ParkPlace's frame, or the bottom-right corner if every place
// touches another display.
func ParkFrame(screens []Screen, target Screen, size layout.Rect) layout.Rect {
	if r, ok := ParkPlace(screens, target, size); ok {
		return r
	}
	f := target.Frame
	return layout.Rect{X: f.X + f.W - ParkLine, Y: f.Y + f.H - ParkLine, W: size.W, H: size.H}
}

// touchesOther reports whether r overlaps any screen other than target.
func touchesOther(screens []Screen, target Screen, r layout.Rect) bool {
	for _, s := range screens {
		if sameScreen(s, target) {
			continue
		}
		if Overlaps(s.Frame, r) {
			return true
		}
	}
	return false
}

// sameScreen reports whether a and b are the same display: they have the
// same ID, or the same frame (two displays can't share a frame unless one
// mirrors the other, which makes them one as far as parking goes). Matching
// by frame too means a target without an ID still recognises itself.
func sameScreen(a, b Screen) bool {
	return (a.ID != "" && a.ID == b.ID) || a.Frame == b.Frame
}

// Overlaps reports whether two rectangles share some area. Rectangles that
// only meet at an edge don't overlap.
func Overlaps(a, b layout.Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W &&
		a.Y < b.Y+b.H && b.Y < a.Y+a.H &&
		a.W > 0 && a.H > 0 && b.W > 0 && b.H > 0
}

// Intersect returns the area two rectangles share, or a zero Rect if they
// don't overlap.
func Intersect(a, b layout.Rect) layout.Rect {
	x0, y0 := max(a.X, b.X), max(a.Y, b.Y)
	x1, y1 := min(a.X+a.W, b.X+b.W), min(a.Y+a.H, b.Y+b.H)
	if x1 <= x0 || y1 <= y0 {
		return layout.Rect{}
	}
	return layout.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

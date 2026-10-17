package platform

import "github.com/itbm/bench/internal/layout"

// ParkLine is how much of a parked window stays on screen, in points. macOS
// doesn't allow a window fully off-screen.
const ParkLine = 1

// ParkFrame returns where to park a window of the given size on target: in a
// bottom corner, outside the visible area, so that only a 1-point line stays
// on screen (macOS doesn't allow a window fully off-screen). It prefers the
// bottom-right corner, and uses the bottom-left one if another display
// touches the bottom-right corner. If both corners touch another display,
// it picks bottom-right anyway.
//
// Only size's width and height are used. With F as target's whole Frame, the
// bottom-right corner puts the window's top-left at (F.X+F.W-1, F.Y+F.H-1),
// and the bottom-left corner at (F.X-W+1, F.Y+F.H-1). A corner touches
// another display when the parked window would overlap that display's
// frame.
func ParkFrame(screens []Screen, target Screen, size layout.Rect) layout.Rect {
	f := target.Frame
	y := f.Y + f.H - ParkLine
	right := layout.Rect{X: f.X + f.W - ParkLine, Y: y, W: size.W, H: size.H}
	if !touchesOther(screens, target, right) {
		return right
	}
	left := layout.Rect{X: f.X - size.W + ParkLine, Y: y, W: size.W, H: size.H}
	if !touchesOther(screens, target, left) {
		return left
	}
	return right
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

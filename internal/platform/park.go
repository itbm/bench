package platform

import "github.com/itbm/bench/internal/layout"

// ParkFrame returns where to park a window of the given size on target: in a
// bottom corner, outside the visible area, so that only a 1-point line stays
// on screen (macOS doesn't allow a window fully off-screen). It prefers the
// bottom-right corner, and uses the bottom-left one if another display
// touches the bottom-right corner. If both corners touch another display,
// it picks bottom-right anyway.
func ParkFrame(screens []Screen, target Screen, size layout.Rect) layout.Rect {
	panic("not implemented")
}

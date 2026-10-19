package mac

import (
	"math"

	"github.com/itbm/bench/internal/layout"
)

// The live preview panel, in points, matching the prototype's .preview: a
// 232 by 145 picture with 8 points of padding and a 1-point border, and a
// caption line (the title, and "Live" once frames arrive) 8 points below
// the picture. The native panel lays itself out from the same numbers.
const (
	previewImageWidth  = 232
	previewImageHeight = 145
	previewPadding     = 8
	previewBorder      = 1
	previewCaption     = 15
	previewWidth       = previewBorder + previewPadding + previewImageWidth + previewPadding + previewBorder
	previewHeight      = previewBorder + previewPadding + previewImageHeight + previewPadding + previewCaption + previewPadding + previewBorder

	// previewInset keeps the panel this far from the content area's left
	// edge, and previewRightInset this far (plus its width) from the right.
	previewInset      = 8
	previewRightInset = 12
	// previewGap is the space between the anchor and the panel.
	previewGap = 6
)

// Capture sizes for ScreenCaptureKit, in pixels: up to twice the picture's
// size (for Retina displays), and never more than twice the window's size.
const (
	captureMaxWidth  = 2 * previewImageWidth
	captureMaxHeight = 2 * previewImageHeight
	captureMaxScale  = 2.0
	captureMin       = 2
)

// previewFrame returns the live preview panel's frame in global
// coordinates, given the hovered sub-tab's rectangle (anchor, in content
// coordinates) and Bench's content frame (global). As in the prototype, the
// panel sits 6 points below the anchor, with its left edge at the anchor's,
// kept inside the content area. Where it would pass the content area's
// bottom, it goes 6 points above the anchor instead, if there's more room
// there: AppKit doesn't keep a borderless panel on its display, so it would
// be cut off, or appear on the display below.
func previewFrame(anchor, content layout.Rect) layout.Rect {
	x := min(anchor.X, content.W-previewWidth-previewRightInset)
	x = max(x, previewInset)
	y := anchor.Y + anchor.H + previewGap
	if y+previewHeight > content.H && anchor.Y > content.H-(anchor.Y+anchor.H) {
		y = anchor.Y - previewGap - previewHeight
	}
	return layout.Rect{X: content.X + x, Y: content.Y + y, W: previewWidth, H: previewHeight}
}

// captureSize returns the pixel size to capture a window of the given size
// (in points) at: as large as fits in the capture maximum, keeping the
// window's aspect ratio, so the picture isn't stretched. A window with no
// size gets the maximum.
func captureSize(w, h float64) (int, int) {
	if w <= 0 || h <= 0 || math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return captureMaxWidth, captureMaxHeight
	}
	scale := min(captureMaxWidth/w, captureMaxHeight/h, captureMaxScale)
	cw := max(captureMin, int(math.Round(w*scale)))
	ch := max(captureMin, int(math.Round(h*scale)))
	return min(cw, captureMaxWidth), min(ch, captureMaxHeight)
}

//go:build ignore

// Command menubar draws the icon of Bench's menu bar item as a 44 by 44
// PNG: the app icon's three panes in the main-and-side layout, as a template
// image, in black with transparency, which macOS tints to suit the menu bar.
// The main pane is outlined, as the focused pane is in the app, and the side
// panes are filled.
//
// Run it from the repository root:
//
//	go run build/darwin/menubar/main.go
//
// It writes internal/ui/menubar.png, or the file named with -o, which
// internal/ui embeds. Wails sizes a menu bar item's icon to the menu bar's
// thickness, 22 points, so the image is drawn for a Retina display, at two
// pixels a point. It uses only the standard library, and its output is the
// same on every run.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// size is the icon's width and height, in pixels.
const size = 44

// samples is how many samples each pixel takes across and down, to smooth
// the edges.
const samples = 4

// The panes, in pixels: 17 by 14.5 points between them, in the middle of the
// image, with lines and gaps of 1.5 points.
var (
	mainPane   = roundRect{x0: 5, y0: 7, x1: 22, y1: 36, r: 4}
	topPane    = roundRect{x0: 25, y0: 7, x1: 39, y1: 20, r: 3}
	bottomPane = roundRect{x0: 25, y0: 23, x1: 39, y1: 36, r: 3}
)

// outline is the width of the main pane's outline, in pixels.
const outline = 3

func main() {
	out := flag.String("o", "internal/ui/menubar.png", "the PNG file to write")
	flag.Parse()
	if err := write(*out, draw()); err != nil {
		fmt.Fprintln(os.Stderr, "menubar:", err)
		os.Exit(1)
	}
}

// draw draws the icon.
func draw() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			a := 0.0
			for sy := range samples {
				for sx := range samples {
					a += coverage(float64(x)+(float64(sx)+0.5)/samples, float64(y)+(float64(sy)+0.5)/samples)
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{A: uint8(math.Round(a / (samples * samples) * 255))})
		}
	}
	return img
}

// coverage returns how much of the point (x, y) the panes cover, from 0 to
// 1.
func coverage(x, y float64) float64 {
	inside := func(rr roundRect) float64 { return clamp(0.5 - rr.distance(x, y)) }
	return clamp(inside(mainPane) - inside(mainPane.inset(outline)) + inside(topPane) + inside(bottomPane))
}

// write encodes the icon as a PNG file.
func write(path string, img image.Image) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating the icon: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("writing the icon: %w", cerr)
		}
	}()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("writing the icon: %w", err)
	}
	return nil
}

// roundRect is a rectangle with rounded corners, in pixels.
type roundRect struct {
	x0, y0, x1, y1 float64
	r              float64 // Corner radius.
}

// inset shrinks the rectangle by d on every side, keeping its corners
// concentric.
func (rr roundRect) inset(d float64) roundRect {
	return roundRect{x0: rr.x0 + d, y0: rr.y0 + d, x1: rr.x1 - d, y1: rr.y1 - d, r: math.Max(rr.r-d, 0)}
}

// distance returns the signed distance from the point (x, y) to the
// rectangle's edge: negative inside, positive outside.
func (rr roundRect) distance(x, y float64) float64 {
	cx, cy := (rr.x0+rr.x1)/2, (rr.y0+rr.y1)/2
	qx := math.Abs(x-cx) - (rr.x1-rr.x0)/2 + rr.r
	qy := math.Abs(y-cy) - (rr.y1-rr.y0)/2 + rr.r
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inside := math.Min(math.Max(qx, qy), 0)
	return outside + inside - rr.r
}

func clamp(v float64) float64 { return math.Min(math.Max(v, 0), 1) }

//go:build ignore

// Command icon draws Bench's app icon as a 1,024 by 1,024 PNG, in the style
// of the UI prototype: a rounded square in the window colour, holding three
// panes in the main-and-side layout, with the main pane outlined in the
// accent colour, as the focused pane is in the app.
//
// Run it from the repository root:
//
//	go run build/darwin/icon/main.go
//
// It writes build/darwin/icon.png, or the file named with -o. The Makefile
// turns the PNG into Bench.icns with sips and iconutil when it builds
// Bench.app. It uses only the standard library, and its output is the same
// on every run.
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
const size = 1024

// Colours, from the prototype's design tokens.
var (
	windowColour = rgb(0x12, 0x14, 0x19) // --bg-window
	edgeColour   = rgb(0x2A, 0x2F, 0x39) // --line
	paneColour   = rgb(0x2A, 0x30, 0x3B) // --bg-tile
	accentColour = rgb(0xF2, 0xA3, 0x3A) // --accent
)

// The icon's geometry, in pixels. The rounded square follows the grid of
// Apple's macOS app icon template: 824 pixels across, centred, leaving room
// for its shadow.
const (
	bodyInset  = 100 // From the canvas edge to the rounded square.
	bodyRadius = 185 // The rounded square's corners.
	edgeWidth  = 4   // A faint edge, so the square shows on dark backgrounds.

	panePadding = 104 // From the rounded square's edge to the panes.
	paneGap     = 40  // Between panes.
	paneRadius  = 40  // The panes' corners.
	mainShare   = 0.6 // The main pane's share of the width.
	outlineSize = 20  // The main pane's accent outline.

	shadowOffset = 10  // How far the shadow falls below the square.
	shadowBlur   = 28  // How far the shadow spreads beyond it.
	shadowAlpha  = 0.3 // The shadow's opacity under the square.
)

func main() {
	out := flag.String("o", "build/darwin/icon.png", "the PNG file to write")
	flag.Parse()
	if err := write(*out, draw()); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
}

// draw draws the icon.
func draw() *canvas {
	c := newCanvas(size)
	body := roundRect{x0: bodyInset, y0: bodyInset, x1: size - bodyInset, y1: size - bodyInset, r: bodyRadius}

	c.fill(shadow(body.offset(0, shadowOffset)))
	c.fill(solid(body, edgeColour))
	c.fill(solid(body.inset(edgeWidth), windowColour))

	primary, top, bottom := panes(body)
	c.fill(solid(primary, accentColour))
	c.fill(solid(primary.inset(outlineSize), paneColour))
	c.fill(solid(top, paneColour))
	c.fill(solid(bottom, paneColour))
	return c
}

// panes lays out the three panes inside the rounded square: the main pane
// on the left, and two panes stacked on the right.
func panes(body roundRect) (primary, top, bottom roundRect) {
	x0, y0 := body.x0+panePadding, body.y0+panePadding
	x1, y1 := body.x1-panePadding, body.y1-panePadding
	split := math.Round(x0 + (x1-x0-paneGap)*mainShare)
	middle := math.Round((y0 + y1) / 2)
	primary = roundRect{x0: x0, y0: y0, x1: split, y1: y1, r: paneRadius}
	top = roundRect{x0: split + paneGap, y0: y0, x1: x1, y1: middle - paneGap/2, r: paneRadius}
	bottom = roundRect{x0: split + paneGap, y0: middle + paneGap/2, x1: x1, y1: y1, r: paneRadius}
	return primary, top, bottom
}

// write encodes the canvas as a PNG file.
func write(path string, c *canvas) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating the icon: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("writing the icon: %w", cerr)
		}
	}()
	if err := png.Encode(f, c.image()); err != nil {
		return fmt.Errorf("writing the icon: %w", err)
	}
	return nil
}

// colour is a colour with components from 0 to 1.
type colour struct{ r, g, b float64 }

func rgb(r, g, b uint8) colour {
	return colour{float64(r) / 255, float64(g) / 255, float64(b) / 255}
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

// offset moves the rectangle.
func (rr roundRect) offset(dx, dy float64) roundRect {
	return roundRect{x0: rr.x0 + dx, y0: rr.y0 + dy, x1: rr.x1 + dx, y1: rr.y1 + dy, r: rr.r}
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

// A shape gives the colour and opacity to paint at a pixel centre.
type shape func(x, y float64) (colour, float64)

// solid fills a rounded rectangle, with anti-aliased edges.
func solid(rr roundRect, c colour) shape {
	return func(x, y float64) (colour, float64) {
		return c, clamp(0.5 - rr.distance(x, y))
	}
}

// shadow is a soft black shadow of a rounded rectangle.
func shadow(rr roundRect) shape {
	return func(x, y float64) (colour, float64) {
		t := clamp(rr.distance(x, y) / shadowBlur)
		return colour{}, shadowAlpha * (1 - t*t*(3-2*t))
	}
}

func clamp(v float64) float64 { return math.Min(math.Max(v, 0), 1) }

// canvas is a square image with premultiplied colour components from 0 to
// 1, so that shapes composite exactly.
type canvas struct {
	n   int
	pix []float64 // r, g, b, a for each pixel, row by row.
}

func newCanvas(n int) *canvas {
	return &canvas{n: n, pix: make([]float64, n*n*4)}
}

// fill paints a shape over the canvas.
func (c *canvas) fill(s shape) {
	for y := range c.n {
		for x := range c.n {
			col, a := s(float64(x)+0.5, float64(y)+0.5)
			if a <= 0 {
				continue
			}
			p := c.pix[(y*c.n+x)*4:]
			p[0] = col.r*a + p[0]*(1-a)
			p[1] = col.g*a + p[1]*(1-a)
			p[2] = col.b*a + p[2]*(1-a)
			p[3] = a + p[3]*(1-a)
		}
	}
}

// image converts the canvas to an image for encoding.
func (c *canvas) image() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.n, c.n))
	for y := range c.n {
		for x := range c.n {
			p := c.pix[(y*c.n+x)*4:]
			a := p[3]
			if a <= 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: to8(p[0] / a),
				G: to8(p[1] / a),
				B: to8(p[2] / a),
				A: to8(a),
			})
		}
	}
	return img
}

// to8 converts a component from 0 to 1 to a byte.
func to8(v float64) uint8 { return uint8(math.Round(clamp(v) * 255)) }

// Package icons makes the small square PNGs that Bench's tiles show, in
// place of an item's monogram: an app's icon, which macOS draws, and a web
// page's icon (its favicon). A page's icon comes from wherever the page
// says, so Bench fetches it without cookies and decodes it here, in Go,
// rather than in native code in Bench's own process, which the page's
// sandbox doesn't cover. Pure Go.
package icons

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"

	// The formats that Fit reads besides PNG and ICO.
	_ "image/gif"
	_ "image/jpeg"
)

// Pixels is how wide and tall a tile's icon is: a tile is 20 points, and
// screens are 2x.
const Pixels = 40

// maxSide is the most pixels wide or tall an image may be. Icons are far
// smaller, and a small file can claim a huge image.
const maxSide = 1024

// ErrNotImage: the data isn't a PNG, GIF, JPEG, or ICO image that Fit can
// read, or it's too big.
var ErrNotImage = errors.New("not an icon Bench can read")

// Fit decodes a PNG, GIF, JPEG, or ICO image and returns it as a PNG
// pixels square: a bigger image scaled down to fit, keeping its shape, and
// a smaller one scaled up by a whole number, so that its pixels stay
// sharp, centred on a transparent square. An ICO's image is its smallest
// at least pixels wide, or else its biggest.
func Fit(data []byte, pixels int) ([]byte, error) {
	if pixels <= 0 || pixels > maxSide {
		return nil, ErrNotImage
	}
	src, err := decode(data, pixels)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, fit(src, pixels)); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// decode decodes an image, refusing one more than maxSide pixels wide or
// tall before it decodes its pixels.
func decode(data []byte, pixels int) (image.Image, error) {
	if isICO(data) {
		return decodeICO(data, pixels)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !sideOK(cfg.Width) || !sideOK(cfg.Height) {
		return nil, ErrNotImage
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}
	return img, nil
}

func sideOK(n int) bool {
	return n > 0 && n <= maxSide
}

// fit draws src on a transparent square pixels wide (see Fit).
func fit(src image.Image, pixels int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, pixels, pixels))
	if w <= 0 || h <= 0 {
		return dst
	}
	if w <= pixels && h <= pixels {
		k := max(1, min(pixels/w, pixels/h))
		ox, oy := (pixels-w*k)/2, (pixels-h*k)/2
		for y := range h * k {
			for x := range w * k {
				dst.Set(ox+x, oy+y, src.At(b.Min.X+x/k, b.Min.Y+y/k))
			}
		}
		return dst
	}
	scale := math.Min(float64(pixels)/float64(w), float64(pixels)/float64(h))
	dw := max(1, int(math.Round(float64(w)*scale)))
	dh := max(1, int(math.Round(float64(h)*scale)))
	ox, oy := (pixels-dw)/2, (pixels-dh)/2
	sx, sy := float64(w)/float64(dw), float64(h)/float64(dh)
	for y := range dh {
		y0, y1 := float64(y)*sy, float64(y+1)*sy
		for x := range dw {
			x0, x1 := float64(x)*sx, float64(x+1)*sx
			dst.SetNRGBA(ox+x, oy+y, average(src, b.Min, x0, x1, y0, y1))
		}
	}
	return dst
}

// average returns the colour of the area [x0, x1) by [y0, y1) of src, in
// source pixels from origin, weighing each pixel by how much of it is in the
// area, with premultiplied alpha, so that transparent pixels' colours
// don't darken the edges.
func average(src image.Image, origin image.Point, x0, x1, y0, y1 float64) color.NRGBA {
	var r, g, b, a, total float64
	for py := int(y0); float64(py) < y1; py++ {
		wy := math.Min(y1, float64(py+1)) - math.Max(y0, float64(py))
		for px := int(x0); float64(px) < x1; px++ {
			wx := math.Min(x1, float64(px+1)) - math.Max(x0, float64(px))
			weight := wx * wy
			if weight <= 0 {
				continue
			}
			cr, cg, cb, ca := src.At(origin.X+px, origin.Y+py).RGBA()
			r += float64(cr) * weight
			g += float64(cg) * weight
			b += float64(cb) * weight
			a += float64(ca) * weight
			total += weight
		}
	}
	if total == 0 || a == 0 {
		return color.NRGBA{}
	}
	unmul := func(v float64) uint8 {
		return uint8(math.Round(math.Min(255, v/a*255)))
	}
	return color.NRGBA{R: unmul(r), G: unmul(g), B: unmul(b), A: uint8(math.Round(a / total / 0xffff * 255))}
}
